package service

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema/policy_schema.json
var schemaFS embed.FS

var policySchema = compilePolicySchema()

func compilePolicySchema() *jsonschema.Schema {
	raw, err := schemaFS.ReadFile("schema/policy_schema.json")
	if err != nil {
		panic(fmt.Sprintf("service: read embedded policy_schema.json: %v", err))
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("policy_schema.json", bytes.NewReader(raw)); err != nil {
		panic(fmt.Sprintf("service: invalid embedded policy_schema.json: %v", err))
	}
	schema, err := compiler.Compile("policy_schema.json")
	if err != nil {
		panic(fmt.Sprintf("service: compile embedded policy_schema.json: %v", err))
	}
	return schema
}

// ValidatePolicyDocument validates req against policy_schema.json. It
// re-encodes req to JSON first so the schema sees exactly the shape a
// client's request body would have — the same validation path whether
// this is called from an internal batch or (in a later task) directly on
// a decoded HTTP request.
func ValidatePolicyDocument(req model.IngestPolicyRequest) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("policy: encode for validation: %w", err)
	}
	var doc interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("policy: decode for validation: %w", err)
	}
	if err := policySchema.Validate(doc); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	return nil
}
