// Package repository provides data access for the Policy MCP: PostgreSQL
// (policy_rules, cost_limits), Qdrant (semantic policy documents), and
// Dragonfly (evaluation cache).
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/doki-stack/mcp-policy/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested row does not exist (or is not
// visible to the caller's org under RLS).
var ErrNotFound = errors.New("not found")

// Store wraps a PostgreSQL connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a pool against databaseURL and verifies connectivity.
func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *Store) Close() {
	s.pool.Close()
}

// Ping verifies the database is reachable; used for readiness checks.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// withOrgTx runs fn inside a transaction with app.current_org_id set to
// orgID for the transaction's duration, so PostgreSQL RLS policies (see
// db-schemas migrations/012_enable_rls.sql) scope every statement to that
// org. set_config(..., true) is transaction-local, so pooled connections
// can safely be reused across different orgs between transactions.
func (s *Store) withOrgTx(ctx context.Context, orgID string, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, orgID); err != nil {
		return fmt.Errorf("set org context: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const policyRuleColumns = `id, org_id, name, description, rule_type, rule_config, severity, enabled, created_at, updated_at`

func scanPolicyRule(row pgx.Row) (*model.PolicyRule, error) {
	var r model.PolicyRule
	err := row.Scan(&r.ID, &r.OrgID, &r.Name, &r.Description, &r.RuleType, &r.RuleConfig, &r.Severity, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreatePolicyRule inserts a new policy rule scoped to r.OrgID.
func (s *Store) CreatePolicyRule(ctx context.Context, r *model.PolicyRule) (*model.PolicyRule, error) {
	var out *model.PolicyRule
	err := s.withOrgTx(ctx, r.OrgID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO policy_rules (org_id, name, description, rule_type, rule_config, severity, enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+policyRuleColumns,
			r.OrgID, r.Name, r.Description, r.RuleType, r.RuleConfig, r.Severity, r.Enabled)
		var err error
		out, err = scanPolicyRule(row)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create policy rule: %w", err)
	}
	return out, nil
}

// GetPolicyRule fetches a single policy rule by id, scoped to orgID.
func (s *Store) GetPolicyRule(ctx context.Context, orgID, id string) (*model.PolicyRule, error) {
	var out *model.PolicyRule
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+policyRuleColumns+` FROM policy_rules WHERE id = $1`, id)
		var err error
		out, err = scanPolicyRule(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get policy rule: %w", err)
	}
	return out, nil
}

// ListPolicyRules returns all policy rules for orgID, optionally filtered to
// only enabled rules.
func (s *Store) ListPolicyRules(ctx context.Context, orgID string, enabledOnly bool) ([]model.PolicyRule, error) {
	var out []model.PolicyRule
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		query := `SELECT ` + policyRuleColumns + ` FROM policy_rules`
		if enabledOnly {
			query += ` WHERE enabled = true`
		}
		query += ` ORDER BY name`

		rows, err := tx.Query(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			r, err := scanPolicyRule(rows)
			if err != nil {
				return err
			}
			out = append(out, *r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list policy rules: %w", err)
	}
	return out, nil
}

// UpdatePolicyRule updates a policy rule's mutable fields, scoped to orgID.
func (s *Store) UpdatePolicyRule(ctx context.Context, orgID, id string, r *model.PolicyRule) (*model.PolicyRule, error) {
	var out *model.PolicyRule
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE policy_rules
			SET name = $1, description = $2, rule_type = $3, rule_config = $4, severity = $5, enabled = $6
			WHERE id = $7
			RETURNING `+policyRuleColumns,
			r.Name, r.Description, r.RuleType, r.RuleConfig, r.Severity, r.Enabled, id)
		var err error
		out, err = scanPolicyRule(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update policy rule: %w", err)
	}
	return out, nil
}

// DeletePolicyRule deletes a policy rule by id, scoped to orgID.
func (s *Store) DeletePolicyRule(ctx context.Context, orgID, id string) error {
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM policy_rules WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete policy rule: %w", err)
	}
	return nil
}

const costLimitColumns = `id, org_id, resource_type, limit_amount, remaining_budget, period, reset_at, created_at, updated_at`

func scanCostLimit(row pgx.Row) (*model.CostLimit, error) {
	var c model.CostLimit
	err := row.Scan(&c.ID, &c.OrgID, &c.ResourceType, &c.LimitAmount, &c.RemainingBudget, &c.Period, &c.ResetAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCostLimit fetches the cost limit for a resource type, scoped to orgID.
func (s *Store) GetCostLimit(ctx context.Context, orgID, resourceType string) (*model.CostLimit, error) {
	var out *model.CostLimit
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+costLimitColumns+` FROM cost_limits WHERE resource_type = $1`, resourceType)
		var err error
		out, err = scanCostLimit(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cost limit: %w", err)
	}
	return out, nil
}

// ListCostLimits returns all cost limits for orgID.
func (s *Store) ListCostLimits(ctx context.Context, orgID string) ([]model.CostLimit, error) {
	var out []model.CostLimit
	err := s.withOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+costLimitColumns+` FROM cost_limits ORDER BY resource_type`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			c, err := scanCostLimit(rows)
			if err != nil {
				return err
			}
			out = append(out, *c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list cost limits: %w", err)
	}
	return out, nil
}
