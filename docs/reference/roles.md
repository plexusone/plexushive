# Roles

A role says what one long-lived agent is for and where it works. It is
durable; the session playing it is not. tmux names are kept short for
interfaces that truncate them, and both they and harness session IDs change
when a session is restarted, so no record names a session. A session
declares its role with the environment variable `HIVE_ROLE`:

```bash
HIVE_ROLE=billing claude --resume <session>
```

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | Stable, lowercase role name; also the file name, the value of `HIVE_ROLE` and the name others address it by |
| `status` | yes | `draft` until the operator confirms it, then `active`; `retired` when no session plays it |
| `mission` | yes | What the role is responsible for, in a sentence or two |
| `autonomy` | yes | `observe`, `prepare` or `execute`; see below |
| `repositories` | no | Where the role works: `{"pattern": …, "affinity": …}` entries; see below |
| `initiatives` | no | Initiatives it works on (`INIT-<SLUG>-NNN`) |
| `peers` | no | Roles it works with most |
| `notes` | no | Open questions while a draft, or constraints that fit no other field |

Autonomy, in increasing order:

- **observe** reads, analyzes and reports. It changes nothing.
- **prepare** also makes changes locally for the operator to publish:
  worktrees, branches, commits, release notes and changelogs. It does not
  push, tag, merge, deploy or send.
- **execute** also makes bounded, low-risk changes in the repositories it
  works in. Pricing, refunds, production deploys, spending and messages to
  customers still need approval at every level.

## Repositories: stewards, not owners

Several roles may change the same repository. `pattern` is one repository,
`github.com/<org>/<repo>`, or a whole organization, `github.com/<org>/*`.
`affinity` is:

- **steward**: settles conflicting changes and reviews breaking ones. At
  most one role stewards a pattern, and an exact repository outranks an
  organization pattern.
- **contributor**: works there regularly.

A repository no role lists is common ground. A role working in a repository
it does not steward uses its own branch or worktree, and sends the steward a
`request` only for breaking or cross-cutting changes. `RolesFor` and
`Steward` answer who works in a repository, and `plexushive roles` prints
the answer.

## Example

```json
{
  "name": "billing",
  "status": "active",
  "mission": "Own the billing service and its payment integrations.",
  "autonomy": "prepare",
  "repositories": [
    {"pattern": "github.com/example/billing", "affinity": "steward"},
    {"pattern": "github.com/example/web", "affinity": "contributor"}
  ],
  "initiatives": ["INIT-BILLING-001"],
  "peers": ["website"]
}
```

## Validation

`Role.Validate` checks one role; `ValidateRoles` checks the set:

- names are lowercase (`^[a-z][a-z0-9-]*$`) and unique, and each file is named after its role
- `status` and `autonomy` take one of their listed values, and `mission` is not blank
- each pattern is `github.com/<org>/<repo>` or `github.com/<org>/*`, listed once per role
- peers name existing roles, and a role is not its own peer
- no pattern has two stewards among roles that are not retired
