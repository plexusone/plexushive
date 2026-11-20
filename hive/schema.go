package hive

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

const schemaBase = "https://github.com/plexusone/plexushive/hive/schema/"

// Schemas maps each file under schema/ to its generator.
var Schemas = map[string]func() ([]byte, error){
	"role.schema.json":     RoleSchema,
	"decision.schema.json": DecisionSchema,
	"report.schema.json":   ReportSchema,
}

// RoleSchema generates the role JSON Schema from Role.
func RoleSchema() ([]byte, error) {
	return schema(&Role{}, "role.schema.json", "Agent role",
		"What one long-lived agent is for, independently of the session that plays it: its mission, autonomy, the repositories it works in and its peers.")
}

// DecisionSchema generates the decision JSON Schema from Decision.
func DecisionSchema() ([]byte, error) {
	return schema(&Decision{}, "decision.schema.json", "Decision",
		"One entry in the append-only decision log: what was decided, why, the alternatives, and what it binds.")
}

func schema(v any, file, title, description string) ([]byte, error) {
	r := &jsonschema.Reflector{}
	s := r.Reflect(v)
	s.ID = jsonschema.ID(schemaBase + file)
	s.Title = title
	s.Description = description
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("hive: schema %s: %w", file, err)
	}
	return append(out, '\n'), nil
}

// ReportSchema generates the status report JSON Schema from Report.
func ReportSchema() ([]byte, error) {
	return schema(&Report{}, "report.schema.json", "Status report",
		"One role's current status: the operator's actions it waits on, its work in progress, what it finished, what is blocked and on whom, what it does next, the decisions it needs and notices for other roles. Reports are kept outside version control, so they can describe unpublished work plainly.")
}
