package service

import (
	"strings"
	"testing"
	"time"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/qdrant/go-client/qdrant"
)

func point(score float32, effectiveDate string) *qdrant.ScoredPoint {
	payload := map[string]any{
		"policy_id":   "id",
		"policy_name": "name",
		"content":     "text",
		"severity":    "medium",
	}
	if effectiveDate != "" {
		payload["effective_date"] = effectiveDate
	}
	return &qdrant.ScoredPoint{
		Score:   score,
		Payload: qdrant.NewValueMap(payload),
	}
}

func TestRerank_PrefersNewerAtSimilarScore(t *testing.T) {
	now := time.Now()
	older := point(0.90, now.AddDate(-2, 0, 0).Format(time.RFC3339))
	newer := point(0.90, now.AddDate(0, 0, -1).Format(time.RFC3339))

	ranked := rerank([]*qdrant.ScoredPoint{older, newer}, 2)
	if len(ranked) != 2 {
		t.Fatalf("len(ranked) = %d, want 2", len(ranked))
	}
	if ranked[0] != newer {
		t.Error("expected the more recent policy to rank first at equal similarity")
	}
}

func TestRerank_HigherScoreCanOutweighRecency(t *testing.T) {
	now := time.Now()
	strongOld := point(0.99, now.AddDate(-3, 0, 0).Format(time.RFC3339))
	weakNew := point(0.10, now.Format(time.RFC3339))

	ranked := rerank([]*qdrant.ScoredPoint{weakNew, strongOld}, 2)
	if ranked[0] != strongOld {
		t.Error("expected the much higher-similarity policy to still rank first")
	}
}

func TestRerank_LimitsResults(t *testing.T) {
	points := make([]*qdrant.ScoredPoint, 10)
	for i := range points {
		points[i] = point(float32(i)/10, "")
	}
	ranked := rerank(points, 5)
	if len(ranked) != 5 {
		t.Errorf("len(ranked) = %d, want 5", len(ranked))
	}
}

func TestRecencyWeight_NoDateReturnsFullWeight(t *testing.T) {
	if got := recencyWeight(time.Now(), time.Time{}); got != 1.0 {
		t.Errorf("recencyWeight with zero date = %v, want 1.0", got)
	}
}

func TestRecencyWeight_DecaysWithAge(t *testing.T) {
	now := time.Now()
	fresh := recencyWeight(now, now.AddDate(0, 0, -1))
	old := recencyWeight(now, now.AddDate(-5, 0, 0))
	if !(fresh > old) {
		t.Errorf("fresh weight %v should exceed old weight %v", fresh, old)
	}
	if old <= 0 {
		t.Errorf("old weight %v should stay positive (floor, not zero)", old)
	}
}

func TestTruncateToTokenBudget_UnderBudgetKeepsAll(t *testing.T) {
	matches := []model.PolicyMatch{
		{Title: "a", Text: "short"},
		{Title: "b", Text: "also short"},
	}
	out := truncateToTokenBudget(matches, MaxContextTokens)
	if len(out) != 2 {
		t.Errorf("len(out) = %d, want 2", len(out))
	}
}

func TestTruncateToTokenBudget_TrimsWhenOverBudget(t *testing.T) {
	big := strings.Repeat("x", 100)
	matches := []model.PolicyMatch{
		{Title: "a", Text: big},
		{Title: "b", Text: big},
		{Title: "c", Text: big},
	}
	// Budget of 10 tokens ≈ 40 chars: first match alone exceeds it and
	// should be truncated with an ellipsis; nothing after it is kept.
	out := truncateToTokenBudget(matches, 10)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if !strings.HasSuffix(out[0].Text, "…") {
		t.Errorf("truncated text should end with an ellipsis, got %q", out[0].Text)
	}
	if max := 40 + len("…"); len(out[0].Text) > max { // 40 chars + ellipsis (3 bytes in UTF-8)
		t.Errorf("truncated text too long: %d bytes, want <= %d", len(out[0].Text), max)
	}
}

func TestBuildConstraints(t *testing.T) {
	matches := []model.PolicyMatch{{Title: "no-public-s3", Text: "S3 buckets must not be public."}}
	got := buildConstraints(matches)
	if len(got) != 1 || !strings.Contains(got[0], "no-public-s3") || !strings.Contains(got[0], "S3 buckets") {
		t.Errorf("buildConstraints = %v", got)
	}
}
