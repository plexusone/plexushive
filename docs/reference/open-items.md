# Open Items

`open-items.json` in the hive directory lists every question waiting on the
operator, wherever it was raised, and keeps it until it is decided or
withdrawn. A decision request in a report lasts only as long as the report
carries it; an open question in a design record, such as an ADR's "Open
questions" section, is in no report at all. Open items keep both in one
list, so the operator can see everything still undecided, by work identity.

```json
{
  "updated": "2026-11-21",
  "items": [
    {
      "key": "billing/coupon-stacking",
      "identity": "billing",
      "source": {"kind": "report"},
      "status": "open",
      "request": {"id": "coupon-stacking", "title": "Allow coupons to stack?", "…": "a full decision request"},
      "related": ["website/checkout-copy"]
    },
    {
      "key": "website/checkout-copy",
      "identity": "website",
      "source": {"kind": "adr", "repository": "github.com/example/web",
        "path": "docs/adr/ADR-004-checkout.md", "document": "ADR-004"},
      "status": "draft",
      "request": {"id": "checkout-copy", "title": "Who writes the checkout wording, website or billing?"}
    }
  ]
}
```

| Field | Required | Meaning |
|---|---|---|
| `updated` | no | Date of the last change, `YYYY-MM-DD` |
| `items[].key` | yes | `<identity>/<request id>`, as in the briefing and `decide` |
| `items[].identity` | yes | The work identity that raised the question, or is to frame it |
| `items[].source` | yes | `kind` is `report`, `adr` or `document`; a document names its `repository`, `path` and, for an ADR, its `document` ID |
| `items[].status` | yes | See below |
| `items[].request` | yes | A [decision request](reports.md); a draft has only `id` and `title` |
| `items[].answer` | when answered | The operator's answer: `option`, `reason` and the date `answered` |
| `items[].decision` | no | The ID of a decision in the log that also records the answer (`decide --record`) |
| `items[].related` | no | Keys of items to decide together with this one |
| `items[].note` | when deferred or withdrawn | Why, or what the operator should weigh |

## Statuses

| Status | Meaning | Becomes |
|---|---|---|
| `draft` | A question with no options yet | `open` when its work identity asks it in a report |
| `open` | A complete decision request waiting on the operator | `decided` (from a report) or `answered` (from a document) when the operator answers; `withdrawn` if dropped |
| `answered` | A document's question the operator answered, waiting for its work identity to write the answer into the document | `decided` once its work identity drops the request |
| `deferred` | Waits on something the note names | `open` when asked again |
| `decided` | Answered, by `answer`, `decision` or both | |
| `withdrawn` | Dropped by its work identity without a decision | `open` if asked again |

A design record moves from proposed to accepted once none of its items
are `draft` or `open`.

## Keeping the list current

- `plexushive decide` records the operator's answer on the item, adding the
  item first if the request is not listed yet. Most answers are point
  decisions: the work identity acts on the answer and records the outcome
  where it belongs, such as a commit, a changelog or an ADR. With
  `--record` the answer also becomes a decision in the log, for working
  rules that bind the agents.
- `plexushive open sync` reads the reports and the decision log: it adds
  requests not yet listed, opens a draft or deferred item when its work
  identity asks a request with the same ID, refreshes open requests,
  marks items answered by a decision with the same ID (binding their work
  identity or every identity) as decided, decides an answered item once
  its work identity drops the request, and withdraws a report's
  question its work identity has dropped without a decision. A work
  identity without a readable report is left as it is, and an open
  question from a document is never withdrawn by a sync.
- To frame a draft, its work identity adds a decision request with the
  draft's ID to its report, with options and a recommendation; the next
  sync opens it and keeps its source.

Add a design record's open questions by hand, as drafts with their
source, and deferred items with their reason. Commit the file with the
rest of the hive.

## Validation

`ValidateOpenItems`, part of `Hive.Validate`, checks that:

- keys are unique and are `<identity>/<request id>`, and the work identity exists
- request IDs match `^[a-z][a-z0-9-]*$` and every item has a title
- an open item is a complete decision request: options, a recommendation among them, situation and approval
- an answered item has its answer and comes from a document; a decided item has its answer or names a decision
- an answer is one of the request's options and is dated `YYYY-MM-DD`; only answered and decided items carry an answer or a decision, and a named decision exists
- deferred and withdrawn items have a note
- an `adr` or `document` source names its repository and path
- related keys are listed
