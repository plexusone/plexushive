package hive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLegacyShapes(t *testing.T) {
	var a Action
	err := decodeStrict([]byte(`{"kind":"tag","description":"x","commit":"3f2a9c1","pullRequest":"https://github.com/o/r/pull/1",
		"version":"v1.0.0","versions":["v1.0.0","sub/v1.0.0"],"unblocks":"the launch"}`), &a)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Commits, []string{"3f2a9c1"}) || len(a.PullRequests) != 1 ||
		!reflect.DeepEqual(a.Versions, []string{"v1.0.0", "sub/v1.0.0"}) || !reflect.DeepEqual(a.Unblocks, []string{"the launch"}) {
		t.Fatalf("action: %+v", a)
	}
	var b Blocker
	if err := decodeStrict([]byte(`{"description":"x","waitingOn":"operator","unblocks":["a","b"]}`), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.WaitingOn, []string{"operator"}) || len(b.Unblocks) != 2 || b.Description != "x" {
		t.Fatalf("blocker: %+v", b)
	}
	if err := decodeStrict([]byte(`{"description":"x","waitingOn":["a"],"surprise":1}`), &b); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := decodeStrict([]byte(`{"kind":"push","description":"x","unblocks":7}`), &a); err == nil {
		t.Fatal("number accepted for unblocks")
	}
	var d DecisionRequest
	if err := decodeStrict([]byte(`{"id":"q","asked":"2026-11-19"}`), &d); err != nil {
		t.Fatal(err)
	}
	if !d.Asked.Equal(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("asked: %v", d.Asked)
	}
	if err := decodeStrict([]byte(`{"id":"q","asked":"yesterday"}`), &d); err == nil {
		t.Fatal("bad asked accepted")
	}
	// What is written uses the lists.
	out, err := json.Marshal(Action{Kind: ActionPush, Description: "x", Commits: []string{"abc1234"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"commit"`) {
		t.Fatalf("wrote a single commit: %s", out)
	}
}

func linkedReport() Report {
	return Report{
		Identity:  "a",
		Generated: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		OperatorActions: []Action{
			{ID: "push", Kind: ActionPush, Description: "push", Repository: "github.com/o/r", PullRequests: []string{"#12"}},
			{ID: "tag", Kind: ActionTag, Description: "tag", After: []string{"push"}, Versions: []string{"v1.0.0", "sub/v1.0.0"}},
			{Kind: ActionMerge, Description: "merge", Decision: "land", Option: "now"},
		},
		DecisionsNeeded: []DecisionRequest{{
			ID: "land", Title: "Land now?", Situation: "s", Approval: "a", Recommendation: "now",
			Options: []Option{{Name: "now", Description: "d", CostAndRisk: "c"}, {Name: "later", Description: "d", CostAndRisk: "c"}},
		}},
	}
}

func TestActionLinks(t *testing.T) {
	r := linkedReport()
	r.Normalize()
	if r.OperatorActions[0].PullRequests[0] != "https://github.com/o/r/pull/12" {
		t.Fatalf("normalize: %v", r.OperatorActions[0].PullRequests)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Report)
		want   string
	}{
		{"duplicate id", func(r *Report) { r.OperatorActions[1].ID = "push" }, `id "push" listed twice`},
		{"unknown after", func(r *Report) { r.OperatorActions[1].After = []string{"nope"} }, `after "nope"`},
		{"cycle", func(r *Report) { r.OperatorActions[0].After = []string{"tag"} }, "waits on itself"},
		{"unknown decision", func(r *Report) { r.OperatorActions[2].Decision = "other" }, `decision "other"`},
		{"unknown option", func(r *Report) { r.OperatorActions[2].Option = "never" }, `option "never"`},
		{"option alone", func(r *Report) { r.OperatorActions[2].Decision = "" }, "option needs the decision"},
		{"bad id", func(r *Report) { r.OperatorActions[0].ID = "Push" }, `id "Push"`},
		{"short pull without repository", func(r *Report) { r.OperatorActions[1].PullRequests = []string{"#3"} }, "needs exactly one repository"},
		{"bad credential", func(r *Report) { r.OperatorActions[1].Credentials = []string{"has space"} }, `credential "has space"`},
		{"bad work state", func(r *Report) {
			r.InProgress = []Work{{Description: "w", Repository: "github.com/o/r", State: StateReleased}}
		}, `state "released"`},
		{"empty waiting on", func(r *Report) { r.Blocked = []Blocker{{Item: Item{Description: "b"}}} }, "waitingOn needs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := linkedReport()
			r.Normalize()
			tc.mutate(&r)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	item := Item{Description: "x", Repositories: []string{"github.com/o/r"}, PullRequests: []string{"#4"}, State: StateUncommitted}
	r = Report{Identity: "a", Generated: time.Now(), Done: []Item{item},
		OperatorActions: []Action{{ID: "deploy-v0.3.0", Kind: ActionDeploy, Description: "d", Repository: "github.com/o/r", PullRequests: []string{"#5"}}}}
	if err := r.Validate(); err != nil {
		t.Fatalf("#<n> with one repository, before normalizing: %v", err)
	}
	r.Normalize()
	if r.Done[0].PullRequests[0] != "https://github.com/o/r/pull/4" || r.Validate() != nil {
		t.Fatalf("item: %+v %v", r.Done[0], r.Validate())
	}
}

func TestBriefingMergesAndConditions(t *testing.T) {
	identities := []WorkIdentity{{Name: "a", Status: IdentityActive}, {Name: "b", Status: IdentityActive}, {Name: "c", Status: IdentityActive}}
	push := Action{Kind: ActionPush, Description: "Push infra.", Command: "git -C infra  push", Repository: "github.com/o/infra", Branch: "main"}
	same := push
	same.Command = "cd infra && git push origin main"
	same.Description = "Push my infra commits."
	a := linkedReport()
	a.OperatorActions = append(a.OperatorActions, push)
	b := Report{Identity: "b", OperatorActions: []Action{same, {Kind: ActionReview, Description: "no command"}}}
	c := Report{Identity: "c", DecisionsNeeded: []DecisionRequest{{ID: "q"}},
		Blocked: []Blocker{{Item: Item{Description: "x"}, WaitingOn: []string{Operator}, Decision: "q"}}}

	br := NewBriefing(identities, nil, []Report{a, b, c})
	var infra *IdentityAction
	for i := range br.Actions {
		if br.Actions[i].Action.Repository == "github.com/o/infra" {
			if infra != nil {
				t.Fatal("identical pushes listed twice")
			}
			infra = &br.Actions[i]
		}
	}
	if infra == nil || !reflect.DeepEqual(infra.Identities, []string{"a", "b"}) || infra.Identity != "a" {
		t.Fatalf("merged: %+v", infra)
	}
	if len(br.Actions) != 5 {
		t.Fatalf("actions: %d", len(br.Actions))
	}
	merge := findAction(br, ActionMerge)
	if merge == nil || merge.Condition != "a/land = now" {
		t.Fatalf("condition: %+v", merge)
	}
	if !reflect.DeepEqual(br.Waiting, []string{"c"}) {
		t.Fatalf("waiting: %v", br.Waiting)
	}

	br = NewBriefing(identities, []Decision{{ID: "land", Option: "later"}}, []Report{a})
	if findAction(br, ActionMerge) != nil {
		t.Fatal("action for the option not chosen is listed")
	}
	br = NewBriefing(identities, []Decision{{ID: "land", Option: "now"}}, []Report{a})
	if m := findAction(br, ActionMerge); m == nil || m.Condition != "" {
		t.Fatalf("decided option: %+v", m)
	}
}

func findAction(b Briefing, kind ActionKind) *IdentityAction {
	for i := range b.Actions {
		if b.Actions[i].Action.Kind == kind {
			return &b.Actions[i]
		}
	}
	return nil
}

func TestReadReportsSkipsUnreadable(t *testing.T) {
	dir := t.TempDir()
	good := linkedReport()
	if err := WriteReport(dir, good); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"newer.json":    `{"identity":"newer","generated":"2026-11-21T00:00:00Z","futureField":1}`,
		"misnamed.json": `{"identity":"other","generated":"2026-11-21T00:00:00Z"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reports, bad, err := ReadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Identity != "a" || len(bad) != 2 {
		t.Fatalf("reports %+v, unreadable %+v", reports, bad)
	}
	if bad[0].Identity != "misnamed" || !strings.Contains(bad[1].String(), `newer.json: json: unknown field "futureField"`) {
		t.Fatalf("unreadable: %+v", bad)
	}
	if _, err := LoadReports(dir); err == nil {
		t.Fatal("LoadReports accepted an unreadable report")
	}
	if reports, bad, err := ReadReports(filepath.Join(dir, "absent")); err != nil || reports != nil || bad != nil {
		t.Fatalf("absent dir: %v %v %v", reports, bad, err)
	}
}
