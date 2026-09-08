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

func doCheckCost(t *testing.T, h *PolicyHandler, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/check-cost", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.CheckCost(rec, req)
	return rec
}

func TestCheckCost_Success(t *testing.T) {
	fake := &fakeEngine{costResp: &model.CostCheckResponse{Allowed: true, RemainingBudget: 500, Limit: 1000}}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doCheckCost(t, h, model.CostCheckRequest{
		OrgID: "11111111-1111-1111-1111-111111111111", ResourceType: "aws_ec2_instance", EstimatedCost: 100,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp model.CostCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Allowed || resp.RemainingBudget != 500 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestCheckCost_MissingOrgID(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	rec := doCheckCost(t, h, model.CostCheckRequest{ResourceType: "aws_ec2_instance", EstimatedCost: 100})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCheckCost_MissingResourceType(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	rec := doCheckCost(t, h, model.CostCheckRequest{OrgID: "11111111-1111-1111-1111-111111111111", EstimatedCost: 100})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCheckCost_FailClosed(t *testing.T) {
	fake := &fakeEngine{costErr: fmt.Errorf("%w: postgres down", service.ErrFailClosed)}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doCheckCost(t, h, model.CostCheckRequest{
		OrgID: "11111111-1111-1111-1111-111111111111", ResourceType: "aws_ec2_instance", EstimatedCost: 100,
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCheckCost_MalformedJSON(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/check-cost", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	h.CheckCost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
