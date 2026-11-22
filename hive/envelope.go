package hive

import (
	"fmt"
	"regexp"
	"strings"
)

// MessageType is what a message between agents asks of its recipient.
type MessageType string

const (
	// MessageQuestion asks for information; nothing changes.
	MessageQuestion MessageType = "question"
	// MessageRequest asks the recipient to change something it owns.
	MessageRequest MessageType = "request"
	// MessageStatus reports progress; no reply is expected.
	MessageStatus MessageType = "status"
	// MessageDecisionNeeded asks the operator to decide. It goes to the
	// decision queue, not to another agent.
	MessageDecisionNeeded MessageType = "decision-needed"
)

// EnvelopePrefix starts the first line of every hive message.
const EnvelopePrefix = "[hive]"

// Envelope is the header agents put on the first line of a message to
// another agent, so the recipient, and later the message record, can tell
// what kind of message it is, who sent it and what it is about.
//
//	[hive] type=request from=billing ref=RMI-BILLING-012
//	Please add a coupon endpoint to the payments API…
type Envelope struct {
	Type MessageType
	// From is the sender's work identity name.
	From string
	// Ref is an initiative or roadmap item ID, when there is one.
	Ref string
}

// Validate checks the envelope's fields.
func (e Envelope) Validate() error {
	switch e.Type {
	case MessageQuestion, MessageRequest, MessageStatus, MessageDecisionNeeded:
	default:
		return fmt.Errorf("hive: envelope type %q must be question, request, status or decision-needed", e.Type)
	}
	if !namePattern.MatchString(e.From) {
		return fmt.Errorf("hive: envelope from %q must be a work identity name", e.From)
	}
	if e.Ref != "" && !refPattern.MatchString(e.Ref) {
		return fmt.Errorf("hive: envelope ref %q must be INIT-<SLUG>-NNN or RMI-<SLUG>-NNN", e.Ref)
	}
	return nil
}

// String renders the envelope as a message's first line.
func (e Envelope) String() string {
	s := fmt.Sprintf("%s type=%s from=%s", EnvelopePrefix, e.Type, e.From)
	if e.Ref != "" {
		s += " ref=" + e.Ref
	}
	return s
}

// Format renders a whole message: the envelope line, then the body.
func Format(e Envelope, body string) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	return e.String() + "\n" + body, nil
}

var fieldPattern = regexp.MustCompile(`^([a-z]+)=(\S+)$`)

// Parse splits a message into its envelope and body. It reports ok=false,
// with no error, when the message has no envelope, so plain messages pass
// through unchanged.
func Parse(msg string) (env Envelope, body string, ok bool, err error) {
	first, rest, _ := strings.Cut(msg, "\n")
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, EnvelopePrefix) {
		return Envelope{}, msg, false, nil
	}
	seen := map[string]bool{}
	for _, f := range strings.Fields(strings.TrimPrefix(first, EnvelopePrefix)) {
		m := fieldPattern.FindStringSubmatch(f)
		if m == nil {
			return Envelope{}, "", true, fmt.Errorf("hive: envelope field %q must be key=value", f)
		}
		if seen[m[1]] {
			return Envelope{}, "", true, fmt.Errorf("hive: envelope field %q repeated", m[1])
		}
		seen[m[1]] = true
		switch m[1] {
		case "type":
			env.Type = MessageType(m[2])
		case "from":
			env.From = m[2]
		case "ref":
			env.Ref = m[2]
		default:
			return Envelope{}, "", true, fmt.Errorf("hive: unknown envelope field %q", m[1])
		}
	}
	if err := env.Validate(); err != nil {
		return Envelope{}, "", true, err
	}
	return env, rest, true, nil
}
