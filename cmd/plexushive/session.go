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
		Use:   "start <role>",
		Short: "Bind a role to this tmux session and print what the session needs to play it",
		Long: "Checks that the role exists and is not retired, binds it to the current\n" +
			"tmux session (so agents already running in it can find it), and prints\n" +
			"the role's context: mission, repositories, peers, binding decisions and\n" +
			"how to report. Outside tmux it prints the export to use instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadContext(*hiveDir, reports, args[0])
			if err != nil {
				return err
			}
			if hive.InTmux() {
				if err := hive.BindTmuxRole(args[0]); err != nil {
					return err
				}
				name, err := hive.TmuxSession()
				if err != nil {
					return err
				}
				cmd.Printf("bound %s to the tmux session %s\n\n", args[0], name)
			} else {
				cmd.Printf("not in tmux: run export %s=%s in this session's shell\n\n", hive.RoleEnv, args[0])
			}
			printContext(cmd, c, *hiveDir)
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	return cmd
}

func contextCmd(hiveDir, role *string) *cobra.Command {
	var reports string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "context [role]",
		Short: "Print what a session needs to play its role, such as after it starts or compacts",
		Long: "Prints the role's context without binding anything. The role is the\n" +
			"argument, else --role, $" + hive.RoleEnv + " or the tmux session's. A\n" +
			"session-start or compaction hook runs it to put the context back.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := *role
			if len(args) == 1 {
				name = args[0]
			}
			name, _, err := hive.ResolveRole(name)
			if err != nil {
				return err
			}
			if name == "" {
				return errors.New("plexushive: no role: pass one, or bind one with plexushive start <role>")
			}
			c, err := loadContext(*hiveDir, reports, name)
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

func whoamiCmd(role *string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the role this session plays and where that was found",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, source, err := hive.ResolveRole(*role)
			if err != nil {
				return err
			}
			session, err := hive.TmuxSession()
			if err != nil {
				return err
			}
			if name == "" {
				cmd.Println("no role: bind one with plexushive start <role>")
			} else {
				cmd.Printf("%s (from %s)\n", name, sourceText(source))
			}
			if session != "" {
				cmd.Printf("tmux session %s\n", session)
			}
			return nil
		},
	}
}

func sourceText(s hive.RoleSource) string {
	switch s {
	case hive.SourceGiven:
		return "--role"
	case hive.SourceEnv:
		return "$" + hive.RoleEnv
	default:
		return "the tmux session"
	}
}

func loadContext(hiveDir, reports, role string) (hive.Context, error) {
	h, err := hive.Load(hiveDir)
	if err != nil {
		return hive.Context{}, err
	}
	dir, err := resolveReports(reports)
	if err != nil {
		return hive.Context{}, err
	}
	rs, err := hive.LoadReports(dir)
	if err != nil {
		return hive.Context{}, err
	}
	return hive.NewContext(h, rs, role)
}

func printContext(cmd *cobra.Command, c hive.Context, hiveDir string) {
	r := c.Role
	cmd.Printf("Role %s (%s, autonomy %s)\n  %s\n", r.Name, r.Status, r.Autonomy, r.Mission)
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
			cmd.Println(strings.TrimRight(fmt.Sprintf("  %-20s %s", p.Role, how), " "))
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
			cmd.Printf("  answered: %s (decisions/%s.json); act on it and drop the request\n", id, id)
		}
	}
	dir := hiveDir
	if abs, err := filepath.Abs(hiveDir); err == nil {
		dir = abs
	}
	cmd.Printf("\nReport at handoffs: when work is ready for the operator, when blocked or\n"+
		"unblocked, when a decision is needed, when starting or stopping in a\n"+
		"repository, after the operator acts for you, and when another role's work\n"+
		"changes your plan. Before working in a repository another role stewards:\n"+
		"  plexushive -C %s who github.com/<org>/<repo>\n"+
		"Submit (plexushive schema report gives the format):\n"+
		"  plexushive -C %s report submit <file>\n", dir, dir)
}
