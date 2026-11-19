package hive

import (
	"fmt"
	"strings"
	"time"
)

// Operator is the name a blocker waits on when it waits on the human
// operator rather than on another role.
const Operator = "operator"

// Report is one role's status at a point in time: what it finished, what is
// blocked, what it does next and what it needs the operator to decide. A
// role keeps one current report, replaced each time it reports; history,
// when wanted, is the caller's concern.
type Report struct {
	// Role is the reporting role's name.
	Role string `json:"role" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Generated is when the report was written. Readers use it to show how
	// fresh the report is.
	Generated time.Time `json:"generated"`
	// Summary is one or two sentences for a briefing.
	Summary string `json:"summary,omitempty"`
	// Done is what was finished since the previous report.
	Done []Item `json:"done,omitempty"`
	// Blocked is what cannot move, and on whom it waits.
	Blocked []Blocker `json:"blocked,omitempty"`
	// Next is what the role does next, in order.
	Next []Item `json:"next,omitempty"`
	// DecisionsNeeded are questions only the operator can settle.
	DecisionsNeeded []DecisionRequest `json:"decisionsNeeded,omitempty"`
}

// Item is one piece of work.
type Item struct {
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty"`
	// Repositories are where the work is, as github.com/<org>/<repo>.
	Repositories []string `json:"repositories,omitempty"`
}

// Blocker is work that cannot move.
type Blocker struct {
	Item
	// WaitingOn is the role that can unblock it, or "operator".
	WaitingOn string `json:"waitingOn" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
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
	// Recommendation is the name of the option the role recommends.
	Recommendation string `json:"recommendation"`
	// Approval says what the operator's answer authorizes, such as a
	// push, a spend or a change to another role's repository.
	Approval string `json:"approval"`
	// Refs are the initiatives and roadmap items it concerns.
	Refs []string `json:"refs,omitempty"`
	// Repositories are the repositories it concerns.
	Repositories []string `json:"repositories,omitempty"`
}

// Option is one choice in a DecisionRequest.
type Option struct {
	// Name identifies the option within its request.
	Name string `json:"name"`
	// Description says what choosing it means.
	Description string `json:"description"`
	// CostAndRisk is what choosing it costs and could go wrong.
	CostAndRisk string `json:"costAndRisk"`
}

// Validate checks one report on its own. ValidateReports also checks the
// names it uses against the roles.
func (r Report) Validate() error {
	var errs []string
	if !namePattern.MatchString(r.Role) {
		errs = append(errs, fmt.Sprintf("role %q must match %s", r.Role, namePattern))
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
	for _, b := range r.Blocked {
		errs = append(errs, b.problems("blocked")...)
		if !namePattern.MatchString(b.WaitingOn) {
			errs = append(errs, fmt.Sprintf("blocked: waitingOn %q must be a role name or %q", b.WaitingOn, Operator))
		}
	}
	ids := map[string]bool{}
	for _, d := range r.DecisionsNeeded {
		if ids[d.ID] {
			errs = append(errs, fmt.Sprintf("decision request %q listed twice", d.ID))
		}
		ids[d.ID] = true
		if err := d.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("report %s: %s", r.Role, strings.Join(errs, "; "))
	}
	return nil
}

func (it Item) problems(section string) []string {
	var errs []string
	if strings.TrimSpace(it.Description) == "" {
		errs = append(errs, section+": description is required")
	}
	errs = append(errs, refProblems(section, it.Refs, it.Repositories)...)
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

// ValidateReports checks reports against the roles: each is valid, comes
// from a role that is not retired, at most one per role, and blockers wait
// on an existing role or the operator.
func ValidateReports(reports []Report, roles []Role) error {
	var errs []string
	byName := map[string]Role{}
	for _, r := range roles {
		byName[r.Name] = r
	}
	seen := map[string]bool{}
	for _, rep := range reports {
		if err := rep.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		if seen[rep.Role] {
			errs = append(errs, fmt.Sprintf("report %s: more than one report", rep.Role))
		}
		seen[rep.Role] = true
		role, ok := byName[rep.Role]
		switch {
		case !ok:
			errs = append(errs, fmt.Sprintf("report %s: role does not exist", rep.Role))
		case role.Status == RoleRetired:
			errs = append(errs, fmt.Sprintf("report %s: role is retired", rep.Role))
		}
		for _, b := range rep.Blocked {
			if _, ok := byName[b.WaitingOn]; !ok && b.WaitingOn != Operator {
				errs = append(errs, fmt.Sprintf("report %s: waits on %q, which is not a role", rep.Role, b.WaitingOn))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("reports: %s", strings.Join(errs, "\n  "))
	}
	return nil
}
