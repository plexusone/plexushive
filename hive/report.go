package hive

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Operator is the name a blocker waits on when it waits on the human
// operator rather than on another work identity.
const Operator = "operator"

// Report is one work identity's status at a point in time: what it finished,
// what is blocked, what it does next and what it needs the operator to decide.
// A work identity keeps one current report, replaced each time it reports;
// history, when wanted, is the caller's concern.
type Report struct {
	// Identity is the reporting work identity's name.
	Identity string `json:"identity" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Generated is when the report was written. Readers use it to show how
	// fresh the report is.
	Generated time.Time `json:"generated"`
	// Session says how to reach the session working under the work identity.
	Session *Session `json:"session,omitempty"`
	// Summary is one or two sentences for a briefing.
	Summary string `json:"summary,omitempty"`
	// Idle says the work identity has nothing to do until it is given work. An
	// idle work identity's report does not go stale.
	Idle bool `json:"idle,omitempty"`
	// OperatorActions are steps only the operator takes, such as pushes,
	// tags and deploys, for work that is ready.
	OperatorActions []Action `json:"operatorActions,omitempty"`
	// InProgress is what the work identity is doing now, and in which
	// repository and branch.
	InProgress []Work `json:"inProgress,omitempty"`
	// Done is what was finished since the previous report.
	Done []Item `json:"done,omitempty"`
	// Blocked is what cannot move, and on whom it waits.
	Blocked []Blocker `json:"blocked,omitempty"`
	// Next is what the work identity does next, in order.
	Next []Item `json:"next,omitempty"`
	// DecisionsNeeded are questions only the operator can settle.
	DecisionsNeeded []DecisionRequest `json:"decisionsNeeded,omitempty"`
	// Notices tell other work identities about something that affects them.
	Notices []Notice `json:"notices,omitempty"`
}

// Item is one piece of work.
type Item struct {
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty" jsonschema:"pattern=^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$"`
	// Repositories are where the work is, as github.com/<org>/<repo>.
	Repositories []string `json:"repositories,omitempty" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	// Branch is the branch the work is on.
	Branch string `json:"branch,omitempty"`
	// Commits are the commits that make up the work.
	Commits []string `json:"commits,omitempty" jsonschema:"pattern=^[0-9a-f]{7\\,40}$"`
	// PullRequests are the URLs of its pull requests.
	PullRequests []string `json:"pullRequests,omitempty" jsonschema:"pattern=^(https://github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+|#[0-9]+)$"`
	// State is how far the work has travelled.
	State ItemState `json:"state,omitempty" jsonschema:"enum=uncommitted,enum=committed,enum=pushed,enum=released,enum=deployed"`
	// Versions are the releases the work went out in.
	Versions []string `json:"versions,omitempty"`
	// Paths are files the work produced, relative to their repository.
	Paths []string `json:"paths,omitempty"`
}

// Blocker is work that cannot move until another work identity, or the
// operator, does something other than a routine action.
type Blocker struct {
	Item
	// WaitingOn are the identities, or "operator", that must act, in order: a
	// push that waits on another work identity's release and then on the
	// operator's yes lists both.
	WaitingOn []string `json:"waitingOn" jsonschema:"minItems=1,pattern=^[a-z][a-z0-9-]*$"`
	// Decision is the ID of the decision request, in the same report, whose
	// answer unblocks it.
	Decision string `json:"decision,omitempty"`
	// Credentials are the IDs, in the credential registry, of credentials
	// it waits on.
	Credentials []string `json:"credentials,omitempty" jsonschema:"pattern=^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$"`
	// Unblocks says what else waits on this, including other work identities'
	// work.
	Unblocks []string `json:"unblocks,omitempty"`
}

// DecisionRequest asks the operator to decide. Once decided, the outcome
// is recorded as a Decision with the same ID.
type DecisionRequest struct {
	// ID is a short, stable slug; it becomes the decision's ID.
	ID string `json:"id" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Title states the question in a few words.
	Title string `json:"title"`
	// Situation is what prompted the question.
	Situation string `json:"situation"`
	// Evidence is what the recommendation rests on.
	Evidence []string `json:"evidence,omitempty"`
	// Options are the choices, at least two.
	Options []Option `json:"options"`
	// Recommendation is the name of the option the work identity recommends.
	Recommendation string `json:"recommendation"`
	// Approval says what the operator's answer authorizes, such as a
	// push, a spend or a change to another work identity's repository.
	Approval string `json:"approval"`
	// Refs are the initiatives and roadmap items it concerns.
	Refs []string `json:"refs,omitempty" jsonschema:"pattern=^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$"`
	// Repositories are the repositories it concerns.
	Repositories []string `json:"repositories,omitempty" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	// Asked is when the request was first raised; report submit sets it and
	// keeps it when the report is replaced, so the briefing can show how
	// long it has waited. A date alone is read as midnight UTC.
	Asked time.Time `json:"asked,omitzero"`
}

// Option is one choice in a DecisionRequest.
type Option struct {
	// Name identifies the option within its request.
	Name string `json:"name"`
	// Description says what choosing it means.
	Description string `json:"description"`
	// CostAndRisk is what choosing it costs and could go wrong.
	CostAndRisk string `json:"costAndRisk"`
	// Irreversible marks an option that cannot be undone once taken.
	Irreversible bool `json:"irreversible,omitempty"`
}

// Validate checks one report on its own. ValidateReports also checks the
// names it uses against the work identities.
func (r Report) Validate() error {
	var errs []string
	if !namePattern.MatchString(r.Identity) {
		errs = append(errs, fmt.Sprintf("work identity %q must match %s", r.Identity, namePattern))
	}
	if r.Generated.IsZero() {
		errs = append(errs, "generated is required")
	}
	for _, it := range r.Done {
		errs = append(errs, it.problems("done")...)
	}
	for _, it := range r.Next {
		errs = append(errs, it.problems("next")...)
	}
	ids := map[string]bool{}
	for _, d := range r.DecisionsNeeded {
		if ids[d.ID] {
			errs = append(errs, fmt.Sprintf("decision request %q listed twice", d.ID))
		}
		ids[d.ID] = true
	}
	for _, b := range r.Blocked {
		errs = append(errs, b.problems("blocked")...)
		if len(b.WaitingOn) == 0 {
			errs = append(errs, "blocked: waitingOn needs a work identity or "+Operator)
		}
		for _, w := range b.WaitingOn {
			if !namePattern.MatchString(w) {
				errs = append(errs, fmt.Sprintf("blocked: waitingOn %q must be a work identity name or %q", w, Operator))
			}
		}
		if b.Decision != "" && !ids[b.Decision] {
			errs = append(errs, fmt.Sprintf("blocked: decision %q is not a decision request in this report", b.Decision))
		}
		errs = append(errs, credentialProblems("blocked", b.Credentials)...)
	}
	for _, a := range r.OperatorActions {
		errs = append(errs, a.problems()...)
	}
	errs = append(errs, r.actionProblems()...)
	for _, w := range r.InProgress {
		errs = append(errs, w.problems()...)
	}
	for _, n := range r.Notices {
		errs = append(errs, n.problems()...)
	}
	for _, d := range r.DecisionsNeeded {
		if err := d.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("report %s: %s", r.Identity, strings.Join(errs, "; "))
	}
	return nil
}

func (it Item) problems(section string) []string {
	var errs []string
	if strings.TrimSpace(it.Description) == "" {
		errs = append(errs, section+": description is required")
	}
	errs = append(errs, refProblems(section, it.Refs, it.Repositories)...)
	repo := ""
	if len(it.Repositories) == 1 {
		repo = it.Repositories[0]
	}
	errs = append(errs, gitProblems(section, it.Commits, it.PullRequests, repo)...)
	if it.State != "" && !slices.Contains(ItemStates, it.State) {
		errs = append(errs, fmt.Sprintf("%s: state %q must be uncommitted, committed, pushed, released or deployed", section, it.State))
	}
	return errs
}

func refProblems(context string, refs, repos []string) []string {
	var errs []string
	for _, r := range refs {
		if !refPattern.MatchString(r) {
			errs = append(errs, fmt.Sprintf("%s: ref %q must be INIT-<SLUG>-NNN or RMI-<SLUG>-NNN", context, r))
		}
	}
	for _, r := range repos {
		if !repositoryPattern.MatchString(r) {
			errs = append(errs, fmt.Sprintf("%s: repository %q must be github.com/<org>/<repo>", context, r))
		}
	}
	return errs
}

// Validate checks one decision request.
func (d DecisionRequest) Validate() error {
	var errs []string
	if !namePattern.MatchString(d.ID) {
		errs = append(errs, fmt.Sprintf("id must match %s", namePattern))
	}
	for _, f := range []struct{ name, value string }{
		{"title", d.Title}, {"situation", d.Situation}, {"approval", d.Approval},
	} {
		if strings.TrimSpace(f.value) == "" {
			errs = append(errs, f.name+" is required")
		}
	}
	if len(d.Options) < 2 {
		errs = append(errs, "at least two options are required")
	}
	names := map[string]bool{}
	for _, o := range d.Options {
		if strings.TrimSpace(o.Name) == "" || strings.TrimSpace(o.Description) == "" || strings.TrimSpace(o.CostAndRisk) == "" {
			errs = append(errs, "each option needs a name, a description and its cost and risk")
		}
		if names[o.Name] {
			errs = append(errs, fmt.Sprintf("option %q listed twice", o.Name))
		}
		names[o.Name] = true
	}
	if !names[d.Recommendation] {
		errs = append(errs, fmt.Sprintf("recommendation %q must name an option", d.Recommendation))
	}
	errs = append(errs, refProblems("decision request", d.Refs, d.Repositories)...)
	if len(errs) > 0 {
		return fmt.Errorf("decision request %s: %s", d.ID, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateReports checks reports against the work identities: each is valid,
// comes from a work identity that is not retired, at most one per work
// identity, and blockers wait on an existing work identity or the operator.
func ValidateReports(reports []Report, identities []WorkIdentity) error {
	var errs []string
	byName := map[string]WorkIdentity{}
	for _, r := range identities {
		byName[r.Name] = r
	}
	seen := map[string]bool{}
	for _, rep := range reports {
		if err := rep.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		if seen[rep.Identity] {
			errs = append(errs, fmt.Sprintf("report %s: more than one report", rep.Identity))
		}
		seen[rep.Identity] = true
		identity, ok := byName[rep.Identity]
		switch {
		case !ok:
			errs = append(errs, fmt.Sprintf("report %s: work identity does not exist", rep.Identity))
		case identity.Status == IdentityRetired:
			errs = append(errs, fmt.Sprintf("report %s: work identity is retired", rep.Identity))
		}
		for _, b := range rep.Blocked {
			for _, w := range b.WaitingOn {
				if _, ok := byName[w]; !ok && w != Operator {
					errs = append(errs, fmt.Sprintf("report %s: waits on %q, which is not a work identity", rep.Identity, w))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("reports: %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// actionProblems checks what links actions to each other and to the
// report's decision requests: IDs are unique, After names actions in the
// report without a cycle, and Decision and Option name a request and one
// of its options.
func (r Report) actionProblems() []string {
	var errs []string
	byID := map[string]Action{}
	for _, a := range r.OperatorActions {
		if a.ID == "" {
			continue
		}
		if _, dup := byID[a.ID]; dup {
			errs = append(errs, fmt.Sprintf("operatorActions: id %q listed twice", a.ID))
		}
		byID[a.ID] = a
	}
	requests := map[string]DecisionRequest{}
	for _, d := range r.DecisionsNeeded {
		requests[d.ID] = d
	}
	for _, a := range r.OperatorActions {
		for _, dep := range a.After {
			if _, ok := byID[dep]; !ok {
				errs = append(errs, fmt.Sprintf("operatorActions: after %q is not an action id in this report", dep))
			}
		}
		if a.Decision == "" {
			continue
		}
		d, ok := requests[a.Decision]
		if !ok {
			errs = append(errs, fmt.Sprintf("operatorActions: decision %q is not a decision request in this report", a.Decision))
			continue
		}
		if a.Option != "" && !slices.ContainsFunc(d.Options, func(o Option) bool { return o.Name == a.Option }) {
			errs = append(errs, fmt.Sprintf("operatorActions: option %q is not an option of %s", a.Option, a.Decision))
		}
	}
	if id := afterCycle(byID); id != "" {
		errs = append(errs, fmt.Sprintf("operatorActions: %q waits on itself through after", id))
	}
	return errs
}

// afterCycle returns an action on a cycle of After links, or "".
func afterCycle(byID map[string]Action) string {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(id string) bool
	visit = func(id string) bool {
		switch state[id] {
		case visiting:
			return true
		case done:
			return false
		}
		state[id] = visiting
		for _, dep := range byID[id].After {
			if _, ok := byID[dep]; ok && visit(dep) {
				return true
			}
		}
		state[id] = done
		return false
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if visit(id) {
			return id
		}
	}
	return ""
}

// Normalize fills in what a work identity may abbreviate: a pull request given
// as #<n> becomes its URL when the action or item names exactly one repository.
// WriteReport normalizes before it validates.
func (r *Report) Normalize() {
	for i := range r.OperatorActions {
		a := &r.OperatorActions[i]
		expandPulls(a.PullRequests, a.Repository)
	}
	for _, items := range [][]Item{r.Done, r.Next} {
		for i := range items {
			items[i].normalize()
		}
	}
	for i := range r.Blocked {
		r.Blocked[i].normalize()
	}
}

func (it *Item) normalize() {
	if len(it.Repositories) == 1 {
		expandPulls(it.PullRequests, it.Repositories[0])
	}
}
