# Work identities

A work identity says what one long-lived agent is for and where it works. It is
durable; the sessions working under it are not. tmux names are kept short for
interfaces that truncate them, and both they and harness session IDs change
when a session is restarted, so no record names a session. A session
declares its work identity with the environment variable `HIVE_IDENTITY`:

```bash
HIVE_IDENTITY=billing claude --resume <session>
```

A work identity is not a role. Role keeps its usual meaning in access
control, a permission level such as `admin` or `viewer` that a principal
holds; a work identity is a line of work, with a purpose and an address,
that sessions take on.

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | Stable, lowercase work identity name; also the file name, the value of `HIVE_IDENTITY` and the name others address it by |
| `status` | yes | `draft` until the operator confirms it, then `active`; `retired` when no session works under it |
| `mission` | yes | What the work identity is responsible for, in a sentence or two |
| `autonomy` | yes | `observe`, `prepare` or `execute`; see below |
| `repositories` | no | Where the work identity works: `{"pattern": …, "affinity": …}` entries; see below |
| `initiatives` | no | Initiatives it works on (`INIT-<SLUG>-NNN`) |
| `peers` | no | Work identities it works with most |
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

Several work identities may change the same repository. `pattern` is one repository,
`github.com/<org>/<repo>`, or a whole organization, `github.com/<org>/*`.
`affinity` is:

- **steward**: settles conflicting changes and reviews breaking ones. At
  most one work identity stewards a pattern, and an exact repository outranks an
  organization pattern.
- **collaborator**: works there regularly.

A repository no work identity lists is common ground. A work identity working in a repository
it does not steward uses its own branch or worktree, and sends the steward a
`request` only for breaking or cross-cutting changes. `IdentitiesFor` and
`Steward` answer who works in a repository, and `plexushive identities` prints
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
    {"pattern": "github.com/example/web", "affinity": "collaborator"}
  ],
  "initiatives": ["INIT-BILLING-001"],
  "peers": ["website"]
}
```

## Validation

`WorkIdentity.Validate` checks one work identity; `ValidateIdentities` checks the set:

- names are lowercase (`^[a-z][a-z0-9-]*$`) and unique, and each file is named after its work identity
- `status` and `autonomy` take one of their listed values, and `mission` is not blank
- each pattern is `github.com/<org>/<repo>` or `github.com/<org>/*`, listed once per work identity
- peers name existing work identities, and a work identity is not its own peer
- no pattern has two stewards among work identities that are not retired

## Names before v0.5.0

Work identities were called roles before v0.5.0. Reading still accepts the
old names, so a hive and its sessions can move one at a time; everything
written uses the new ones.

| Before v0.5.0 | Now |
|---|---|
| `roles/` | `identities/` (a hive with both is refused) |
| `HIVE_ROLE` | `HIVE_IDENTITY`, read first |
| `--role` | `--identity` |
| `plexushive roles` | `plexushive identities` |
| `"role"` in a report, `"roles"` in a decision | `"identity"`, `"identities"` |
| `contributor` affinity | `collaborator` |

`plexushive whoami` says when it found the old variable; run
`plexushive start <identity>` again to bind the new one.
