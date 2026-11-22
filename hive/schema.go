package hive

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

const schemaBase = "https://github.com/plexusone/plexushive/hive/schema/"

// Schemas maps each file under schema/ to its generator.
var Schemas = map[string]func() ([]byte, error){
	"identity.schema.json": IdentitySchema,
	"decision.schema.json": DecisionSchema,
	"report.schema.json":   ReportSchema,
}

// IdentitySchema generates the work identity JSON Schema from WorkIdentity.
func IdentitySchema() ([]byte, error) {
	return schema(&WorkIdentity{}, "identity.schema.json", "Agent work identity",
		"What one long-lived agent is for, independently of the sessions that work under it: its mission, autonomy, the repositories it works in and its peers.")
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
		"One work identity's current status: the operator's actions it waits on, its work in progress, what it finished, what is blocked and on whom, what it does next, the decisions it needs and notices for other work identities. Reports are kept outside version control, so they can describe unpublished work plainly.")
}
