package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/plexushive/hive"
)

// copyHive copies the example work identities and decisions to a temporary hive.
func copyHive(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"identities", "decisions"} {
		if err := os.Mkdir(filepath.Join(root, sub), 0o750); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(example, sub))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(example, sub, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, sub, e.Name()), data, 0o600); err != nil { //nolint:gosec // test copies fixture files into a temporary directory
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestDecide(t *testing.T) {
	root := copyHive(t)
	reports := example + "/reports"
	out, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "stack-two", "--reason", "Partners first.", "--reports", reports)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{
		"wrote decisions/coupon-stacking.json",
		"send to billing:",
		"[hive] type=status from=operator",
		"decided coupon-stacking: stack-two",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	h, err := hive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "-C", root, "brief", "--reports", reports)
	if err != nil || !strings.Contains(out, "Answered, still in reports (1)") || !strings.Contains(out, "Decisions needed (0)") {
		t.Fatalf("brief after deciding: %v\n%s", err, out)
	}
	if _, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "stack-two", "--reports", reports); err == nil {
		t.Error("deciding twice should fail")
	}
	for _, args := range [][]string{
		{"decide", "billing-coupon-stacking", "x"},
		{"decide", "billing/nope", "x"},
		{"decide", "website/coupon-stacking", "x"},
	} {
		args = append([]string{"-C", root}, append(args, "--reports", reports)...)
		if _, err := run(t, args...); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
}

func TestWho(t *testing.T) {
	out, err := run(t, "-C", example, "who", "github.com/example/web", "--reports", example+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"steward      website          agent website-1, tmux web",
		"collaborator billing",
		"billing          refund-export",
		"website          account-link",
		"website          push       Push the launch page branch.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	out, err = run(t, "-C", example, "who", "github.com/other/repo", "--reports", t.TempDir())
	if err != nil || !strings.Contains(out, "common ground") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := run(t, "-C", example, "who", "not-a-repo"); err == nil {
		t.Error("want error for a bad repository")
	}
}

func TestKeepAsked(t *testing.T) {
	first := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	gen := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	prev := []hive.Report{{Identity: "a", DecisionsNeeded: []hive.DecisionRequest{{ID: "old", Asked: first}}}}
	r := hive.Report{Identity: "a", Generated: gen, DecisionsNeeded: []hive.DecisionRequest{{ID: "old"}, {ID: "new"}}}
	keepAsked(&r, prev)
	if !r.DecisionsNeeded[0].Asked.Equal(first) || !r.DecisionsNeeded[1].Asked.Equal(gen) {
		t.Fatalf("asked: %+v", r.DecisionsNeeded)
	}
}

func TestVersion(t *testing.T) {
	out, err := run(t, "--version")
	if err != nil || !strings.Contains(out, "plexushive version") {
		t.Fatalf("%v: %q", err, out)
	}
}
