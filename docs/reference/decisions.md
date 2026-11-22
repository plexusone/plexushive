# Decisions

The decision log holds what was decided, why, and what was considered and
rejected. Write one when a choice will constrain later work, especially
work by another agent.

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Short, stable slug; also the file name |
| `title` | yes | The decision in a few words |
| `status` | yes | `accepted`, or `superseded` once replaced |
| `decision` | yes | What was decided |
| `reasons` | yes | Why, one entry per reason |
| `alternatives` | no | `{"option": …, "rejected": …}` for each option not taken |
| `refs` | no | `INIT-…` and `RMI-…` IDs it belongs to or constrains |
| `identities` | no | Work identities it binds; empty means all |
| `repositories` | no | Repositories it constrains, so an agent can find the decisions that apply to its code |
| `supersedes` / `supersededBy` | no | Links to the decisions it replaces or that replaced it; both sides must agree |

Rules:

- **Append-only.** Never rewrite a decision's substance. Replace it with a
  new decision that lists it in `supersedes`, and set the old one's `status`
  to `superseded` and its `supersededBy`.
- **No dates.** When a decision was made comes from git history.
- **No duplication.** If a decision is already recorded in an ADR or a
  spec's PLAN, reference that initiative instead of restating it, unless
  agents outside that repository need to find it.


## Example

```json
{
  "id": "stewards-not-owners",
  "title": "Repositories have stewards, not owners",
  "status": "accepted",
  "decision": "Several work identities may change a repository; at most one stewards it and settles conflicts.",
  "reasons": [
    "Work crosses repositories, and one work identity per repository does not scale."
  ],
  "alternatives": [
    {"option": "One owning work identity per repository", "rejected": "Forces every cross-repository change through a message."}
  ],
  "identities": ["billing", "website"],
  "repositories": ["github.com/example/web"],
  "supersedes": ["one-steward-per-repository"]
}
```

## Validation

`Decision.Validate` checks one decision; `ValidateDecisions` checks the log:

- IDs are unique slugs, and each file is named after its decision
- `title`, `decision` and at least one reason are present
- an accepted decision has no `supersededBy`; a superseded one has it
- `refs` are `INIT-<SLUG>-NNN` or `RMI-<SLUG>-NNN`, and repositories are `github.com/<org>/<repo>`
- `identities` name existing work identities
- `supersedes` and `supersededBy` agree on both sides
