# CLI

```text
plexushive [command] [-C dir]
```

| Flag | Default | Meaning |
|---|---|---|
| `-C`, `--dir` | `.` | Hive directory holding `identities/` and `decisions/` |
| `--identity` | `$HIVE_IDENTITY`, else the tmux session's | The work identity this session works under |
| `--version` | | Print the version the binary was built from |

Commands that need the session's work identity look for it in that order: `--identity`,
the `HIVE_IDENTITY` environment variable, then the environment of the tmux
session the command runs in. The last reaches agents started before the
work identity was bound, since `tmux set-environment` reaches only processes started
after it but can always be read back. `HIVE_ROLE`, the name before v0.5.0,
is read after `HIVE_IDENTITY` in both places, and `--role` and
`plexushive roles` still work but are deprecated.

## start

Binds a work identity to the current tmux session and prints what the session needs
to work under it. The work identity must exist and not be retired. Outside tmux it prints
the `export` to use instead.

```text
$ plexushive -C my-hive start billing
bound billing to the tmux session billing

Work identity billing (active, autonomy prepare)
  Own the billing service and its payment integrations.

Repositories
  steward      github.com/example/billing
  collaborator github.com/example/web

Initiatives: INIT-BILLING-001

Peers
  website              agent website-1, tmux web

Decisions that bind you (1); read each in decisions/<id>.json
  stewards-not-owners                          Repositories have stewards, not owners

Your report
  generated 2026-11-19 09:00 UTC

Report at handoffs: when work is ready for the operator, when blocked or
unblocked, when a decision is needed, when starting or stopping in a
repository, after the operator acts for you, and when another work identity's work
changes your plan. Before working in a repository another work identity stewards:
  plexushive -C /path/to/my-hive who github.com/<org>/<repo>
Submit (plexushive schema report gives the format):
  plexushive -C /path/to/my-hive report submit <file>
```

The binding decisions are the accepted decisions that name the work identity and
those that name no work identity. "Your report" lists requests the decision log has
answered, for the work identity to act on and drop.

## context

Prints the same context as `start` without binding anything, for the work identity
given, else the session's work identity. A hook that runs when a session starts or
after its context is compacted calls it to put the context back. `--json`
prints it for other programs (`identity`, `decisions`, `peers`, `report`,
`answered`).

## whoami

Shows the work identity the session works under, where it was found, and the tmux session.

```text
$ plexushive whoami
billing (from the tmux session)
tmux session billing
```

When the work identity came from `HIVE_ROLE`, it says so and how to bind the
new name.

## validate

Loads every work identity and decision and checks them together. Exits non-zero and
lists every problem when a record is invalid.

```bash
$ plexushive -C my-hive validate
2 work identities and 1 decision are valid
```

## identities

Lists the work identities, not retired, that work in a repository: the steward first,
then collaborators by name. The last column says whether the work identity names the
repository itself or its organization.

```bash
$ plexushive -C my-hive identities github.com/example/web
steward      website              repository
collaborator billing              repository
```

A repository no work identity lists is reported as common ground.

## report submit

Validates a report and stores it as the work identity's current report, replacing
the previous one atomically. Reads a file, or standard input when no file
or `-` is given. An empty `identity` is the session's work identity (see above), an
empty `generated` is set to the current time, and inside tmux an empty
`session.tmux` is set to the session's name. A pull request given as `#<n>`
is expanded when the action or item names one repository. The work identity must
exist in the hive directory.

```bash
$ plexushive -C my-hive report submit report.json
stored the billing report
```

| Flag | Default | Meaning |
|---|---|---|
| `--reports` | `$HIVE_REPORTS`, else the per-user data directory | Where reports are stored |

## report show

Prints a work identity's stored report as JSON: the work identity given, else the session's
work identity. Edit it and submit it again to change one part without retyping the
rest.

```bash
plexushive -C my-hive report show > report.json   # edit, then:
plexushive -C my-hive report submit report.json
```

## brief

Gathers every work identity's report into one briefing.

```text
$ plexushive -C my-hive brief
Your actions (3)
  push
    website        Push the launch page branch.
                   id: push-launch
                   $ git -C web push -u origin launch-page
                   then the work identity: Open the pull request.
  tag
    billing        Tag the coupon release. [irreversible]
                   versions: v1.4.0
                   $ git -C billing tag v1.4.0 && git -C billing push origin v1.4.0
  configure
    website        Set the analytics key for the launch page.
                   after: push-launch
                   credentials: web/prod/ANALYTICS_KEY
                   1. Create a site key in the analytics console.
                   2. Store it as the ANALYTICS_KEY repository secret.

Decisions needed (1)
  billing/coupon-stacking: Allow coupons to stack?
    recommends no-stacking, asked 26h ago; approval: A pricing change.

Blocked (2)
  website        waiting on operator     The launch page needs final wording for the pricing banner.
  billing        waiting on website      Refund export needs the account page link.
                                         then: The refund report for finance.

In progress (1)
  github.com/example/web  [more than one work identity]
    billing        refund-export            Refund export link on the account page.
    website        account-link (committed) Account page link.

Notices (1)
  billing        Date parsing bug in a shared library

Status (2)
  billing        2h       Coupons are in review; the refund export waits on the website.
  website        2h       Launch page is ready for review.
```

A step several work identities ask for is listed once with all their names. An
action that applies under an option of an open decision is marked `only if
decided: <identity>/<id> = <option>`; once decided, it is kept for that option
and dropped for any other. A blocker waiting on several parties reads
`waiting on website, then operator`. In the status, `idle` marks a work identity
with nothing to do and `decision` one whose only open work waits on its
decision requests.

Answered requests that are still in reports, active work identities without a report,
ignored reports from unknown or retired work identities, and unreadable reports (such
as one from a newer version) are listed when there are any. `--json` prints the same briefing as JSON (`actions`,
`decisions`, `answered`, `blocked`, `inProgress`, `notices`, `reports`,
`idle`, `waiting`, `missing`, `ignored`, `unreadable`) for other programs; each action
carries the `identities` that asked for it and any `condition`, each decision
a `key` of `identity/id`, and reports their `generated` time.
`--reports` works as for `report submit`.

## who

Shows who to talk to before changing a repository: its steward and
collaborators, with how to reach their sessions when their reports say, the
work in progress there, and the operator actions waiting on it.

```text
$ plexushive -C my-hive who github.com/example/web
Work identities
  steward      website          agent website-1, tmux web
  collaborator billing
In progress
  billing          refund-export            Refund export link on the account page.
  website          account-link (committed) Account page link.
Waiting on the operator
  website          push       Push the launch page branch.
  website          configure  Set the analytics key for the launch page.
```

## decide

Answers a decision request from a work identity's current report. The chosen option
becomes the decision and the others its alternatives; the request's
situation and evidence become its reasons, after `--reason` when given.
The decision is written to `decisions/<id>.json` under the request's ID,
the item is marked decided in `open-items.json` when it is listed there,
and the command prints the message to send the work identity. It never replaces an
existing decision: a changed decision is a new one that supersedes it.

```text
$ plexushive -C my-hive decide billing/coupon-stacking no-stacking --reason "Partners can wait a release."
wrote decisions/coupon-stacking.json
marked billing/coupon-stacking decided in open-items.json
send to billing:
[hive] type=status from=operator decided coupon-stacking: no-stacking. Read decisions/coupon-stacking.json and drop the request from your report.
```

Commit the new decision with the rest of the hive directory.

## open

Lists [open items](open-items.md): open questions with their
recommendation first, then drafts that still need options, then deferred
ones. Give a work identity to list only its items.

```text
$ plexushive -C my-hive open
Open, waiting on the operator (1)
  billing/coupon-stacking
      Allow coupons to stack?
      recommends no-stacking; decide with website/checkout-copy

Draft, waiting on its work identity for options (1)
  website/checkout-copy  [ADR-004]
      Who writes the checkout wording, website or billing?

Deferred (1)
  website/cookie-banner  [ADR-004]
      Which cookie banner the checkout shows
      note: Waits on the privacy review.
```

| Flag | Meaning |
|---|---|
| `--status` | Only these statuses, comma-separated or repeated |
| `--all` | Include decided and withdrawn items |
| `--json` | Print the items as JSON |

## open sync

Brings `open-items.json` up to date with the reports and the decision log
and prints each change; `--dry-run` prints without writing. See
[keeping the list current](open-items.md#keeping-the-list-current).

```text
$ plexushive -C my-hive open sync
added      website/hero-image
opened     website/checkout-copy
withdrawn  billing/coupon-stacking
```

## schema

Prints the JSON Schema for a record type: `decision`, `report`, `identity` or `open-items`.

```bash
plexushive schema identity > identity.schema.json
```
