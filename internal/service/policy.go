package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/repository"
	"github.com/qdrant/go-client/qdrant"
)

// ErrFailClosed wraps any error that must result in a 503
// POLICY_UNAVAILABLE response (ADR-005) rather than a degraded result.
// Embedding and Qdrant failures are fail-closed; cache failures are not
// (see PolicyEngine.Evaluate).
var ErrFailClosed = errors.New("policy evaluation unavailable")

// TopKMatches is how many policies evaluate-policy returns after re-ranking.
const TopKMatches = 5

// MaxContextTokens bounds the combined size of returned policy text so a
// downstream LLM prompt stays predictable. Estimated at ~4 chars/token
// (no tokenizer dependency here) rather than counted exactly.
const MaxContextTokens = 2000

const approxCharsPerToken = 4

// PolicyEngine implements the evaluate-policy tool: embed the query, search
// Qdrant, re-rank, and (best-effort) cache the result.
type PolicyEngine struct {
	embedder *EmbeddingService
	qdrant   *repository.QdrantRepo
	cache    *repository.CacheRepo
}

// NewPolicyEngine wires an embedder, Qdrant repo, and cache repo together.
func NewPolicyEngine(embedder *EmbeddingService, qdrantRepo *repository.QdrantRepo, cache *repository.CacheRepo) *PolicyEngine {
	return &PolicyEngine{embedder: embedder, qdrant: qdrantRepo, cache: cache}
}

// Evaluate runs the full evaluate-policy flow. A returned error wrapping
// ErrFailClosed means the caller must respond 503 POLICY_UNAVAILABLE and
// must NOT fall back to a cached or partial result — per ADR-005 there is
// no degraded mode for the embedding/Qdrant path. A cache lookup or write
// failure, by contrast, is swallowed here: it only costs latency.
func (e *PolicyEngine) Evaluate(ctx context.Context, req model.EvaluateRequest) (*model.EvaluateResponse, error) {
	hash := repository.QueryHash(req.Query, req.ResourceType, req.Environment)

	var cached model.EvaluateResponse
	if found, err := e.cache.GetEvaluation(ctx, req.OrgID, hash, &cached); err == nil && found {
		return &cached, nil
	}
	// Cache miss or cache error: fall through to Qdrant either way.

	vector, err := e.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("%w: embed query: %v", ErrFailClosed, err)
	}

	// Search wider than TopKMatches so re-ranking (which reorders by more
	// than raw similarity) has something to work with.
	points, err := e.qdrant.Search(ctx, req.OrgID, vector, TopKMatches*4)
	if err != nil {
		return nil, fmt.Errorf("%w: search policies: %v", ErrFailClosed, err)
	}

	ranked := rerank(points, TopKMatches)
	matches := make([]model.PolicyMatch, len(ranked))
	for i, p := range ranked {
		matches[i] = repository.PolicyFromPoint(p)
	}
	matches = truncateToTokenBudget(matches, MaxContextTokens)

	resp := &model.EvaluateResponse{
		Policies:    matches,
		Constraints: buildConstraints(matches),
		// Real violation detection needs the proposed resource
		// configuration (e.g. from agent-automation's generated plan),
		// which evaluate-policy's {query, resource_type, environment}
		// input doesn't carry. Left empty rather than faked; the caller
		// (agent-review, Phase 2) is where plan-vs-policy comparison
		// belongs.
		Violations: []model.Violation{},
	}

	if err := e.cache.SetEvaluation(ctx, req.OrgID, hash, resp); err != nil {
		// Best-effort: caching failures never affect the response.
		_ = err
	}

	return resp, nil
}

// rerank reorders points by similarity score combined with a recency
// weight (newer effective_date preferred), then returns the top limit.
// Qdrant's own ordering is pure similarity; this is what makes the result
// a "re-rank" rather than a plain top-k.
func rerank(points []*qdrant.ScoredPoint, limit int) []*qdrant.ScoredPoint {
	type scored struct {
		point     *qdrant.ScoredPoint
		composite float32
	}

	now := time.Now()
	ranked := make([]scored, len(points))
	for i, p := range points {
		ranked[i] = scored{point: p, composite: p.GetScore() * recencyWeight(now, effectiveDateOf(p))}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].composite > ranked[j].composite
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]*qdrant.ScoredPoint, len(ranked))
	for i, r := range ranked {
		out[i] = r.point
	}
	return out
}

func effectiveDateOf(p *qdrant.ScoredPoint) time.Time {
	raw := p.GetPayload()["effective_date"].GetStringValue()
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// recencyWeight decays gently over a year and floors out rather than
// hitting zero, so an old-but-highly-relevant policy still surfaces.
func recencyWeight(now, effectiveDate time.Time) float32 {
	if effectiveDate.IsZero() || effectiveDate.After(now) {
		return 1.0
	}
	days := now.Sub(effectiveDate).Hours() / 24
	return float32(1.0 / (1.0 + days/365.0))
}

// truncateToTokenBudget trims the combined policy text to roughly
// maxTokens (at ~4 chars/token), dropping or truncating trailing matches
// once the budget is spent.
func truncateToTokenBudget(matches []model.PolicyMatch, maxTokens int) []model.PolicyMatch {
	budget := maxTokens * approxCharsPerToken
	used := 0
	out := make([]model.PolicyMatch, 0, len(matches))
	for _, m := range matches {
		if used >= budget {
			break
		}
		remaining := budget - used
		if len(m.Text) > remaining {
			m.Text = m.Text[:remaining] + "…"
			out = append(out, m)
			break
		}
		used += len(m.Text)
		out = append(out, m)
	}
	return out
}

func buildConstraints(matches []model.PolicyMatch) []string {
	constraints := make([]string, len(matches))
	for i, m := range matches {
		constraints[i] = fmt.Sprintf("%s: %s", m.Title, m.Text)
	}
	return constraints
}
