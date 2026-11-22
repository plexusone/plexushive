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
	var record bool
	cmd := &cobra.Command{
		Use:   "decide <identity>/<id> <option>",
		Short: "Answer a decision request",
		Long: "Records the operator's answer on the request's open item in\n" +
			"open-items.json and prints the message to send the work identity. Most\n" +
			"answers are point decisions: the work identity acts on the answer and\n" +
			"records the outcome where it belongs, such as a commit, a changelog or\n" +
			"an ADR. A question from an ADR stays answered until its work identity has\n" +
			"written the answer into it. With --record the answer also becomes a\n" +
			"decision in decisions/<id>.json, for working rules that bind the agents;\n" +
			"a decision is a new file and never replaces an existing one.",
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
				it, listed := h.Open.Item(args[0])
				if !listed || it.Status != hive.ItemOpen {
					return fmt.Errorf("plexushive: neither the %s report nor the open items have decision request %q", identity, id)
				}
				req = it.Request
			}
			d, err := req.Decide(identity, args[1], reason)
			if err != nil {
				return err
			}
			decisions := h.Decisions
			decisionID := ""
			if record {
				decisions = append(decisions, d)
				decisionID = d.ID
				if err := hive.ValidateDecisions(decisions, h.Identities); err != nil {
					return err
				}
			}
			answer := hive.Answer{Option: args[1], Reason: strings.TrimSpace(reason), Answered: now().Format("2006-01-02")}
			it, err := h.Open.Record(identity, req, answer, decisionID)
			if err != nil {
				return err
			}
			h.Open.Updated = answer.Answered
			if err := hive.ValidateOpenItems(h.Open, h.Identities, decisions); err != nil {
				return err
			}
			if record {
				if err := hive.WriteDecision(*hiveDir, d); err != nil {
					return err
				}
				cmd.Printf("wrote decisions/%s.json\n", d.ID)
			}
			if err := hive.WriteOpenItems(*hiveDir, h.Open); err != nil {
				return err
			}
			cmd.Printf("marked %s %s in %s\n", args[0], it.Status, hive.OpenItemsFile)
			env := hive.Envelope{Type: hive.MessageStatus, From: hive.Operator}
			cmd.Printf("send to %s:\n%s %s\n", identity, env.String(), answerMessage(it, record))
			if req.Authorization {
				cmd.Println("this answer authorizes the work identity to act beyond its autonomy: keep it as an approval")
			}
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	cmd.Flags().StringVar(&reason, "reason", "", "why, sent with the answer and recorded on the open item")
	cmd.Flags().BoolVar(&record, "record", false, "also record the answer as a decision in decisions/, for working rules that bind the agents")
	return cmd
}

// answerMessage tells the work identity what was decided and what to do
// with it.
func answerMessage(it *hive.OpenItem, recorded bool) string {
	id, a := it.Request.ID, it.Answer
	msg := fmt.Sprintf("decided %s: %s.", id, a.Option)
	if a.Reason != "" {
		msg += " Reason: " + a.Reason
		if !strings.HasSuffix(a.Reason, ".") {
			msg += "."
		}
	}
	switch {
	case recorded:
		msg += fmt.Sprintf(" Read decisions/%s.json and drop the request from your report.", id)
	case it.Status == hive.ItemAnswered:
		where := it.Source.Document
		if where == "" {
			where = it.Source.Path
		}
		msg += fmt.Sprintf(" Write the answer into %s (%s), then drop the request from your report.", where, it.Source.Path)
	default:
		msg += " Act on it, record the outcome where it belongs (a commit, a changelog, an ADR), and drop the request from your report."
	}
	return msg
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
			b := hive.NewHiveBriefing(h, rs)
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
