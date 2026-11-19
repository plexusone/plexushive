# Status Reports

Each role keeps one current status report: what it finished, what is
blocked and on whom, what it does next, and the decisions it needs from the
operator. `plexushive brief` gathers every report into one briefing:
decisions first, then blockers (those waiting on the operator first), then
each role's status, then active roles that have not reported.

```json
{
  "role": "billing",
  "generated": "2026-11-19T09:00:00Z",
  "summary": "Coupons are in review; the refund export waits on the website.",
  "done": [{"description": "Added coupon validation to checkout.", "refs": ["RMI-BILLING-012"]}],
  "blocked": [{"description": "Refund export needs the account page link.", "waitingOn": "website"}],
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
  }]
}
```

| Field | Required | Meaning |
|---|---|---|
| `role` | yes | The reporting role; `report submit` fills it from `HIVE_ROLE` |
| `generated` | yes | When it was written (RFC 3339); `report submit` fills it with the current time |
| `summary` | no | One or two sentences for the briefing |
| `done`, `next` | no | Items: `description`, and optional `refs` and `repositories` |
| `blocked` | no | Items plus `waitingOn`: the role that can unblock it, or `operator` |
| `decisionsNeeded` | no | Decision requests; see below |

A decision request needs an `id` (which becomes the decision's ID once
decided), a `title`, the `situation`, at least two `options`, each with a
`name`, `description` and `costAndRisk`, a `recommendation` naming one of
the options, and the `approval` the answer grants. `evidence`, `refs` and
`repositories` are optional. When the operator decides, record the outcome
as a decision with the same ID.

Reports describe the sessions on one machine and change often, so they are
kept out of the hive repository: in `$HIVE_REPORTS` when set, otherwise in
`plexushive/reports` under the platform's per-user data directory. Each
role's report is `<role>.json`, replaced atomically on each submit.

```bash
plexushive -C my-hive report submit report.json   # or pipe the JSON to standard input
plexushive -C my-hive brief                       # read the briefing
plexushive -C my-hive brief --json                # the same, for other programs
```

The name `operator` is reserved for `waitingOn`; no role may use it.

## Validation

`Report.Validate` checks one report; `ValidateReports` checks reports
against the roles:

- the role exists, is not retired, and has at most one report
- `generated` is set, and every item has a description
- `waitingOn` names an existing role or `operator`
- refs are `INIT-<SLUG>-NNN` or `RMI-<SLUG>-NNN`, and repositories are `github.com/<org>/<repo>`
- decision request IDs are unique within the report, there are at least two
  complete options with distinct names, and the recommendation names one

## In Go

```go
dir, err := hive.ReportsDir()
err = hive.WriteReport(dir, report)

reports, err := hive.LoadReports(dir)
b := hive.NewBriefing(h.Roles, reports) // Decisions, Blocked, Reports, Missing
```
