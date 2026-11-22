package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/plexushive/hive"
)

func TestOpen(t *testing.T) {
	out, err := run(t, "-C", example, "open")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Open, waiting on the operator (1)",
		"  billing/coupon-stacking\n      Allow coupons to stack?\n      recommends no-stacking; decide with website/checkout-copy",
		"Draft, waiting on its work identity for options (1)",
		"  website/checkout-copy  [ADR-004]",
		"Deferred (1)",
		"      note: Waits on the privacy review.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("open does not contain %q:\n%s", want, out)
		}
	}
	out, err = run(t, "-C", example, "open", "website", "--status", "draft")
	if err != nil || strings.Contains(out, "coupon") || strings.Contains(out, "Deferred") || !strings.Contains(out, "checkout-copy") {
		t.Fatalf("filtered: %v %q", err, out)
	}
	out, err = run(t, "-C", example, "open", "--status", "decided")
	if err != nil || !strings.Contains(out, "no open items") {
		t.Fatalf("no decided items: %v %q", err, out)
	}
	if _, err := run(t, "-C", example, "open", "--status", "done"); err == nil {
		t.Fatal("unknown status accepted")
	}
	out, err = run(t, "-C", example, "open", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var o hive.OpenItems
	if err := json.Unmarshal([]byte(out), &o); err != nil || len(o.Items) != 3 {
		t.Fatalf("json: %v %d", err, len(o.Items))
	}
}

func TestOpenSync(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	root := copyHive(t)
	reports := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(reports, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// billing dropped coupon-stacking; website frames its draft and asks
	// something new.
	write("billing.json", `{"identity":"billing","generated":"2026-11-22T08:00:00Z","summary":"s"}`)
	opts := `"options":[{"name":"web","description":"d","costAndRisk":"c"},{"name":"billing","description":"d","costAndRisk":"c"}],"recommendation":"web","situation":"s","approval":"a"`
	write("website.json", `{"identity":"website","generated":"2026-11-22T08:00:00Z","decisionsNeeded":[
		{"id":"checkout-copy","title":"Who writes the checkout wording?",`+opts+`},
		{"id":"hero-image","title":"Which hero image?",`+opts+`}]}`)

	out, err := run(t, "-C", root, "open", "sync", "--reports", reports, "--dry-run")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{"added      website/hero-image", "opened     website/checkout-copy", "withdrawn  billing/coupon-stacking"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync does not contain %q:\n%s", want, out)
		}
	}
	h, err := hive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if it, _ := h.Open.Item("billing/coupon-stacking"); it.Status != hive.ItemOpen {
		t.Fatal("--dry-run wrote the file")
	}

	if out, err := run(t, "-C", root, "open", "sync", "--reports", reports); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	h, err = hive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	draft, _ := h.Open.Item("website/checkout-copy")
	if draft.Status != hive.ItemOpen || draft.Source.Document != "ADR-004" || draft.Request.Recommendation != "web" {
		t.Fatalf("framed draft: %+v", draft)
	}
	if h.Open.Updated != "2026-11-22" || h.Open.Items[0].Key != "billing/coupon-stacking" {
		t.Fatalf("written: %s %s", h.Open.Updated, h.Open.Items[0].Key)
	}
	out, err = run(t, "-C", root, "open", "sync", "--reports", reports)
	if err != nil || !strings.Contains(out, "up to date") {
		t.Fatalf("second sync: %v %q", err, out)
	}
}

func TestOpenShowsAnswers(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	root := copyHive(t)
	if _, err := run(t, "-C", root, "decide", "billing/coupon-stacking", "stack-two", "--reason", "Partners first.", "--reports", example+"/reports"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "-C", root, "open", "billing", "--all")
	if err != nil || !strings.Contains(out, "Decided (1)") || !strings.Contains(out, "answered stack-two on 2026-11-22 (Partners first.)") {
		t.Fatalf("open --all: %v\n%s", err, out)
	}
}
