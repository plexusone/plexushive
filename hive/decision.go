package hive

import (
	"fmt"
	"regexp"
	"strings"
)

// DecisionStatus is whether a decision is still in force.
type DecisionStatus string

const (
	// DecisionAccepted is in force.
	DecisionAccepted DecisionStatus = "accepted"
	// DecisionSuperseded has been replaced; SupersededBy names the
	// replacement.
	DecisionSuperseded DecisionStatus = "superseded"
)

// Decision is one entry in the append-only decision log. Each decision is
// its own file, named after its ID, so agents writing at the same time never
// edit the same file. When it was made comes from git history, not from the
// record.
type Decision struct {
	// ID is a short, stable slug that also names the file.
	ID string `json:"id" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Title states the decision in a few words.
	Title string `json:"title"`
	// Status is accepted until a later decision supersedes it.
	Status DecisionStatus `json:"status" jsonschema:"enum=accepted,enum=superseded"`
	// Decision is what was decided.
	Decision string `json:"decision"`
	// Option is the name of the option chosen, when the decision answers a
	// decision request; actions that apply only under an option follow it.
	Option string `json:"option,omitempty"`
	// Reasons are why.
	Reasons []string `json:"reasons"`
	// Alternatives are the options considered and not taken.
	Alternatives []Alternative `json:"alternatives,omitempty"`
	// Refs are the initiatives and roadmap items the decision belongs to or
	// constrains: INIT-<SLUG>-NNN or RMI-<SLUG>-NNN.
	Refs []string `json:"refs,omitempty"`
	// Identities are the work identities the decision binds, by name. Empty
	// means it binds every work identity.
	Identities []string `json:"identities,omitempty"`
	// Repositories are the repositories the decision constrains, as
	// github.com/<org>/<repo>, so an agent can find the decisions that
	// apply to the code it works on.
	Repositories []string `json:"repositories,omitempty"`
	// Supersedes names decisions this one replaces.
	Supersedes []string `json:"supersedes,omitempty"`
	// SupersededBy names the decision that replaced this one.
	SupersededBy string `json:"supersededBy,omitempty"`
}

// Alternative is an option considered and not taken.
type Alternative struct {
	// Option is what was considered.
	Option string `json:"option"`
	// Rejected says why it was not taken.
	Rejected string `json:"rejected"`
}

var refPattern = regexp.MustCompile(`^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$`)

// Validate checks one decision on its own.
func (d Decision) Validate() error {
	var errs []string
	if !namePattern.MatchString(d.ID) {
		errs = append(errs, fmt.Sprintf("id %q must match %s", d.ID, namePattern))
	}
	if strings.TrimSpace(d.Title) == "" {
		errs = append(errs, "title is required")
	}
	if strings.TrimSpace(d.Decision) == "" {
		errs = append(errs, "decision is required")
	}
	if len(d.Reasons) == 0 {
		errs = append(errs, "at least one reason is required")
	}
	switch d.Status {
	case DecisionAccepted:
		if d.SupersededBy != "" {
			errs = append(errs, "an accepted decision has no supersededBy")
		}
	case DecisionSuperseded:
		if d.SupersededBy == "" {
			errs = append(errs, "a superseded decision names its supersededBy")
		}
	default:
		errs = append(errs, fmt.Sprintf("status %q must be accepted or superseded", d.Status))
	}
	for _, a := range d.Alternatives {
		if strings.TrimSpace(a.Option) == "" || strings.TrimSpace(a.Rejected) == "" {
			errs = append(errs, "each alternative needs an option and why it was rejected")
		}
	}
	for _, r := range d.Refs {
		if !refPattern.MatchString(r) {
			errs = append(errs, fmt.Sprintf("ref %q must be INIT-<SLUG>-NNN or RMI-<SLUG>-NNN", r))
		}
	}
	for _, r := range d.Repositories {
		if !repositoryPattern.MatchString(r) {
			errs = append(errs, fmt.Sprintf("repository %q must be github.com/<org>/<repo>", r))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("decision %s: %s", d.ID, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateDecisions checks the log as a whole: each decision is valid, IDs
// are unique, references between decisions resolve both ways, and named
// work identities exist.
func ValidateDecisions(decisions []Decision, identities []WorkIdentity) error {
	var errs []string
	byID := map[string]Decision{}
	for _, d := range decisions {
		if err := d.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		if _, ok := byID[d.ID]; ok {
			errs = append(errs, fmt.Sprintf("decision %s: duplicate id", d.ID))
		}
		byID[d.ID] = d
	}
	known := map[string]bool{}
	for _, r := range identities {
		known[r.Name] = true
	}
	for _, d := range decisions {
		for _, r := range d.Identities {
			if !known[r] {
				errs = append(errs, fmt.Sprintf("decision %s: work identity %q does not exist", d.ID, r))
			}
		}
		for _, s := range d.Supersedes {
			old, ok := byID[s]
			switch {
			case !ok:
				errs = append(errs, fmt.Sprintf("decision %s: supersedes unknown %q", d.ID, s))
			case old.SupersededBy != d.ID:
				errs = append(errs, fmt.Sprintf("decision %s: %s must name it in supersededBy", d.ID, s))
			}
		}
		if d.SupersededBy != "" {
			next, ok := byID[d.SupersededBy]
			if !ok {
				errs = append(errs, fmt.Sprintf("decision %s: supersededBy unknown %q", d.ID, d.SupersededBy))
			} else if !contains(next.Supersedes, d.ID) {
				errs = append(errs, fmt.Sprintf("decision %s: %s must list it in supersedes", d.ID, d.SupersededBy))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("decisions: %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
