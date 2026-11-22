package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/plexushive/hive"
)

func runIn(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}

func TestReportSubmit(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	fixClock(t, at)
	dir := t.TempDir()
	t.Setenv(hive.IdentityEnv, "billing")

	out, err := runIn(t, `{"summary":"working"}`, "-C", example, "report", "submit", "--reports", dir)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(out, "stored the billing report") {
		t.Fatalf("output: %q", out)
	}
	reports, err := hive.LoadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Identity != "billing" || !reports[0].Generated.Equal(at) {
		t.Fatalf("stored: %+v", reports)
	}

	file := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(file, []byte(`{"identity":"website","generated":"2026-11-19T10:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(t, "", "-C", example, "report", "submit", file, "--reports", dir); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	t.Setenv(hive.ReportsEnv, dir)
	bad := map[string]string{
		"unknown work identity": `{"identity":"nobody"}`,
		"unknown field":         `{"extra":1}`,
		"not json":              `x`,
		"invalid":               `{"done":[{"description":""}]}`,
	}
	for name, in := range bad {
		if _, err := runIn(t, in, "-C", example, "report", "submit", "-"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := runIn(t, "", "-C", example, "report", "submit", filepath.Join(dir, "missing.json")); err == nil {
		t.Error("want error for a missing file")
	}
	if _, err := runIn(t, "{}", "-C", t.TempDir(), "report", "submit"); err == nil {
		t.Error("want error for an empty hive directory")
	}
	file = filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runIn(t, "{}", "-C", example, "report", "submit", "--reports", filepath.Join(file, "sub")); err == nil {
		t.Error("want error writing under a file")
	}
}

func TestBrief(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC))
	out, err := run(t, "-C", example, "brief", "--reports", example+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Decisions needed (1)",
		"coupon-stacking: Allow coupons to stack?",
		"recommends no-stacking",
		"waiting on operator",
		"billing        2h",
		"website        2h",
		"Your actions (3)",
		"then the work identity: Open the pull request.",
		"  configure\n    website        Set the analytics key for the launch page.",
		"after: push-launch",
		"credentials: web/prod/ANALYTICS_KEY",
		"1. Create a site key in the analytics console.",
		"account-link (committed)",
		"  push\n    website        Push the launch page branch.",
		"$ git -C web push -u origin launch-page",
		"Tag the coupon release. [irreversible]",
		"github.com/example/web  [more than one work identity]",
		"billing        refund-export",
		"then: The refund report for finance.",
		"Notices (1)",
		"billing/coupon-stacking: Allow coupons to stack?",
		"asked 26h ago",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("brief does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No report") {
		t.Errorf("no work identity should be missing:\n%s", out)
	}

	empty := t.TempDir()
	t.Setenv(hive.ReportsEnv, empty)
	out, err = run(t, "-C", example, "brief")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No report: billing, website") {
		t.Errorf("brief without reports:\n%s", out)
	}

	out, err = run(t, "-C", example, "brief", "--json", "--reports", example+"/reports")
	if err != nil {
		t.Fatal(err)
	}
	var b hive.Briefing
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if len(b.Decisions) != 1 || len(b.Blocked) != 2 || len(b.Reports) != 2 {
		t.Fatalf("json briefing: %+v", b)
	}

	if _, err := run(t, "-C", t.TempDir(), "brief"); err == nil {
		t.Error("want error for an empty hive directory")
	}
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "x.json"), []byte(`{"identity":"y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "-C", example, "brief", "--reports", bad)
	if err != nil || !strings.Contains(out, `x.json: work identity "y" does not match the file name`) {
		t.Errorf("misnamed report: %v\n%s", err, out)
	}
}

func TestAge(t *testing.T) {
	tests := map[time.Duration]string{
		30 * time.Second: "now",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		72 * time.Hour:   "3d",
	}
	for d, want := range tests {
		if got := age(d); got != want {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
}
