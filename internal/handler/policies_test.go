package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/service"
)

func doGetPolicies(t *testing.T, h *PolicyHandler, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/get-policies", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.GetPolicies(rec, req)
	return rec
}

func TestGetPolicies_Success(t *testing.T) {
	fake := &fakeEngine{getPoliciesResults: []model.PolicyMatch{{PolicyID: "p1", Title: "no-public-s3"}}}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doGetPolicies(t, h, model.GetPoliciesRequest{
		OrgID: "11111111-1111-1111-1111-111111111111",
		Query: "s3 buckets",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp getPoliciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Policies) != 1 || resp.Policies[0].PolicyID != "p1" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestGetPolicies_MissingOrgID(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	rec := doGetPolicies(t, h, model.GetPoliciesRequest{Query: "s3 buckets"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPolicies_MissingQueryAndResourceType(t *testing.T) {
	fake := &fakeEngine{getPoliciesErr: service.ErrMissingQuery}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doGetPolicies(t, h, model.GetPoliciesRequest{OrgID: "11111111-1111-1111-1111-111111111111"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPolicies_FailClosed(t *testing.T) {
	fake := &fakeEngine{getPoliciesErr: fmt.Errorf("%w: qdrant down", service.ErrFailClosed)}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doGetPolicies(t, h, model.GetPoliciesRequest{
		OrgID: "11111111-1111-1111-1111-111111111111",
		Query: "s3 buckets",
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPolicies_MalformedJSON(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/get-policies", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	h.GetPolicies(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
