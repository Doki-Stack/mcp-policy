package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
)

// TestSeedPolicies_ValidAgainstSchema guards against seed/policies.json
// drifting out of sync with policy_schema.json — a change to either one
// that breaks the other fails CI instead of surfacing at deploy time via
// seed/seed.sh.
func TestSeedPolicies_ValidAgainstSchema(t *testing.T) {
	raw, err := os.ReadFile("../../seed/policies.json")
	if err != nil {
		t.Fatalf("read seed/policies.json: %v", err)
	}

	var docs []model.IngestPolicyRequest
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatalf("decode seed/policies.json: %v", err)
	}

	if len(docs) < 8 || len(docs) > 10 {
		t.Errorf("seed/policies.json has %d policies, want 8-10 per A15's acceptance criteria", len(docs))
	}

	seenIDs := make(map[string]bool, len(docs))
	for i, doc := range docs {
		if err := ValidatePolicyDocument(doc); err != nil {
			t.Errorf("seed policy[%d] %q fails schema validation: %v", i, doc.Title, err)
		}
		if seenIDs[doc.PolicyID] {
			t.Errorf("seed policy[%d] %q has a duplicate policy_id: %s", i, doc.Title, doc.PolicyID)
		}
		seenIDs[doc.PolicyID] = true
	}
}
