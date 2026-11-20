# CLI

```text
plexushive [command] [-C dir]
```

| Flag | Default | Meaning |
|---|---|---|
| `-C`, `--dir` | `.` | Hive directory holding `roles/` and `decisions/` |
| `--version` | | Print the version the binary was built from |

## validate

Loads every role and decision and checks them together. Exits non-zero and
lists every problem when a record is invalid.

```bash
$ plexushive -C my-hive validate
2 roles and 1 decision are valid
```

## roles

Lists the roles, not retired, that work in a repository: the steward first,
then contributors by name. The last column says whether the role names the
repository itself or its organization.

```bash
$ plexushive -C my-hive roles github.com/example/web
steward      website              repository
contributor  billing              repository
```

A repository no role lists is reported as common ground.

## report submit

Validates a report and stores it as the role's current report, replacing
the previous one atomically. Reads a file, or standard input when no file
or `-` is given. An empty `role` is taken from `HIVE_ROLE`, and an empty
`generated` is set to the current time. The role must exist in the hive
directory.

```bash
$ HIVE_ROLE=billing plexushive -C my-hive report submit report.json
stored the billing report
```

| Flag | Default | Meaning |
|---|---|---|
| `--reports` | `$HIVE_REPORTS`, else the per-user data directory | Where reports are stored |

## brief

Gathers every role's report into one briefing.

```text
$ plexushive -C my-hive brief
Your actions (2)
  push
    website        Push the launch page branch.
                   $ git -C web push -u origin launch-page
  tag
    billing        Tag the coupon release. [irreversible]
                   $ git -C billing tag v1.4.0 && git -C billing push origin v1.4.0

Decisions needed (1)
  billing/coupon-stacking: Allow coupons to stack?
    recommends no-stacking, asked 26h ago; approval: A pricing change.

Blocked (2)
  website        waiting on operator     The launch page needs final wording for the pricing banner.
  billing        waiting on website      Refund export needs the account page link.
                                         then: The refund report for finance.

In progress (1)
  github.com/example/web  [more than one role]
    billing        refund-export            Refund export link on the account page.
    website        account-link             Account page link.

Notices (1)
  billing        Date parsing bug in a shared library

Status (2)
  billing        2h       Coupons are in review; the refund export waits on the website.
  website        2h       Launch page is ready for review.
```

Answered requests that are still in reports, active roles without a report
and ignored reports from unknown or retired roles are listed when there
are any. `--json` prints the same briefing as JSON (`actions`,
`decisions`, `answered`, `blocked`, `inProgress`, `notices`, `reports`,
`idle`, `missing`, `ignored`) for other programs; each decision carries a
`key` of `role/id`, and reports carry their `generated` time.
`--reports` works as for `report submit`.

## who

Shows who to talk to before changing a repository: its steward and
contributors, with how to reach their sessions when their reports say, the
work in progress there, and the operator actions waiting on it.

```text
$ plexushive -C my-hive who github.com/example/web
Roles
  steward      website          agent website-1, tmux web
  contributor  billing
In progress
  billing          refund-export            Refund export link on the account page.
  website          account-link             Account page link.
Waiting on the operator
  website          push     Push the launch page branch.
```

## decide

Answers a decision request from a role's current report. The chosen option
becomes the decision and the others its alternatives; the request's
situation and evidence become its reasons, after `--reason` when given.
The decision is written to `decisions/<id>.json` under the request's ID,
and the command prints the message to send the role. It never replaces an
existing decision: a changed decision is a new one that supersedes it.

```text
$ plexushive -C my-hive decide billing/coupon-stacking no-stacking --reason "Partners can wait a release."
wrote decisions/coupon-stacking.json
send to billing:
[hive] type=status from=operator decided coupon-stacking: no-stacking. Read decisions/coupon-stacking.json and drop the request from your report.
```

Commit the new decision with the rest of the hive directory.

## schema

Prints the JSON Schema for a record type: `decision`, `report` or `role`.

```bash
plexushive schema role > role.schema.json
```
