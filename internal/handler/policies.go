package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/service"
	"github.com/doki-stack/shared-go/envelope"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type getPoliciesResponse struct {
	Policies []model.PolicyMatch `json:"policies"`
}

// GetPolicies handles POST /mcp/v1/tools/get-policies.
func (h *PolicyHandler) GetPolicies(w http.ResponseWriter, r *http.Request) {
	var req model.GetPoliciesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "invalid request body")
		return
	}
	if _, err := uuid.Parse(req.OrgID); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "org_id must be a valid UUID")
		return
	}

	matches, err := h.engine.GetPolicies(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMissingQuery):
			writeError(w, r, http.StatusBadRequest, envelope.BadRequest, err.Error())
		case errors.Is(err, service.ErrFailClosed):
			h.log.WithContext(r.Context()).Warn("policy lookup unavailable, failing closed", zap.Error(err))
			writeError(w, r, http.StatusServiceUnavailable, envelope.PolicyUnavailable, "policy lookup unavailable")
		default:
			h.log.WithContext(r.Context()).Error("get-policies failed", zap.Error(err))
			writeError(w, r, http.StatusInternalServerError, envelope.InternalError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, getPoliciesResponse{Policies: matches})
}
