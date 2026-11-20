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

// ItemState is how far finished work has travelled.
type ItemState string

const (
	// StateCommitted is committed locally.
	StateCommitted ItemState = "committed"
	// StatePushed is pushed to the remote.
	StatePushed ItemState = "pushed"
	// StateReleased is tagged or published.
	StateReleased ItemState = "released"
	// StateDeployed is running in production.
	StateDeployed ItemState = "deployed"
)

// Action is a step only the operator takes, such as a push, tag or deploy.
// It is not a blocker: nothing is stuck, the work is ready. A briefing
// lists actions first because they are quick to clear and unblock most.
type Action struct {
	// Kind groups the action with others like it.
	Kind ActionKind `json:"kind" jsonschema:"enum=push,enum=merge,enum=tag,enum=deploy,enum=create-repository,enum=configure,enum=test,enum=review,enum=other"`
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Command is what to run, when there is one command.
	Command string `json:"command,omitempty"`
	// Repository is github.com/<org>/<repo>.
	Repository string `json:"repository,omitempty"`
	// Branch is the branch to push or merge.
	Branch string `json:"branch,omitempty"`
	// Commit is the commit to push, merge or tag.
	Commit string `json:"commit,omitempty"`
	// PullRequest is the pull request's URL.
	PullRequest string `json:"pullRequest,omitempty"`
	// Version is the version to tag or deploy.
	Version string `json:"version,omitempty"`
	// Checks are links to the CI runs or other evidence that it is ready.
	Checks []string `json:"checks,omitempty"`
	// Irreversible marks an action that cannot be undone, such as a tag on
	// a public Go module or a push to a public repository.
	Irreversible bool `json:"irreversible,omitempty"`
	// Unblocks says what waits on this action, including other roles' work.
	Unblocks string `json:"unblocks,omitempty"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty"`
}

// Work is something a role is doing now, and where. Two roles in the same
// repository is what a briefing warns about.
type Work struct {
	// Description says what, in a sentence.
	Description string `json:"description"`
	// Repository is github.com/<org>/<repo>.
	Repository string `json:"repository"`
	// Branch is the branch the work is on.
	Branch string `json:"branch,omitempty"`
	// Worktree names the worktree, when the work is not in the main
	// checkout.
	Worktree string `json:"worktree,omitempty"`
	// Base is the commit or tag the branch starts from.
	Base string `json:"base,omitempty"`
	// Refs are the initiatives and roadmap items it belongs to.
	Refs []string `json:"refs,omitempty"`
}

// Notice tells other roles about something that affects them, such as a
// bug in a shared dependency or a change to a shared file.
type Notice struct {
	// Title states it in a few words.
	Title string `json:"title"`
	// Description says what happened and what to do about it.
	Description string `json:"description"`
	// Repositories are the repositories it affects.
	Repositories []string `json:"repositories,omitempty"`
	// Modules are the Go modules or packages it affects, so roles that
	// depend on them can find it.
	Modules []string `json:"modules,omitempty"`
	// Refs are the initiatives and roadmap items it concerns.
	Refs []string `json:"refs,omitempty"`
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
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
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
	if a.Repository != "" && !repositoryPattern.MatchString(a.Repository) {
		errs = append(errs, fmt.Sprintf("operatorActions: repository %q must be github.com/<org>/<repo>", a.Repository))
	}
	errs = append(errs, gitProblems("operatorActions", a.Commit, a.PullRequest)...)
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
	return append(errs, refProblems("inProgress", w.Refs, nil)...)
}

func (n Notice) problems() []string {
	var errs []string
	if strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Description) == "" {
		errs = append(errs, "notices: title and description are required")
	}
	return append(errs, refProblems("notices", n.Refs, n.Repositories)...)
}

func gitProblems(context, commit, pr string, more ...string) []string {
	var errs []string
	if commit != "" && !commitPattern.MatchString(commit) {
		errs = append(errs, fmt.Sprintf("%s: commit %q must be a hexadecimal hash", context, commit))
	}
	for _, p := range append([]string{pr}, more...) {
		if p != "" && !pullRequestPattern.MatchString(p) {
			errs = append(errs, fmt.Sprintf("%s: pull request %q must be https://github.com/<org>/<repo>/pull/<n>", context, p))
		}
	}
	return errs
}

func kindList() string {
	s := make([]string, len(ActionKinds))
	for i, k := range ActionKinds {
		s[i] = string(k)
	}
	return strings.Join(s, ", ")
}
