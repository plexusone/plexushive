package hive

import (
	"encoding/json"
	"fmt"
	"time"
)

// Reports written before lists replaced single values still load: a single
// commit, pull request or version joins its list, and a single unblocks or
// waitingOn string becomes a one-element list. Only reading accepts the old
// shapes; the schema and every report written describe the lists.

// UnmarshalJSON reads an action, accepting the single-value fields of
// earlier reports.
func (a *Action) UnmarshalJSON(data []byte) error {
	type plain Action
	v := struct {
		*plain
		Commit      string          `json:"commit"`
		PullRequest string          `json:"pullRequest"`
		Version     string          `json:"version"`
		Unblocks    json.RawMessage `json:"unblocks"`
	}{plain: (*plain)(a)}
	if err := decodeStrict(data, &v); err != nil {
		return err
	}
	a.Commits = appendSet(a.Commits, v.Commit)
	a.PullRequests = appendSet(a.PullRequests, v.PullRequest)
	a.Versions = appendSet(a.Versions, v.Version)
	unblocks, err := stringOrList("unblocks", v.Unblocks)
	a.Unblocks = unblocks
	return err
}

// UnmarshalJSON reads a blocker, accepting a single waitingOn or unblocks
// string.
func (b *Blocker) UnmarshalJSON(data []byte) error {
	type plain Blocker
	v := struct {
		*plain
		WaitingOn json.RawMessage `json:"waitingOn"`
		Unblocks  json.RawMessage `json:"unblocks"`
	}{plain: (*plain)(b)}
	if err := decodeStrict(data, &v); err != nil {
		return err
	}
	waiting, err := stringOrList("waitingOn", v.WaitingOn)
	if err != nil {
		return err
	}
	unblocks, err := stringOrList("unblocks", v.Unblocks)
	b.WaitingOn, b.Unblocks = waiting, unblocks
	return err
}

// UnmarshalJSON reads a decision request, accepting a date alone for
// asked.
func (d *DecisionRequest) UnmarshalJSON(data []byte) error {
	type plain DecisionRequest
	v := struct {
		*plain
		Asked string `json:"asked"`
	}{plain: (*plain)(d)}
	if err := decodeStrict(data, &v); err != nil {
		return err
	}
	if v.Asked == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.DateOnly} {
		if t, err := time.Parse(layout, v.Asked); err == nil {
			d.Asked = t
			return nil
		}
	}
	return fmt.Errorf("asked %q must be a date or an RFC 3339 time", v.Asked)
}

// stringOrList reads raw as a list of strings, or as one string.
func stringOrList(field string, raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, fmt.Errorf("%s must be a list of strings", field)
	}
	if one == "" {
		return nil, nil
	}
	return []string{one}, nil
}

// appendSet adds s to list unless it is empty or already there.
func appendSet(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
