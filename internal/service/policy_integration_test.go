//go:build integration

// This file requires Docker and is excluded from normal `go test ./...`
// runs (build tag "integration"). Run it explicitly:
//
//	go test -tags=integration ./internal/service/... -run TestEvaluate_FailsClosedWhenQdrantGoesDown -v
//
// It has NOT been executed in the sandbox this project was built in (no
// Docker available there) — it compiles and type-checks
// (`go build -tags=integration ./...` was verified), but has not been run
// against a real container. Run it once Docker is available and fix
// forward if anything doesn't hold up in practice.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/repository"
	tcqdrant "github.com/testcontainers/testcontainers-go/modules/qdrant"
)

// newTestCacheRepoForIntegration mirrors repository.newTestCacheRepo (an
// unexported test helper in a different package, so not reusable here
// directly): an in-process fake Redis, no live Dragonfly needed — this
// test is about the Qdrant fail-closed path, not the cache.
func newTestCacheRepoForIntegration(t *testing.T) *repository.CacheRepo {
	t.Helper()
	mr := miniredis.RunT(t)
	repo, err := repository.NewCacheRepo(mr.Addr())
	if err != nil {
		t.Fatalf("NewCacheRepo failed: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

// mockEmbeddingServer stands in for Ollama: this test is about the Qdrant
// fail-closed path specifically, not embedding generation, so a real
// nomic-embed-text isn't needed here (same reasoning as embedding_test.go).
func mockEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float32, EmbeddingDimensions)
		for i := range vec {
			vec[i] = float32(i) / float32(len(vec))
		}
		json.NewEncoder(w).Encode(embeddingResponse{Embedding: vec})
	}))
}

// TestEvaluate_FailsClosedWhenQdrantGoesDown is A16's integration test:
// a real Qdrant container backs a real QdrantRepo/PolicyEngine, a policy
// is ingested and evaluated successfully, the container is then stopped
// mid-test to simulate an outage, and the next Evaluate call must fail
// closed (wrap service.ErrFailClosed) rather than degrade or panic.
func TestEvaluate_FailsClosedWhenQdrantGoesDown(t *testing.T) {
	ctx := context.Background()

	qdrantContainer, err := tcqdrant.Run(ctx, "qdrant/qdrant:v1.11.0")
	if err != nil {
		t.Fatalf("start qdrant container: %v", err)
	}
	t.Cleanup(func() {
		if err := qdrantContainer.Terminate(context.Background()); err != nil {
			t.Logf("terminate qdrant container: %v", err)
		}
	})

	grpcEndpoint, err := qdrantContainer.GRPCEndpoint(ctx)
	if err != nil {
		t.Fatalf("get qdrant grpc endpoint: %v", err)
	}

	qdrantRepo, err := repository.NewQdrantRepo(repository.QdrantConfig{
		URL:              grpcEndpoint,
		BreakerThreshold: 3,
		BreakerTimeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewQdrantRepo failed: %v", err)
	}
	t.Cleanup(func() { qdrantRepo.Close() })

	if err := qdrantRepo.EnsureCollection(ctx); err != nil {
		t.Fatalf("EnsureCollection failed: %v", err)
	}

	embeddingServer := mockEmbeddingServer(t)
	t.Cleanup(embeddingServer.Close)
	embedder := NewEmbeddingService(embeddingServer.URL, "nomic-embed-text", 5*time.Second)

	cacheRepo := newTestCacheRepoForIntegration(t)
	engine := NewPolicyEngine(embedder, qdrantRepo, cacheRepo)

	orgID := "11111111-1111-1111-1111-111111111111"
	ingestResults, err := engine.IngestPolicies(ctx, []model.IngestPolicyRequest{{
		OrgID:    orgID,
		PolicyID: "22222222-2222-2222-2222-222222222222",
		Title:    "no-public-s3",
		Text:     "S3 buckets must not have public ACLs.",
	}})
	if err != nil {
		t.Fatalf("IngestPolicies failed: %v", err)
	}
	if len(ingestResults) != 1 || ingestResults[0].Error != nil {
		t.Fatalf("ingest result: %+v", ingestResults)
	}

	// Sanity check: evaluate-policy works end-to-end against the live
	// container before we pull the rug out.
	if _, err := engine.Evaluate(ctx, model.EvaluateRequest{OrgID: orgID, Query: "can I make an S3 bucket public?"}); err != nil {
		t.Fatalf("Evaluate should succeed while Qdrant is up: %v", err)
	}

	// Kill Qdrant mid-test.
	stopTimeout := 5 * time.Second
	if err := qdrantContainer.Stop(ctx, &stopTimeout); err != nil {
		t.Fatalf("stop qdrant container: %v", err)
	}

	_, err = engine.Evaluate(ctx, model.EvaluateRequest{OrgID: orgID, Query: "can I make an S3 bucket public?"})
	if err == nil {
		t.Fatal("Evaluate should fail once Qdrant is down, not silently degrade")
	}
	if !errors.Is(err, ErrFailClosed) {
		t.Fatalf("Evaluate error should wrap ErrFailClosed once Qdrant is down, got: %v", err)
	}
}
