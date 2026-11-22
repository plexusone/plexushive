package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Decide turns a decision request into the decision that answers it: the chosen
// option becomes the decision, the others its alternatives. reason, when given,
// is the first reason; the request's situation and evidence follow. The
// decision binds the requesting work identity and keeps the request's ID, refs
// and repositories, so the work identity finds its answer under the ID it asked
// with.
func (d DecisionRequest) Decide(identity, option, reason string) (Decision, error) {
	var chosen *Option
	for i := range d.Options {
		if d.Options[i].Name == option {
			chosen = &d.Options[i]
		}
	}
	if chosen == nil {
		names := make([]string, len(d.Options))
		for i, o := range d.Options {
			names[i] = o.Name
		}
		return Decision{}, fmt.Errorf("hive: decision request %s has no option %q (options: %s)", d.ID, option, strings.Join(names, ", "))
	}
	out := Decision{
		ID:           d.ID,
		Title:        fmt.Sprintf("%s Decided: %s", strings.TrimSpace(d.Title), chosen.Name),
		Status:       DecisionAccepted,
		Option:       chosen.Name,
		Decision:     fmt.Sprintf("%s Approved: %s", strings.TrimSpace(chosen.Description), strings.TrimSpace(d.Approval)),
		Refs:         d.Refs,
		Identities:   []string{identity},
		Repositories: d.Repositories,
	}
	if option != d.Recommendation {
		out.Decision = fmt.Sprintf("%s (%s recommended %s.)", out.Decision, identity, d.Recommendation)
	}
	if strings.TrimSpace(reason) != "" {
		out.Reasons = append(out.Reasons, strings.TrimSpace(reason))
	}
	out.Reasons = append(out.Reasons, d.Situation)
	out.Reasons = append(out.Reasons, d.Evidence...)
	for _, o := range d.Options {
		if o.Name != option {
			out.Alternatives = append(out.Alternatives, Alternative{
				Option:   fmt.Sprintf("%s: %s", o.Name, o.Description),
				Rejected: "Not chosen. Its cost and risk: " + o.CostAndRisk,
			})
		}
	}
	return out, out.Validate()
}

// WriteDecision adds d to the decision log under root as
// decisions/<id>.json. It refuses to replace an existing decision: the log
// is append-only, and a changed decision is a new one that supersedes it.
func WriteDecision(root string, d Decision) error {
	if err := d.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	path := filepath.Join(root, "decisions", d.ID+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is built from a validated ID
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("hive: decision %s already exists; supersede it with a new decision", d.ID)
	}
	if err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return errors.Join(err, f.Close())
}
