// Package handler wires HTTP requests to the Policy MCP's service layer.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/service"
	"github.com/doki-stack/shared-go/envelope"
	"github.com/doki-stack/shared-go/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Engine is the slice of *service.PolicyEngine this handler needs.
// Defined here (not in service) so tests can inject a fake without
// standing up a live Qdrant/Ollama.
type Engine interface {
	Evaluate(ctx context.Context, req model.EvaluateRequest) (*model.EvaluateResponse, error)
	IngestPolicies(ctx context.Context, docs []model.IngestPolicyRequest) ([]service.IngestResult, error)
	GetPolicies(ctx context.Context, req model.GetPoliciesRequest) ([]model.PolicyMatch, error)
}

// CostEngine is the slice of *service.CostChecker this handler needs.
type CostEngine interface {
	CheckCost(ctx context.Context, req model.CostCheckRequest) (*model.CostCheckResponse, error)
}

// PolicyHandler serves the Policy MCP's tool endpoints.
type PolicyHandler struct {
	engine     Engine
	costEngine CostEngine
	log        *logger.Logger
}

// NewPolicyHandler wires a PolicyHandler to its service layer and logger.
func NewPolicyHandler(engine Engine, costEngine CostEngine, log *logger.Logger) *PolicyHandler {
	return &PolicyHandler{engine: engine, costEngine: costEngine, log: log}
}

// EvaluatePolicy handles POST /mcp/v1/tools/evaluate-policy.
func (h *PolicyHandler) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	var req model.EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, "invalid request body")
		return
	}
	if err := validateEvaluateRequest(req); err != nil {
		writeError(w, r, http.StatusBadRequest, envelope.BadRequest, err.Error())
		return
	}

	resp, err := h.engine.Evaluate(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrFailClosed) {
			// Per ADR-005: no degraded response here, ever — a plain 503
			// telling the caller to block is the correct behavior, not a bug.
			h.log.WithContext(r.Context()).Warn("policy evaluation unavailable, failing closed", zap.Error(err))
			writeError(w, r, http.StatusServiceUnavailable, envelope.PolicyUnavailable, "policy evaluation unavailable")
			return
		}
		h.log.WithContext(r.Context()).Error("policy evaluation failed", zap.Error(err))
		writeError(w, r, http.StatusInternalServerError, envelope.InternalError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func validateEvaluateRequest(req model.EvaluateRequest) error {
	if _, err := uuid.Parse(req.OrgID); err != nil {
		return errors.New("org_id must be a valid UUID")
	}
	if req.Query == "" {
		return errors.New("query is required")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	env := envelope.New(code, message, envelope.WithContext(r.Context()), envelope.WithRetryable(status == http.StatusServiceUnavailable))
	envelope.WriteJSON(w, status, env)
}
