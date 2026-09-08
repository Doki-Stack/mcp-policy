package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/repository"
)

// CostStore is the slice of *repository.Store this service needs.
type CostStore interface {
	GetCostLimit(ctx context.Context, orgID, resourceType string) (*model.CostLimit, error)
}

// CostChecker implements the check-cost tool.
type CostChecker struct {
	store CostStore
}

// NewCostChecker wires a CostChecker to its PostgreSQL store.
func NewCostChecker(store CostStore) *CostChecker {
	return &CostChecker{store: store}
}

// CheckCost reports whether estimated_cost fits the org's remaining budget
// for resource_type. If PostgreSQL is unreachable this fails closed
// (ErrFailClosed) — the caller cannot verify budget and must block, same
// as a Qdrant outage on the policy path. If PostgreSQL is reachable but no
// cost_limits row exists for this org+resource_type, that means no budget
// policy was configured for it: this is treated as "no constraint" (allowed,
// with Limit/RemainingBudget both 0), not as a fail-closed condition —
// an absent policy is a legitimate configuration state, not an outage.
func (c *CostChecker) CheckCost(ctx context.Context, req model.CostCheckRequest) (*model.CostCheckResponse, error) {
	limit, err := c.store.GetCostLimit(ctx, req.OrgID, req.ResourceType)
	if errors.Is(err, repository.ErrNotFound) {
		return &model.CostCheckResponse{Allowed: true, RemainingBudget: 0, Limit: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: get cost limit: %v", ErrFailClosed, err)
	}

	return &model.CostCheckResponse{
		Allowed:         req.EstimatedCost <= limit.RemainingBudget,
		RemainingBudget: limit.RemainingBudget,
		Limit:           limit.LimitAmount,
	}, nil
}
