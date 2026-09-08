package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// EvaluationCacheTTL is how long a policy evaluation result is cached.
const EvaluationCacheTTL = 24 * time.Hour

// CacheRepo wraps Dragonfly (Redis-protocol) for caching policy evaluation
// results. Unlike QdrantRepo, the cache is fail-open (see
// db-schemas/docs/implementation-plan/06-non-pg-data-stores.md): a cache
// miss or error simply means the caller falls through to Qdrant, so methods
// here return plain errors and it is the caller's job to treat them as
// non-fatal.
type CacheRepo struct {
	client *redis.Client
}

// NewCacheRepo connects to addr, which may be "host:port" or a
// "redis://" URL.
func NewCacheRepo(addr string) (*CacheRepo, error) {
	opts, err := parseRedisAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("cache: parse addr: %w", err)
	}
	return &CacheRepo{client: redis.NewClient(opts)}, nil
}

func parseRedisAddr(addr string) (*redis.Options, error) {
	if strings.Contains(addr, "://") {
		return redis.ParseURL(addr)
	}
	return &redis.Options{Addr: addr}, nil
}

// Close closes the underlying connection pool.
func (c *CacheRepo) Close() error {
	return c.client.Close()
}

// Ping checks connectivity.
func (c *CacheRepo) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// QueryHash returns the cache key suffix for a normalized evaluate-policy
// query: SHA-256 of "query|resource_type|environment".
func QueryHash(query, resourceType, environment string) string {
	sum := sha256.Sum256([]byte(query + "|" + resourceType + "|" + environment))
	return hex.EncodeToString(sum[:])
}

func evaluationCacheKey(orgID, queryHash string) string {
	return fmt.Sprintf("policy:%s:%s", orgID, queryHash)
}

// GetEvaluation returns a cached evaluation result. found is false on a
// cache miss; err is non-nil only for an actual Dragonfly/decode failure —
// callers must treat both as "fall through to Qdrant", not fail closed.
func (c *CacheRepo) GetEvaluation(ctx context.Context, orgID, queryHash string, out interface{}) (found bool, err error) {
	raw, err := c.client.Get(ctx, evaluationCacheKey(orgID, queryHash)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cache: get: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false, fmt.Errorf("cache: decode: %w", err)
	}
	return true, nil
}

// SetEvaluation caches an evaluation result for EvaluationCacheTTL.
func (c *CacheRepo) SetEvaluation(ctx context.Context, orgID, queryHash string, value interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: encode: %w", err)
	}
	if err := c.client.Set(ctx, evaluationCacheKey(orgID, queryHash), raw, EvaluationCacheTTL).Err(); err != nil {
		return fmt.Errorf("cache: set: %w", err)
	}
	return nil
}

// InvalidateOrg deletes every cached evaluation for orgID. Called after a
// policy is ingested/updated so stale evaluations aren't served.
func (c *CacheRepo) InvalidateOrg(ctx context.Context, orgID string) error {
	pattern := fmt.Sprintf("policy:%s:*", orgID)
	var cursor uint64
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("cache: scan: %w", err)
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache: del: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
