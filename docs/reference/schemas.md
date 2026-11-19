# JSON Schemas

The Go types in package `hive` are the source of truth. The schemas are
generated from them and committed under
[`hive/schema/`](https://github.com/plexusone/plexushive/tree/main/hive/schema):

| Schema | `$id` |
|---|---|
| Role | `https://github.com/plexusone/plexushive/hive/schema/role.schema.json` |
| Decision | `https://github.com/plexusone/plexushive/hive/schema/decision.schema.json` |
| Report | `https://github.com/plexusone/plexushive/hive/schema/report.schema.json` |

Print them with `plexushive schema decision|report|role`, or generate them in Go
with `hive.RoleSchema()`, `hive.DecisionSchema()` and `hive.ReportSchema()`.

The schemas check each file's shape. Checks across files, such as one
steward per pattern or supersede links agreeing, need `Hive.Validate`.

To regenerate the committed files after changing a type:

```bash
go test ./hive -run TestSchemas -update
```
