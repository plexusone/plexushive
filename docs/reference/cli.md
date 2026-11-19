# CLI

```text
plexushive [command] [-C dir]
```

| Flag | Default | Meaning |
|---|---|---|
| `-C`, `--dir` | `.` | Hive directory holding `roles/` and `decisions/` |

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
Decisions needed (1)
  billing        coupon-stacking: Allow coupons to stack?
                 recommends no-stacking; approval: A pricing change.

Blocked (2)
  website        waiting on operator     Publishing the launch page needs a push.
  billing        waiting on website      Refund export needs the account page link.

Status (2)
  billing        2h       Coupons are in review; the refund export waits on the website.
  website        2h       Launch page is ready for review.
```

Active roles without a report are listed last. `--json` prints the same
briefing as JSON (`decisions`, `blocked`, `reports`, `missing`) for other
programs. `--reports` works as for `report submit`.

## schema

Prints the JSON Schema for a record type: `decision`, `report` or `role`.

```bash
plexushive schema role > role.schema.json
```
