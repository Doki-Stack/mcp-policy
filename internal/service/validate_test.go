package service

import (
	"errors"
	"testing"
	"time"

	"github.com/doki-stack/mcp-policy/internal/model"
)

func validDoc() model.IngestPolicyRequest {
	return model.IngestPolicyRequest{
		OrgID:               "11111111-1111-1111-1111-111111111111",
		PolicyID:            "22222222-2222-2222-2222-222222222222",
		Title:               "no-public-s3",
		Text:                "S3 buckets must not have public ACLs.",
		ComplianceFramework: "SOC2",
		EffectiveDate:       time.Now(),
	}
}

func TestValidatePolicyDocument_Valid(t *testing.T) {
	if err := ValidatePolicyDocument(validDoc()); err != nil {
		t.Fatalf("expected valid document to pass, got: %v", err)
	}
}

func TestValidatePolicyDocument_EmptyTitleFailsMinLength(t *testing.T) {
	doc := validDoc()
	doc.Title = ""
	err := ValidatePolicyDocument(doc)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("error should wrap ErrInvalidPolicy, got: %v", err)
	}
}

// TestPolicySchema_RequiresOrgID exercises the schema's "required" keyword
// directly (key absence, not just an empty value) — IngestPolicyRequest's
// json tags mean ValidatePolicyDocument always emits every key, even when
// empty, so this bypasses the typed struct to validate the schema itself.
func TestPolicySchema_RequiresOrgID(t *testing.T) {
	doc := map[string]interface{}{
		"policy_id": "22222222-2222-2222-2222-222222222222",
		"title":     "no-public-s3",
		"text":      "S3 buckets must not have public ACLs.",
	}
	if err := policySchema.Validate(doc); err == nil {
		t.Fatal("expected validation error for missing org_id key")
	}
}

func TestPolicySchema_RejectsUnknownField(t *testing.T) {
	doc := map[string]interface{}{
		"org_id":    "11111111-1111-1111-1111-111111111111",
		"policy_id": "22222222-2222-2222-2222-222222222222",
		"title":     "no-public-s3",
		"text":      "S3 buckets must not have public ACLs.",
		"unknown":   "field",
	}
	if err := policySchema.Validate(doc); err == nil {
		t.Fatal("expected validation error for unknown field (additionalProperties: false)")
	}
}

func TestValidatePolicyDocument_BadUUID(t *testing.T) {
	doc := validDoc()
	doc.OrgID = "not-a-uuid"
	if err := ValidatePolicyDocument(doc); err == nil {
		t.Fatal("expected validation error for malformed org_id")
	}
}

func TestValidatePolicyDocument_TextTooLong(t *testing.T) {
	doc := validDoc()
	big := make([]byte, 10001)
	for i := range big {
		big[i] = 'x'
	}
	doc.Text = string(big)
	if err := ValidatePolicyDocument(doc); err == nil {
		t.Fatal("expected validation error for text exceeding max length")
	}
}
