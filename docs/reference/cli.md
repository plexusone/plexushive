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

## schema

Prints the JSON Schema for a record type, `role` or `decision`.

```bash
plexushive schema role > role.schema.json
```
