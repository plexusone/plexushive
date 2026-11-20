# Status Reports

Each role keeps one current status report. `plexushive brief` gathers every
report into one briefing, in the order the operator acts on it:

1. **Your actions**: pushes, merges, tags, deploys and setup steps for work
   that is ready, grouped by kind, with the exact command
2. **Decisions needed**: open decision requests, keyed `role/id`, with how
   long each has waited
3. **Answered**: requests the decision log already answers, which the role
   should drop
4. **Blocked**: work that cannot move, those waiting on the operator first
5. **In progress**: who is working in which repository and branch;
   repositories with more than one role are listed first and flagged
6. **Notices**: what one role told the others
7. **Status**: each role's summary, with the report's age or `idle`
8. Active roles that have not reported, and reports ignored because their
   role does not exist or is retired

```json
{
  "role": "billing",
  "generated": "2026-11-19T09:00:00Z",
  "session": {"tmux": "billing", "agent": "billing-1"},
  "summary": "Coupons are in review; the refund export waits on the website.",
  "operatorActions": [{
    "kind": "tag",
    "description": "Tag the coupon release.",
    "command": "git -C billing tag v1.4.0 && git -C billing push origin v1.4.0",
    "repository": "github.com/example/billing",
    "version": "v1.4.0",
    "checks": ["https://github.com/example/billing/actions/runs/1"],
    "irreversible": true
  }],
  "inProgress": [{
    "description": "Refund export link on the account page.",
    "repository": "github.com/example/web",
    "branch": "refund-export",
    "worktree": "web-refund"
  }],
  "done": [{"description": "Added coupon validation to checkout.", "branch": "coupons", "commits": ["9b1d7e2"], "state": "committed"}],
  "blocked": [{
    "description": "Refund export needs the account page link.",
    "waitingOn": "website",
    "unblocks": "The refund report for finance."
  }],
  "next": [{"description": "Release the coupon endpoint."}],
  "decisionsNeeded": [{
    "id": "coupon-stacking",
    "title": "Allow coupons to stack?",
    "situation": "Two launch partners want their coupons to combine.",
    "evidence": ["Both partners asked in writing."],
    "options": [
      {"name": "no-stacking", "description": "Keep one coupon per order.", "costAndRisk": "May lose one partner's launch traffic."},
      {"name": "stack-two", "description": "Allow at most two coupons.", "costAndRisk": "A day of work; discounts can reach 40%."}
    ],
    "recommendation": "no-stacking",
    "approval": "A pricing change."
  }],
  "notices": [{
    "title": "Date parsing bug in a shared library",
    "description": "Version 2.1 of the date library drops time zones; pin 2.0 until 2.2.",
    "modules": ["example.com/dates"]
  }]
}
```

| Field | Required | Meaning |
|---|---|---|
| `role` | yes | The reporting role; `report submit` fills it from `HIVE_ROLE` |
| `generated` | yes | When it was written (RFC 3339); `report submit` fills it with the current time |
| `session` | no | How to reach the session: `tmux` name and `agent` name |
| `summary` | no | One or two sentences for the briefing |
| `idle` | no | The role has nothing to do until it is given work |
| `operatorActions` | no | Steps only the operator takes; see below |
| `inProgress` | no | Work under way: `description`, `repository`, and optional `branch`, `worktree`, `base`, `refs` |
| `done`, `next` | no | Items; see below |
| `blocked` | no | Items plus `waitingOn`, and optional `decision` and `unblocks` |
| `decisionsNeeded` | no | Decision requests; see below |
| `notices` | no | `title`, `description`, and optional `repositories`, `modules`, `refs` |

## Items

An item has a `description` and optionally:

| Field | Meaning |
|---|---|
| `refs` | `INIT-<SLUG>-NNN` or `RMI-<SLUG>-NNN` |
| `repositories` | `github.com/<org>/<repo>` |
| `branch` | The branch the work is on |
| `commits` | Commit hashes |
| `pullRequests` | Pull request URLs |
| `state` | `committed`, `pushed`, `released` or `deployed` |
| `paths` | Files the work produced, relative to their repository |

A blocker adds `waitingOn` (a role or `operator`), `decision` (the ID of a
decision request in the same report whose answer unblocks it) and
`unblocks` (what else waits on it, including other roles' work).

## Operator actions and blockers

Use an operator action, not a blocker, when the work is ready and only
needs the operator's hands: a push, merge, tag, deploy, new repository,
account or secret, a test only the operator can run, or a review. Use a
blocker when something is genuinely stuck, on another role or on an answer
from the operator.

| Field | Meaning |
|---|---|
| `kind` | `push`, `merge`, `tag`, `deploy`, `create-repository`, `configure`, `test`, `review` or `other` (required) |
| `description` | What, in a sentence (required) |
| `command` | The command to run |
| `repository`, `branch`, `commit`, `pullRequest`, `version` | What it acts on |
| `checks` | Links to CI runs or other evidence it is ready |
| `irreversible` | It cannot be undone, such as a tag on a public Go module |
| `unblocks` | What waits on it |
| `refs` | Initiatives and roadmap items |

## Decision requests

A decision request needs an `id` (which becomes the decision's ID once
decided), a `title`, the `situation`, at least two `options`, each with a
`name`, `description` and `costAndRisk`, a `recommendation` naming one of
the options, and the `approval` the answer grants. `evidence`, `refs` and
`repositories` are optional; an option can be marked `irreversible`.
`asked` is when the request was first raised: `report submit` sets it and
keeps it across later reports that carry the same request.

The operator answers with `plexushive decide <role>/<id> <option>`, which
writes the decision to the log under the request's ID and prints the
message to send the role. The role reads the decision and drops the
request from its next report; until it does, the briefing lists the
request as answered.

## Where reports live

Reports describe the sessions on one machine and change often, so they are
kept out of the hive repository: in `$HIVE_REPORTS` when set, otherwise in
`plexushive/reports` under the platform's per-user data directory. Each
role's report is `<role>.json`, replaced atomically on each submit.

```bash
plexushive -C my-hive report submit report.json   # or pipe the JSON to standard input
plexushive -C my-hive brief                       # read the briefing
plexushive -C my-hive brief --json                # the same, for other programs
plexushive -C my-hive who github.com/example/web  # steward, work in progress, waiting actions
plexushive -C my-hive decide billing/coupon-stacking no-stacking
```

The name `operator` is reserved for `waitingOn`; no role may use it.

## Validation

`Report.Validate` checks one report; `ValidateReports` checks reports
against the roles:

- the role exists, is not retired, and has at most one report
- `generated` is set, and every item, action, work entry and notice has
  its required fields
- `waitingOn` names an existing role or `operator`, and a blocker's
  `decision` names a request in the same report
- action kinds and item states are known values
- refs are `INIT-<SLUG>-NNN` or `RMI-<SLUG>-NNN`, repositories are
  `github.com/<org>/<repo>`, commits are hexadecimal hashes and pull
  requests are GitHub pull request URLs
- decision request IDs are unique within the report, there are at least two
  complete options with distinct names, and the recommendation names one

## In Go

```go
dir, err := hive.ReportsDir()
err = hive.WriteReport(dir, report)

reports, err := hive.LoadReports(dir)
b := hive.NewBriefing(h.Roles, h.Decisions, reports)

d, err := request.Decide("billing", "no-stacking", "Partners can wait a release.")
err = hive.WriteDecision("my-hive", d)
```
