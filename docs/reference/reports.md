# Status Reports

Each work identity keeps one current status report. `plexushive brief` gathers every
report into one briefing, in the order the operator acts on it:

1. **Your actions**: pushes, merges, tags, deploys and setup steps for work
   that is ready, grouped by kind, with the exact command
2. **Decisions needed**: open decision requests, keyed `identity/id`, with how
   long each has waited
3. **Answered**: requests the decision log already answers, which the work identity
   should drop
4. **Blocked**: work that cannot move, those waiting on the operator first
5. **In progress**: who is working in which repository and branch;
   repositories with more than one work identity are listed first and flagged
6. **Notices**: what one work identity told the others
7. **Status**: each work identity's summary, with the report's age or `idle`
8. Active work identities that have not reported, and reports ignored because their
   work identity does not exist or is retired

```json
{
  "identity": "billing",
  "generated": "2026-11-19T09:00:00Z",
  "session": {"tmux": "billing", "agent": "billing-1"},
  "summary": "Coupons are ready to release; the refund export waits on the website.",
  "operatorActions": [
    {
      "id": "push-coupons",
      "kind": "push",
      "description": "Push the coupon release.",
      "command": "git -C billing push origin main",
      "repository": "github.com/example/billing",
      "branch": "main",
      "commits": ["9b1d7e2"]
    },
    {
      "id": "tag-coupons",
      "kind": "tag",
      "description": "Tag the coupon release.",
      "after": ["push-coupons"],
      "command": "git -C billing tag v1.4.0 9b1d7e2 && git -C billing push origin v1.4.0",
      "repository": "github.com/example/billing",
      "versions": ["v1.4.0"],
      "checks": ["https://github.com/example/billing/actions/runs/1"],
      "irreversible": true,
      "followUp": "Record the release in the changelog index."
    },
    {
      "kind": "configure",
      "description": "Set the payment provider's webhook secret in production.",
      "steps": ["Copy the signing secret from the provider's webhook page.", "Store it as /billing/prod/WEBHOOK_SECRET."],
      "credentials": ["billing/prod/WEBHOOK_SECRET"]
    }
  ],
  "inProgress": [{
    "description": "Refund export link on the account page.",
    "repository": "github.com/example/web",
    "branch": "refund-export",
    "worktree": "web-refund",
    "state": "committed"
  }],
  "done": [{"description": "Added coupon validation to checkout.", "branch": "main", "commits": ["9b1d7e2"], "state": "committed"}],
  "blocked": [{
    "description": "Refund export needs the account page link.",
    "waitingOn": ["website"],
    "unblocks": ["The refund report for finance."]
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
| `identity` | yes | The reporting work identity; `report submit` fills it from `HIVE_IDENTITY` |
| `generated` | yes | When it was written (RFC 3339); `report submit` fills it with the current time |
| `session` | no | How to reach the session: `tmux` name and `agent` name; inside tmux, `report submit` fills `tmux` |
| `summary` | no | One or two sentences for the briefing |
| `idle` | no | The work identity has nothing to do until it is given work |
| `operatorActions` | no | Steps only the operator takes; see below |
| `inProgress` | no | Work under way: `description`, `repository`, and optional `branch`, `worktree`, `base`, `state` (`uncommitted`, `committed` or `pushed`), `credentials`, `refs` |
| `done`, `next` | no | Items; see below |
| `blocked` | no | Items plus `waitingOn`, and optional `decision`, `credentials` and `unblocks` |
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
| `pullRequests` | Pull request URLs, or `#<n>` when the item names exactly one repository |
| `state` | `uncommitted`, `committed`, `pushed`, `released` or `deployed` |
| `versions` | The releases the work went out in: `v1.2.0` when the item names one repository, else `<module>@<version>` |
| `paths` | Files the work produced, relative to their repository |

Finished work that is not yet pushed can stay in `inProgress` with its
`state`, instead of being listed twice.

A blocker adds `waitingOn` (the work identities, or `operator`, that must act, in
order: a push that waits on another work identity's release and then on the
operator's yes lists both), `decision` (the ID of a decision request in the
same report whose answer unblocks it), `credentials` it waits on and
`unblocks` (what else waits on it, including other work identities' work).

## Operator actions and blockers

Use an operator action, not a blocker, when the work is ready and only
needs the operator's hands: a push, merge, tag, deploy, new repository,
account or secret, a test only the operator can run, or a review. Use a
blocker when something is genuinely stuck, on another work identity or on an answer
from the operator.

| Field | Meaning |
|---|---|
| `kind` | `push`, `merge`, `tag`, `deploy`, `create-repository`, `configure`, `test`, `review` or `other` (required) |
| `description` | What, in a sentence (required) |
| `id` | Names the action, so other actions in the report can wait on it; lowercase letters, digits, `-` and `.` |
| `command` | The command to run |
| `steps` | What to do by hand, in order, when one command will not do, such as setting a secret whose value must not appear in the report |
| `after` | IDs of actions in the same report that must be done first, such as the push a tag needs |
| `decision`, `option` | A decision request in the same report and the option the action applies under; the briefing marks it until the operator decides, then keeps or drops it |
| `repository`, `branch` | Where it acts |
| `commits`, `pullRequests`, `versions` | What it acts on; a nested module's tag adds its own version on the same commit |
| `paths` | Files the operator works with, relative to the repository |
| `credentials` | Credential registry IDs it creates, sets or needs; never values |
| `checks` | Links to CI runs or other evidence it is ready |
| `irreversible` | It cannot be undone, such as a tag on a public Go module |
| `ongoing` | It is kept up rather than done once, such as a local webhook listener while a work identity tests |
| `followUp` | What the work identity does once the operator has acted, such as opening a pull request |
| `unblocks` | What waits on it |
| `refs` | Initiatives and roadmap items |

The briefing lists a step once when several work identities ask for it, naming them
all: a push to the same repository and branch, or any action with the same
kind, repository, branch and command.

## Decision requests

A decision request needs an `id` (which becomes the decision's ID once
decided), a `title`, the `situation`, at least two `options`, each with a
`name`, `description` and `costAndRisk`, a `recommendation` naming one of
the options, and the `approval` the answer grants. `evidence`, `refs` and
`repositories` are optional; an option can be marked `irreversible`.
`asked` is when the request was first raised: `report submit` sets it and
keeps it across later reports that carry the same request, so leave it out.
A date alone is accepted.

The operator answers with `plexushive decide <identity>/<id> <option>`, which
records the answer on the request's [open item](open-items.md) and prints
the message to send the work identity. Most answers are point decisions:
the work identity acts on the answer, records the outcome where it belongs
(a commit, a changelog, an ADR), and drops the request from its next
report; until it does, the briefing lists the request as answered. With
`--record` the answer also becomes a decision in the log, for working rules
that bind the agents. A work identity whose only open work waits on its own requests
shows as `decision` in the briefing's status, neither idle nor blocked.

## Where reports live

Reports describe the sessions on one machine and change often, so they are
kept out of the hive repository: in `$HIVE_REPORTS` when set, otherwise in
`plexushive/reports` under the platform's per-user data directory. Each
work identity's report is `<identity>.json`, replaced atomically on each submit.
A report the reader cannot decode, such as one written by a newer version,
is listed in the briefing as unreadable rather than stopping it.

```bash
plexushive -C my-hive report submit report.json   # or pipe the JSON to standard input
plexushive -C my-hive brief                       # read the briefing
plexushive -C my-hive brief --json                # the same, for other programs
plexushive -C my-hive who github.com/example/web  # steward, work in progress, waiting actions
plexushive -C my-hive decide billing/coupon-stacking no-stacking
```

The name `operator` is reserved for `waitingOn`; no work identity may use it.

## Validation

`Report.Validate` checks one report; `ValidateReports` checks reports
against the work identities:

- the work identity exists, is not retired, and has at most one report
- `generated` is set, and every item, action, work entry and notice has
  its required fields
- `waitingOn` names an existing work identity or `operator`, and a blocker's
  `decision` names a request in the same report
- action kinds and item states are known values
- refs are `INIT-<SLUG>-NNN` or `RMI-<SLUG>-NNN`, repositories are
  `github.com/<org>/<repo>`, commits are hexadecimal hashes, pull
  requests are GitHub pull request URLs (`#<n>` is expanded when one
  repository is named) and credentials are registry IDs such as
  `app/prod/DB_URL`; the schema carries the same patterns
- action IDs are unique, `after` names actions in the report without a
  cycle, and an action's `decision` and `option` name a request in the
  report and one of its options
- decision request IDs are unique within the report, there are at least two
  complete options with distinct names, and the recommendation names one

## Earlier reports

Reports written for v0.3 still load: a single `commit`, `pullRequest` or
`version` on an action joins its list, and a single `waitingOn` or
`unblocks` string becomes a one-element list. Write the lists in new
reports.

## In Go

```go
dir, err := hive.ReportsDir()
err = hive.WriteReport(dir, report)

reports, unreadable, err := hive.ReadReports(dir)   // LoadReports fails on any unreadable report
b := hive.NewBriefing(h.Work identities, h.Decisions, reports)
b.SetUnreadable(unreadable)

d, err := request.Decide("billing", "no-stacking", "Partners can wait a release.")
err = hive.WriteDecision("my-hive", d)
```
