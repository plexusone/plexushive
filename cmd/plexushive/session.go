package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/plexusone/plexushive/hive"
)

func startCmd(hiveDir *string) *cobra.Command {
	var reports string
	cmd := &cobra.Command{
		Use:   "start <identity>",
		Short: "Bind a work identity to this tmux session and print what the session needs to work under it",
		Long: "Checks that the work identity exists and is not retired, binds it to the\n" +
			"current tmux session (so agents already running in it can find it), and\n" +
			"prints its context: mission, repositories, peers, binding decisions and\n" +
			"how to report. Outside tmux it prints the export to use instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadContext(cmd, *hiveDir, reports, args[0])
			if err != nil {
				return err
			}
			if hive.InTmux() {
				if err := hive.BindTmuxIdentity(args[0]); err != nil {
					return err
				}
				name, err := hive.TmuxSession()
				if err != nil {
					return err
				}
				cmd.Printf("bound %s to the tmux session %s\n\n", args[0], name)
			} else {
				cmd.Printf("not in tmux: run export %s=%s in this session's shell\n\n", hive.IdentityEnv, args[0])
			}
			printContext(cmd, c, *hiveDir)
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	return cmd
}

func contextCmd(hiveDir, identity *string) *cobra.Command {
	var reports string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "context [identity]",
		Short: "Print what a session needs to work under its work identity, such as after it starts or compacts",
		Long: "Prints the work identity's context without binding anything. The identity\n" +
			"is the argument, else --identity, $" + hive.IdentityEnv + " or the tmux session's\n" +
			"(then $" + hive.LegacyIdentityEnv + "). A session-start or compaction hook runs it\n" +
			"to put the context back.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := *identity
			if len(args) == 1 {
				name = args[0]
			}
			name, _, err := hive.ResolveIdentity(name)
			if err != nil {
				return err
			}
			if name == "" {
				return errors.New("plexushive: no work identity: pass one, or bind one with plexushive start <identity>")
			}
			c, err := loadContext(cmd, *hiveDir, reports, name)
			if err != nil {
				return err
			}
			if asJSON {
				out, err := json.MarshalIndent(c, "", "  ")
				if err != nil {
					return fmt.Errorf("plexushive: %w", err)
				}
				cmd.Println(string(out))
				return nil
			}
			printContext(cmd, c, *hiveDir)
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the context as JSON")
	return cmd
}

func whoamiCmd(identity *string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the work identity this session works under and where that was found",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, source, err := hive.ResolveIdentity(*identity)
			if err != nil {
				return err
			}
			session, err := hive.TmuxSession()
			if err != nil {
				return err
			}
			if name == "" {
				cmd.Println("no work identity: bind one with plexushive start <identity>")
			} else {
				cmd.Printf("%s (from %s)\n", name, sourceText(source))
				if source.Legacy() {
					cmd.Printf("%s is the old name: bind again with plexushive start %s\n", hive.LegacyIdentityEnv, name)
				}
			}
			if session != "" {
				cmd.Printf("tmux session %s\n", session)
			}
			return nil
		},
	}
}

func sourceText(s hive.IdentitySource) string {
	switch s {
	case hive.SourceGiven:
		return "--identity"
	case hive.SourceEnv:
		return "$" + hive.IdentityEnv
	case hive.SourceLegacyEnv:
		return "$" + hive.LegacyIdentityEnv
	case hive.SourceLegacyTmux:
		return "the tmux session's " + hive.LegacyIdentityEnv
	default:
		return "the tmux session"
	}
}

func loadContext(cmd *cobra.Command, hiveDir, reports, identity string) (hive.Context, error) {
	h, err := hive.Load(hiveDir)
	if err != nil {
		return hive.Context{}, err
	}
	dir, err := resolveReports(reports)
	if err != nil {
		return hive.Context{}, err
	}
	rs, _, err := readReports(cmd, dir)
	if err != nil {
		return hive.Context{}, err
	}
	return hive.NewContext(h, rs, identity)
}

func printContext(cmd *cobra.Command, c hive.Context, hiveDir string) {
	r := c.Identity
	cmd.Printf("Work identity %s (%s, autonomy %s)\n  %s\n", r.Name, r.Status, r.Autonomy, r.Mission)
	if len(r.Repositories) > 0 {
		cmd.Println("\nRepositories")
		for _, a := range r.Repositories {
			cmd.Printf("  %-12s %s\n", a.Affinity, a.Pattern)
		}
	}
	if len(r.Initiatives) > 0 {
		cmd.Printf("\nInitiatives: %s\n", strings.Join(r.Initiatives, ", "))
	}
	if len(c.Peers) > 0 {
		cmd.Println("\nPeers")
		for _, p := range c.Peers {
			how := ""
			if p.Session != nil {
				how = reach(*p.Session)
			}
			cmd.Println(strings.TrimRight(fmt.Sprintf("  %-20s %s", p.Identity, how), " "))
		}
	}
	if r.Notes != "" {
		cmd.Printf("\nNotes: %s\n", r.Notes)
	}
	cmd.Printf("\nDecisions that bind you (%d); read each in decisions/<id>.json\n", len(c.Decisions))
	for _, d := range c.Decisions {
		cmd.Printf("  %-44s %s\n", d.ID, d.Title)
	}
	cmd.Println("\nYour report")
	if c.Report == nil {
		cmd.Println("  none yet")
	} else {
		cmd.Printf("  generated %s\n", c.Report.Generated.Format("2006-01-02 15:04 MST"))
		for _, id := range c.Answered {
			if a, ok := c.Answers[id]; ok {
				reason := ""
				if a.Reason != "" {
					reason = " (" + a.Reason + ")"
				}
				cmd.Printf("  answered: %s: %s%s; act on it, record the outcome where it belongs, and drop the request\n", id, a.Option, reason)
				continue
			}
			cmd.Printf("  answered: %s (decisions/%s.json); act on it and drop the request\n", id, id)
		}
	}
	dir := hiveDir
	if abs, err := filepath.Abs(hiveDir); err == nil {
		dir = abs
	}
	cmd.Printf("\nReport at handoffs: when work is ready for the operator, when blocked or\n"+
		"unblocked, when a decision is needed, when starting or stopping in a\n"+
		"repository, after the operator acts for you, and when another work\n"+
		"identity's work changes your plan. Before working in a repository another\n"+
		"work identity stewards:\n"+
		"  plexushive -C %s who github.com/<org>/<repo>\n"+
		"Submit (plexushive schema report gives the format):\n"+
		"  plexushive -C %s report submit <file>\n", dir, dir)
}
