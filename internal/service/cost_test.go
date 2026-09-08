package service

import (
	"context"
	"errors"
	"testing"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/doki-stack/mcp-policy/internal/repository"
)

type fakeCostStore struct {
	limit *model.CostLimit
	err   error
}

func (f *fakeCostStore) GetCostLimit(ctx context.Context, orgID, resourceType string) (*model.CostLimit, error) {
	return f.limit, f.err
}

func TestCheckCost_WithinBudget(t *testing.T) {
	store := &fakeCostStore{limit: &model.CostLimit{LimitAmount: 1000, RemainingBudget: 500}}
	checker := NewCostChecker(store)

	resp, err := checker.CheckCost(context.Background(), model.CostCheckRequest{
		OrgID: "org-1", ResourceType: "aws_ec2_instance", EstimatedCost: 200,
	})
	if err != nil {
		t.Fatalf("CheckCost failed: %v", err)
	}
	if !resp.Allowed {
		t.Error("expected allowed = true when estimated cost is under remaining budget")
	}
	if resp.RemainingBudget != 500 || resp.Limit != 1000 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestCheckCost_ExceedsBudget(t *testing.T) {
	store := &fakeCostStore{limit: &model.CostLimit{LimitAmount: 1000, RemainingBudget: 100}}
	checker := NewCostChecker(store)

	resp, err := checker.CheckCost(context.Background(), model.CostCheckRequest{
		OrgID: "org-1", ResourceType: "aws_ec2_instance", EstimatedCost: 200,
	})
	if err != nil {
		t.Fatalf("CheckCost failed: %v", err)
	}
	if resp.Allowed {
		t.Error("expected allowed = false when estimated cost exceeds remaining budget")
	}
}

func TestCheckCost_ExactlyAtBudget(t *testing.T) {
	store := &fakeCostStore{limit: &model.CostLimit{LimitAmount: 1000, RemainingBudget: 200}}
	checker := NewCostChecker(store)

	resp, err := checker.CheckCost(context.Background(), model.CostCheckRequest{
		OrgID: "org-1", ResourceType: "aws_ec2_instance", EstimatedCost: 200,
	})
	if err != nil {
		t.Fatalf("CheckCost failed: %v", err)
	}
	if !resp.Allowed {
		t.Error("expected allowed = true when estimated cost exactly equals remaining budget")
	}
}

func TestCheckCost_NoLimitConfigured(t *testing.T) {
	store := &fakeCostStore{err: repository.ErrNotFound}
	checker := NewCostChecker(store)

	resp, err := checker.CheckCost(context.Background(), model.CostCheckRequest{
		OrgID: "org-1", ResourceType: "aws_lambda_function", EstimatedCost: 999999,
	})
	if err != nil {
		t.Fatalf("CheckCost failed: %v", err)
	}
	if !resp.Allowed {
		t.Error("expected allowed = true when no cost limit is configured for this resource type")
	}
}

func TestCheckCost_DatabaseDownFailsClosed(t *testing.T) {
	store := &fakeCostStore{err: errors.New("connection refused")}
	checker := NewCostChecker(store)

	_, err := checker.CheckCost(context.Background(), model.CostCheckRequest{
		OrgID: "org-1", ResourceType: "aws_ec2_instance", EstimatedCost: 100,
	})
	if !errors.Is(err, ErrFailClosed) {
		t.Fatalf("expected ErrFailClosed when the database is unreachable, got: %v", err)
	}
}
