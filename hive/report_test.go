package hive

import (
	"os"
	"path/filepath"
	"reflect"
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
		Blocked:         []Blocker{{Item: Item{Description: "stuck"}, WaitingOn: Operator}},
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
		{"bad waiting on", func(r *Report) { r.Blocked[0].WaitingOn = "" }, "waitingOn"},
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
	waits := Report{Role: "alpha", Generated: rep.Generated, Blocked: []Blocker{{Item: Item{Description: "x"}, WaitingOn: "nobody"}}}
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
	b := NewBriefing(h.Roles, reports)
	if len(b.Decisions) != 1 || b.Decisions[0].Role != "billing" || b.Decisions[0].Request.ID != "coupon-stacking" {
		t.Fatalf("decisions: %+v", b.Decisions)
	}
	if len(b.Blocked) != 2 || b.Blocked[0].Blocker.WaitingOn != Operator || b.Blocked[1].Role != "billing" {
		t.Fatalf("blocked: %+v", b.Blocked)
	}
	if len(b.Reports) != 2 || b.Reports[0].Role != "billing" {
		t.Fatalf("reports: %+v", b.Reports)
	}
	if b.Missing == nil || len(b.Missing) != 0 {
		t.Fatalf("missing: %v", b.Missing)
	}
	b = NewBriefing(h.Roles, reports[:1])
	if !reflect.DeepEqual(b.Missing, []string{"website"}) {
		t.Fatalf("missing: %v", b.Missing)
	}
	two := []Report{
		{Role: "b", DecisionsNeeded: []DecisionRequest{{ID: "z"}, {ID: "a"}}},
		{Role: "a", DecisionsNeeded: []DecisionRequest{{ID: "m"}}},
	}
	b = NewBriefing(nil, two)
	if b.Decisions[0].Role != "a" || b.Decisions[1].Request.ID != "a" {
		t.Fatalf("decision order: %+v", b.Decisions)
	}
}
