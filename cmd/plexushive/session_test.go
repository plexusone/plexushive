package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/plexushive/hive"
)

// fakeTmux runs the commands as if inside the tmux session named session,
// with env as its environment.
func fakeTmux(t *testing.T, session string, env map[string]string) {
	t.Helper()
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	saved := hive.Tmux
	hive.Tmux = func(args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return session, nil
		case "show-environment":
			if v, ok := env[args[1]]; ok {
				return args[1] + "=" + v, nil
			}
			return "", errors.New("unknown variable: " + args[1])
		case "set-environment":
			env[args[1]] = args[2]
			return "", nil
		}
		return "", errors.New("unexpected tmux call")
	}
	t.Cleanup(func() { hive.Tmux = saved })
}

func TestStartContextWhoami(t *testing.T) {
	t.Setenv(hive.IdentityEnv, "")
	t.Setenv(hive.LegacyIdentityEnv, "")
	t.Setenv(hive.ReportsEnv, example+"/reports")
	env := map[string]string{}
	fakeTmux(t, "billing-tmux", env)

	out, err := run(t, "whoami")
	if err != nil || !strings.Contains(out, "no work identity") || !strings.Contains(out, "tmux session billing-tmux") {
		t.Fatalf("whoami before start: %v %q", err, out)
	}
	if _, err := run(t, "-C", example, "start", "nobody"); err == nil {
		t.Fatal("started an unknown work identity")
	}
	out, err = run(t, "-C", example, "start", "billing")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{
		"bound billing to the tmux session billing-tmux",
		"Work identity billing (active, autonomy prepare)",
		"steward      github.com/example/billing",
		"website              agent website-1, tmux web",
		"Decisions that bind you",
		"report submit <file>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("start does not contain %q:\n%s", want, out)
		}
	}
	if env[hive.IdentityEnv] != "billing" {
		t.Fatalf("tmux environment: %v", env)
	}
	out, err = run(t, "whoami")
	if err != nil || !strings.Contains(out, "billing (from the tmux session)") {
		t.Fatalf("whoami: %v %q", err, out)
	}
	out, err = run(t, "--identity", "website", "whoami")
	if err != nil || !strings.Contains(out, "website (from --identity)") {
		t.Fatalf("whoami --identity: %v %q", err, out)
	}
	out, err = run(t, "-C", example, "context")
	if err != nil || !strings.Contains(out, "Work identity billing") || strings.Contains(out, "bound") {
		t.Fatalf("context: %v %q", err, out)
	}
	out, err = run(t, "-C", example, "context", "website", "--json")
	if err != nil || !strings.Contains(out, `"name": "website"`) {
		t.Fatalf("context --json: %v %q", err, out)
	}
}

func TestContextWithoutIdentity(t *testing.T) {
	t.Setenv(hive.IdentityEnv, "")
	t.Setenv(hive.LegacyIdentityEnv, "")
	t.Setenv("TMUX", "")
	if _, err := run(t, "-C", example, "context"); err == nil || !strings.Contains(err.Error(), "no work identity") {
		t.Fatalf("context without a work identity: %v", err)
	}
	out, err := run(t, "-C", example, "start", "website", "--reports", t.TempDir())
	if err != nil || !strings.Contains(out, "export HIVE_IDENTITY=website") || !strings.Contains(out, "none yet") {
		t.Fatalf("start outside tmux: %v %q", err, out)
	}
}

func TestSubmitFillsTmux(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC))
	t.Setenv(hive.IdentityEnv, "")
	t.Setenv(hive.LegacyIdentityEnv, "")
	fakeTmux(t, "web", map[string]string{hive.IdentityEnv: "website"})
	dir := t.TempDir()
	out, err := runIn(t, `{"session":{"agent":"website-1"},"summary":"s"}`, "-C", example, "report", "submit", "--reports", dir)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	reports, err := hive.LoadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Identity != "website" || reports[0].Session.Tmux != "web" || reports[0].Session.Agent != "website-1" {
		t.Fatalf("stored: %+v", reports)
	}
}

func TestUnreadableReport(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC))
	t.Setenv("TMUX", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "website.json"), []byte(`{"identity":"website","futureField":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runIn(t, `{"identity":"billing","summary":"s"}`, "-C", example, "report", "submit", "--reports", dir)
	if err != nil {
		t.Fatalf("submit beside an unreadable report: %v %s", err, out)
	}
	out, err = run(t, "-C", example, "brief", "--reports", dir)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	for _, want := range []string{"billing        now", "Unreadable reports", `website.json: json: unknown field "futureField"`} {
		if !strings.Contains(out, want) {
			t.Errorf("brief does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No report: website") {
		t.Errorf("a work identity with an unreadable report is listed as missing:\n%s", out)
	}
	out, err = run(t, "-C", example, "who", "github.com/example/web", "--reports", dir)
	if err != nil || !strings.Contains(out, "skipped an unreadable report: website.json") {
		t.Fatalf("who: %v %q", err, out)
	}
}

func TestSubmitShortPullRequest(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC))
	t.Setenv("TMUX", "")
	dir := t.TempDir()
	in := `{"identity":"billing","done":[{"description":"d","repositories":["github.com/example/billing"],"pullRequests":["#9"]}]}`
	if out, err := runIn(t, in, "-C", example, "report", "submit", "--reports", dir); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	reports, err := hive.LoadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reports[0].Done[0].PullRequests[0]; got != "https://github.com/example/billing/pull/9" {
		t.Fatalf("stored %q", got)
	}
}

func TestReportShow(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv(hive.IdentityEnv, "")
	t.Setenv(hive.LegacyIdentityEnv, "")
	reports := example + "/reports"
	out, err := run(t, "-C", example, "report", "show", "website", "--reports", reports)
	if err != nil || !strings.Contains(out, `"identity": "website"`) || !strings.Contains(out, `"commits": [`) {
		t.Fatalf("show: %v %q", err, out)
	}
	dir := t.TempDir()
	out, err = runIn(t, out, "-C", example, "report", "submit", "--reports", dir)
	if err != nil {
		t.Fatalf("resubmitting the shown report: %v %s", err, out)
	}
	if _, err := run(t, "-C", example, "report", "show", "--reports", reports); err == nil || !strings.Contains(err.Error(), "no work identity") {
		t.Fatalf("show without a work identity: %v", err)
	}
	if _, err := run(t, "-C", example, "--identity", "billing", "report", "show", "--reports", dir); err == nil || !strings.Contains(err.Error(), "no readable report") {
		t.Fatalf("show a missing report: %v", err)
	}
}
