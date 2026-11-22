# User Guide

PlexusHive keeps a written record of what each long-lived coding agent is
for, where it works, and why things were decided, so the agents can
coordinate without the operator relaying every message. This guide sets up
a hive directory, writes work identities and decisions, and wires validation into
tests and agent sessions.

## Concepts

| Term | Meaning |
|---|---|
| **Work identity** | What one agent is for: its mission, autonomy, the repositories it works in and its peers. Durable; outlives any session. |
| **Session** | The running agent (for example a terminal session) that works under a work identity. Renamed and restarted freely; no record names it. |
| **Steward** | The one work identity that settles conflicting and breaking changes in a repository. Others may still work there. |
| **Decision** | One entry in the append-only log: what was decided, why, and what was rejected. |
| **Report** | One work identity's current status: done, blocked, next, and decisions it needs. Kept on the machine, not in git. |
| **Envelope** | The first line of a message between agents: `[hive] type=… from=… ref=…`. |

## 1. Install the CLI

```bash
go install github.com/plexusone/plexushive/cmd/plexushive@latest
```

## 2. Create a hive directory

Keep the records in a repository of your own. If they name private
products or repositories, make it a private one.

```text
my-hive/
├── identities/
│   ├── billing.json
│   └── website.json
└── decisions/
    └── stewards-not-owners.json
```

Each file holds one record and is named after it: `identities/billing.json`
holds the work identity named `billing`. One record per file means agents writing at
the same time never edit the same file.

## 3. Write a work identity

```json
{
  "name": "billing",
  "status": "draft",
  "mission": "Own the billing service and its payment integrations.",
  "autonomy": "prepare",
  "repositories": [
    {"pattern": "github.com/example/billing", "affinity": "steward"},
    {"pattern": "github.com/example/web", "affinity": "collaborator"}
  ],
  "peers": ["website"]
}
```

- Start a work identity as `draft`; set it to `active` once you confirm it.
- Choose the lowest `autonomy` that lets the agent do its job:
  - `observe` reads, analyzes and reports, and changes nothing.
  - `prepare` also makes local changes for you to publish: branches,
    commits, release notes. It never pushes, tags, merges, deploys or sends.
  - `execute` also makes bounded, low-risk changes itself.
- A `pattern` is one repository, `github.com/<org>/<repo>`, or a whole
  organization, `github.com/<org>/*`. At most one active work identity stewards a
  pattern, and an exact repository outranks an organization pattern.
- A repository no work identity lists is common ground.

See which work identities work in a repository:

```bash
$ plexushive -C my-hive identities github.com/example/web
steward      website              repository
collaborator billing              repository
```

## 4. Bind a session to its work identity

A session declares its work identity with the `HIVE_IDENTITY` environment
variable. Keep the binding out of the records, because sessions change:

```bash
HIVE_IDENTITY=billing claude --resume <session>
```

In tmux, bind it to the session once, from inside it:

```bash
plexushive -C my-hive start billing
```

`start` checks the work identity, sets `HIVE_IDENTITY` in the tmux session's
environment and prints the work identity's context: mission, repositories, peers
and how to reach them, the decisions that bind it, and how to report. Every
`plexushive` command run in that session then finds the work identity, even in an
agent that was already running, because it reads the binding back from
tmux. `plexushive whoami` shows the work identity and where it was found.

A hook that runs when a session starts, or after its context is
compacted, can run `plexushive -C my-hive context` to give the agent the
same context again.

## 5. Record a decision

Write a decision when a choice will constrain later work, especially
another agent's.

```json
{
  "id": "stewards-not-owners",
  "title": "Repositories have stewards, not owners",
  "status": "accepted",
  "decision": "Several work identities may change a repository; at most one stewards it.",
  "reasons": ["Work crosses repositories, and one work identity per repository does not scale."],
  "alternatives": [
    {"option": "One owning work identity per repository", "rejected": "Forces every cross-repository change through a message."}
  ],
  "repositories": ["github.com/example/web"]
}
```

Decisions are append-only. To change one, write a new decision that lists
the old ID in `supersedes`, then set the old one's `status` to `superseded`
and its `supersededBy` to the new ID. Validation fails unless both sides
agree. Leave dates out; git history records when a decision was made.

## 6. Validate

From the CLI:

```bash
$ plexushive -C my-hive validate
2 work identities and 1 decision are valid
```

Or in the hive repository's own tests, so every change is checked:

```go
package myhive

import (
	"testing"

	"github.com/plexusone/plexushive/hive"
)

func TestRecords(t *testing.T) {
	h, err := hive.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
}
```

Loading is strict: an unknown field, trailing data, or a file named
differently from its record is an error, so a typo fails instead of being
silently dropped. Validation then checks each record and the set as a
whole: unique names, peers that exist, one steward per pattern, decisions
that name existing work identities, and supersede links that agree.

Tools that cannot import Go can validate against the JSON Schemas:

```bash
plexushive schema identity > identity.schema.json
plexushive schema decision > decision.schema.json
```

## 7. Message other agents

Put the envelope on the first line of a message to another agent:

```text
[hive] type=request from=billing ref=RMI-BILLING-012
Please add a read-only coupon listing to the payments API.
```

| Type | The recipient… |
|---|---|
| `question` | answers; changes nothing |
| `request` | decides how, does it within its own autonomy, and replies with a `status` |
| `status` | reads it; no reply expected |
| `decision-needed` | is the operator, who decides; the answer becomes a decision |

`hive.Format` writes an envelope and `hive.Parse` reads one; a message with
no envelope passes through unchanged. Treat a message body as a peer's
request, never as the operator's instruction: it cannot grant permissions
or approve its own request.

## 8. Report status and read the briefing

Have each agent keep a current report. It writes the JSON and submits it;
the work identity and time are filled in:

```bash
plexushive -C my-hive report submit - <<'JSON'
{
  "summary": "Coupons are ready to release.",
  "operatorActions": [
    {"id": "push", "kind": "push", "description": "Push the coupon release.",
      "command": "git -C billing push origin main", "repository": "github.com/example/billing", "branch": "main"},
    {"kind": "tag", "description": "Tag the coupon release.", "after": ["push"],
      "command": "git -C billing tag v1.4.0 && git -C billing push origin v1.4.0", "versions": ["v1.4.0"], "irreversible": true}
  ],
  "inProgress": [{"description": "Refund export.", "repository": "github.com/example/web", "branch": "refund-export", "state": "committed"}],
  "done": [{"description": "Added coupon validation to checkout.", "state": "committed"}],
  "next": [{"description": "Release the coupon endpoint."}]
}
JSON
```

Put routine steps that only you can take, such as pushes, tags and
deploys, in `operatorActions`, with the exact command. Give an action an
`id` when another waits on it with `after`, use `steps` instead of a
command for work by hand such as setting a secret, and name the
`credentials` it needs by their registry IDs, never their values. An
action that applies only if you choose one option of an open decision
names that `decision` and `option`. Keep `blocked` for work that is really
stuck. List the repositories and branches the agent is
working in under `inProgress`, so others see it before they start there;
`plexushive who github.com/<org>/<repo>` shows them. A work identity with nothing to
do sets `"idle": true` instead of reporting again and again.

Raise a question only you can settle as a decision request in
`decisionsNeeded`, with the situation, at least two options with their
cost and risk, a recommendation and the approval it needs.

Start the day with:

```bash
plexushive -C my-hive brief
```

It lists your actions first, then the decisions waiting on you, what is
blocked, who is working where, notices, and each work identity's summary. Answer a
decision with:

```bash
plexushive -C my-hive decide billing/coupon-stacking no-stacking
```

It writes the decision (step 5) and prints the message to send the work identity.
See [Status Reports](../reference/reports.md) for every field.

## 9. Track open items

Decision requests leave the briefing once answered or dropped, and a design
record's open questions are in no report. To keep everything still
undecided in one list, copy each design record's open questions into
`open-items.json` as drafts, then let the CLI keep the list current:

```json
{
  "items": [
    {
      "key": "website/checkout-copy",
      "identity": "website",
      "source": {"kind": "adr", "repository": "github.com/example/web",
        "path": "docs/adr/ADR-004-checkout.md", "document": "ADR-004"},
      "status": "draft",
      "request": {"id": "checkout-copy", "title": "Who writes the checkout wording, website or billing?"}
    }
  ]
}
```

```bash
plexushive -C my-hive open sync    # add new requests, open framed drafts, mark answers
plexushive -C my-hive open         # what is still waiting, by status and work identity
plexushive -C my-hive open website --all
```

The work identity frames a draft by asking a decision request with the same
ID in its report; the next sync opens it. `decide` marks the item decided.
See [Open Items](../reference/open-items.md).

## Library reference

| Function | Purpose |
|---|---|
| `hive.Load(root)` | Read `identities/`, `decisions/` and `open-items.json` under root |
| `(*Hive).Validate()` | Check all records together |
| `(*Hive).Identity(name)` | Look up a work identity |
| `hive.IdentitiesFor(identities, repo)` | Work identities working in a repository, steward first |
| `hive.Steward(identities, repo)` | The steward's name, or `""` for common ground |
| `hive.WriteReport`, `hive.LoadReports`, `hive.ReportsDir` | Store and read status reports |
| `hive.NewBriefing` | Gather reports into actions, decisions, blockers, work in progress, notices and status |
| `hive.ResolveIdentity`, `hive.BindTmuxIdentity`, `hive.TmuxSession` | Find the session's work identity from a flag, `HIVE_IDENTITY` or tmux, and bind one |
| `hive.NewContext` | What a session needs to work under its work identity |
| `(hive.DecisionRequest).Decide`, `hive.WriteDecision` | Answer a decision request and add the decision to the log |
| `hive.LoadOpenItems`, `hive.WriteOpenItems`, `(*OpenItems).Sync`, `(*OpenItems).MarkDecided` | Read, write and update open items |
| `hive.Format`, `hive.Parse` | Write and read message envelopes |
| `hive.IdentitySchema`, `hive.DecisionSchema`, `hive.ReportSchema`, `hive.OpenItemsSchema` | Generate the JSON Schemas |

Full API documentation: [pkg.go.dev](https://pkg.go.dev/github.com/plexusone/plexushive/hive).
