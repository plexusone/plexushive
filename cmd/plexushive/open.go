package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/plexusone/plexushive/hive"
)

func openCmd(hiveDir *string) *cobra.Command {
	var statuses []string
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:   "open [identity]",
		Short: "List the questions waiting on the operator, by status and work identity",
		Long: "Lists open-items.json: open questions with their recommendation first,\n" +
			"then drafts that still need options, then deferred ones. Decided and\n" +
			"withdrawn items are shown with --all or --status. Give a work identity\n" +
			"to list only its items. Run open sync to bring the list up to date.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := hive.Load(*hiveDir)
			if err != nil {
				return err
			}
			want, err := wantedStatuses(statuses, all)
			if err != nil {
				return err
			}
			var items []hive.OpenItem
			for _, it := range h.Open.Items {
				if len(args) == 1 && it.Identity != args[0] {
					continue
				}
				if want[it.Status] {
					items = append(items, it)
				}
			}
			if asJSON {
				out, err := json.MarshalIndent(hive.OpenItems{Updated: h.Open.Updated, Items: items}, "", "  ")
				if err != nil {
					return fmt.Errorf("plexushive: %w", err)
				}
				cmd.Println(string(out))
				return nil
			}
			printOpenItems(cmd, items)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&statuses, "status", nil, "only these statuses: draft, open, deferred, decided, withdrawn")
	cmd.Flags().BoolVar(&all, "all", false, "include decided and withdrawn items")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the items as JSON")
	cmd.AddCommand(openSyncCmd(hiveDir))
	return cmd
}

func wantedStatuses(statuses []string, all bool) (map[hive.ItemStatus]bool, error) {
	want := map[hive.ItemStatus]bool{}
	for _, s := range statuses {
		st := hive.ItemStatus(s)
		if !slices.Contains(hive.ItemStatuses, st) {
			return nil, fmt.Errorf("plexushive: status %q must be draft, open, deferred, decided or withdrawn", s)
		}
		want[st] = true
	}
	if len(want) == 0 {
		for _, st := range hive.ItemStatuses {
			want[st] = all || !st.Closed()
		}
	}
	return want, nil
}

var statusHeadings = map[hive.ItemStatus]string{
	hive.ItemOpen:      "Open, waiting on the operator",
	hive.ItemDraft:     "Draft, waiting on its work identity for options",
	hive.ItemDeferred:  "Deferred",
	hive.ItemDecided:   "Decided",
	hive.ItemWithdrawn: "Withdrawn",
}

func printOpenItems(cmd *cobra.Command, items []hive.OpenItem) {
	if len(items) == 0 {
		cmd.Println("no open items")
		return
	}
	first := true
	for _, st := range hive.ItemStatuses {
		var group []hive.OpenItem
		for _, it := range items {
			if it.Status == st {
				group = append(group, it)
			}
		}
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].Key < group[j].Key })
		if !first {
			cmd.Println()
		}
		first = false
		cmd.Printf("%s (%d)\n", statusHeadings[st], len(group))
		for _, it := range group {
			where := ""
			if it.Source.Document != "" {
				where = "  [" + it.Source.Document + "]"
			}
			cmd.Printf("  %s%s\n      %s\n", it.Key, where, it.Request.Title)
			var details []string
			if st == hive.ItemOpen && it.Request.Recommendation != "" {
				details = append(details, "recommends "+it.Request.Recommendation)
			}
			if it.Decision != "" {
				details = append(details, "decisions/"+it.Decision+".json")
			}
			if len(it.Related) > 0 {
				details = append(details, "decide with "+strings.Join(it.Related, ", "))
			}
			if len(details) > 0 {
				cmd.Printf("      %s\n", strings.Join(details, "; "))
			}
			if it.Note != "" {
				cmd.Printf("      note: %s\n", it.Note)
			}
		}
	}
}

func openSyncCmd(hiveDir *string) *cobra.Command {
	var reports string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Bring open-items.json up to date with the reports and the decision log",
		Long: "Adds decision requests not yet listed, opens a draft whose work identity\n" +
			"now asks it in a report, marks items the decision log answers as\n" +
			"decided, and withdraws report questions their work identity dropped\n" +
			"without a decision. Work identities without a readable report are left\n" +
			"as they are.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			res := h.Open.Sync(rs, h.Decisions)
			if !res.Changed() {
				cmd.Println("open items are up to date")
				return nil
			}
			for _, c := range []struct {
				label string
				keys  []string
			}{{"added", res.Added}, {"opened", res.Opened}, {"updated", res.Updated}, {"decided", res.Decided}, {"withdrawn", res.Withdrawn}} {
				for _, k := range c.keys {
					cmd.Printf("%-10s %s\n", c.label, k)
				}
			}
			if dryRun {
				return nil
			}
			h.Open.Updated = now().Format("2006-01-02")
			if err := hive.ValidateOpenItems(h.Open, h.Identities, h.Decisions); err != nil {
				return err
			}
			return hive.WriteOpenItems(*hiveDir, h.Open)
		},
	}
	reportsFlag(cmd, &reports)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the changes without writing them")
	return cmd
}
