package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/service"
	"github.com/doki-stack/shared-go/logger"
)

// fakeEngine lets handler tests exercise every response path without a
// live Qdrant/Ollama behind PolicyEngine.
type fakeEngine struct {
	resp *model.EvaluateResponse
	err  error

	ingestResults []service.IngestResult
	ingestErr     error
}

func (f *fakeEngine) Evaluate(ctx context.Context, req model.EvaluateRequest) (*model.EvaluateResponse, error) {
	return f.resp, f.err
}

func (f *fakeEngine) IngestPolicies(ctx context.Context, docs []model.IngestPolicyRequest) ([]service.IngestResult, error) {
	return f.ingestResults, f.ingestErr
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New("test", logger.WithDevelopment(true))
	if err != nil {
		t.Fatalf("logger.New failed: %v", err)
	}
	return log
}

func doEvaluate(t *testing.T, h *PolicyHandler, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/evaluate-policy", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.EvaluatePolicy(rec, req)
	return rec
}

func TestEvaluatePolicy_Success(t *testing.T) {
	want := &model.EvaluateResponse{
		Policies:    []model.PolicyMatch{{PolicyID: "p1", Title: "no-public-s3"}},
		Constraints: []string{"no-public-s3: ..."},
		Violations:  []model.Violation{},
	}
	h := NewPolicyHandler(&fakeEngine{resp: want}, testLogger(t))

	rec := doEvaluate(t, h, model.EvaluateRequest{
		OrgID: "11111111-1111-1111-1111-111111111111",
		Query: "can I make an S3 bucket public?",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got model.EvaluateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Policies) != 1 || got.Policies[0].PolicyID != "p1" {
		t.Errorf("got = %+v", got)
	}
}

func TestEvaluatePolicy_FailClosed(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{err: fmt.Errorf("%w: qdrant down", service.ErrFailClosed)}, testLogger(t))

	rec := doEvaluate(t, h, model.EvaluateRequest{
		OrgID: "11111111-1111-1111-1111-111111111111",
		Query: "can I make an S3 bucket public?",
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env["error_code"] != "POLICY_UNAVAILABLE" {
		t.Errorf("error_code = %v, want POLICY_UNAVAILABLE", env["error_code"])
	}
	if env["retryable"] != true {
		t.Errorf("retryable = %v, want true", env["retryable"])
	}
}

func TestEvaluatePolicy_InternalError(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{err: fmt.Errorf("unexpected panic in re-ranker")}, testLogger(t))

	rec := doEvaluate(t, h, model.EvaluateRequest{
		OrgID: "11111111-1111-1111-1111-111111111111",
		Query: "query",
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestEvaluatePolicy_MissingOrgID(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, testLogger(t))

	rec := doEvaluate(t, h, model.EvaluateRequest{Query: "query"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestEvaluatePolicy_MissingQuery(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, testLogger(t))

	rec := doEvaluate(t, h, model.EvaluateRequest{OrgID: "11111111-1111-1111-1111-111111111111"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestEvaluatePolicy_MalformedJSON(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, testLogger(t))

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/evaluate-policy", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	h.EvaluatePolicy(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
