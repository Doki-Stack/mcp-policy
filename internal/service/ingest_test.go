package service

import (
	"context"
	"errors"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
)

// TestIngestPolicies_BatchTooLarge and TestIngestPolicies_AllInvalid don't
// touch the embedder, Qdrant, or cache at all (batch-size and per-doc
// validation both short-circuit before any of those are called), so a
// PolicyEngine with nil dependencies is safe here — this genuinely
// exercises IngestPolicies without needing a live Qdrant/Ollama.

func TestIngestPolicies_BatchTooLarge(t *testing.T) {
	engine := NewPolicyEngine(nil, nil, nil)
	docs := make([]model.IngestPolicyRequest, MaxIngestBatchSize+1)
	for i := range docs {
		docs[i] = validDoc()
	}

	_, err := engine.IngestPolicies(context.Background(), docs)
	if !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("expected ErrBatchTooLarge, got: %v", err)
	}
}

func TestIngestPolicies_AllInvalid(t *testing.T) {
	engine := NewPolicyEngine(nil, nil, nil)
	invalid := validDoc()
	invalid.Title = "" // fails minLength before embed/qdrant are ever reached

	results, err := engine.IngestPolicies(context.Background(), []model.IngestPolicyRequest{invalid, invalid})
	if err != nil {
		t.Fatalf("IngestPolicies returned a top-level error for a per-doc failure: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if !errors.Is(r.Error, ErrInvalidPolicy) {
			t.Errorf("result[%d].Error = %v, want ErrInvalidPolicy", i, r.Error)
		}
	}
}

func TestIngestPolicies_EmptyBatch(t *testing.T) {
	engine := NewPolicyEngine(nil, nil, nil)
	results, err := engine.IngestPolicies(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error for empty batch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
