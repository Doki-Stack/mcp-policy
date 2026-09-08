// Package model holds the Policy MCP's domain types: the structured PostgreSQL
// records (policy_rules, cost_limits) and the semantic policy documents indexed
// in Qdrant, plus the request/response shapes for the MCP tool endpoints.
package model

import (
	"encoding/json"
	"time"
)

// Severity mirrors the PostgreSQL severity_level enum.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// BudgetPeriod mirrors the PostgreSQL budget_period enum.
type BudgetPeriod string

const (
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
)

// PolicyRule is a row in the PostgreSQL policy_rules table — a structured,
// non-semantic rule definition (distinct from the semantic policy documents
// stored in Qdrant; see Policy below).
type PolicyRule struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	RuleType    string          `json:"rule_type"`
	RuleConfig  json.RawMessage `json:"rule_config"`
	Severity    Severity        `json:"severity"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CostLimit is a row in the PostgreSQL cost_limits table.
type CostLimit struct {
	ID              string       `json:"id"`
	OrgID           string       `json:"org_id"`
	ResourceType    string       `json:"resource_type"`
	LimitAmount     float64      `json:"limit_amount"`
	RemainingBudget float64      `json:"remaining_budget"`
	Period          BudgetPeriod `json:"period"`
	ResetAt         time.Time    `json:"reset_at"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// Policy is a semantic policy document — the unit indexed into and retrieved
// from the Qdrant `policies` collection (see
// db-schemas/docs/implementation-plan/06-non-pg-data-stores.md for the point
// schema this maps to).
type Policy struct {
	PolicyID            string    `json:"policy_id"`
	OrgID               string    `json:"org_id"`
	Title               string    `json:"title"`
	Text                string    `json:"text"`
	ComplianceFramework string    `json:"compliance_framework,omitempty"`
	EffectiveDate       time.Time `json:"effective_date"`
	Severity            Severity  `json:"severity"`
}

// EvaluateRequest is the input to POST /mcp/v1/tools/evaluate-policy.
type EvaluateRequest struct {
	OrgID        string `json:"org_id"`
	Query        string `json:"query"`
	ResourceType string `json:"resource_type,omitempty"`
	Environment  string `json:"environment,omitempty"`
}

// PolicyMatch is one retrieved policy with its relevance ranking.
type PolicyMatch struct {
	PolicyID       string   `json:"policy_id"`
	Title          string   `json:"title"`
	Text           string   `json:"text"`
	RelevanceScore float32  `json:"relevance_score"`
	Severity       Severity `json:"severity"`
}

// Violation flags a specific policy the request appears to violate.
type Violation struct {
	PolicyID    string   `json:"policy_id"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
}

// EvaluateResponse is the output of evaluate-policy.
type EvaluateResponse struct {
	Policies    []PolicyMatch `json:"policies"`
	Constraints []string      `json:"constraints"`
	Violations  []Violation   `json:"violations"`
}

// IngestPolicyRequest is one entry of POST /mcp/v1/tools/ingest-policy
// (the endpoint accepts either a single object or a batch of up to 100).
type IngestPolicyRequest struct {
	OrgID               string    `json:"org_id"`
	PolicyID            string    `json:"policy_id"`
	Title               string    `json:"title"`
	Text                string    `json:"text"`
	ComplianceFramework string    `json:"compliance_framework,omitempty"`
	EffectiveDate       time.Time `json:"effective_date,omitempty"`
}

// GetPoliciesRequest is the input to POST /mcp/v1/tools/get-policies.
type GetPoliciesRequest struct {
	OrgID        string `json:"org_id"`
	Query        string `json:"query,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
}

// CostCheckRequest is the input to the check-cost tool.
type CostCheckRequest struct {
	OrgID         string  `json:"org_id"`
	ResourceType  string  `json:"resource_type"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// CostCheckResponse is the output of the check-cost tool.
type CostCheckResponse struct {
	Allowed         bool    `json:"allowed"`
	RemainingBudget float64 `json:"remaining_budget"`
	Limit           float64 `json:"limit"`
}
