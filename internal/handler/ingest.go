package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/service"
	"github.com/doki-stack/shared-go/envelope"
	"go.uber.org/zap"
)

type ingestResultDTO struct {
	PolicyID string `json:"policy_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

type ingestResponse struct {
	Results []ingestResultDTO `json:"results"`
}

// IngestPolicy handles POST /mcp/v1/tools/ingest-policy. The body may be a
// single policy document or a JSON array of up to
// service.MaxIngestBatchSize documents. The response is always 200 with a
// per-document results array — one document's schema or embedding failure
// doesn't fail the whole batch, so callers must check each result's
// success field rather than relying on the HTTP status alone.
func (h *PolicyHandler) IngestPolicy(w http.ResponseWriter, r *http.Request) {
	docs, err := decodeIngestRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.PolicyIngestInvalid, err.Error())
		return
	}

	results, err := h.engine.IngestPolicies(r.Context(), docs)
	if err != nil {
		if errors.Is(err, service.ErrBatchTooLarge) {
			writeError(w, r, http.StatusBadRequest, envelope.PolicyIngestInvalid, err.Error())
			return
		}
		h.log.WithContext(r.Context()).Error("policy ingest failed", zap.Error(err))
		writeError(w, r, http.StatusInternalServerError, envelope.InternalError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, ingestResponse{Results: toIngestResultDTOs(results)})
}

// decodeIngestRequest accepts either a single JSON object or a JSON array,
// per the "single object or a batch of up to 100" contract.
func decodeIngestRequest(r *http.Request) ([]model.IngestPolicyRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("failed to read request body")
	}

	var batch []model.IngestPolicyRequest
	if err := json.Unmarshal(body, &batch); err == nil {
		if len(batch) == 0 {
			return nil, errors.New("request body must contain at least one policy document")
		}
		return batch, nil
	}

	var single model.IngestPolicyRequest
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, errors.New("invalid request body: expected a policy document or an array of policy documents")
	}
	return []model.IngestPolicyRequest{single}, nil
}

func toIngestResultDTOs(results []service.IngestResult) []ingestResultDTO {
	out := make([]ingestResultDTO, len(results))
	for i, r := range results {
		dto := ingestResultDTO{PolicyID: r.PolicyID, Success: r.Error == nil}
		if r.Error != nil {
			dto.Error = r.Error.Error()
		}
		out[i] = dto
	}
	return out
}
