package repository

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// newTestCacheRepo spins up an in-process fake Redis (miniredis) so cache
// behavior is exercised for real, without needing a live Dragonfly.
func newTestCacheRepo(t *testing.T) *CacheRepo {
	t.Helper()
	mr := miniredis.RunT(t)
	repo, err := NewCacheRepo(mr.Addr())
	if err != nil {
		t.Fatalf("NewCacheRepo failed: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

type testEval struct {
	Policies []string `json:"policies"`
}

func TestCacheRepo_SetGetEvaluation(t *testing.T) {
	repo := newTestCacheRepo(t)
	ctx := context.Background()
	orgID := "org-1"
	hash := QueryHash("s3 public access", "aws_s3_bucket", "prod")

	found, err := repo.GetEvaluation(ctx, orgID, hash, &testEval{})
	if err != nil {
		t.Fatalf("GetEvaluation (miss) failed: %v", err)
	}
	if found {
		t.Fatal("expected cache miss before Set")
	}

	want := testEval{Policies: []string{"no-public-s3"}}
	if err := repo.SetEvaluation(ctx, orgID, hash, want); err != nil {
		t.Fatalf("SetEvaluation failed: %v", err)
	}

	var got testEval
	found, err = repo.GetEvaluation(ctx, orgID, hash, &got)
	if err != nil {
		t.Fatalf("GetEvaluation (hit) failed: %v", err)
	}
	if !found {
		t.Fatal("expected cache hit after Set")
	}
	if len(got.Policies) != 1 || got.Policies[0] != "no-public-s3" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCacheRepo_InvalidateOrg(t *testing.T) {
	repo := newTestCacheRepo(t)
	ctx := context.Background()

	if err := repo.SetEvaluation(ctx, "org-a", "hash1", testEval{Policies: []string{"p1"}}); err != nil {
		t.Fatalf("SetEvaluation org-a/hash1 failed: %v", err)
	}
	if err := repo.SetEvaluation(ctx, "org-a", "hash2", testEval{Policies: []string{"p2"}}); err != nil {
		t.Fatalf("SetEvaluation org-a/hash2 failed: %v", err)
	}
	if err := repo.SetEvaluation(ctx, "org-b", "hash1", testEval{Policies: []string{"p3"}}); err != nil {
		t.Fatalf("SetEvaluation org-b/hash1 failed: %v", err)
	}

	if err := repo.InvalidateOrg(ctx, "org-a"); err != nil {
		t.Fatalf("InvalidateOrg failed: %v", err)
	}

	var out testEval
	if found, _ := repo.GetEvaluation(ctx, "org-a", "hash1", &out); found {
		t.Error("org-a/hash1 should be invalidated")
	}
	if found, _ := repo.GetEvaluation(ctx, "org-a", "hash2", &out); found {
		t.Error("org-a/hash2 should be invalidated")
	}
	if found, err := repo.GetEvaluation(ctx, "org-b", "hash1", &out); err != nil || !found {
		t.Error("org-b/hash1 should be unaffected by org-a's invalidation")
	}
}

func TestQueryHash_Deterministic(t *testing.T) {
	h1 := QueryHash("query", "aws_s3_bucket", "prod")
	h2 := QueryHash("query", "aws_s3_bucket", "prod")
	if h1 != h2 {
		t.Error("QueryHash should be deterministic for identical inputs")
	}
	if h3 := QueryHash("query", "aws_s3_bucket", "staging"); h3 == h1 {
		t.Error("QueryHash should differ when environment differs")
	}
}

func TestParseRedisAddr(t *testing.T) {
	opts, err := parseRedisAddr("dragonfly:6379")
	if err != nil {
		t.Fatalf("parseRedisAddr failed: %v", err)
	}
	if opts.Addr != "dragonfly:6379" {
		t.Errorf("Addr = %q, want dragonfly:6379", opts.Addr)
	}

	opts, err = parseRedisAddr("redis://user:pass@dragonfly:6379/0")
	if err != nil {
		t.Fatalf("parseRedisAddr (URL) failed: %v", err)
	}
	if opts.Addr != "dragonfly:6379" {
		t.Errorf("Addr = %q, want dragonfly:6379", opts.Addr)
	}
}
