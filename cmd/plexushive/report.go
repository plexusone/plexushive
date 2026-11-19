package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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

func reportCmd(hiveDir *string) *cobra.Command {
	cmd := &cobra.Command{Use: "report", Short: "Write status reports"}
	var reports string
	submit := &cobra.Command{
		Use:   "submit [file|-]",
		Short: "Validate a report and store it as the role's current report",
		Long: "Reads a report as JSON from a file or standard input. An empty role is\n" +
			"taken from $" + hive.RoleEnv + " and an empty generated time is set to now.\n" +
			"The role must exist in the hive directory.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := readReport(cmd.InOrStdin(), args)
			if err != nil {
				return err
			}
			if r.Role == "" {
				r.Role = os.Getenv(hive.RoleEnv)
			}
			if r.Generated.IsZero() {
				r.Generated = now().UTC().Truncate(time.Second)
			}
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			if err := hive.ValidateReports([]hive.Report{r}, h.Roles); err != nil {
				return err
			}
			dir, err := resolveReports(reports)
			if err != nil {
				return err
			}
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
		Short: "Show decisions needed, blockers and each role's status",
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
			rs, err := hive.LoadReports(dir)
			if err != nil {
				return err
			}
			b := hive.NewBriefing(h.Roles, rs)
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
	cmd.Printf("Decisions needed (%d)\n", len(b.Decisions))
	for _, d := range b.Decisions {
		cmd.Printf("  %-14s %s: %s\n", d.Role, d.Request.ID, d.Request.Title)
		cmd.Printf("  %-14s recommends %s; approval: %s\n", "", d.Request.Recommendation, d.Request.Approval)
	}
	cmd.Printf("\nBlocked (%d)\n", len(b.Blocked))
	for _, bl := range b.Blocked {
		cmd.Printf("  %-14s waiting on %-12s %s\n", bl.Role, bl.Blocker.WaitingOn, bl.Blocker.Description)
	}
	cmd.Printf("\nStatus (%d)\n", len(b.Reports))
	for _, r := range b.Reports {
		cmd.Printf("  %-14s %-8s %s\n", r.Role, age(r.Age(at)), r.Summary)
	}
	if len(b.Missing) > 0 {
		cmd.Printf("\nNo report: %s\n", strings.Join(b.Missing, ", "))
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
