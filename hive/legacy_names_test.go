package hive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "roles", "billing.json"), `{"name":"billing","status":"active","mission":"m","autonomy":"prepare",
		"repositories":[{"pattern":"github.com/o/billing","affinity":"steward"},{"pattern":"github.com/o/web","affinity":"contributor"}]}`)
	writeFile(t, filepath.Join(root, "decisions", "d.json"), `{"id":"d","title":"t","status":"accepted","decision":"x","reasons":["r"],"roles":["billing"]}`)
	h, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := h.Identities[0].Repositories[1].Affinity; got != AffinityCollaborator {
		t.Fatalf("contributor read as %q", got)
	}
	if !reflect.DeepEqual(h.Decisions[0].Identities, []string{"billing"}) {
		t.Fatalf("decision roles: %v", h.Decisions[0].Identities)
	}

	writeFile(t, filepath.Join(root, "identities", "billing.json"), `{}`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "both exist") {
		t.Fatalf("both directories: %v", err)
	}

	var r Report
	if err := decodeStrict([]byte(`{"role":"billing"}`), &r); err != nil || r.Identity != "billing" {
		t.Fatalf("report role: %q %v", r.Identity, err)
	}
	if err := decodeStrict([]byte(`{"role":"billing","identity":"web"}`), &r); err == nil {
		t.Fatal("a report naming two identities was accepted")
	}
	var d Decision
	if err := decodeStrict([]byte(`{"id":"d","roles":["a"],"identities":["b"]}`), &d); err == nil {
		t.Fatal("a decision with roles and identities was accepted")
	}
	var a Affinity
	if err := decodeStrict([]byte(`7`), &a); err == nil {
		t.Fatal("a number was accepted as an affinity")
	}
}

func TestResolveLegacyIdentity(t *testing.T) {
	t.Setenv(IdentityEnv, "")
	t.Setenv(LegacyIdentityEnv, "billing")
	t.Setenv("TMUX", "")
	if id, src, err := ResolveIdentity(""); err != nil || id != "billing" || src != SourceLegacyEnv || !src.Legacy() {
		t.Fatalf("legacy environment: %q %q %v", id, src, err)
	}
	env := map[string]string{}
	fakeTmux(t, "web", env)
	if err := BindTmuxIdentity("website"); err != nil {
		t.Fatal(err)
	}
	if id, src, _ := ResolveIdentity(""); id != "website" || src != SourceTmux || src.Legacy() {
		t.Fatalf("a new binding wins over the old variable: %q %q", id, src)
	}
	t.Setenv(LegacyIdentityEnv, "")
	delete(env, IdentityEnv)
	env[LegacyIdentityEnv] = "billing"
	if id, src, _ := ResolveIdentity(""); id != "billing" || src != SourceLegacyTmux {
		t.Fatalf("legacy tmux: %q %q", id, src)
	}
}
