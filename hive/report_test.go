package hive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/grokify/oscompat/testutil"
)

func validRequest() DecisionRequest {
	return DecisionRequest{ID: "q", Title: "Q?", Situation: "s", Approval: "a", Recommendation: "x",
		Options: []Option{{Name: "x", Description: "d", CostAndRisk: "c"}, {Name: "y", Description: "d", CostAndRisk: "c"}}}
}

func validReport() Report {
	return Report{Role: "alpha", Generated: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Done:            []Item{{Description: "did", Refs: []string{"RMI-A-001"}, Repositories: []string{"github.com/example/a"}}},
		Blocked:         []Blocker{{Item: Item{Description: "stuck"}, WaitingOn: []string{Operator}}},
		Next:            []Item{{Description: "will"}},
		DecisionsNeeded: []DecisionRequest{validRequest()}}
}

func TestReportValidate(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*Report)
		wantErr string
	}{
		{"valid", func(*Report) {}, ""},
		{"minimal", func(r *Report) { *r = Report{Role: "alpha", Generated: r.Generated} }, ""},
		{"bad role", func(r *Report) { r.Role = "Alpha" }, "role"},
		{"no time", func(r *Report) { r.Generated = time.Time{} }, "generated"},
		{"empty done", func(r *Report) { r.Done[0].Description = " " }, "done: description"},
		{"bad next ref", func(r *Report) { r.Next[0].Refs = []string{"X"} }, "next: ref"},
		{"bad repository", func(r *Report) { r.Done[0].Repositories = []string{"a/b"} }, "repository"},
		{"bad waiting on", func(r *Report) { r.Blocked[0].WaitingOn = []string{""} }, "waitingOn"},
		{"duplicate request", func(r *Report) { r.DecisionsNeeded = append(r.DecisionsNeeded, validRequest()) }, "twice"},
		{"bad request", func(r *Report) { r.DecisionsNeeded[0].Title = "" }, "title is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validReport()
			tt.edit(&r)
			checkErr(t, r.Validate(), tt.wantErr)
		})
	}
}

func TestDecisionRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*DecisionRequest)
		wantErr string
	}{
		{"valid", func(*DecisionRequest) {}, ""},
		{"bad id", func(d *DecisionRequest) { d.ID = "Q" }, "id"},
		{"no situation", func(d *DecisionRequest) { d.Situation = "" }, "situation"},
		{"no approval", func(d *DecisionRequest) { d.Approval = "" }, "approval"},
		{"one option", func(d *DecisionRequest) { d.Options = d.Options[:1] }, "two options"},
		{"incomplete option", func(d *DecisionRequest) { d.Options[1].CostAndRisk = "" }, "cost and risk"},
		{"repeated option", func(d *DecisionRequest) { d.Options[1].Name = "x" }, "twice"},
		{"unknown recommendation", func(d *DecisionRequest) { d.Recommendation = "z" }, "recommendation"},
		{"bad ref", func(d *DecisionRequest) { d.Refs = []string{"X-1"} }, "ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validRequest()
			d.Options = append([]Option(nil), d.Options...)
			tt.edit(&d)
			checkErr(t, d.Validate(), tt.wantErr)
		})
	}
}

func TestValidateReports(t *testing.T) {
	roles := []Role{validRole(), {Name: "gone", Status: RoleRetired}}
	rep := validReport()
	checkErr(t, ValidateReports([]Report{rep}, roles), "")
	checkErr(t, ValidateReports([]Report{rep, rep}, roles), "more than one")
	unknown := rep
	unknown.Role = "beta"
	checkErr(t, ValidateReports([]Report{unknown}, roles), "does not exist")
	retired := Report{Role: "gone", Generated: rep.Generated}
	checkErr(t, ValidateReports([]Report{retired}, roles), "retired")
	waits := Report{Role: "alpha", Generated: rep.Generated, Blocked: []Blocker{{Item: Item{Description: "x"}, WaitingOn: []string{"nobody"}}}}
	checkErr(t, ValidateReports([]Report{waits}, roles), "not a role")
	invalid := Report{Role: "alpha"}
	checkErr(t, ValidateReports([]Report{invalid}, roles), "generated")
}

func TestOperatorIsReserved(t *testing.T) {
	r := validRole()
	r.Name = Operator
	checkErr(t, r.Validate(), "reserved")
}

func TestWriteLoadReports(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	got, err := LoadReports(dir)
	if err != nil || got != nil {
		t.Fatalf("missing directory: %v %v", got, err)
	}
	rep := validReport()
	if err := WriteReport(dir, rep); err != nil {
		t.Fatal(err)
	}
	rep.Summary = "replaced"
	if err := WriteReport(dir, rep); err != nil {
		t.Fatal(err)
	}
	got, err = LoadReports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Report{rep}) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, rep)
	}
	if err := WriteReport(dir, Report{Role: "alpha"}); err == nil {
		t.Fatal("want error writing an invalid report")
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.json"), []byte(`{"role":"alpha","generated":"2026-01-02T03:04:05Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReports(dir); err == nil {
		t.Fatal("want a file name error")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteReport(filepath.Join(file, "sub"), validReport()); err == nil {
		t.Fatal("want error writing under a file")
	}
}

func TestReportsDir(t *testing.T) {
	t.Setenv(ReportsEnv, "/some/dir")
	if dir, err := ReportsDir(); err != nil || dir != "/some/dir" {
		t.Fatalf("ReportsDir = %q, %v", dir, err)
	}
	t.Setenv(ReportsEnv, "")
	data := t.TempDir()
	testutil.SetTempHomeDir(t)
	t.Setenv("XDG_DATA_HOME", data) // Linux and macOS, ahead of the home directory
	t.Setenv("LOCALAPPDATA", data)  // Windows
	dir, err := ReportsDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(data, AppName, "reports") {
		t.Fatalf("ReportsDir = %q", dir)
	}
}

func TestAge(t *testing.T) {
	r := validReport()
	if got := r.Age(r.Generated.Add(time.Hour)); got != time.Hour {
		t.Fatalf("Age = %v", got)
	}
}

func TestBriefing(t *testing.T) {
	h, err := Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	reports, err := LoadReports(filepath.Join("testdata", "reports"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReports(reports, h.Roles); err != nil {
		t.Fatal(err)
	}
	b := NewBriefing(h.Roles, h.Decisions, reports)
	if len(b.Decisions) != 1 || b.Decisions[0].Role != "billing" || b.Decisions[0].Request.ID != "coupon-stacking" {
		t.Fatalf("decisions: %+v", b.Decisions)
	}
	if len(b.Blocked) != 2 || b.Blocked[0].Blocker.WaitingOn[0] != Operator || b.Blocked[1].Role != "billing" {
		t.Fatalf("blocked: %+v", b.Blocked)
	}
	if len(b.Reports) != 2 || b.Reports[0].Role != "billing" {
		t.Fatalf("reports: %+v", b.Reports)
	}
	if b.Missing == nil || len(b.Missing) != 0 {
		t.Fatalf("missing: %v", b.Missing)
	}
	if b.Decisions[0].Key != "billing/coupon-stacking" {
		t.Fatalf("key: %q", b.Decisions[0].Key)
	}
	if len(b.Actions) != 3 || b.Actions[0].Action.Kind != ActionPush || b.Actions[1].Action.Kind != ActionTag ||
		b.Actions[2].Action.Kind != ActionConfigure {
		t.Fatalf("actions: %+v", b.Actions)
	}
	if len(b.InProgress) != 1 || !b.InProgress[0].Shared || len(b.InProgress[0].Work) != 2 {
		t.Fatalf("in progress: %+v", b.InProgress)
	}
	if len(b.Notices) != 1 || b.Notices[0].Role != "billing" {
		t.Fatalf("notices: %+v", b.Notices)
	}
	b = NewBriefing(h.Roles, h.Decisions, reports[:1])
	if !reflect.DeepEqual(b.Missing, []string{"website"}) {
		t.Fatalf("missing: %v", b.Missing)
	}
	two := []Report{
		{Role: "b", DecisionsNeeded: []DecisionRequest{{ID: "z"}, {ID: "a"}}},
		{Role: "a", DecisionsNeeded: []DecisionRequest{{ID: "m"}}},
	}
	b = NewBriefing([]Role{{Name: "a", Status: RoleActive}, {Name: "b", Status: RoleActive}}, nil, two)
	if b.Decisions[0].Role != "a" || b.Decisions[1].Request.ID != "a" {
		t.Fatalf("decision order: %+v", b.Decisions)
	}
}

func TestBriefingAnsweredIdleIgnored(t *testing.T) {
	roles := []Role{
		{Name: "a", Status: RoleActive},
		{Name: "old", Status: RoleRetired},
	}
	reports := []Report{
		{Role: "a", Idle: true, DecisionsNeeded: []DecisionRequest{{ID: "done-already"}}},
		{Role: "old"},
		{Role: "ghost"},
	}
	b := NewBriefing(roles, []Decision{{ID: "done-already"}}, reports)
	if len(b.Decisions) != 0 || len(b.Answered) != 1 || b.Answered[0].Key != "a/done-already" {
		t.Fatalf("answered: %+v / %+v", b.Decisions, b.Answered)
	}
	if !reflect.DeepEqual(b.Idle, []string{"a"}) {
		t.Fatalf("idle: %v", b.Idle)
	}
	if !reflect.DeepEqual(b.Ignored, []string{"ghost", "old"}) || len(b.Reports) != 1 {
		t.Fatalf("ignored: %v, reports %d", b.Ignored, len(b.Reports))
	}
}

func TestReportNewFieldsInvalid(t *testing.T) {
	r := Report{
		Role:      "a",
		Generated: time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC),
		OperatorActions: []Action{
			{Kind: "launch", Description: ""},
			{Kind: ActionPush, Description: "x", Repository: "web", Commits: []string{"XYZ"}, PullRequests: []string{"pr 9"}},
		},
		InProgress: []Work{{Description: "x", Repository: "web"}},
		Notices:    []Notice{{Title: "x"}},
		Done:       []Item{{Description: "x", State: "shipped", Commits: []string{"nothex"}, PullRequests: []string{"9"}}},
		Blocked:    []Blocker{{Item: Item{Description: "x"}, WaitingOn: []string{"b"}, Decision: "missing"}},
	}
	err := r.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, want := range []string{
		`kind "launch"`, "operatorActions: description is required", `repository "web"`,
		`commit "XYZ"`, `pull request "pr 9"`, `inProgress: repository "web"`,
		"notices: title and description", `state "shipped"`, `commit "nothex"`,
		`pull request "9"`, `decision "missing"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestDecide(t *testing.T) {
	reports, err := LoadReports(filepath.Join("testdata", "reports"))
	if err != nil {
		t.Fatal(err)
	}
	req := reports[0].DecisionsNeeded[0]
	if _, err := req.Decide("billing", "free-for-all", ""); err == nil || !strings.Contains(err.Error(), "no-stacking, stack-two") {
		t.Fatalf("unknown option: %v", err)
	}
	d, err := req.Decide("billing", "stack-two", "Partner traffic matters more this quarter.")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "coupon-stacking" || d.Status != DecisionAccepted || d.Option != "stack-two" || !reflect.DeepEqual(d.Roles, []string{"billing"}) {
		t.Fatalf("decision: %+v", d)
	}
	if !strings.Contains(d.Decision, "Allow at most two coupons.") || !strings.Contains(d.Decision, "recommended no-stacking") {
		t.Fatalf("decision text: %q", d.Decision)
	}
	if d.Reasons[0] != "Partner traffic matters more this quarter." || len(d.Alternatives) != 1 {
		t.Fatalf("reasons/alternatives: %+v", d)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "decisions"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := WriteDecision(root, d); err != nil {
		t.Fatal(err)
	}
	if err := WriteDecision(root, d); err == nil || !strings.Contains(err.Error(), "supersede") {
		t.Fatalf("second write: %v", err)
	}
	got, err := LoadDecisions(root)
	if err != nil || len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("load: %v %+v", err, got)
	}
	d.Reasons = nil
	if err := WriteDecision(root, d); err == nil {
		t.Fatal("invalid decision written")
	}
}
