package hive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func exampleOpen(t *testing.T) *Hive {
	t.Helper()
	h, err := Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Open.Items) != 3 {
		t.Fatalf("open items: %d", len(h.Open.Items))
	}
	return h
}

func TestValidateOpenItems(t *testing.T) {
	h := exampleOpen(t)
	if err := ValidateOpenItems(h.Open, h.Identities, h.Decisions); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*OpenItems)
		want   string
	}{
		{"updated", func(o *OpenItems) { o.Updated = "yesterday" }, "YYYY-MM-DD"},
		{"key", func(o *OpenItems) { o.Items[1].Key = "website/other" }, "key must be"},
		{"twice", func(o *OpenItems) { o.Items[2] = o.Items[1] }, "listed twice"},
		{"identity", func(o *OpenItems) { o.Items[1].Identity, o.Items[1].Key = "nobody", "nobody/checkout-copy" }, `work identity "nobody"`},
		{"request id", func(o *OpenItems) { o.Items[1].Request.ID, o.Items[1].Key = "Bad", "website/Bad" }, `request id "Bad"`},
		{"title", func(o *OpenItems) { o.Items[1].Request.Title = " " }, "title is required"},
		{"status", func(o *OpenItems) { o.Items[1].Status = "done" }, `status "done"`},
		{"source kind", func(o *OpenItems) { o.Items[0].Source.Kind = "chat" }, `source kind "chat"`},
		{"document source", func(o *OpenItems) { o.Items[1].Source.Path = "" }, "repository and path"},
		{"open incomplete", func(o *OpenItems) { o.Items[1].Status = ItemOpen }, "option"},
		{"decided without decision", func(o *OpenItems) { o.Items[1].Status = ItemDecided }, "has its answer or names its decision"},
		{"decision while open", func(o *OpenItems) { o.Items[0].Decision = "x" }, "only an answered or decided item"},
		{"unknown decision", func(o *OpenItems) { o.Items[1].Status, o.Items[1].Decision = ItemDecided, "nope" }, `decision "nope"`},
		{"deferred without note", func(o *OpenItems) { o.Items[2].Note = "" }, "says why"},
		{"related", func(o *OpenItems) { o.Items[0].Related = []string{"x/y"} }, `related "x/y"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := exampleOpen(t)
			tc.mutate(&h.Open)
			err := h.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSyncOpenItems(t *testing.T) {
	h := exampleOpen(t)
	billing := Report{Identity: "billing", DecisionsNeeded: []DecisionRequest{h.Open.Items[0].Request}}
	if res := h.Open.Sync([]Report{billing}, nil); res.Changed() {
		t.Fatalf("unchanged report: %+v", res)
	}
	changed := billing
	changed.DecisionsNeeded = []DecisionRequest{h.Open.Items[0].Request}
	changed.DecisionsNeeded[0].Title = "Allow coupons to combine?"
	if res := h.Open.Sync([]Report{changed}, nil); !reflect.DeepEqual(res.Updated, []string{"billing/coupon-stacking"}) {
		t.Fatalf("refresh: %+v", res)
	}

	// A deferred item its work identity now asks is opened, and a new
	// request is added.
	framed := h.Open.Items[0].Request
	framed.ID = "cookie-banner"
	extra := framed
	extra.ID = "hero-image"
	res := h.Open.Sync([]Report{{Identity: "website", DecisionsNeeded: []DecisionRequest{framed, extra}}}, nil)
	if !reflect.DeepEqual(res.Opened, []string{"website/cookie-banner"}) || !reflect.DeepEqual(res.Added, []string{"website/hero-image"}) {
		t.Fatalf("opened and added: %+v", res)
	}
	if it, _ := h.Open.Item("website/cookie-banner"); it.Note != "" || it.Source.Kind != SourceADR {
		t.Fatalf("opened item: %+v", it)
	}

	// Answered by a decision binding billing; the deferred website item is
	// left alone while website has no readable report.
	h = exampleOpen(t)
	decided := []Decision{{ID: "coupon-stacking", Identities: []string{"billing"}}}
	res = h.Open.Sync(nil, decided)
	if !reflect.DeepEqual(res.Decided, []string{"billing/coupon-stacking"}) || len(res.Withdrawn) != 0 {
		t.Fatalf("decided: %+v", res)
	}
	if it, _ := h.Open.Item("billing/coupon-stacking"); it.Decision != "coupon-stacking" {
		t.Fatalf("decision: %+v", it)
	}
	// A decided item stays decided while the report still carries it.
	if res := h.Open.Sync([]Report{billing}, decided); res.Changed() {
		t.Fatalf("decided item reopened: %+v", res)
	}
	// A decision binding another identity does not answer it.
	h = exampleOpen(t)
	if res := h.Open.Sync(nil, []Decision{{ID: "coupon-stacking", Identities: []string{"website"}}}); res.Changed() {
		t.Fatalf("other identity's decision: %+v", res)
	}
	// Only report questions are withdrawn; a design record's stays.
	h.Open.Items[1].Status = ItemOpen
	res = h.Open.Sync([]Report{{Identity: "billing"}, {Identity: "website"}}, nil)
	if !reflect.DeepEqual(res.Withdrawn, []string{"billing/coupon-stacking"}) {
		t.Fatalf("withdrawn: %+v", res)
	}
	if it, _ := h.Open.Item("billing/coupon-stacking"); it.Note != WithdrawnNote {
		t.Fatalf("note: %q", it.Note)
	}
	if h.Open.MarkDecided("billing/coupon-stacking", "x") || h.Open.MarkDecided("nobody/x", "x") {
		t.Fatal("marked a closed or missing item")
	}
	if !h.Open.MarkDecided("website/checkout-copy", "checkout-copy") {
		t.Fatal("did not mark an open item")
	}
}

func TestLoadWriteOpenItems(t *testing.T) {
	dir := t.TempDir()
	o, err := LoadOpenItems(dir)
	if err != nil || o.Items != nil {
		t.Fatalf("missing file: %+v %v", o, err)
	}
	o = OpenItems{Items: []OpenItem{{Key: "b/x"}, {Key: "a/y"}}}
	if err := WriteOpenItems(dir, o); err != nil {
		t.Fatal(err)
	}
	back, err := LoadOpenItems(dir)
	if err != nil || back.Items[0].Key != "a/y" {
		t.Fatalf("sorted: %+v %v", back, err)
	}
	if err := os.WriteFile(filepath.Join(dir, OpenItemsFile), []byte(`{"items":[],"extra":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOpenItems(dir); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load accepted a hive without identities")
	}
}

func TestRecordAnswer(t *testing.T) {
	h := exampleOpen(t)
	a := Answer{Option: "stack-two", Reason: "r", Answered: "2026-11-22"}
	req := h.Open.Items[0].Request
	it, err := h.Open.Record("billing", req, a, "")
	if err != nil || it.Status != ItemDecided || it.Answer.Option != "stack-two" {
		t.Fatalf("report question: %+v %v", it, err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if opt, ok := h.Answered("billing", "coupon-stacking"); !ok || opt != "stack-two" {
		t.Fatalf("Answered: %q %v", opt, ok)
	}
	if _, ok := h.Answered("website", "coupon-stacking"); ok {
		t.Fatal("answered for another identity")
	}
	if _, err := h.Open.Record("billing", req, a, ""); err == nil {
		t.Fatal("answered twice")
	}

	// A new request is added; a document's question is answered, not decided.
	extra := req
	extra.ID = "refund-window"
	if it, err := h.Open.Record("billing", extra, a, ""); err != nil || it.Key != "billing/refund-window" {
		t.Fatalf("new request: %+v %v", it, err)
	}
	framed := req
	framed.ID = "checkout-copy"
	it, err = h.Open.Record("website", framed, a, "")
	if err != nil || it.Status != ItemAnswered || it.Source.Kind != SourceADR {
		t.Fatalf("design question: %+v %v", it, err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	h.Open.Items[2].Status, h.Open.Items[2].Note = ItemWithdrawn, "n"
	if _, err := h.Open.Record("website", DecisionRequest{ID: "cookie-banner"}, a, ""); err == nil {
		t.Fatal("answered a withdrawn item")
	}

	// A decision in the log answers too, binding its identities.
	h.Decisions = append(h.Decisions, Decision{ID: "hero", Option: "big", Identities: []string{"website"}})
	if opt, ok := h.Answered("website", "hero"); !ok || opt != "big" {
		t.Fatalf("decision answer: %q %v", opt, ok)
	}
}

func TestAnswerValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*OpenItem)
		want   string
	}{
		{"answered without answer", func(it *OpenItem) { it.Status = ItemAnswered; it.Answer = nil }, "has its answer"},
		{"answered report question", func(it *OpenItem) { it.Status = ItemAnswered }, "decided when answered"},
		{"bad date", func(it *OpenItem) { it.Answer.Answered = "today" }, `answered "today"`},
		{"unknown option", func(it *OpenItem) { it.Answer.Option = "never" }, `answer "never"`},
		{"answer while open", func(it *OpenItem) { it.Status = ItemOpen }, "only an answered or decided item"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := exampleOpen(t)
			it := &h.Open.Items[0]
			it.Status, it.Answer = ItemDecided, &Answer{Option: "stack-two", Answered: "2026-11-22"}
			tc.mutate(it)
			if err := h.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestHiveBriefingAnswers(t *testing.T) {
	h := exampleOpen(t)
	reports, err := LoadReports("testdata/reports")
	if err != nil {
		t.Fatal(err)
	}
	if b := NewHiveBriefing(h, reports); len(b.Decisions) != 1 || len(b.Answered) != 0 {
		t.Fatalf("before: %d open, %d answered", len(b.Decisions), len(b.Answered))
	}
	if _, err := h.Open.Record("billing", h.Open.Items[0].Request, Answer{Option: "no-stacking", Answered: "2026-11-22"}, ""); err != nil {
		t.Fatal(err)
	}
	b := NewHiveBriefing(h, reports)
	if len(b.Decisions) != 0 || len(b.Answered) != 1 || b.Answered[0].Option != "no-stacking" {
		t.Fatalf("after: %+v", b.Answered)
	}
	c, err := NewContext(h, reports, "billing")
	if err != nil || c.Answers["coupon-stacking"].Option != "no-stacking" || len(c.Answered) != 1 {
		t.Fatalf("context: %+v %v", c.Answers, err)
	}

	// The website report drops nothing, so its answered design question
	// stays answered until the request leaves the report.
	it, err := h.Open.Record("website", DecisionRequest{ID: "checkout-copy", Title: "t"}, Answer{Option: "web", Answered: "2026-11-22"}, "")
	if err != nil || it.Status != ItemAnswered {
		t.Fatal(err)
	}
	withRequest := Report{Identity: "website", DecisionsNeeded: []DecisionRequest{{ID: "checkout-copy"}}}
	if res := h.Open.Sync([]Report{withRequest}, nil); res.Changed() {
		t.Fatalf("still asked: %+v", res)
	}
	if res := h.Open.Sync([]Report{{Identity: "website"}}, nil); !reflect.DeepEqual(res.Decided, []string{"website/checkout-copy"}) {
		t.Fatalf("dropped: %+v", res)
	}
}
