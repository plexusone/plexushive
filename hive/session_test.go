package hive

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeTmux replaces Tmux with an in-memory session environment.
func fakeTmux(t *testing.T, session string, env map[string]string) *[]string {
	t.Helper()
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	var calls []string
	saved := Tmux
	Tmux = func(args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		switch args[0] {
		case "display-message":
			return session, nil
		case "show-environment":
			v, ok := env[args[1]]
			if !ok {
				return "", errors.New("tmux show-environment " + args[1] + ": unknown variable: " + args[1])
			}
			return args[1] + "=" + v, nil
		case "set-environment":
			env[args[1]] = args[2]
			return "", nil
		}
		return "", errors.New("unexpected tmux call")
	}
	t.Cleanup(func() { Tmux = saved })
	return &calls
}

func TestResolveRole(t *testing.T) {
	t.Setenv(RoleEnv, "")
	t.Setenv("TMUX", "")
	if role, src, err := ResolveRole(""); err != nil || role != "" || src != "" {
		t.Fatalf("outside tmux: %q %q %v", role, src, err)
	}
	if role, src, _ := ResolveRole("given"); role != "given" || src != SourceGiven {
		t.Fatalf("given: %q %q", role, src)
	}
	env := map[string]string{}
	fakeTmux(t, "web", env)
	if role, _, err := ResolveRole(""); err != nil || role != "" {
		t.Fatalf("unbound: %q %v", role, err)
	}
	if err := BindTmuxRole("website"); err != nil {
		t.Fatal(err)
	}
	if role, src, err := ResolveRole(""); err != nil || role != "website" || src != SourceTmux {
		t.Fatalf("tmux: %q %q %v", role, src, err)
	}
	t.Setenv(RoleEnv, "billing")
	if role, src, _ := ResolveRole(""); role != "billing" || src != SourceEnv {
		t.Fatalf("env wins over tmux: %q %q", role, src)
	}
	if name, err := TmuxSession(); err != nil || name != "web" {
		t.Fatalf("session: %q %v", name, err)
	}
}

func TestTmuxErrors(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := BindTmuxRole("x"); err == nil {
		t.Fatal("bound outside tmux")
	}
	if name, err := TmuxSession(); err != nil || name != "" {
		t.Fatalf("session outside tmux: %q %v", name, err)
	}
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	saved := Tmux
	t.Cleanup(func() { Tmux = saved })
	Tmux = func(...string) (string, error) { return "", errors.New("no server running") }
	if _, err := TmuxRole(); err == nil {
		t.Fatal("tmux failure hidden")
	}
	Tmux = func(...string) (string, error) { return "-" + RoleEnv, nil }
	if role, err := TmuxRole(); err != nil || role != "" {
		t.Fatalf("removed variable: %q %v", role, err)
	}
}

func TestNewContext(t *testing.T) {
	h, err := Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	reports, err := LoadReports("testdata/reports")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewContext(h, reports, "nobody"); err == nil {
		t.Fatal("unknown role accepted")
	}
	h.Decisions = append(h.Decisions, Decision{ID: "coupon-stacking", Status: DecisionAccepted, Roles: []string{"billing"}})
	c, err := NewContext(h, reports, "billing")
	if err != nil {
		t.Fatal(err)
	}
	if c.Report == nil || c.Report.Role != "billing" || !reflect.DeepEqual(c.Answered, []string{"coupon-stacking"}) {
		t.Fatalf("report/answered: %+v %v", c.Report, c.Answered)
	}
	for _, d := range c.Decisions {
		if d.Status != DecisionAccepted || (len(d.Roles) > 0 && !contains(d.Roles, "billing")) {
			t.Fatalf("decision that does not bind billing: %+v", d)
		}
	}
	for _, p := range c.Peers {
		if p.Role == "website" && (p.Session == nil || p.Session.Tmux != "web") {
			t.Fatalf("peer session: %+v", p)
		}
	}
}
