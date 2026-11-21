package hive

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// A session finds the role it plays in three places, in order: a value the
// caller was given (such as a --role flag), the HIVE_ROLE environment
// variable, and the environment of the tmux session it runs in. The last
// reaches agents started before the role was bound: tmux set-environment
// changes the session's environment for new processes only, but the value
// can always be read back from tmux.

// RoleSource says where a role was found.
type RoleSource string

const (
	// SourceGiven is a role the caller passed in, such as a flag.
	SourceGiven RoleSource = "given"
	// SourceEnv is the HIVE_ROLE environment variable.
	SourceEnv RoleSource = "environment"
	// SourceTmux is the tmux session's environment.
	SourceTmux RoleSource = "tmux"
)

// Tmux runs a tmux command and returns its standard output. Tests replace
// it.
var Tmux = func(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output() //nolint:gosec // fixed program; arguments are tmux subcommands
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("tmux: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// InTmux reports whether the process runs inside a tmux session.
func InTmux() bool {
	return os.Getenv("TMUX") != ""
}

// TmuxSession returns the name of the tmux session the process runs in, or
// "" outside tmux.
func TmuxSession() (string, error) {
	if !InTmux() {
		return "", nil
	}
	return Tmux("display-message", "-p", "#S")
}

// TmuxRole returns the role bound to the current tmux session, or "" when
// none is bound or the process is not in tmux.
func TmuxRole() (string, error) {
	if !InTmux() {
		return "", nil
	}
	out, err := Tmux("show-environment", RoleEnv)
	if err != nil {
		// tmux reports an unset variable as an error.
		if strings.Contains(err.Error(), "unknown variable") {
			return "", nil
		}
		return "", err
	}
	name, value, ok := strings.Cut(out, "=")
	if !ok || name != RoleEnv {
		return "", nil
	}
	return value, nil
}

// BindTmuxRole binds role to the current tmux session, so every agent and
// shell in it, including those already running, can find it.
func BindTmuxRole(role string) error {
	if !InTmux() {
		return errors.New("hive: not in a tmux session")
	}
	_, err := Tmux("set-environment", RoleEnv, role)
	return err
}

// ResolveRole finds the role a session plays: given when set, else
// HIVE_ROLE, else the tmux session's environment. It returns "" with no
// error when none is set.
func ResolveRole(given string) (string, RoleSource, error) {
	if given != "" {
		return given, SourceGiven, nil
	}
	if env := os.Getenv(RoleEnv); env != "" {
		return env, SourceEnv, nil
	}
	role, err := TmuxRole()
	if err != nil || role == "" {
		return "", "", err
	}
	return role, SourceTmux, nil
}

// Context is what a session needs when it starts or resumes playing a
// role: the role, the decisions that bind it, how to reach its peers, and
// its current report with the requests the operator has answered.
type Context struct {
	Role Role `json:"role"`
	// Decisions are the accepted decisions that bind the role: those that
	// name it and those that name no role.
	Decisions []Decision `json:"decisions"`
	// Peers are the role's peers and, when they have reported one, their
	// sessions.
	Peers []Peer `json:"peers"`
	// Report is the role's current report, if it has one.
	Report *Report `json:"report,omitempty"`
	// Answered are the IDs of the report's decision requests that the
	// decision log answers.
	Answered []string `json:"answered"`
}

// Peer is a peer role and how to reach its session.
type Peer struct {
	Role    string   `json:"role"`
	Session *Session `json:"session,omitempty"`
}

// NewContext builds the context for role from the hive and the current
// reports.
func NewContext(h *Hive, reports []Report, role string) (Context, error) {
	r, ok := h.Role(role)
	if !ok {
		return Context{}, fmt.Errorf("hive: role %q does not exist", role)
	}
	if r.Status == RoleRetired {
		return Context{}, fmt.Errorf("hive: role %q is retired", role)
	}
	c := Context{Role: r, Decisions: []Decision{}, Peers: []Peer{}, Answered: []string{}}
	decided := map[string]bool{}
	for _, d := range h.Decisions {
		decided[d.ID] = true
		if d.Status == DecisionAccepted && (len(d.Roles) == 0 || contains(d.Roles, role)) {
			c.Decisions = append(c.Decisions, d)
		}
	}
	sessions := map[string]*Session{}
	for i := range reports {
		rep := reports[i]
		sessions[rep.Role] = rep.Session
		if rep.Role == role {
			c.Report = &rep
		}
	}
	for _, p := range r.Peers {
		c.Peers = append(c.Peers, Peer{Role: p, Session: sessions[p]})
	}
	if c.Report != nil {
		for _, d := range c.Report.DecisionsNeeded {
			if decided[d.ID] {
				c.Answered = append(c.Answered, d.ID)
			}
		}
	}
	return c, nil
}
