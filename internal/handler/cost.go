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

// CheckCost handles POST /mcp/v1/tools/check-cost.
func (h *PolicyHandler) CheckCost(w http.ResponseWriter, r *http.Request) {
	var req model.CostCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "invalid request body")
		return
	}
	if _, err := uuid.Parse(req.OrgID); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "org_id must be a valid UUID")
		return
	}
	if req.ResourceType == "" {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "resource_type is required")
		return
	}

	resp, err := h.costEngine.CheckCost(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrFailClosed) {
			h.log.WithContext(r.Context()).Warn("cost check unavailable, failing closed", zap.Error(err))
			writeError(w, r, http.StatusServiceUnavailable, envelope.PolicyUnavailable, "cost check unavailable")
			return
		}
		h.log.WithContext(r.Context()).Error("check-cost failed", zap.Error(err))
		writeError(w, r, http.StatusInternalServerError, envelope.InternalError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
