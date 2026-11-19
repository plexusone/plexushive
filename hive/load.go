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
	RolesDir     = "roles"
	DecisionsDir = "decisions"
)

// Hive is the contents of a hive directory.
type Hive struct {
	Roles     []Role
	Decisions []Decision
}

// Load reads the roles and decisions under root. It checks the files'
// syntax and names; call Validate to check their content.
func Load(root string) (*Hive, error) {
	roles, err := LoadRoles(root)
	if err != nil {
		return nil, err
	}
	decisions, err := LoadDecisions(root)
	if err != nil {
		return nil, err
	}
	return &Hive{Roles: roles, Decisions: decisions}, nil
}

// Validate checks the roles and decisions together.
func (h *Hive) Validate() error {
	return errors.Join(ValidateRoles(h.Roles), ValidateDecisions(h.Decisions, h.Roles))
}

// Role returns the role named name, or false when there is none.
func (h *Hive) Role(name string) (Role, bool) {
	for _, r := range h.Roles {
		if r.Name == name {
			return r, true
		}
	}
	return Role{}, false
}

// LoadRoles reads every roles/*.json under root. A file must be named after
// the role it holds.
func LoadRoles(root string) ([]Role, error) {
	var out []Role
	err := eachJSON(filepath.Join(root, RolesDir), func(stem string, data []byte) error {
		var r Role
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
