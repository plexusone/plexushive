package main

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/plexusone/plexushive/hive"
)

func decideCmd(hiveDir *string) *cobra.Command {
	var reports, reason string
	cmd := &cobra.Command{
		Use:   "decide <identity>/<id> <option>",
		Short: "Answer a decision request and add the decision to the log",
		Long: "Finds the request in the work identity's current report, writes the\n" +
			"chosen option to decisions/<id>.json, marks the item decided in\n" +
			"open-items.json when it is listed, and prints the message to send.\n" +
			"The decision is a new file to commit; it never replaces an existing one.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, id, ok := strings.Cut(args[0], "/")
			if !ok {
				return fmt.Errorf("plexushive: %q must be <identity>/<id>", args[0])
			}
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			dir, err := resolveReports(reports)
			if err != nil {
				return err
			}
			rs, _, err := readReports(cmd, dir)
			if err != nil {
				return err
			}
			req, ok := findRequest(rs, identity, id)
			if !ok {
				return fmt.Errorf("plexushive: the %s report has no decision request %q", identity, id)
			}
			d, err := req.Decide(identity, args[1], reason)
			if err != nil {
				return err
			}
			if err := hive.ValidateDecisions(append(h.Decisions, d), h.Identities); err != nil {
				return err
			}
			if err := hive.WriteDecision(*hiveDir, d); err != nil {
				return err
			}
			cmd.Printf("wrote decisions/%s.json\n", d.ID)
			if h.Open.MarkDecided(args[0], d.ID) {
				h.Open.Updated = now().Format("2006-01-02")
				if err := hive.WriteOpenItems(*hiveDir, h.Open); err != nil {
					return err
				}
				cmd.Printf("marked %s decided in %s\n", args[0], hive.OpenItemsFile)
			}
			env := hive.Envelope{Type: hive.MessageStatus, From: hive.Operator}
			cmd.Printf("send to %s:\n%s decided %s: %s. Read decisions/%s.json and drop the request from your report.\n",
				identity, env.String(), d.ID, args[1], d.ID)
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded as the decision's first reason")
	return cmd
}

func findRequest(rs []hive.Report, identity, id string) (hive.DecisionRequest, bool) {
	for _, r := range rs {
		if r.Identity != identity {
			continue
		}
		for _, d := range r.DecisionsNeeded {
			if d.ID == id {
				return d, true
			}
		}
	}
	return hive.DecisionRequest{}, false
}

func whoCmd(hiveDir *string) *cobra.Command {
	var reports string
	cmd := &cobra.Command{
		Use:   "who github.com/<org>/<repo>",
		Short: "Show who stewards a repository, who is working in it now and what waits on the operator there",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			matches, err := hive.IdentitiesFor(h.Identities, repo)
			if err != nil {
				return err
			}
			dir, err := resolveReports(reports)
			if err != nil {
				return err
			}
			rs, _, err := readReports(cmd, dir)
			if err != nil {
				return err
			}
			sessions := map[string]string{}
			for _, r := range rs {
				if r.Session != nil {
					sessions[r.Identity] = reach(*r.Session)
				}
			}
			cmd.Println("Work identities")
			if len(matches) == 0 {
				cmd.Println("  none: the repository is common ground")
			}
			for _, m := range matches {
				cmd.Println(strings.TrimRight(fmt.Sprintf("  %-12s %-16s %s", m.Affinity, m.Identity, sessions[m.Identity]), " "))
			}
			b := hive.NewBriefing(h.Identities, h.Decisions, rs)
			cmd.Println("In progress")
			for _, rw := range b.InProgress {
				if rw.Repository != repo {
					continue
				}
				for _, w := range rw.Work {
					where := w.Work.Branch
					if w.Work.State != "" {
						where += " (" + string(w.Work.State) + ")"
					}
					cmd.Printf("  %-16s %-24s %s\n", w.Identity, where, w.Work.Description)
				}
			}
			cmd.Println("Waiting on the operator")
			for _, a := range b.Actions {
				if a.Action.Repository == repo {
					cmd.Printf("  %-16s %-10s %s\n", strings.Join(a.Identities, ", "), a.Action.Kind, a.Action.Description)
				}
			}
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	return cmd
}

// reach renders how to message a session.
func reach(s hive.Session) string {
	var parts []string
	if s.Agent != "" {
		parts = append(parts, "agent "+s.Agent)
	}
	if s.Tmux != "" {
		parts = append(parts, "tmux "+s.Tmux)
	}
	return strings.Join(parts, ", ")
}

// version is the module version the binary was built from, or "devel".
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "devel"
}
