package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/plexusone/plexushive/hive"
)

// now is the clock; tests replace it.
var now = time.Now

func reportsFlag(cmd *cobra.Command, dir *string) {
	cmd.Flags().StringVar(dir, "reports", "", "reports directory (default $"+hive.ReportsEnv+", else the per-user data directory)")
}

func resolveReports(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	return hive.ReportsDir()
}

// readReports reads the reports in dir, warning on standard error about
// any it cannot decode instead of failing.
func readReports(cmd *cobra.Command, dir string) ([]hive.Report, []hive.UnreadableReport, error) {
	rs, bad, err := hive.ReadReports(dir)
	for _, u := range bad {
		cmd.PrintErrf("skipped an unreadable report: %s\n", u)
	}
	return rs, bad, err
}

func reportCmd(hiveDir, role *string) *cobra.Command {
	cmd := &cobra.Command{Use: "report", Short: "Write status reports"}
	var reports string
	submit := &cobra.Command{
		Use:   "submit [file|-]",
		Short: "Validate a report and store it as the role's current report",
		Long: "Reads a report as JSON from a file or standard input. An empty role is\n" +
			"taken from --role, else $" + hive.RoleEnv + ", else the tmux session's\n" +
			"environment. An empty generated time is set to now, and inside tmux an\n" +
			"empty session.tmux is set to the session's name. The role must exist in\n" +
			"the hive directory.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := readReport(cmd.InOrStdin(), args)
			if err != nil {
				return err
			}
			if r.Role == "" {
				if r.Role, _, err = hive.ResolveRole(*role); err != nil {
					return err
				}
			}
			if r.Generated.IsZero() {
				r.Generated = now().UTC().Truncate(time.Second)
			}
			if err := fillTmux(&r); err != nil {
				return err
			}
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			r.Normalize()
			if err := hive.ValidateReports([]hive.Report{r}, h.Roles); err != nil {
				return err
			}
			dir, err := resolveReports(reports)
			if err != nil {
				return err
			}
			// Earlier reports only supply when each request was first asked,
			// so one this version cannot read is no reason to refuse.
			prev, _, err := hive.ReadReports(dir)
			if err != nil {
				return err
			}
			keepAsked(&r, prev)
			if err := hive.WriteReport(dir, r); err != nil {
				return err
			}
			cmd.Printf("stored the %s report\n", r.Role)
			return nil
		},
	}
	reportsFlag(submit, &reports)
	cmd.AddCommand(submit)
	return cmd
}

// fillTmux sets the report's tmux session name when it is empty and the
// command runs inside tmux.
func fillTmux(r *hive.Report) error {
	if r.Session != nil && r.Session.Tmux != "" {
		return nil
	}
	name, err := hive.TmuxSession()
	if err != nil || name == "" {
		return err
	}
	if r.Session == nil {
		r.Session = &hive.Session{}
	}
	r.Session.Tmux = name
	return nil
}

// keepAsked fills in when each decision request was first asked: from the
// role's previous report when it carried the same request, else the
// report's generated time.
func keepAsked(r *hive.Report, prev []hive.Report) {
	asked := map[string]time.Time{}
	for _, p := range prev {
		if p.Role == r.Role {
			for _, d := range p.DecisionsNeeded {
				asked[d.ID] = d.Asked
			}
		}
	}
	for i := range r.DecisionsNeeded {
		d := &r.DecisionsNeeded[i]
		if !d.Asked.IsZero() {
			continue
		}
		if t, ok := asked[d.ID]; ok && !t.IsZero() {
			d.Asked = t
		} else {
			d.Asked = r.Generated
		}
	}
}

// readReport decodes one report strictly from the named file, or from in
// when no file or "-" is named.
func readReport(in io.Reader, args []string) (r hive.Report, err error) {
	if len(args) == 1 && args[0] != "-" {
		f, err := os.Open(args[0])
		if err != nil {
			return r, err
		}
		defer func() { err = errors.Join(err, f.Close()) }()
		in = f
	}
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, fmt.Errorf("plexushive: report: %w", err)
	}
	return r, nil
}

func briefCmd(hiveDir *string) *cobra.Command {
	var reports string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Show operator actions, decisions needed, blockers, work in progress and each role's status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			dir, err := resolveReports(reports)
			if err != nil {
				return err
			}
			rs, bad, err := hive.ReadReports(dir)
			if err != nil {
				return err
			}
			b := hive.NewBriefing(h.Roles, h.Decisions, rs)
			b.SetUnreadable(bad)
			if asJSON {
				out, err := json.MarshalIndent(b, "", "  ")
				if err != nil {
					return fmt.Errorf("plexushive: %w", err)
				}
				cmd.Println(string(out))
				return nil
			}
			printBriefing(cmd, b, now())
			return nil
		},
	}
	reportsFlag(cmd, &reports)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the briefing as JSON")
	return cmd
}

func printBriefing(cmd *cobra.Command, b hive.Briefing, at time.Time) {
	cmd.Printf("Your actions (%d)\n", len(b.Actions))
	kind := hive.ActionKind("")
	for _, a := range b.Actions {
		if a.Action.Kind != kind {
			kind = a.Action.Kind
			cmd.Printf("  %s\n", kind)
		}
		printAction(cmd, a)
	}
	cmd.Printf("\nDecisions needed (%d)\n", len(b.Decisions))
	for _, d := range b.Decisions {
		waited := ""
		if !d.Request.Asked.IsZero() {
			waited = ", asked " + age(at.Sub(d.Request.Asked)) + " ago"
		}
		cmd.Printf("  %s: %s\n", d.Key, d.Request.Title)
		cmd.Printf("    recommends %s%s; approval: %s\n", d.Request.Recommendation, waited, d.Request.Approval)
	}
	if len(b.Answered) > 0 {
		cmd.Printf("\nAnswered, still in reports (%d)\n", len(b.Answered))
		for _, d := range b.Answered {
			cmd.Printf("  %s: see decisions/%s.json\n", d.Key, d.Request.ID)
		}
	}
	cmd.Printf("\nBlocked (%d)\n", len(b.Blocked))
	for _, bl := range b.Blocked {
		cmd.Printf("  %-14s waiting on %-12s %s\n", bl.Role, strings.Join(bl.Blocker.WaitingOn, ", then "), bl.Blocker.Description)
		if len(bl.Blocker.Credentials) > 0 {
			cmd.Printf("  %-14s %-23s credentials: %s\n", "", "", strings.Join(bl.Blocker.Credentials, ", "))
		}
		for _, u := range bl.Blocker.Unblocks {
			cmd.Printf("  %-14s %-23s then: %s\n", "", "", u)
		}
	}
	cmd.Printf("\nIn progress (%d)\n", len(b.InProgress))
	for _, rw := range b.InProgress {
		shared := ""
		if rw.Shared {
			shared = "  [more than one role]"
		}
		cmd.Printf("  %s%s\n", rw.Repository, shared)
		for _, w := range rw.Work {
			where := w.Work.Branch
			if where == "" {
				where = "-"
			}
			if w.Work.State != "" {
				where += " (" + string(w.Work.State) + ")"
			}
			cmd.Printf("    %-14s %-24s %s\n", w.Role, where, w.Work.Description)
		}
	}
	if len(b.Notices) > 0 {
		cmd.Printf("\nNotices (%d)\n", len(b.Notices))
		for _, n := range b.Notices {
			cmd.Printf("  %-14s %s\n", n.Role, n.Notice.Title)
		}
	}
	cmd.Printf("\nStatus (%d)\n", len(b.Reports))
	for _, r := range b.Reports {
		state := age(r.Age(at))
		switch {
		case r.Idle:
			state = "idle"
		case slices.Contains(b.Waiting, r.Role):
			state = "decision"
		}
		cmd.Printf("  %-14s %-8s %s\n", r.Role, state, r.Summary)
	}
	if len(b.Missing) > 0 {
		cmd.Printf("\nNo report: %s\n", strings.Join(b.Missing, ", "))
	}
	if len(b.Ignored) > 0 {
		cmd.Printf("Ignored reports from unknown or retired roles: %s\n", strings.Join(b.Ignored, ", "))
	}
	if len(b.Unreadable) > 0 {
		cmd.Printf("\nUnreadable reports (upgrade plexushive, or ask the role to resubmit)\n")
		for _, u := range b.Unreadable {
			cmd.Printf("  %s\n", u)
		}
	}
}

// printAction prints one operator action: who asked, what, the condition
// it applies under, what it waits on, and how to do it.
func printAction(cmd *cobra.Command, a hive.RoleAction) {
	var marks []string
	if a.Action.Irreversible {
		marks = append(marks, "irreversible")
	}
	if a.Action.Ongoing {
		marks = append(marks, "ongoing")
	}
	mark := ""
	if len(marks) > 0 {
		mark = " [" + strings.Join(marks, ", ") + "]"
	}
	who := strings.Join(a.Roles, ", ")
	if who == "" {
		who = a.Role
	}
	cmd.Printf("    %-14s %s%s\n", who, a.Action.Description, mark)
	note := func(label, text string) {
		cmd.Printf("    %-14s %s%s\n", "", label, text)
	}
	if a.Action.ID != "" {
		note("id: ", a.Action.ID)
	}
	if a.Condition != "" {
		note("only if decided: ", a.Condition)
	}
	if len(a.Action.After) > 0 {
		note("after: ", strings.Join(a.Action.After, ", "))
	}
	if len(a.Action.Versions) > 0 {
		note("versions: ", strings.Join(a.Action.Versions, ", "))
	}
	if len(a.Action.Credentials) > 0 {
		note("credentials: ", strings.Join(a.Action.Credentials, ", "))
	}
	if a.Action.Command != "" {
		note("$ ", a.Action.Command)
	}
	for i, step := range a.Action.Steps {
		note(fmt.Sprintf("%d. ", i+1), step)
	}
	if a.Action.FollowUp != "" {
		note("then the role: ", a.Action.FollowUp)
	}
}

// age renders a duration the way a briefing reads it: "now", "5m", "3h",
// "2d".
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}
