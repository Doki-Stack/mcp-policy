package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doki-stack/mcp-policy/internal/model"
)

// ErrInvalidPolicy means a policy document failed schema validation
// (POLICY_INGEST_INVALID at the handler layer).
var ErrInvalidPolicy = errors.New("policy document invalid")

// ErrBatchTooLarge means an ingest batch exceeded MaxIngestBatchSize.
var ErrBatchTooLarge = errors.New("ingest batch too large")

// MaxIngestBatchSize is the largest batch ingest-policy accepts in one call.
const MaxIngestBatchSize = 100

// DefaultSeverity is applied to ingested policies, since ingest-policy's
// input doesn't carry a severity field but the Qdrant payload schema
// (db-schemas/docs/implementation-plan/06-non-pg-data-stores.md) has one.
const DefaultSeverity = model.SeverityMedium

// IngestResult reports the outcome of ingesting a single policy document.
type IngestResult struct {
	PolicyID string
	Error    error
}

// IngestPolicies validates, embeds, and indexes up to MaxIngestBatchSize
// policy documents. One document's failure (schema or embedding) doesn't
// abort the rest of the batch — each gets its own IngestResult. Only a
// batch-size violation returns a top-level error.
//
// After indexing, every org touched by a successful ingest has its
// evaluation cache invalidated (best-effort — a failed invalidation just
// means slightly slower convergence, not a correctness problem, since the
// cache TTL is only 24h).
func (e *PolicyEngine) IngestPolicies(ctx context.Context, docs []model.IngestPolicyRequest) ([]IngestResult, error) {
	if len(docs) > MaxIngestBatchSize {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrBatchTooLarge, len(docs), MaxIngestBatchSize)
	}

	results := make([]IngestResult, len(docs))
	orgsToInvalidate := make(map[string]struct{})

	for i, doc := range docs {
		if doc.EffectiveDate.IsZero() {
			doc.EffectiveDate = time.Now()
		}

		if err := ValidatePolicyDocument(doc); err != nil {
			results[i] = IngestResult{PolicyID: doc.PolicyID, Error: err}
			continue
		}

		vector, err := e.embedder.Embed(ctx, doc.Text)
		if err != nil {
			results[i] = IngestResult{PolicyID: doc.PolicyID, Error: fmt.Errorf("%w: embed: %v", ErrFailClosed, err)}
			continue
		}

		policy := model.Policy{
			PolicyID:            doc.PolicyID,
			OrgID:               doc.OrgID,
			Title:               doc.Title,
			Text:                doc.Text,
			ComplianceFramework: doc.ComplianceFramework,
			EffectiveDate:       doc.EffectiveDate,
			Severity:            DefaultSeverity,
		}
		if err := e.qdrant.UpsertPolicy(ctx, policy, vector); err != nil {
			results[i] = IngestResult{PolicyID: doc.PolicyID, Error: fmt.Errorf("%w: upsert: %v", ErrFailClosed, err)}
			continue
		}

		results[i] = IngestResult{PolicyID: doc.PolicyID}
		orgsToInvalidate[doc.OrgID] = struct{}{}
	}

	for orgID := range orgsToInvalidate {
		if err := e.cache.InvalidateOrg(ctx, orgID); err != nil {
			_ = err // best-effort; see doc comment above
		}
	}

	return results, nil
}
