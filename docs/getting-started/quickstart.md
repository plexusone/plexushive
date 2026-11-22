# Quick Start

Create a hive with one work identity and one decision, then validate it.

```bash
mkdir -p my-hive/work identities my-hive/decisions
```

`my-hive/identities/billing.json`:

```json
{
  "name": "billing",
  "status": "draft",
  "mission": "Own the billing service and its payment integrations.",
  "autonomy": "prepare",
  "repositories": [
    {"pattern": "github.com/example/billing", "affinity": "steward"}
  ]
}
```

`my-hive/decisions/one-billing-service.json`:

```json
{
  "id": "one-billing-service",
  "title": "One service owns billing",
  "status": "accepted",
  "decision": "All charges go through the billing service.",
  "reasons": ["One place to audit every charge."],
  "identities": ["billing"]
}
```

Validate it and ask who works in the billing repository:

```bash
$ plexushive -C my-hive validate
1 work identity and 1 decision are valid

$ plexushive -C my-hive identities github.com/example/billing
steward      billing              repository
```

Start the agent's session with its work identity:

```bash
HIVE_IDENTITY=billing claude --resume <session>
```

The [User Guide](../guides/user-guide.md) covers each step in depth.
