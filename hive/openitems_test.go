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
		{"decided without decision", func(o *OpenItems) { o.Items[1].Status = ItemDecided }, "names its decision"},
		{"decision while open", func(o *OpenItems) { o.Items[0].Decision = "x" }, "only a decided item"},
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
