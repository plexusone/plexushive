# PlexusHive

Formats and tools for coordinating long-lived coding agents: the **work identity**
each agent works under, the **decisions** that bind them, the **status reports**
they file, and the **envelope** they put on messages to each other.

When a dozen agent sessions each work for days across hundreds of
repositories, the human operator ends up as the only router and the only
memory. PlexusHive writes down what each agent is for, where it works, who
settles conflicts in a repository, and why things were decided, in plain
JSON files that every agent can read and tests can validate.

## Principles

- **Work identities, not sessions.** A work identity is durable. A session working under it
  declares it with `HIVE_IDENTITY` or `plexushive start`, so renaming or
  restarting a session changes nothing in the records.
- **Stewards, not owners.** Several work identities may work in a repository; at most
  one stewards it and settles conflicts.
- **An append-only decision log.** Decisions are superseded, never
  rewritten.
- **Go types are the source of truth.** JSON Schemas are generated from
  them, and loading is strict, so a typo fails instead of being dropped.
- **Your records stay yours.** This module holds the formats and code; the
  records live in a hive directory of your own, usually a private
  repository.

## At a glance

```text
my-hive/
├── identities/<name>.json  one file per work identity
└── decisions/<id>.json     the append-only decision log
```

```bash
plexushive -C my-hive validate                        # check every work identity and decision
plexushive -C my-hive identities github.com/example/web  # who works there, steward first
plexushive -C my-hive brief                           # your actions, decisions, blockers, work in progress
plexushive -C my-hive start billing                   # bind a work identity to this tmux session
```

## Next steps

- [Installation](getting-started/installation.md)
- [Quick Start](getting-started/quickstart.md)
- [User Guide](guides/user-guide.md)
- [API Reference](https://pkg.go.dev/github.com/plexusone/plexushive/hive)
