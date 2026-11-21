package hive

import (
	"fmt"
	"regexp"
	"strings"
)

// ActionKind is what the operator is asked to do.
type ActionKind string

const (
	// ActionPush pushes a branch.
	ActionPush ActionKind = "push"
	// ActionMerge merges a pull request or branch.
	ActionMerge ActionKind = "merge"
	// ActionTag creates and pushes a release tag.
	ActionTag ActionKind = "tag"
	// ActionDeploy deploys a release.
	ActionDeploy ActionKind = "deploy"
	// ActionCreateRepository creates a repository or its remote.
	ActionCreateRepository ActionKind = "create-repository"
	// ActionConfigure sets up an account, secret, domain or service.
	ActionConfigure ActionKind = "configure"
	// ActionTest tries something only the operator can, such as a purchase
	// or a document in another application.
	ActionTest ActionKind = "test"
	// ActionReview reviews work that is not committed or not yet proposed.
	ActionReview ActionKind = "review"
	// ActionOther is anything else.
	ActionOther ActionKind = "other"
)

// ActionKinds lists the kinds in the order a briefing groups them: what
// publishes work first, then setup, then review.
var ActionKinds = []ActionKind{
	ActionPush, ActionMerge, ActionTag, ActionDeploy, ActionCreateRepository,
	ActionConfigure, ActionTest, ActionReview, ActionOther,
}

// ItemState is how far work has travelled.
type ItemState string

const (
	// StateUncommitted is finished in the working tree but not committed,
	// such as work waiting on the operator's review.
	StateUncommitted ItemState = "uncommitted"
	// StateCommitted is committed locally.
	StateCommitted ItemState = "committed"
	// StatePushed is pushed to the remote.
	StatePushed ItemState = "pushed"
	// StateReleased is tagged or published.
	StateReleased ItemState = "released"
	// StateDeployed is running in production.
	StateDeployed ItemState = "deployed"
)

// ItemStates lists the states in the order work travels.
var ItemStates = []ItemState{StateUncommitted, StateCommitted, StatePushed, StateReleased, StateDeployed}

// Action is a step only the operator takes, such as a push, tag or deploy.
// It is not a blocker: nothing is stuck, the work is ready. A briefing
// lists actions first because they are quick to clear and unblock most.
type Action struct {
	// ID names the action within the report, so other actions can wait on
	// it with After.
	ID string `json:"id,omitempty" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Kind groups the action with others like it.
	Kind ActionKind `json:"kind" jsonschema:"enum=push,enum=merge,enum=tag,enum=deploy,enum=create-repository,enum=configure,enum=test,enum=review,enum=other"`
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Command is what to run, when there is one command.
	Command string `json:"command,omitempty"`
	// Steps are what to do by hand, in order, when one command will not
	// do, such as setting a secret whose value must not appear here.
	Steps []string `json:"steps,omitempty"`
	// After lists the IDs of actions in the same report that must be done
	// first, such as the push a tag needs.
	After []string `json:"after,omitempty" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Decision is the ID of a decision request in the same report; the
	// action applies only if the operator chooses Option.
	Decision string `json:"decision,omitempty"`
	// Option is the option of Decision the action applies under.
	Option string `json:"option,omitempty"`
	// Repository is github.com/<org>/<repo>.
	Repository string `json:"repository,omitempty" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	// Branch is the branch to push or merge.
	Branch string `json:"branch,omitempty"`
	// Commits are the commits to push, merge or tag; for a push, the head
	// commit is enough.
	Commits []string `json:"commits,omitempty" jsonschema:"pattern=^[0-9a-f]{7\\,40}$"`
	// PullRequests are the pull requests' URLs, or #<n> with Repository.
	PullRequests []string `json:"pullRequests,omitempty" jsonschema:"pattern=^(https://github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+|#[0-9]+)$"`
	// Versions are the versions to tag or deploy; a nested module adds its
	// own, such as v0.1.0 and oscal/v0.1.0 on one commit.
	Versions []string `json:"versions,omitempty"`
	// Paths are files the operator works with, relative to Repository.
	Paths []string `json:"paths,omitempty"`
	// Credentials are the IDs, in the credential registry, of credentials
	// the action creates, sets or needs; never their values.
	Credentials []string `json:"credentials,omitempty" jsonschema:"pattern=^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$"`
	// Checks are links to the CI runs or other evidence that it is ready.
	Checks []string `json:"checks,omitempty"`
	// Irreversible marks an action that cannot be undone, such as a tag on
	// a public Go module or a push to a public repository.
	Irreversible bool `json:"irreversible,omitempty"`
	// Ongoing marks an action that is kept up rather than done once, such
	// as keeping a local webhook listener running while a role tests.
	Ongoing bool `json:"ongoing,omitempty"`
	// FollowUp is what the role does once the operator has acted, such as
	// opening a pull request or recording a deploy.
	FollowUp string `json:"followUp,omitempty"`
	// Unblocks says what waits on this action, including other roles' work.
	Unblocks []string `json:"unblocks,omitempty"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty" jsonschema:"pattern=^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$"`
}

// Work is something a role is doing now, and where. Two roles in the same
// repository is what a briefing warns about.
type Work struct {
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Repository is github.com/<org>/<repo>.
	Repository string `json:"repository" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	// Branch is the branch the work is on.
	Branch string `json:"branch,omitempty"`
	// Worktree names the worktree, when the work is not in the main
	// checkout.
	Worktree string `json:"worktree,omitempty"`
	// Base is the commit or tag the branch starts from.
	Base string `json:"base,omitempty"`
	// State is how far the work has got: uncommitted, committed or pushed.
	// Work that is finished but not yet pushed stays here, with its state,
	// rather than also being listed as done.
	State ItemState `json:"state,omitempty" jsonschema:"enum=uncommitted,enum=committed,enum=pushed"`
	// Credentials are the IDs, in the credential registry, of credentials
	// the work needs.
	Credentials []string `json:"credentials,omitempty" jsonschema:"pattern=^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty" jsonschema:"pattern=^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$"`
}

// Notice tells other roles about something that affects them, such as a
// bug in a shared dependency or a change to a shared file.
type Notice struct {
	// Title states it in a few words.
	Title string `json:"title"`
	// Description says what happened and what to do about it.
	Description string `json:"description"`
	// Repositories are the repositories it affects.
	Repositories []string `json:"repositories,omitempty" jsonschema:"pattern=^github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"`
	// Modules are the Go modules or packages it affects, so roles that
	// depend on them can find it.
	Modules []string `json:"modules,omitempty"`
	// Refs are the initiatives and roadmap items it concerns.
	Refs []string `json:"refs,omitempty" jsonschema:"pattern=^(INIT|RMI)-[A-Z0-9]+-[0-9]{3}$"`
}

// Session says how to reach the session playing a role. It is reported,
// not recorded with the role, because sessions come and go.
type Session struct {
	// Tmux is the tmux session name.
	Tmux string `json:"tmux,omitempty"`
	// Agent is the name other agents use to message the session.
	Agent string `json:"agent,omitempty"`
}

var (
	pullRequestPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+$`)
	shortPullPattern   = regexp.MustCompile(`^#([0-9]+)$`)
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	credentialPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)
)

func (a Action) problems() []string {
	var errs []string
	known := false
	for _, k := range ActionKinds {
		known = known || a.Kind == k
	}
	if !known {
		errs = append(errs, fmt.Sprintf("operatorActions: kind %q is not one of %s", a.Kind, kindList()))
	}
	if strings.TrimSpace(a.Description) == "" {
		errs = append(errs, "operatorActions: description is required")
	}
	if a.ID != "" && !namePattern.MatchString(a.ID) {
		errs = append(errs, fmt.Sprintf("operatorActions: id %q must match %s", a.ID, namePattern))
	}
	if a.Option != "" && a.Decision == "" {
		errs = append(errs, "operatorActions: option needs the decision it belongs to")
	}
	if a.Repository != "" && !repositoryPattern.MatchString(a.Repository) {
		errs = append(errs, fmt.Sprintf("operatorActions: repository %q must be github.com/<org>/<repo>", a.Repository))
	}
	errs = append(errs, gitProblems("operatorActions", a.Commits, a.PullRequests)...)
	errs = append(errs, credentialProblems("operatorActions", a.Credentials)...)
	return append(errs, refProblems("operatorActions", a.Refs, nil)...)
}

func (w Work) problems() []string {
	var errs []string
	if strings.TrimSpace(w.Description) == "" {
		errs = append(errs, "inProgress: description is required")
	}
	if !repositoryPattern.MatchString(w.Repository) {
		errs = append(errs, fmt.Sprintf("inProgress: repository %q must be github.com/<org>/<repo>", w.Repository))
	}
	switch w.State {
	case "", StateUncommitted, StateCommitted, StatePushed:
	default:
		errs = append(errs, fmt.Sprintf("inProgress: state %q must be uncommitted, committed or pushed", w.State))
	}
	errs = append(errs, credentialProblems("inProgress", w.Credentials)...)
	return append(errs, refProblems("inProgress", w.Refs, nil)...)
}

func (n Notice) problems() []string {
	var errs []string
	if strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Description) == "" {
		errs = append(errs, "notices: title and description are required")
	}
	return append(errs, refProblems("notices", n.Refs, n.Repositories)...)
}

func gitProblems(context string, commits, prs []string) []string {
	var errs []string
	for _, c := range commits {
		if !commitPattern.MatchString(c) {
			errs = append(errs, fmt.Sprintf("%s: commit %q must be a hexadecimal hash", context, c))
		}
	}
	for _, p := range prs {
		switch {
		case pullRequestPattern.MatchString(p):
		case shortPullPattern.MatchString(p):
			errs = append(errs, fmt.Sprintf("%s: pull request %q needs exactly one repository to expand it; give the full URL", context, p))
		default:
			errs = append(errs, fmt.Sprintf("%s: pull request %q must be https://github.com/<org>/<repo>/pull/<n>, or #<n> with one repository", context, p))
		}
	}
	return errs
}

func credentialProblems(context string, ids []string) []string {
	var errs []string
	for _, id := range ids {
		if !credentialPattern.MatchString(id) {
			errs = append(errs, fmt.Sprintf("%s: credential %q must be a registry ID such as app/prod/DB_URL", context, id))
		}
	}
	return errs
}

// expandPulls turns #<n> into the pull request's URL in repo, when repo is
// known.
func expandPulls(prs []string, repo string) {
	if repo == "" {
		return
	}
	for i, p := range prs {
		if m := shortPullPattern.FindStringSubmatch(p); m != nil {
			prs[i] = "https://" + repo + "/pull/" + m[1]
		}
	}
}

func kindList() string {
	s := make([]string, len(ActionKinds))
	for i, k := range ActionKinds {
		s[i] = string(k)
	}
	return strings.Join(s, ", ")
}
