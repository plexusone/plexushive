package hive

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// A session finds the work identity it works under in three places, in
// order: a value the caller was given (such as an --identity flag), the
// HIVE_IDENTITY environment variable, and the environment of the tmux
// session it runs in. The last reaches agents started before the identity
// was bound: tmux set-environment changes the session's environment for new
// processes only, but the value can always be read back from tmux. The
// name HIVE_ROLE, from before roles became work identities, is read after
// HIVE_IDENTITY in both places.

// IdentitySource says where a work identity was found.
type IdentitySource string

const (
	// SourceGiven is a work identity the caller passed in, such as a flag.
	SourceGiven IdentitySource = "given"
	// SourceEnv is the HIVE_IDENTITY environment variable.
	SourceEnv IdentitySource = "environment"
	// SourceTmux is HIVE_IDENTITY in the tmux session's environment.
	SourceTmux IdentitySource = "tmux"
	// SourceLegacyEnv is the HIVE_ROLE environment variable.
	SourceLegacyEnv IdentitySource = "legacy environment"
	// SourceLegacyTmux is HIVE_ROLE in the tmux session's environment.
	SourceLegacyTmux IdentitySource = "legacy tmux"
)

// Legacy reports whether the work identity was found under the old name
// HIVE_ROLE.
func (s IdentitySource) Legacy() bool {
	return s == SourceLegacyEnv || s == SourceLegacyTmux
}

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

// TmuxIdentity returns the work identity bound to the current tmux
// session, or "" when none is bound or the process is not in tmux.
func TmuxIdentity() (string, error) {
	return tmuxEnv(IdentityEnv)
}

// tmuxEnv reads one variable from the tmux session's environment.
func tmuxEnv(name string) (string, error) {
	if !InTmux() {
		return "", nil
	}
	out, err := Tmux("show-environment", name)
	if err != nil {
		// tmux reports an unset variable as an error.
		if strings.Contains(err.Error(), "unknown variable") {
			return "", nil
		}
		return "", err
	}
	key, value, ok := strings.Cut(out, "=")
	if !ok || key != name {
		return "", nil
	}
	return value, nil
}

// BindTmuxIdentity binds a work identity to the current tmux session, so
// every agent and shell in it, including those already running, can find
// it.
func BindTmuxIdentity(identity string) error {
	if !InTmux() {
		return errors.New("hive: not in a tmux session")
	}
	_, err := Tmux("set-environment", IdentityEnv, identity)
	return err
}

// ResolveIdentity finds the work identity a session works under: given
// when set, else HIVE_IDENTITY in the environment or the tmux session,
// else HIVE_ROLE in either. It returns "" with no error when none is set.
func ResolveIdentity(given string) (string, IdentitySource, error) {
	if given != "" {
		return given, SourceGiven, nil
	}
	for _, name := range []string{IdentityEnv, LegacyIdentityEnv} {
		env, tmux := SourceEnv, SourceTmux
		if name == LegacyIdentityEnv {
			env, tmux = SourceLegacyEnv, SourceLegacyTmux
		}
		if v := os.Getenv(name); v != "" {
			return v, env, nil
		}
		v, err := tmuxEnv(name)
		if err != nil {
			return "", "", err
		}
		if v != "" {
			return v, tmux, nil
		}
	}
	return "", "", nil
}

// Context is what a session needs when it starts or resumes work under a
// work identity: the identity, the decisions that bind it, how to reach its
// peers, and its current report with the requests the operator has
// answered.
type Context struct {
	Identity WorkIdentity `json:"identity"`
	// Decisions are the accepted decisions that bind the identity: those
	// that name it and those that name no identity.
	Decisions []Decision `json:"decisions"`
	// Peers are the identity's peers and, when they have reported one,
	// their sessions.
	Peers []Peer `json:"peers"`
	// Report is the identity's current report, if it has one.
	Report *Report `json:"report,omitempty"`
	// Answers are the operator's answers to the report's requests, by
	// request ID, when they are recorded on open items rather than in the
	// decision log.
	Answers map[string]Answer `json:"answers,omitempty"`
	// Answered are the IDs of the report's decision requests the operator
	// has answered, on an open item or in the decision log.
	Answered []string `json:"answered"`
}

// Peer is a peer work identity and how to reach its session.
type Peer struct {
	Identity string   `json:"identity"`
	Session  *Session `json:"session,omitempty"`
}

// NewContext builds the context for a work identity from the hive and the
// current reports.
func NewContext(h *Hive, reports []Report, identity string) (Context, error) {
	r, ok := h.Identity(identity)
	if !ok {
		return Context{}, fmt.Errorf("hive: work identity %q does not exist", identity)
	}
	if r.Status == IdentityRetired {
		return Context{}, fmt.Errorf("hive: work identity %q is retired", identity)
	}
	c := Context{Identity: r, Decisions: []Decision{}, Peers: []Peer{}, Answered: []string{}}
	for _, d := range h.Decisions {
		if d.Status == DecisionAccepted && (len(d.Identities) == 0 || contains(d.Identities, identity)) {
			c.Decisions = append(c.Decisions, d)
		}
	}
	sessions := map[string]*Session{}
	for i := range reports {
		rep := reports[i]
		sessions[rep.Identity] = rep.Session
		if rep.Identity == identity {
			c.Report = &rep
		}
	}
	for _, p := range r.Peers {
		c.Peers = append(c.Peers, Peer{Identity: p, Session: sessions[p]})
	}
	if c.Report != nil {
		for _, d := range c.Report.DecisionsNeeded {
			if _, ok := h.Answered(identity, d.ID); ok {
				c.Answered = append(c.Answered, d.ID)
				if it, ok := h.Open.Item(identity + "/" + d.ID); ok && it.Answer != nil {
					if c.Answers == nil {
						c.Answers = map[string]Answer{}
					}
					c.Answers[d.ID] = *it.Answer
				}
			}
		}
	}
	return c, nil
}
