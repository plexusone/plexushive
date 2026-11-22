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
	// ItemAnswered is a question from a document that the operator has
	// answered; it waits for its work identity to write the answer into
	// the document, and is decided once the work identity drops the
	// request from its report.
	ItemAnswered ItemStatus = "answered"
	// ItemDeferred waits on something its note names.
	ItemDeferred ItemStatus = "deferred"
	// ItemDecided is answered: by the operator's Answer, by the decision its
	// Decision names, or both.
	ItemDecided ItemStatus = "decided"
	// ItemWithdrawn was dropped by the work identity that raised it; its
	// note says why.
	ItemWithdrawn ItemStatus = "withdrawn"
)

// ItemStatuses lists the statuses in the order they are shown.
var ItemStatuses = []ItemStatus{ItemOpen, ItemAnswered, ItemDraft, ItemDeferred, ItemDecided, ItemWithdrawn}

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

// Answer is the operator's answer to an open item. Most answers are point
// decisions that unblock work: the answer reaches the work identity, and
// its lasting record is the result, such as a commit, a changelog entry or
// an ADR. Only working rules for the agents also become decisions in the
// log.
type Answer struct {
	// Option is the chosen option's name.
	Option string `json:"option"`
	// Reason is why, when the operator gave one.
	Reason string `json:"reason,omitempty"`
	// Answered is the date, as YYYY-MM-DD.
	Answered string `json:"answered" jsonschema:"pattern=^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
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
	Status   ItemStatus      `json:"status" jsonschema:"enum=draft,enum=open,enum=answered,enum=deferred,enum=decided,enum=withdrawn"`
	Request  DecisionRequest `json:"request"`
	// Answer is the operator's answer to an answered or decided item.
	Answer *Answer `json:"answer,omitempty"`
	// Decision is the ID of the decision in the log that records the
	// answer, when it was recorded as one.
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
		out = append(out, fmt.Sprintf("status %q must be draft, open, answered, deferred, decided or withdrawn", it.Status))
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
	out = append(out, it.answerProblems(decided)...)
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

func (it OpenItem) answerProblems(decided map[string]bool) []string {
	var out []string
	answered := it.Status == ItemAnswered || it.Status == ItemDecided
	switch {
	case it.Status == ItemDecided && it.Answer == nil && it.Decision == "":
		out = append(out, "a decided item has its answer or names its decision")
	case it.Status == ItemAnswered && it.Answer == nil:
		out = append(out, "an answered item has its answer")
	case !answered && (it.Answer != nil || it.Decision != ""):
		out = append(out, "only an answered or decided item has an answer or a decision")
	}
	if it.Status == ItemAnswered && it.Source.Kind == SourceReport {
		out = append(out, "a question from a report is decided when answered, not answered")
	}
	if it.Decision != "" && !decided[it.Decision] {
		out = append(out, fmt.Sprintf("decision %q does not exist", it.Decision))
	}
	if a := it.Answer; a != nil {
		if _, err := time.Parse(time.DateOnly, a.Answered); err != nil {
			out = append(out, fmt.Sprintf("answered %q must be YYYY-MM-DD", a.Answered))
		}
		if len(it.Request.Options) > 0 && !slices.ContainsFunc(it.Request.Options, func(o Option) bool { return o.Name == a.Option }) {
			out = append(out, fmt.Sprintf("answer %q is not one of the request's options", a.Option))
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
//     identity becomes decided, and so does an answered item once its work
//     identity drops the request, having written the answer into the
//     document
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
			if it.Status == ItemDecided || it.Status == ItemAnswered {
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
		if it.Status == ItemAnswered {
			if reported[it.Identity] && !asked[it.Key] {
				it.Status = ItemDecided
				res.Decided = append(res.Decided, it.Key)
			}
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

// Record records the operator's answer to the item with key and returns
// the item. decision, when not empty, is the ID of the decision that also
// records it in the log. A question from a report is then decided; one
// from a document is answered until its work identity has written the
// answer into it. An item not listed yet is added from req, which must be
// its decision request.
func (o *OpenItems) Record(identity string, req DecisionRequest, a Answer, decision string) (*OpenItem, error) {
	key := identity + "/" + req.ID
	it, ok := o.Item(key)
	if !ok {
		o.Items = append(o.Items, OpenItem{Key: key, Identity: identity, Source: ItemSource{Kind: SourceReport}, Status: ItemOpen, Request: req})
		it = &o.Items[len(o.Items)-1]
	}
	switch it.Status {
	case ItemDecided, ItemAnswered:
		return nil, fmt.Errorf("hive: %s is already %s", key, it.Status)
	case ItemWithdrawn:
		return nil, fmt.Errorf("hive: %s was withdrawn by its work identity", key)
	}
	if len(req.Options) > 0 {
		it.Request = req
	}
	it.Status = ItemDecided
	if it.Source.Kind != SourceReport {
		it.Status = ItemAnswered
	}
	it.Answer, it.Decision, it.Note = &a, decision, ""
	return it, nil
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

// Answered returns the option chosen for the work identity's request id,
// from its open item's answer or else from a decision with the same ID that
// binds it. The option is empty when a decision answers the request without
// naming one.
func (h *Hive) Answered(identity, id string) (string, bool) {
	if it, ok := h.Open.Item(identity + "/" + id); ok && it.Answer != nil {
		return it.Answer.Option, true
	}
	for _, d := range h.Decisions {
		if d.ID == id && (len(d.Identities) == 0 || slices.Contains(d.Identities, identity)) {
			return d.Option, true
		}
	}
	return "", false
}
