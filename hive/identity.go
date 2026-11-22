// Package hive holds the formats that coordinate the long-lived agent
// sessions: work identities (what each agent is for and where it works),
// decisions (the append-only record of why), and the envelope agents put on
// messages to each other.
//
// A work identity is durable; the sessions that work under it are not.
// Sessions are renamed, restarted and replaced, so nothing here names a
// session. A session declares its work identity with the HIVE_IDENTITY
// environment variable. Role is left to mean an access level, as in
// role-based access control; a work identity is not a role.
//
// The records live in a hive directory, usually in a private repository,
// one JSON file per record:
//
//	identities/<name>.json  one file per work identity
//	decisions/<id>.json     the append-only decision log
//
// Load reads a hive directory and Validate checks it as a whole. The Go
// types are the source of truth; the JSON Schemas under schema/ are
// generated from them.
package hive

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// IdentityEnv is the environment variable a session sets to the name of
// the work identity it works under.
const IdentityEnv = "HIVE_IDENTITY"

// LegacyIdentityEnv is IdentityEnv's name before roles became work
// identities; it is still read, after IdentityEnv.
const LegacyIdentityEnv = "HIVE_ROLE"

// IdentityStatus is where a work identity is in its life.
type IdentityStatus string

const (
	// IdentityDraft is proposed and not yet confirmed by the operator.
	IdentityDraft IdentityStatus = "draft"
	// IdentityActive is confirmed and in force.
	IdentityActive IdentityStatus = "active"
	// IdentityRetired is kept for history; no session works under it.
	IdentityRetired IdentityStatus = "retired"
)

// Autonomy is how far an agent may go without asking, in increasing order.
type Autonomy string

const (
	// AutonomyObserve reads, analyzes and reports; it changes nothing.
	AutonomyObserve Autonomy = "observe"
	// AutonomyPrepare also makes changes locally for the operator to
	// publish: worktrees, branches, commits, release notes and changelogs.
	// It does not push, tag, merge, deploy or send.
	AutonomyPrepare Autonomy = "prepare"
	// AutonomyExecute also carries out bounded, low-risk changes in the
	// repositories it works in. Pricing, refunds, production deploys,
	// spending and messages to customers still need approval.
	AutonomyExecute Autonomy = "execute"
)

// Affinity is how a work identity relates to a repository. Several work
// identities may work in the same repository; affinity says who settles
// conflicts there.
type Affinity string

const (
	// AffinitySteward settles conflicting changes and reviews breaking ones.
	// A repository has at most one steward.
	AffinitySteward Affinity = "steward"
	// AffinityCollaborator works in the repository regularly. Before
	// v0.5.0 it was named contributor, which is still read.
	AffinityCollaborator Affinity = "collaborator"
)

// RepositoryAffinity ties a work identity to one repository, or to every
// repository in an organization.
type RepositoryAffinity struct {
	// Pattern is github.com/<org>/<repo>, or github.com/<org>/* for every
	// repository in the organization. An exact repository takes precedence
	// over an organization pattern.
	Pattern string `json:"pattern" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/([A-Za-z0-9_.-]+|\\*)$"`
	// Affinity is steward or collaborator.
	Affinity Affinity `json:"affinity" jsonschema:"enum=steward,enum=collaborator"`
}

// WorkIdentity records what one long-lived agent is for and where it
// works, independently of the sessions that work under it.
type WorkIdentity struct {
	// Name is the work identity's stable name, the name other agents
	// address it by and the value of HIVE_IDENTITY in the sessions that work
	// under it.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Status is draft until the operator confirms the work identity.
	Status IdentityStatus `json:"status" jsonschema:"enum=draft,enum=active,enum=retired"`
	// Mission says, in one or two sentences, what the work identity is
	// responsible for.
	Mission string `json:"mission"`
	// Autonomy is how far the work identity may go without asking.
	Autonomy Autonomy `json:"autonomy" jsonschema:"enum=observe,enum=prepare,enum=execute"`
	// Repositories are where the work identity works. A repository no
	// work identity lists is common ground any of them may change.
	Repositories []RepositoryAffinity `json:"repositories,omitempty"`
	// Initiatives are the initiatives it works on, as INIT-<SLUG>-NNN.
	Initiatives []string `json:"initiatives,omitempty"`
	// Peers are the work identities this one works with most.
	Peers []string `json:"peers,omitempty"`
	// Notes is free text for the operator: open questions while the work
	// identity is a draft, or constraints that do not fit another field.
	Notes string `json:"notes,omitempty"`
}

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	repositoryPattern = regexp.MustCompile(`^github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	affinityPattern   = regexp.MustCompile(`^github\.com/[A-Za-z0-9_.-]+/([A-Za-z0-9_.-]+|\*)$`)
	initiativePattern = regexp.MustCompile(`^INIT-[A-Z0-9]+-[0-9]{3}$`)
)

// Validate checks one work identity on its own. Checks that need the
// others, such as peers existing and repositories having one steward, are
// in ValidateIdentities.
func (r WorkIdentity) Validate() error {
	var errs []string
	if !namePattern.MatchString(r.Name) {
		errs = append(errs, fmt.Sprintf("name %q must match %s", r.Name, namePattern))
	}
	if r.Name == Operator {
		errs = append(errs, fmt.Sprintf("name %q is reserved for the operator", Operator))
	}
	switch r.Status {
	case IdentityDraft, IdentityActive, IdentityRetired:
	default:
		errs = append(errs, fmt.Sprintf("status %q must be draft, active or retired", r.Status))
	}
	if strings.TrimSpace(r.Mission) == "" {
		errs = append(errs, "mission is required")
	}
	switch r.Autonomy {
	case AutonomyObserve, AutonomyPrepare, AutonomyExecute:
	default:
		errs = append(errs, fmt.Sprintf("autonomy %q must be observe, prepare or execute", r.Autonomy))
	}
	seen := map[string]bool{}
	for _, a := range r.Repositories {
		if !affinityPattern.MatchString(a.Pattern) {
			errs = append(errs, fmt.Sprintf("repository %q must be github.com/<org>/<repo> or github.com/<org>/*", a.Pattern))
		}
		if seen[a.Pattern] {
			errs = append(errs, fmt.Sprintf("repository %q listed twice", a.Pattern))
		}
		seen[a.Pattern] = true
		switch a.Affinity {
		case AffinitySteward, AffinityCollaborator:
		default:
			errs = append(errs, fmt.Sprintf("repository %q: affinity %q must be steward or collaborator", a.Pattern, a.Affinity))
		}
	}
	for _, i := range r.Initiatives {
		if !initiativePattern.MatchString(i) {
			errs = append(errs, fmt.Sprintf("initiative %q must be INIT-<SLUG>-NNN", i))
		}
	}
	for _, p := range r.Peers {
		if p == r.Name {
			errs = append(errs, "a work identity cannot be its own peer")
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("work identity %s: %s", r.Name, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateIdentities checks a set of work identities together: each is
// valid, names are unique, peers name existing identities, and no pattern
// has two stewards among identities that are not retired.
func ValidateIdentities(identities []WorkIdentity) error {
	var errs []string
	byName := map[string]bool{}
	for _, r := range identities {
		if err := r.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		if byName[r.Name] {
			errs = append(errs, fmt.Sprintf("work identity %s: duplicate name", r.Name))
		}
		byName[r.Name] = true
	}
	steward := map[string]string{}
	for _, r := range identities {
		for _, p := range r.Peers {
			if !byName[p] {
				errs = append(errs, fmt.Sprintf("work identity %s: peer %q has no work identity", r.Name, p))
			}
		}
		if r.Status == IdentityRetired {
			continue
		}
		for _, a := range r.Repositories {
			if a.Affinity != AffinitySteward {
				continue
			}
			if prev, ok := steward[a.Pattern]; ok {
				errs = append(errs, fmt.Sprintf("%s has two stewards: %s and %s", a.Pattern, prev, r.Name))
				continue
			}
			steward[a.Pattern] = r.Name
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("work identities: %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// Match is one work identity's affinity for a repository.
type Match struct {
	Identity string
	Affinity Affinity
	// Exact is true when the work identity names the repository itself
	// rather than its organization.
	Exact bool
}

// IdentitiesFor returns the work identities, not retired, that work in
// repo (github.com/<org>/<repo>): the steward first, then collaborators by
// name. An exact steward outranks an organization steward, which is then
// reported as a collaborator, so at most one steward is returned.
func IdentitiesFor(identities []WorkIdentity, repo string) ([]Match, error) {
	if !repositoryPattern.MatchString(repo) {
		return nil, fmt.Errorf("hive: repository %q must be github.com/<org>/<repo>", repo)
	}
	org := repo[:strings.LastIndex(repo, "/")+1] + "*"
	var out []Match
	exactSteward := false
	for _, r := range identities {
		if r.Status == IdentityRetired {
			continue
		}
		var best *Match
		for _, a := range r.Repositories {
			if a.Pattern != repo && a.Pattern != org {
				continue
			}
			m := Match{Identity: r.Name, Affinity: a.Affinity, Exact: a.Pattern == repo}
			if best == nil || rank(m) > rank(*best) {
				best = &m
			}
		}
		if best == nil {
			continue
		}
		if best.Exact && best.Affinity == AffinitySteward {
			exactSteward = true
		}
		out = append(out, *best)
	}
	for i, m := range out {
		if exactSteward && !m.Exact && m.Affinity == AffinitySteward {
			out[i].Affinity = AffinityCollaborator
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i].Affinity == AffinitySteward, out[j].Affinity == AffinitySteward
		if si != sj {
			return si
		}
		return out[i].Identity < out[j].Identity
	})
	return out, nil
}

// rank orders one work identity's matches for the same repository:
// steward over collaborator, then exact over organization.
func rank(m Match) int {
	n := 0
	if m.Affinity == AffinitySteward {
		n += 2
	}
	if m.Exact {
		n++
	}
	return n
}

// Steward returns the work identity that stewards repo, or "" when none
// does and the repository is common ground.
func Steward(identities []WorkIdentity, repo string) (string, error) {
	matches, err := IdentitiesFor(identities, repo)
	if err != nil {
		return "", err
	}
	if len(matches) > 0 && matches[0].Affinity == AffinitySteward {
		return matches[0].Identity, nil
	}
	return "", nil
}
