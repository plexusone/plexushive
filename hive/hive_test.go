package hive

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/grokify/oscompat/testutil"
)

var update = flag.Bool("update", false, "regenerate the files under schema/")

// TestExample loads and validates the example hive in testdata/.
func TestExample(t *testing.T) {
	h, err := Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(h.Roles) != 3 || len(h.Decisions) != 2 {
		t.Fatalf("loaded %d roles and %d decisions", len(h.Roles), len(h.Decisions))
	}
	if r, ok := h.Role("billing"); !ok || r.Autonomy != AutonomyPrepare {
		t.Fatalf("Role(billing) = %+v, %v", r, ok)
	}
	if _, ok := h.Role("nobody"); ok {
		t.Fatal("Role(nobody) found")
	}
	s, err := Steward(h.Roles, "github.com/example/web")
	if err != nil || s != "website" {
		t.Fatalf("Steward(web) = %q, %v", s, err)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("want error for a directory without roles/")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, RolesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("want error for a directory without decisions/")
	}
	if err := os.Mkdir(filepath.Join(root, DecisionsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DecisionsDir, "x.json"), []byte(`{"id":"y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "file name") {
		t.Fatalf("want a file name error, got %v", err)
	}
	h := &Hive{Decisions: []Decision{{ID: "x"}}}
	if err := h.Validate(); err == nil {
		t.Fatal("want error for an invalid decision")
	}
}

// TestSchemas checks that schema/ matches the Go types. Run
// `go test -run TestSchemas -update` after changing a type. The comparison
// ignores CRLF versus LF, so it passes on a Windows checkout.
func TestSchemas(t *testing.T) {
	for file, gen := range Schemas {
		t.Run(file, func(t *testing.T) {
			want, err := gen()
			if err != nil {
				t.Fatal(err)
			}
			testutil.AssertGolden(t, filepath.Join("schema", file), want, *update)
		})
	}
}

func validRole() Role {
	return Role{Name: "alpha", Status: RoleActive, Mission: "Own product A.", Autonomy: AutonomyPrepare,
		Repositories: []RepositoryAffinity{{Pattern: "github.com/example/a", Affinity: AffinitySteward}}}
}

func validDecision() Decision {
	return Decision{ID: "use-x", Title: "Use X", Status: DecisionAccepted, Decision: "Use X.", Reasons: []string{"It fits."}}
}

func TestRoleValidate(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*Role)
		wantErr string
	}{
		{"valid", func(*Role) {}, ""},
		{"org pattern", func(r *Role) {
			r.Repositories = []RepositoryAffinity{{Pattern: "github.com/example/*", Affinity: AffinityContributor}}
		}, ""},
		{"bad name", func(r *Role) { r.Name = "Alpha" }, "name"},
		{"bad status", func(r *Role) { r.Status = "paused" }, "status"},
		{"no mission", func(r *Role) { r.Mission = " " }, "mission"},
		{"bad autonomy", func(r *Role) { r.Autonomy = "full" }, "autonomy"},
		{"bad pattern", func(r *Role) { r.Repositories[0].Pattern = "example/a" }, "must be github.com"},
		{"partial glob", func(r *Role) { r.Repositories[0].Pattern = "github.com/example/a*" }, "must be github.com"},
		{"bad affinity", func(r *Role) { r.Repositories[0].Affinity = "owner" }, "affinity"},
		{"repeated pattern", func(r *Role) {
			r.Repositories = append(r.Repositories, RepositoryAffinity{Pattern: "github.com/example/a", Affinity: AffinityContributor})
		}, "twice"},
		{"bad initiative", func(r *Role) { r.Initiatives = []string{"INIT-x-1"} }, "initiative"},
		{"self peer", func(r *Role) { r.Peers = []string{"alpha"} }, "own peer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRole()
			tt.edit(&r)
			checkErr(t, r.Validate(), tt.wantErr)
		})
	}
}

func TestValidateRoles(t *testing.T) {
	a := validRole()
	b := validRole()
	b.Name = "beta"
	b.Repositories = []RepositoryAffinity{{Pattern: "github.com/example/a", Affinity: AffinityContributor}}

	t.Run("shared as contributor", func(t *testing.T) {
		b := b
		b.Peers = []string{"alpha"}
		checkErr(t, ValidateRoles([]Role{a, b}), "")
	})
	t.Run("duplicate name", func(t *testing.T) {
		checkErr(t, ValidateRoles([]Role{a, a}), "duplicate name")
	})
	t.Run("unknown peer", func(t *testing.T) {
		b := b
		b.Peers = []string{"gamma"}
		checkErr(t, ValidateRoles([]Role{a, b}), "peer")
	})
	t.Run("two stewards", func(t *testing.T) {
		b := b
		b.Repositories = a.Repositories
		checkErr(t, ValidateRoles([]Role{a, b}), "two stewards")
	})
	t.Run("retired steward does not conflict", func(t *testing.T) {
		b := b
		b.Repositories = a.Repositories
		b.Status = RoleRetired
		checkErr(t, ValidateRoles([]Role{a, b}), "")
	})
}

func TestRolesFor(t *testing.T) {
	aff := func(p string, a Affinity) RepositoryAffinity { return RepositoryAffinity{Pattern: p, Affinity: a} }
	roles := []Role{
		{Name: "orgwide", Repositories: []RepositoryAffinity{aff("github.com/acme/*", AffinitySteward)}},
		{Name: "app", Repositories: []RepositoryAffinity{
			aff("github.com/acme/app", AffinitySteward), aff("github.com/acme/*", AffinityContributor)}},
		{Name: "helper", Repositories: []RepositoryAffinity{aff("github.com/acme/app", AffinityContributor)}},
		{Name: "gone", Status: RoleRetired, Repositories: []RepositoryAffinity{aff("github.com/acme/app", AffinityContributor)}},
	}
	tests := []struct {
		repo, steward string
		want          []Match
	}{
		{"github.com/acme/app", "app", []Match{
			{Role: "app", Affinity: AffinitySteward, Exact: true},
			{Role: "helper", Affinity: AffinityContributor, Exact: true},
			{Role: "orgwide", Affinity: AffinityContributor},
		}},
		{"github.com/acme/lib", "orgwide", []Match{
			{Role: "orgwide", Affinity: AffinitySteward},
			{Role: "app", Affinity: AffinityContributor},
		}},
		{"github.com/other/x", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			got, err := RolesFor(roles, tt.repo)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RolesFor:\n got %+v\nwant %+v", got, tt.want)
			}
			s, err := Steward(roles, tt.repo)
			if err != nil {
				t.Fatal(err)
			}
			if s != tt.steward {
				t.Fatalf("Steward = %q, want %q", s, tt.steward)
			}
		})
	}
	if _, err := RolesFor(roles, "acme/app"); err == nil {
		t.Fatal("want error for a malformed repository")
	}
	if _, err := Steward(roles, "github.com/acme/*"); err == nil {
		t.Fatal("want error for a pattern passed as a repository")
	}
}

func TestDecisionValidate(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*Decision)
		wantErr string
	}{
		{"valid", func(*Decision) {}, ""},
		{"bad id", func(d *Decision) { d.ID = "Use X" }, "id"},
		{"no title", func(d *Decision) { d.Title = "" }, "title"},
		{"no decision", func(d *Decision) { d.Decision = "" }, "decision is required"},
		{"no reasons", func(d *Decision) { d.Reasons = nil }, "reason"},
		{"accepted but superseded", func(d *Decision) { d.SupersededBy = "use-y" }, "accepted"},
		{"superseded without successor", func(d *Decision) { d.Status = DecisionSuperseded }, "superseded"},
		{"bad alternative", func(d *Decision) { d.Alternatives = []Alternative{{Option: "Y"}} }, "alternative"},
		{"bad ref", func(d *Decision) { d.Refs = []string{"PLAN-1"} }, "ref"},
		{"bad repository", func(d *Decision) { d.Repositories = []string{"a/b"} }, "repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDecision()
			tt.edit(&d)
			checkErr(t, d.Validate(), tt.wantErr)
		})
	}
}

func TestValidateDecisions(t *testing.T) {
	old := validDecision()
	old.Status = DecisionSuperseded
	old.SupersededBy = "use-y"
	next := validDecision()
	next.ID = "use-y"
	next.Supersedes = []string{"use-x"}

	t.Run("valid chain", func(t *testing.T) {
		checkErr(t, ValidateDecisions([]Decision{old, next}, nil), "")
	})
	t.Run("one-sided supersedes", func(t *testing.T) {
		old := validDecision()
		checkErr(t, ValidateDecisions([]Decision{old, next}, nil), "supersededBy")
	})
	t.Run("unknown successor", func(t *testing.T) {
		checkErr(t, ValidateDecisions([]Decision{old}, nil), "unknown")
	})
	t.Run("duplicate id", func(t *testing.T) {
		d := validDecision()
		checkErr(t, ValidateDecisions([]Decision{d, d}, nil), "duplicate id")
	})
	t.Run("unknown role", func(t *testing.T) {
		d := validDecision()
		d.Roles = []string{"alpha"}
		checkErr(t, ValidateDecisions([]Decision{d}, nil), "does not exist")
		checkErr(t, ValidateDecisions([]Decision{d}, []Role{validRole()}), "")
	})
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name, file, body, wantErr string
	}{
		{"unknown field", "alpha.json", `{"name":"alpha","status":"active","mission":"m","autonomy":"observe","extra":1}`, "unknown field"},
		{"name mismatch", "beta.json", `{"name":"alpha","status":"active","mission":"m","autonomy":"observe"}`, "file name"},
		{"trailing data", "alpha.json", `{"name":"alpha","status":"active","mission":"m","autonomy":"observe"} {}`, "trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, RolesDir)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadRoles(root)
			checkErr(t, err, tt.wantErr)
		})
	}
}

func checkErr(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("want error containing %q, got nil", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("want error containing %q, got %v", want, err)
	}
}
