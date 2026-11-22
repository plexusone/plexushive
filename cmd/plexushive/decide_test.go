package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/plexushive/hive"
)

// copyHive copies the example work identities, decisions and open items to
// a temporary hive.
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
	data, err := os.ReadFile(filepath.Join(example, hive.OpenItemsFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, hive.OpenItemsFile), data, 0o600); err != nil { //nolint:gosec // test copies a fixture file into a temporary directory
		t.Fatal(err)
	}
	return root
}

func TestDecide(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	root := copyHive(t)
	reports := example + "/reports"
	out, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "stack-two", "--reason", "Partners first.", "--reports", reports)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{
		"marked billing/coupon-stacking decided in open-items.json",
		"send to billing:",
		"[hive] type=status from=operator decided coupon-stacking: stack-two. Reason: Partners first. Act on it, record the outcome",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "wrote decisions/") {
		t.Errorf("a point decision wrote a decision file:\n%s", out)
	}
	h, err := hive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	it, ok := h.Open.Item("billing/coupon-stacking")
	if !ok || it.Status != hive.ItemDecided || it.Decision != "" || it.Answer == nil ||
		*it.Answer != (hive.Answer{Option: "stack-two", Reason: "Partners first.", Answered: "2026-11-22"}) {
		t.Fatalf("open item after deciding: %+v %+v", it, it.Answer)
	}
	if len(h.Decisions) != 2 {
		t.Fatalf("decisions: %d", len(h.Decisions))
	}
	out, err = run(t, "-C", root, "brief", "--reports", reports)
	for _, want := range []string{"Answered, still in reports (1)", "billing/coupon-stacking: answered stack-two", "Decisions needed (0)", "Open items: 1 draft, 1 deferred"} {
		if err != nil || !strings.Contains(out, want) {
			t.Errorf("brief after deciding: %v: missing %q\n%s", err, want, out)
		}
	}
	t.Setenv("TMUX", "")
	out, err = run(t, "-C", root, "context", "billing", "--reports", reports)
	if err != nil || !strings.Contains(out, "answered: coupon-stacking: stack-two (Partners first.)") {
		t.Fatalf("context after deciding: %v\n%s", err, out)
	}
	if _, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "stack-two", "--reports", reports); err == nil || !strings.Contains(err.Error(), "already decided") {
		t.Errorf("deciding twice: %v", err)
	}
	for _, args := range [][]string{
		{"decide", "billing-coupon-stacking", "x"},
		{"decide", "billing/nope", "x"},
		{"decide", "website/coupon-stacking", "x"},
		{"decide", "website/checkout-copy", "x"},
	} {
		args = append([]string{"-C", root}, append(args, "--reports", reports)...)
		if _, err := run(t, args...); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
}

func TestDecideRecord(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	root := copyHive(t)
	reports := example + "/reports"
	out, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "no-stacking", "--record", "--reports", reports)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(out, "wrote decisions/coupon-stacking.json") || !strings.Contains(out, "Read decisions/coupon-stacking.json") {
		t.Fatalf("record: %s", out)
	}
	h, err := hive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if it, _ := h.Open.Item("billing/coupon-stacking"); it.Decision != "coupon-stacking" || it.Answer.Option != "no-stacking" {
		t.Fatalf("recorded item: %+v", it)
	}
	if d := h.Decisions[0]; d.ID != "coupon-stacking" || d.Option != "no-stacking" {
		t.Fatalf("decision: %+v", d)
	}
}

func TestDecideDesignQuestion(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	root := copyHive(t)
	reports := t.TempDir()
	req := `{"id":"checkout-copy","title":"Who writes the checkout wording?","situation":"s","approval":"a","recommendation":"web",` +
		`"options":[{"name":"web","description":"d","costAndRisk":"c"},{"name":"billing","description":"d","costAndRisk":"c"}]}`
	report := `{"identity":"website","generated":"2026-11-22T08:00:00Z","decisionsNeeded":[` + req + `]}`
	if err := os.WriteFile(filepath.Join(reports, "website.json"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "-C", root, "decide", "website/checkout-copy", "web", "--reports", reports)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(out, "marked website/checkout-copy answered") || !strings.Contains(out, "Write the answer into ADR-004 (docs/adr/ADR-004-checkout.md)") {
		t.Fatalf("design question: %s", out)
	}
	// The work identity writes it into the ADR and drops the request.
	if err := os.WriteFile(filepath.Join(reports, "website.json"), []byte(`{"identity":"website","generated":"2026-11-22T10:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "-C", root, "open", "sync", "--reports", reports)
	if err != nil || !strings.Contains(out, "decided    website/checkout-copy") {
		t.Fatalf("sync after the ADR is written: %v %s", err, out)
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
