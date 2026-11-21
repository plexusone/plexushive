package main

import (
	"errors"
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
	t.Setenv(hive.RoleEnv, "")
	t.Setenv(hive.ReportsEnv, example+"/reports")
	env := map[string]string{}
	fakeTmux(t, "billing-tmux", env)

	out, err := run(t, "whoami")
	if err != nil || !strings.Contains(out, "no role") || !strings.Contains(out, "tmux session billing-tmux") {
		t.Fatalf("whoami before start: %v %q", err, out)
	}
	if _, err := run(t, "-C", example, "start", "nobody"); err == nil {
		t.Fatal("started an unknown role")
	}
	out, err = run(t, "-C", example, "start", "billing")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{
		"bound billing to the tmux session billing-tmux",
		"Role billing (active, autonomy prepare)",
		"steward      github.com/example/billing",
		"website              agent website-1, tmux web",
		"Decisions that bind you",
		"report submit <file>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("start does not contain %q:\n%s", want, out)
		}
	}
	if env[hive.RoleEnv] != "billing" {
		t.Fatalf("tmux environment: %v", env)
	}
	out, err = run(t, "whoami")
	if err != nil || !strings.Contains(out, "billing (from the tmux session)") {
		t.Fatalf("whoami: %v %q", err, out)
	}
	out, err = run(t, "--role", "website", "whoami")
	if err != nil || !strings.Contains(out, "website (from --role)") {
		t.Fatalf("whoami --role: %v %q", err, out)
	}
	out, err = run(t, "-C", example, "context")
	if err != nil || !strings.Contains(out, "Role billing") || strings.Contains(out, "bound") {
		t.Fatalf("context: %v %q", err, out)
	}
	out, err = run(t, "-C", example, "context", "website", "--json")
	if err != nil || !strings.Contains(out, `"name": "website"`) {
		t.Fatalf("context --json: %v %q", err, out)
	}
}

func TestContextWithoutRole(t *testing.T) {
	t.Setenv(hive.RoleEnv, "")
	t.Setenv("TMUX", "")
	if _, err := run(t, "-C", example, "context"); err == nil || !strings.Contains(err.Error(), "no role") {
		t.Fatalf("context without a role: %v", err)
	}
	out, err := run(t, "-C", example, "start", "website", "--reports", t.TempDir())
	if err != nil || !strings.Contains(out, "export HIVE_ROLE=website") || !strings.Contains(out, "none yet") {
		t.Fatalf("start outside tmux: %v %q", err, out)
	}
}

func TestSubmitFillsTmux(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC))
	t.Setenv(hive.RoleEnv, "")
	fakeTmux(t, "web", map[string]string{hive.RoleEnv: "website"})
	dir := t.TempDir()
	out, err := runIn(t, `{"session":{"agent":"website-1"},"summary":"s"}`, "-C", example, "report", "submit", "--reports", dir)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	reports, err := hive.LoadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Role != "website" || reports[0].Session.Tmux != "web" || reports[0].Session.Agent != "website-1" {
		t.Fatalf("stored: %+v", reports)
	}
}
