# User Guide

PlexusHive keeps a written record of what each long-lived coding agent is
for, where it works, and why things were decided, so the agents can
coordinate without the operator relaying every message. This guide sets up
a hive directory, writes roles and decisions, and wires validation into
tests and agent sessions.

## Concepts

| Term | Meaning |
|---|---|
| **Role** | What one agent is for: its mission, autonomy, the repositories it works in and its peers. Durable; outlives any session. |
| **Session** | The running agent (for example a terminal session) that plays a role. Renamed and restarted freely; no record names it. |
| **Steward** | The one role that settles conflicting and breaking changes in a repository. Others may still work there. |
| **Decision** | One entry in the append-only log: what was decided, why, and what was rejected. |
| **Report** | One role's current status: done, blocked, next, and decisions it needs. Kept on the machine, not in git. |
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
├── roles/
│   ├── billing.json
│   └── website.json
└── decisions/
    └── stewards-not-owners.json
```

Each file holds one record and is named after it: `roles/billing.json`
holds the role named `billing`. One record per file means agents writing at
the same time never edit the same file.

## 3. Write a role

```json
{
  "name": "billing",
  "status": "draft",
  "mission": "Own the billing service and its payment integrations.",
  "autonomy": "prepare",
  "repositories": [
    {"pattern": "github.com/example/billing", "affinity": "steward"},
    {"pattern": "github.com/example/web", "affinity": "contributor"}
  ],
  "peers": ["website"]
}
```

- Start a role as `draft`; set it to `active` once you confirm it.
- Choose the lowest `autonomy` that lets the agent do its job:
  - `observe` reads, analyzes and reports, and changes nothing.
  - `prepare` also makes local changes for you to publish: branches,
    commits, release notes. It never pushes, tags, merges, deploys or sends.
  - `execute` also makes bounded, low-risk changes itself.
- A `pattern` is one repository, `github.com/<org>/<repo>`, or a whole
  organization, `github.com/<org>/*`. At most one active role stewards a
  pattern, and an exact repository outranks an organization pattern.
- A repository no role lists is common ground.

See which roles work in a repository:

```bash
$ plexushive -C my-hive roles github.com/example/web
steward      website              repository
contributor  billing              repository
```

## 4. Bind a session to its role

A session declares the role it plays with the `HIVE_ROLE` environment
variable. Keep the binding out of the records, because sessions change:

```bash
HIVE_ROLE=billing claude --resume <session>
```

In tmux, set it on the session so every pane and restart inherits it:

```bash
tmux set-environment -t <session> HIVE_ROLE billing
```

A session hook can then read `HIVE_ROLE`, load the role with
`hive.Load(dir)` and `(*Hive).Role(name)`, and give the agent its mission,
autonomy and repositories at the start of each session.

## 5. Record a decision

Write a decision when a choice will constrain later work, especially
another agent's.

```json
{
  "id": "stewards-not-owners",
  "title": "Repositories have stewards, not owners",
  "status": "accepted",
  "decision": "Several roles may change a repository; at most one stewards it.",
  "reasons": ["Work crosses repositories, and one role per repository does not scale."],
  "alternatives": [
    {"option": "One owning role per repository", "rejected": "Forces every cross-repository change through a message."}
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
2 roles and 1 decision are valid
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
that name existing roles, and supersede links that agree.

Tools that cannot import Go can validate against the JSON Schemas:

```bash
plexushive schema role > role.schema.json
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

Have each agent file a report when asked and at the end of a work block.
It writes the JSON and submits it; the role and time are filled in:

```bash
plexushive -C my-hive report submit - <<'JSON'
{
  "summary": "Coupons are in review.",
  "done": [{"description": "Added coupon validation to checkout."}],
  "blocked": [{"description": "Publishing needs a push.", "waitingOn": "operator"}],
  "next": [{"description": "Release the coupon endpoint."}]
}
JSON
```

Raise a question only the operator can settle as a decision request in
`decisionsNeeded`, with the situation, at least two options with their
cost and risk, a recommendation and the approval it needs. Once you
decide, record a decision with the request's ID (step 5).

Start the day with:

```bash
plexushive -C my-hive brief
```

It lists the decisions waiting on you, then what is blocked (on you
first), then each role's summary with the report's age, then active roles
that have not reported. See [Status Reports](../reference/reports.md) for
every field.

## Library reference

| Function | Purpose |
|---|---|
| `hive.Load(root)` | Read `roles/` and `decisions/` under root |
| `(*Hive).Validate()` | Check all records together |
| `(*Hive).Role(name)` | Look up a role |
| `hive.RolesFor(roles, repo)` | Roles working in a repository, steward first |
| `hive.Steward(roles, repo)` | The steward's name, or `""` for common ground |
| `hive.WriteReport`, `hive.LoadReports`, `hive.ReportsDir` | Store and read status reports |
| `hive.NewBriefing` | Gather reports into decisions, blockers, status and missing roles |
| `hive.Format`, `hive.Parse` | Write and read message envelopes |
| `hive.RoleSchema`, `hive.DecisionSchema`, `hive.ReportSchema` | Generate the JSON Schemas |

Full API documentation: [pkg.go.dev](https://pkg.go.dev/github.com/plexusone/plexushive/hive).
