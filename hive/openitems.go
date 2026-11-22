package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/grokify/oscompat/fs"
)

// OpenItemsFile is the file under the hive root that tracks open items.
const OpenItemsFile = "open-items.json"

// ItemStatus is where an open item is in its life.
type ItemStatus string

const (
	// ItemDraft is a question with no options yet, such as one copied from
	// a design record's open questions.
	ItemDraft ItemStatus = "draft"
	// ItemOpen has options and a recommendation and waits on the operator.
	ItemOpen ItemStatus = "open"
	// ItemDeferred waits on something its note names.
	ItemDeferred ItemStatus = "deferred"
	// ItemDecided is answered by the decision its Decision names.
	ItemDecided ItemStatus = "decided"
	// ItemWithdrawn was dropped by the work identity that raised it; its
	// note says why.
	ItemWithdrawn ItemStatus = "withdrawn"
)

// ItemStatuses lists the statuses in the order they are shown.
var ItemStatuses = []ItemStatus{ItemOpen, ItemDraft, ItemDeferred, ItemDecided, ItemWithdrawn}

// Closed reports whether the item needs nothing more from the operator.
func (s ItemStatus) Closed() bool {
	return s == ItemDecided || s == ItemWithdrawn
}

// SourceKind says where an open item was raised.
type SourceKind string

const (
	// SourceReport is a decision request in a work identity's report.
	SourceReport SourceKind = "report"
	// SourceADR is an architecture decision record's open question.
	SourceADR SourceKind = "adr"
	// SourceDocument is an open question in another document, such as a
	// specification.
	SourceDocument SourceKind = "document"
)

// ItemSource says where an open item was raised. A question from a
// document names the repository, the path in it and the document's own
// identifier, such as ADR-013.
type ItemSource struct {
	Kind       SourceKind `json:"kind" jsonschema:"enum=report,enum=adr,enum=document"`
	Repository string     `json:"repository,omitempty" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	Path       string     `json:"path,omitempty"`
	Document   string     `json:"document,omitempty"`
}

// OpenItem is one question waiting on the operator, tracked until it is
// decided or withdrawn. Request is in the decision request format; a draft
// has only its ID and title until its work identity writes the options in
// a report.
type OpenItem struct {
	// Key is <identity>/<request ID>, as in the briefing and decide.
	Key string `json:"key"`
	// Identity is the work identity that raised the question or is to
	// frame it.
	Identity string          `json:"identity" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	Source   ItemSource      `json:"source"`
	Status   ItemStatus      `json:"status" jsonschema:"enum=draft,enum=open,enum=deferred,enum=decided,enum=withdrawn"`
	Request  DecisionRequest `json:"request"`
	// Decision is the ID of the decision that answers a decided item.
	Decision string `json:"decision,omitempty"`
	// Related are the keys of items to decide together with this one.
	Related []string `json:"related,omitempty"`
	// Note is for the operator: why an item is deferred or withdrawn, or
	// what to weigh.
	Note string `json:"note,omitempty"`
}

// OpenItems is the hive's list of questions waiting on the operator, from
// work identities' reports and from design records, kept until each is
// closed.
type OpenItems struct {
	// Updated is the date of the last change, as YYYY-MM-DD.
	Updated string     `json:"updated,omitempty" jsonschema:"pattern=^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
	Items   []OpenItem `json:"items"`
}

// Item returns the item with key, or false.
func (o *OpenItems) Item(key string) (*OpenItem, bool) {
	for i := range o.Items {
		if o.Items[i].Key == key {
			return &o.Items[i], true
		}
	}
	return nil, false
}

// LoadOpenItems reads open-items.json under root. A hive without the file
// has no open items.
func LoadOpenItems(root string) (OpenItems, error) {
	path := filepath.Join(root, OpenItemsFile)
	data, err := os.ReadFile(path) //nolint:gosec // the path is the hive's own file
	if errors.Is(err, os.ErrNotExist) {
		return OpenItems{}, nil
	}
	if err != nil {
		return OpenItems{}, fmt.Errorf("hive: %w", err)
	}
	var o OpenItems
	if err := decodeStrict(data, &o); err != nil {
		return OpenItems{}, fmt.Errorf("hive: %s: %w", path, err)
	}
	return o, nil
}

// WriteOpenItems writes o to open-items.json under root, sorted by key,
// replacing the file atomically.
func WriteOpenItems(root string, o OpenItems) error {
	sort.SliceStable(o.Items, func(i, j int) bool { return o.Items[i].Key < o.Items[j].Key })
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	if err := fs.WriteFileAtomic(filepath.Join(root, OpenItemsFile), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	return nil
}

// ValidateOpenItems checks the items against the work identities and the
// decision log: keys are <identity>/<id> and unique, identities exist, an
// open item is a complete decision request, a decided item names an
// existing decision, deferred and withdrawn items say why, a document
// source says where, and related keys are listed.
func ValidateOpenItems(o OpenItems, identities []WorkIdentity, decisions []Decision) error {
	if o.Updated != "" {
		if _, err := time.Parse(time.DateOnly, o.Updated); err != nil {
			return fmt.Errorf("open items: updated %q must be YYYY-MM-DD", o.Updated)
		}
	}
	names := map[string]bool{}
	for _, w := range identities {
		names[w.Name] = true
	}
	decided := map[string]bool{}
	for _, d := range decisions {
		decided[d.ID] = true
	}
	keys := map[string]int{}
	for _, it := range o.Items {
		keys[it.Key]++
	}
	var errs []string
	for _, it := range o.Items {
		for _, p := range it.problems(names, decided, keys) {
			errs = append(errs, fmt.Sprintf("open item %s: %s", it.Key, p))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("open items: %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func (it OpenItem) problems(identities, decided map[string]bool, keys map[string]int) []string {
	var out []string
	if keys[it.Key] > 1 {
		out = append(out, "listed twice")
	}
	if it.Key != it.Identity+"/"+it.Request.ID {
		out = append(out, "key must be <identity>/<request id>")
	}
	if !identities[it.Identity] {
		out = append(out, fmt.Sprintf("work identity %q does not exist", it.Identity))
	}
	if !namePattern.MatchString(it.Request.ID) {
		out = append(out, fmt.Sprintf("request id %q must match %s", it.Request.ID, namePattern))
	}
	if strings.TrimSpace(it.Request.Title) == "" {
		out = append(out, "request title is required")
	}
	if !slices.Contains(ItemStatuses, it.Status) {
		out = append(out, fmt.Sprintf("status %q must be draft, open, deferred, decided or withdrawn", it.Status))
	}
	switch it.Source.Kind {
	case SourceReport:
	case SourceADR, SourceDocument:
		if !repositoryPattern.MatchString(it.Source.Repository) || it.Source.Path == "" {
			out = append(out, fmt.Sprintf("a %s source needs its repository and path", it.Source.Kind))
		}
	default:
		out = append(out, fmt.Sprintf("source kind %q must be report, adr or document", it.Source.Kind))
	}
	if it.Status == ItemOpen {
		if err := it.Request.Validate(); err != nil {
			out = append(out, err.Error())
		}
	}
	switch {
	case it.Status == ItemDecided && it.Decision == "":
		out = append(out, "a decided item names its decision")
	case it.Status != ItemDecided && it.Decision != "":
		out = append(out, "only a decided item names a decision")
	case it.Decision != "" && !decided[it.Decision]:
		out = append(out, fmt.Sprintf("decision %q does not exist", it.Decision))
	}
	if (it.Status == ItemDeferred || it.Status == ItemWithdrawn) && strings.TrimSpace(it.Note) == "" {
		out = append(out, fmt.Sprintf("a %s item says why in its note", it.Status))
	}
	for _, r := range it.Related {
		if keys[r] == 0 {
			out = append(out, fmt.Sprintf("related %q is not listed", r))
		}
	}
	return out
}

// SyncResult lists the keys Sync changed, by what happened to them.
type SyncResult struct {
	Added     []string `json:"added"`
	Opened    []string `json:"opened"`
	Updated   []string `json:"updated"`
	Decided   []string `json:"decided"`
	Withdrawn []string `json:"withdrawn"`
}

// Changed reports whether Sync changed anything.
func (r SyncResult) Changed() bool {
	return len(r.Added)+len(r.Opened)+len(r.Updated)+len(r.Decided)+len(r.Withdrawn) > 0
}

// WithdrawnNote is the note Sync puts on a request its work identity
// dropped without a decision.
const WithdrawnNote = "Dropped from the report without a recorded decision."

// Sync brings the items up to date with the reports and the decision log:
//
//   - a decision request not yet listed is added as open
//   - a listed request is refreshed from the report, and a draft, deferred
//     or withdrawn item whose work identity now asks it is opened; this is
//     how a design record's question gets its options
//   - an item answered by a decision with the same ID that binds its work
//     identity becomes decided
//   - an open item raised in a report that its work identity has dropped,
//     with no decision, becomes withdrawn
//
// Only the reports given are consulted: a work identity with no readable
// report has nothing withdrawn.
func (o *OpenItems) Sync(reports []Report, decisions []Decision) SyncResult {
	var res SyncResult
	asked := map[string]bool{}
	reported := map[string]bool{}
	for _, r := range reports {
		reported[r.Identity] = true
		for _, req := range r.DecisionsNeeded {
			key := r.Identity + "/" + req.ID
			asked[key] = true
			it, ok := o.Item(key)
			if !ok {
				o.Items = append(o.Items, OpenItem{Key: key, Identity: r.Identity, Source: ItemSource{Kind: SourceReport}, Status: ItemOpen, Request: req})
				res.Added = append(res.Added, key)
				continue
			}
			if it.Status == ItemDecided {
				continue
			}
			if it.Status != ItemOpen {
				it.Status = ItemOpen
				it.Note = ""
				it.Request = req
				res.Opened = append(res.Opened, key)
				continue
			}
			if !requestsEqual(it.Request, req) {
				it.Request = req
				res.Updated = append(res.Updated, key)
			}
		}
	}
	for i := range o.Items {
		it := &o.Items[i]
		if it.Status.Closed() {
			continue
		}
		if d, ok := answer(decisions, it); ok {
			it.Status, it.Decision = ItemDecided, d
			res.Decided = append(res.Decided, it.Key)
			continue
		}
		if it.Status == ItemOpen && it.Source.Kind == SourceReport && reported[it.Identity] && !asked[it.Key] {
			it.Status, it.Note = ItemWithdrawn, WithdrawnNote
			res.Withdrawn = append(res.Withdrawn, it.Key)
		}
	}
	return res
}

// answer returns the decision that answers it: same ID as the request,
// binding its work identity or every identity.
func answer(decisions []Decision, it *OpenItem) (string, bool) {
	for _, d := range decisions {
		if d.ID == it.Request.ID && (len(d.Identities) == 0 || slices.Contains(d.Identities, it.Identity)) {
			return d.ID, true
		}
	}
	return "", false
}

func requestsEqual(a, b DecisionRequest) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// MarkDecided records that decision answers the item with key, if it is
// listed and not already closed, and reports whether it changed anything.
func (o *OpenItems) MarkDecided(key, decision string) bool {
	it, ok := o.Item(key)
	if !ok || it.Status.Closed() {
		return false
	}
	it.Status, it.Decision, it.Note = ItemDecided, decision, ""
	return true
}
