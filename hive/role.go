// Package hive holds the formats that coordinate the long-lived agent
// sessions: roles (what each agent is for and where it works), decisions
// (the append-only record of why), and the envelope agents put on messages
// to each other.
//
// A role is durable; the session playing it is not. Sessions are renamed,
// restarted and replaced, so nothing here names a session. A session
// declares the role it plays with the HIVE_ROLE environment variable.
//
// The records live in a hive directory, usually in a private repository,
// one JSON file per record:
//
//	roles/<name>.json      one file per role
//	decisions/<id>.json    the append-only decision log
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

// RoleEnv is the environment variable a session sets to the name of the
// role it plays.
const RoleEnv = "HIVE_ROLE"

// RoleStatus is where a role is in its life.
type RoleStatus string

const (
	// RoleDraft is proposed and not yet confirmed by the operator.
	RoleDraft RoleStatus = "draft"
	// RoleActive is confirmed and in force.
	RoleActive RoleStatus = "active"
	// RoleRetired is kept for history; no session plays it.
	RoleRetired RoleStatus = "retired"
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

// Affinity is how a role relates to a repository. Several roles may work in
// the same repository; affinity says who settles conflicts there.
type Affinity string

const (
	// AffinitySteward settles conflicting changes and reviews breaking ones.
	// A repository has at most one steward.
	AffinitySteward Affinity = "steward"
	// AffinityContributor works in the repository regularly.
	AffinityContributor Affinity = "contributor"
)

// RepositoryAffinity ties a role to one repository, or to every repository
// in an organization.
type RepositoryAffinity struct {
	// Pattern is github.com/<org>/<repo>, or github.com/<org>/* for every
	// repository in the organization. An exact repository takes precedence
	// over an organization pattern.
	Pattern string `json:"pattern" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/([A-Za-z0-9_.-]+|\\*)$"`
	// Affinity is steward or contributor.
	Affinity Affinity `json:"affinity" jsonschema:"enum=steward,enum=contributor"`
}

// Role records what one long-lived agent is for and where it works,
// independently of the session that plays it.
type Role struct {
	// Name is the role's stable name, the name other agents address it by
	// and the value of HIVE_ROLE in the session that plays it.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Status is draft until the operator confirms the role.
	Status RoleStatus `json:"status" jsonschema:"enum=draft,enum=active,enum=retired"`
	// Mission says, in one or two sentences, what the role is responsible
	// for.
	Mission string `json:"mission"`
	// Autonomy is how far the role may go without asking.
	Autonomy Autonomy `json:"autonomy" jsonschema:"enum=observe,enum=prepare,enum=execute"`
	// Repositories are where the role works. A repository no role lists is
	// common ground any role may change.
	Repositories []RepositoryAffinity `json:"repositories,omitempty"`
	// Initiatives are the initiatives the role works on, as INIT-<SLUG>-NNN.
	Initiatives []string `json:"initiatives,omitempty"`
	// Peers are roles this one works with most.
	Peers []string `json:"peers,omitempty"`
	// Notes is free text for the operator: open questions while the role
	// is a draft, or constraints that do not fit another field.
	Notes string `json:"notes,omitempty"`
}

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	repositoryPattern = regexp.MustCompile(`^github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	affinityPattern   = regexp.MustCompile(`^github\.com/[A-Za-z0-9_.-]+/([A-Za-z0-9_.-]+|\*)$`)
	initiativePattern = regexp.MustCompile(`^INIT-[A-Z0-9]+-[0-9]{3}$`)
)

// Validate checks one role on its own. Checks that need the other roles,
// such as peers existing and repositories having one steward, are in
// ValidateRoles.
func (r Role) Validate() error {
	var errs []string
	if !namePattern.MatchString(r.Name) {
		errs = append(errs, fmt.Sprintf("name %q must match %s", r.Name, namePattern))
	}
	if r.Name == Operator {
		errs = append(errs, fmt.Sprintf("name %q is reserved for the operator", Operator))
	}
	switch r.Status {
	case RoleDraft, RoleActive, RoleRetired:
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
		case AffinitySteward, AffinityContributor:
		default:
			errs = append(errs, fmt.Sprintf("repository %q: affinity %q must be steward or contributor", a.Pattern, a.Affinity))
		}
	}
	for _, i := range r.Initiatives {
		if !initiativePattern.MatchString(i) {
			errs = append(errs, fmt.Sprintf("initiative %q must be INIT-<SLUG>-NNN", i))
		}
	}
	for _, p := range r.Peers {
		if p == r.Name {
			errs = append(errs, "a role cannot be its own peer")
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("role %s: %s", r.Name, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateRoles checks a set of roles together: each is valid, names are
// unique, peers name existing roles, and no pattern has two stewards among
// roles that are not retired.
func ValidateRoles(roles []Role) error {
	var errs []string
	byName := map[string]bool{}
	for _, r := range roles {
		if err := r.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		if byName[r.Name] {
			errs = append(errs, fmt.Sprintf("role %s: duplicate name", r.Name))
		}
		byName[r.Name] = true
	}
	steward := map[string]string{}
	for _, r := range roles {
		for _, p := range r.Peers {
			if !byName[p] {
				errs = append(errs, fmt.Sprintf("role %s: peer %q has no role", r.Name, p))
			}
		}
		if r.Status == RoleRetired {
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
		return fmt.Errorf("roles: %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// Match is one role's affinity for a repository.
type Match struct {
	Role     string
	Affinity Affinity
	// Exact is true when the role names the repository itself rather than
	// its organization.
	Exact bool
}

// RolesFor returns the roles, not retired, that work in repo
// (github.com/<org>/<repo>): the steward first, then contributors by name.
// An exact steward outranks an organization steward, which is then
// reported as a contributor, so at most one steward is returned.
func RolesFor(roles []Role, repo string) ([]Match, error) {
	if !repositoryPattern.MatchString(repo) {
		return nil, fmt.Errorf("hive: repository %q must be github.com/<org>/<repo>", repo)
	}
	org := repo[:strings.LastIndex(repo, "/")+1] + "*"
	var out []Match
	exactSteward := false
	for _, r := range roles {
		if r.Status == RoleRetired {
			continue
		}
		var best *Match
		for _, a := range r.Repositories {
			if a.Pattern != repo && a.Pattern != org {
				continue
			}
			m := Match{Role: r.Name, Affinity: a.Affinity, Exact: a.Pattern == repo}
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
			out[i].Affinity = AffinityContributor
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i].Affinity == AffinitySteward, out[j].Affinity == AffinitySteward
		if si != sj {
			return si
		}
		return out[i].Role < out[j].Role
	})
	return out, nil
}

// rank orders one role's matches for the same repository: steward over
// contributor, then exact over organization.
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

// Steward returns the role that stewards repo, or "" when no role does and
// the repository is common ground.
func Steward(roles []Role, repo string) (string, error) {
	matches, err := RolesFor(roles, repo)
	if err != nil {
		return "", err
	}
	if len(matches) > 0 && matches[0].Affinity == AffinitySteward {
		return matches[0].Role, nil
	}
	return "", nil
}
