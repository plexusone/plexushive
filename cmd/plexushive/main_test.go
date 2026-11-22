package main

import (
	"bytes"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

const example = "../../hive/testdata"

func TestValidate(t *testing.T) {
	out, err := run(t, "-C", example, "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "3 work identities and 2 decisions are valid") {
		t.Fatalf("output: %q", out)
	}
	if _, err := run(t, "-C", t.TempDir(), "validate"); err == nil {
		t.Fatal("want error for an empty directory")
	}
}

func TestCount(t *testing.T) {
	if got := count(1, "work identity", "work identities"); got != "1 work identity" {
		t.Fatal(got)
	}
	if got := count(0, "decision", "decisions"); got != "0 decisions" {
		t.Fatal(got)
	}
}

func TestIdentities(t *testing.T) {
	tests := []struct{ repo, want string }{
		{"github.com/example/web", "steward      website"},
		{"github.com/example-docs/guide", "organization"},
		{"github.com/other/x", "common ground"},
	}
	for _, tt := range tests {
		out, err := run(t, "-C", example, "identities", tt.repo)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, tt.want) {
			t.Fatalf("work identities %s: %q does not contain %q", tt.repo, out, tt.want)
		}
	}
	if _, err := run(t, "-C", example, "identities", "example/web"); err == nil {
		t.Fatal("want error for a malformed repository")
	}
	if _, err := run(t, "-C", t.TempDir(), "identities", "github.com/example/web"); err == nil {
		t.Fatal("want error for an empty directory")
	}
}

func TestSchema(t *testing.T) {
	out, err := run(t, "schema", "identity")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"title": "Agent work identity"`) {
		t.Fatalf("output: %.200q", out)
	}
	if _, err := run(t, "schema", "agent"); err == nil {
		t.Fatal("want error for an unknown schema")
	}
}
