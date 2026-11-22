package hive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Directories under the hive root.
const (
	IdentitiesDir = "identities"
	// LegacyIdentitiesDir is IdentitiesDir's name before roles became work
	// identities; it is read when IdentitiesDir does not exist.
	LegacyIdentitiesDir = "roles"
	DecisionsDir        = "decisions"
)

// Hive is the contents of a hive directory.
type Hive struct {
	Identities []WorkIdentity
	Decisions  []Decision
}

// Load reads the work identities and decisions under root. It checks the files'
// syntax and names; call Validate to check their content.
func Load(root string) (*Hive, error) {
	identities, err := LoadIdentities(root)
	if err != nil {
		return nil, err
	}
	decisions, err := LoadDecisions(root)
	if err != nil {
		return nil, err
	}
	return &Hive{Identities: identities, Decisions: decisions}, nil
}

// Validate checks the work identities and decisions together.
func (h *Hive) Validate() error {
	return errors.Join(ValidateIdentities(h.Identities), ValidateDecisions(h.Decisions, h.Identities))
}

// Identity returns the work identity named name, or false when there is none.
func (h *Hive) Identity(name string) (WorkIdentity, bool) {
	for _, r := range h.Identities {
		if r.Name == name {
			return r, true
		}
	}
	return WorkIdentity{}, false
}

// LoadIdentities reads every identities/*.json under root, or
// roles/*.json when root has no identities directory but one written before
// v0.5.0. A file must be named after the work identity it holds; a root
// with both directories is refused, since one of them would be ignored.
func LoadIdentities(root string) ([]WorkIdentity, error) {
	dir, err := identitiesDir(root)
	if err != nil {
		return nil, err
	}
	var out []WorkIdentity
	err = eachJSON(dir, func(stem string, data []byte) error {
		var r WorkIdentity
		if err := decodeStrict(data, &r); err != nil {
			return err
		}
		if r.Name != stem {
			return fmt.Errorf("name %q does not match the file name", r.Name)
		}
		out = append(out, r)
		return nil
	})
	return out, err
}

// identitiesDir returns the directory LoadIdentities reads.
func identitiesDir(root string) (string, error) {
	current := filepath.Join(root, IdentitiesDir)
	legacy := filepath.Join(root, LegacyIdentitiesDir)
	_, errCurrent := os.Stat(current)
	_, errLegacy := os.Stat(legacy)
	switch {
	case errCurrent == nil && errLegacy == nil:
		return "", fmt.Errorf("hive: %s and %s both exist; move every record into %s", current, legacy, IdentitiesDir)
	case errCurrent != nil && errLegacy == nil:
		return legacy, nil
	}
	return current, nil
}

// LoadDecisions reads every decisions/*.json under root. A file must be
// named after the decision it holds.
func LoadDecisions(root string) ([]Decision, error) {
	var out []Decision
	err := eachJSON(filepath.Join(root, DecisionsDir), func(stem string, data []byte) error {
		var d Decision
		if err := decodeStrict(data, &d); err != nil {
			return err
		}
		if d.ID != stem {
			return fmt.Errorf("id %q does not match the file name", d.ID)
		}
		out = append(out, d)
		return nil
	})
	return out, err
}

// eachJSON calls fn for each .json file in dir, in name order.
func eachJSON(dir string, fn func(stem string, data []byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("hive: %w", err)
		}
		if err := fn(strings.TrimSuffix(name, ".json"), data); err != nil {
			return fmt.Errorf("hive: %s: %w", path, err)
		}
	}
	return nil
}

// decodeStrict rejects unknown fields, so a misspelled field fails instead
// of being dropped.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data after the JSON value")
	}
	return nil
}
