package repository

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/shared-go/breaker"
	"github.com/qdrant/go-client/qdrant"
	"github.com/sony/gobreaker/v2"
)

// PoliciesCollection is the Qdrant collection holding semantic policy
// documents (see db-schemas/docs/implementation-plan/06-non-pg-data-stores.md).
const PoliciesCollection = "policies"

// PolicyVectorDimensions is the embedding size configured on the collection.
const PolicyVectorDimensions = 768

// QdrantRepo wraps the Qdrant client with a circuit breaker. Per ADR-005,
// callers must treat ANY error from Search/UpsertPolicy as fail-closed —
// there is no degraded/unfiltered fallback, whether the breaker is open or
// the underlying call simply failed.
type QdrantRepo struct {
	client *qdrant.Client
	cb     *breaker.CircuitBreaker
}

// QdrantConfig configures NewQdrantRepo.
type QdrantConfig struct {
	URL              string
	APIKey           string
	BreakerThreshold int
	BreakerTimeout   time.Duration
}

// NewQdrantRepo connects to Qdrant over gRPC and wraps calls in a circuit
// breaker that opens after BreakerThreshold consecutive failures.
func NewQdrantRepo(cfg QdrantConfig) (*QdrantRepo, error) {
	host, port, useTLS, err := parseQdrantURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("qdrant: parse url: %w", err)
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 cfg.APIKey,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: connect: %w", err)
	}

	threshold := uint32(cfg.BreakerThreshold)
	cb := breaker.New("qdrant",
		breaker.WithReadyToTrip(func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= threshold
		}),
		breaker.WithTimeout(cfg.BreakerTimeout),
	)

	return &QdrantRepo{client: client, cb: cb}, nil
}

// parseQdrantURL accepts "host:port", "grpc://host:port", or
// "http(s)://host:port" and returns the host, gRPC port (default 6334 if
// unspecified), and whether to use TLS.
func parseQdrantURL(raw string) (host string, port int, useTLS bool, err error) {
	if !strings.Contains(raw, "://") {
		raw = "grpc://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", 0, false, err
	}
	host = u.Hostname()
	if host == "" {
		return "", 0, false, fmt.Errorf("missing host in %q", raw)
	}
	if portStr := u.Port(); portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return "", 0, false, fmt.Errorf("invalid port in %q: %w", raw, err)
		}
	} else {
		port = 6334
	}
	useTLS = u.Scheme == "https" || u.Scheme == "grpcs"
	return host, port, useTLS, nil
}

// Close releases the underlying gRPC connection.
func (r *QdrantRepo) Close() error {
	return r.client.Close()
}

// Ping checks Qdrant liveness; used for readiness checks.
func (r *QdrantRepo) Ping(ctx context.Context) error {
	_, err := r.client.HealthCheck(ctx)
	return err
}

// EnsureCollection creates the policies collection if it doesn't exist yet.
// Idempotent — safe to call on every startup.
func (r *QdrantRepo) EnsureCollection(ctx context.Context) error {
	exists, err := r.client.CollectionExists(ctx, PoliciesCollection)
	if err != nil {
		return fmt.Errorf("qdrant: check collection: %w", err)
	}
	if exists {
		return nil
	}
	err = r.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: PoliciesCollection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     PolicyVectorDimensions,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: create collection: %w", err)
	}
	return nil
}

// Search runs a top-k similarity search over the policies collection,
// filtered to orgID. Any error — including an open circuit breaker — means
// the caller must fail closed (ADR-005); there is no fallback query.
func (r *QdrantRepo) Search(ctx context.Context, orgID string, vector []float32, limit uint64) ([]*qdrant.ScoredPoint, error) {
	result, err := r.cb.Execute(func() (interface{}, error) {
		return r.client.Query(ctx, &qdrant.QueryPoints{
			CollectionName: PoliciesCollection,
			Query:          qdrant.NewQueryDense(vector),
			Filter: &qdrant.Filter{
				Must: []*qdrant.Condition{qdrant.NewMatchKeyword("org_id", orgID)},
			},
			Limit:       qdrant.PtrOf(limit),
			WithPayload: qdrant.NewWithPayload(true),
		})
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, fmt.Errorf("qdrant: circuit open: %w", err)
		}
		return nil, fmt.Errorf("qdrant: query: %w", err)
	}
	return result.([]*qdrant.ScoredPoint), nil
}

// UpsertPolicy indexes (or re-indexes) a single policy document.
func (r *QdrantRepo) UpsertPolicy(ctx context.Context, p model.Policy, vector []float32) error {
	payload := qdrant.NewValueMap(map[string]any{
		"org_id":               p.OrgID,
		"policy_id":            p.PolicyID,
		"policy_name":          p.Title,
		"content":              p.Text,
		"effective_date":       p.EffectiveDate.Format(time.RFC3339),
		"severity":             string(p.Severity),
		"compliance_framework": p.ComplianceFramework,
	})

	_, err := r.cb.Execute(func() (interface{}, error) {
		return r.client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: PoliciesCollection,
			Wait:           qdrant.PtrOf(true),
			Points: []*qdrant.PointStruct{
				{
					Id:      qdrant.NewIDUUID(p.PolicyID),
					Vectors: qdrant.NewVectorsDense(vector),
					Payload: payload,
				},
			},
		})
	})
	if err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}
	return nil
}

// PolicyFromPoint reconstructs a Policy from a Qdrant scored point's payload.
func PolicyFromPoint(p *qdrant.ScoredPoint) model.PolicyMatch {
	payload := p.GetPayload()
	return model.PolicyMatch{
		PolicyID:       payload["policy_id"].GetStringValue(),
		Title:          payload["policy_name"].GetStringValue(),
		Text:           payload["content"].GetStringValue(),
		RelevanceScore: p.GetScore(),
		Severity:       model.Severity(payload["severity"].GetStringValue()),
	}
}
