package hive

import (
	"errors"
	"reflect"
	"testing"
)

// fakeTmux replaces Tmux with an in-memory session environment.
func fakeTmux(t *testing.T, session string, env map[string]string) {
	t.Helper()
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	saved := Tmux
	Tmux = func(args ...string) (string, error) {
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
}

func TestResolveIdentity(t *testing.T) {
	t.Setenv(IdentityEnv, "")
	t.Setenv(LegacyIdentityEnv, "")
	t.Setenv("TMUX", "")
	if identity, src, err := ResolveIdentity(""); err != nil || identity != "" || src != "" {
		t.Fatalf("outside tmux: %q %q %v", identity, src, err)
	}
	if identity, src, _ := ResolveIdentity("given"); identity != "given" || src != SourceGiven {
		t.Fatalf("given: %q %q", identity, src)
	}
	env := map[string]string{}
	fakeTmux(t, "web", env)
	if identity, _, err := ResolveIdentity(""); err != nil || identity != "" {
		t.Fatalf("unbound: %q %v", identity, err)
	}
	if err := BindTmuxIdentity("website"); err != nil {
		t.Fatal(err)
	}
	if identity, src, err := ResolveIdentity(""); err != nil || identity != "website" || src != SourceTmux {
		t.Fatalf("tmux: %q %q %v", identity, src, err)
	}
	t.Setenv(IdentityEnv, "billing")
	if identity, src, _ := ResolveIdentity(""); identity != "billing" || src != SourceEnv {
		t.Fatalf("env wins over tmux: %q %q", identity, src)
	}
	if name, err := TmuxSession(); err != nil || name != "web" {
		t.Fatalf("session: %q %v", name, err)
	}
}

func TestTmuxErrors(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := BindTmuxIdentity("x"); err == nil {
		t.Fatal("bound outside tmux")
	}
	if name, err := TmuxSession(); err != nil || name != "" {
		t.Fatalf("session outside tmux: %q %v", name, err)
	}
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	saved := Tmux
	t.Cleanup(func() { Tmux = saved })
	Tmux = func(...string) (string, error) { return "", errors.New("no server running") }
	if _, err := TmuxIdentity(); err == nil {
		t.Fatal("tmux failure hidden")
	}
	Tmux = func(...string) (string, error) { return "-" + IdentityEnv, nil }
	if identity, err := TmuxIdentity(); err != nil || identity != "" {
		t.Fatalf("removed variable: %q %v", identity, err)
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
		t.Fatal("unknown work identity accepted")
	}
	h.Decisions = append(h.Decisions, Decision{ID: "coupon-stacking", Status: DecisionAccepted, Identities: []string{"billing"}})
	c, err := NewContext(h, reports, "billing")
	if err != nil {
		t.Fatal(err)
	}
	if c.Report == nil || c.Report.Identity != "billing" || !reflect.DeepEqual(c.Answered, []string{"coupon-stacking"}) {
		t.Fatalf("report/answered: %+v %v", c.Report, c.Answered)
	}
	for _, d := range c.Decisions {
		if d.Status != DecisionAccepted || (len(d.Identities) > 0 && !contains(d.Identities, "billing")) {
			t.Fatalf("decision that does not bind billing: %+v", d)
		}
	}
	for _, p := range c.Peers {
		if p.Identity == "website" && (p.Session == nil || p.Session.Tmux != "web") {
			t.Fatalf("peer session: %+v", p)
		}
	}
}
