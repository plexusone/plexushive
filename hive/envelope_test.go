package hive

import "testing"

func TestFormatParseRoundTrip(t *testing.T) {
	envs := []Envelope{
		{Type: MessageRequest, From: "billing", Ref: "RMI-BILLING-012"},
		{Type: MessageStatus, From: "docs"},
		{Type: MessageDecisionNeeded, From: "coordinator", Ref: "INIT-HIVE-001"},
	}
	for _, want := range envs {
		msg, err := Format(want, "line one\nline two")
		if err != nil {
			t.Fatal(err)
		}
		got, body, ok, err := Parse(msg)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || got != want || body != "line one\nline two" {
			t.Fatalf("round trip of %+v: got %+v, %q, ok=%v", want, got, body, ok)
		}
	}
}

func TestParsePlainMessage(t *testing.T) {
	_, body, ok, err := Parse("just a note\nwith two lines")
	if err != nil || ok || body != "just a note\nwith two lines" {
		t.Fatalf("plain message: ok=%v body=%q err=%v", ok, body, err)
	}
}

func TestParseFieldOrderAndSpacing(t *testing.T) {
	env, body, ok, err := Parse("  [hive]  from=docs   type=question\nbody")
	if err != nil || !ok || env.Type != MessageQuestion || env.From != "docs" || body != "body" {
		t.Fatalf("got %+v %q ok=%v err=%v", env, body, ok, err)
	}
}

func TestParseRejects(t *testing.T) {
	bad := map[string]string{
		"unknown type":   "[hive] type=chat from=docs",
		"missing from":   "[hive] type=status",
		"bad from":       "[hive] type=status from=Docs",
		"bad ref":        "[hive] type=status from=docs ref=TASK-1",
		"unknown field":  "[hive] type=status from=docs to=billing",
		"repeated field": "[hive] type=status type=question from=docs",
		"not key=value":  "[hive] status from=docs",
	}
	for name, msg := range bad {
		t.Run(name, func(t *testing.T) {
			_, _, ok, err := Parse(msg)
			if !ok || err == nil {
				t.Fatalf("want an envelope error, got ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestFormatRejectsInvalid(t *testing.T) {
	if _, err := Format(Envelope{Type: "chat", From: "docs"}, "x"); err == nil {
		t.Fatal("want error for an unknown type")
	}
}
