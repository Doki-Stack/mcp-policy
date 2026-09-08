package repository

import (
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/qdrant/go-client/qdrant"
)

func TestParseQdrantURL(t *testing.T) {
	cases := []struct {
		raw      string
		wantHost string
		wantPort int
		wantTLS  bool
		wantErr  bool
	}{
		{raw: "qdrant:6334", wantHost: "qdrant", wantPort: 6334},
		{raw: "grpc://qdrant.data.svc.cluster.local:6334", wantHost: "qdrant.data.svc.cluster.local", wantPort: 6334},
		{raw: "https://qdrant:6334", wantHost: "qdrant", wantPort: 6334, wantTLS: true},
		{raw: "qdrant", wantHost: "qdrant", wantPort: 6334},
		{raw: "://bad", wantErr: true},
	}

	for _, c := range cases {
		host, port, tls, err := parseQdrantURL(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseQdrantURL(%q): expected error, got none", c.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseQdrantURL(%q) failed: %v", c.raw, err)
		}
		if host != c.wantHost || port != c.wantPort || tls != c.wantTLS {
			t.Errorf("parseQdrantURL(%q) = (%q, %d, %v), want (%q, %d, %v)",
				c.raw, host, port, tls, c.wantHost, c.wantPort, c.wantTLS)
		}
	}
}

func TestPolicyFromPoint(t *testing.T) {
	point := &qdrant.ScoredPoint{
		Score: 0.87,
		Payload: qdrant.NewValueMap(map[string]any{
			"policy_id":   "11111111-1111-1111-1111-111111111111",
			"policy_name": "no-public-s3",
			"content":     "S3 buckets must not have public ACLs.",
			"severity":    "critical",
		}),
	}

	got := PolicyFromPoint(point)
	if got.PolicyID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("PolicyID = %q", got.PolicyID)
	}
	if got.Title != "no-public-s3" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Text != "S3 buckets must not have public ACLs." {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Severity != model.SeverityCritical {
		t.Errorf("Severity = %q, want critical", got.Severity)
	}
	if got.RelevanceScore != 0.87 {
		t.Errorf("RelevanceScore = %v, want 0.87", got.RelevanceScore)
	}
}
