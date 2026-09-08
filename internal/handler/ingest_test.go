package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/service"
)

func doIngest(t *testing.T, h *PolicyHandler, rawBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/tools/ingest-policy", bytes.NewReader([]byte(rawBody)))
	rec := httptest.NewRecorder()
	h.IngestPolicy(rec, req)
	return rec
}

const oneDocJSON = `{
	"org_id": "11111111-1111-1111-1111-111111111111",
	"policy_id": "22222222-2222-2222-2222-222222222222",
	"title": "no-public-s3",
	"text": "S3 buckets must not have public ACLs."
}`

func TestIngestPolicy_SingleObject(t *testing.T) {
	fake := &fakeEngine{ingestResults: []service.IngestResult{{PolicyID: "22222222-2222-2222-2222-222222222222"}}}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doIngest(t, h, oneDocJSON)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != 1 || !resp.Results[0].Success {
		t.Errorf("resp = %+v", resp)
	}
}

func TestIngestPolicy_BatchArray(t *testing.T) {
	fake := &fakeEngine{ingestResults: []service.IngestResult{
		{PolicyID: "a"},
		{PolicyID: "b", Error: errors.New("boom")},
	}}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doIngest(t, h, "["+oneDocJSON+","+oneDocJSON+"]")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Error("results[0] should be successful")
	}
	if resp.Results[1].Success || resp.Results[1].Error != "boom" {
		t.Errorf("results[1] = %+v, want failure with error \"boom\"", resp.Results[1])
	}
}

func TestIngestPolicy_EmptyArray(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	rec := doIngest(t, h, "[]")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestIngestPolicy_MalformedJSON(t *testing.T) {
	h := NewPolicyHandler(&fakeEngine{}, &fakeEngine{}, testLogger(t))

	rec := doIngest(t, h, "{not json")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestIngestPolicy_BatchTooLarge(t *testing.T) {
	fake := &fakeEngine{ingestErr: service.ErrBatchTooLarge}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doIngest(t, h, oneDocJSON)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env["error_code"] != "POLICY_INGEST_INVALID" {
		t.Errorf("error_code = %v, want POLICY_INGEST_INVALID", env["error_code"])
	}
}

func TestIngestPolicy_InternalError(t *testing.T) {
	fake := &fakeEngine{ingestErr: errors.New("qdrant exploded")}
	h := NewPolicyHandler(fake, fake, testLogger(t))

	rec := doIngest(t, h, oneDocJSON)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}
