package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroutingrule"
)

// CostRoutingRuleRepository implements costroutingrule.Repository.
type CostRoutingRuleRepository struct{ db *DB }

// NewCostRoutingRuleRepository constructs the repository.
func NewCostRoutingRuleRepository(db *DB) *CostRoutingRuleRepository {
	return &CostRoutingRuleRepository{db: db}
}

var _ costroutingrule.Repository = (*CostRoutingRuleRepository)(nil)

const crrCols = `crr_rule_id,crr_priority,crr_condition::text,crr_action_type,crr_action_target,crr_is_active,crr_created_by,crr_created_at`

// Create inserts a new active rule.
func (r *CostRoutingRuleRepository) Create(ctx context.Context, rule *costroutingrule.Rule) error {
	const q = `
		INSERT INTO cost_routing_rule (
			crr_priority, crr_condition, crr_action_type, crr_action_target,
			crr_is_active, crr_created_by, crr_created_at
		) VALUES ($1, $2::jsonb, $3, NULLIF($4,''), $5, $6, $7)
		RETURNING crr_rule_id`
	if err := r.db.QueryRowContext(ctx, q,
		rule.Priority, rule.Condition, rule.ActionType, rule.ActionTarget,
		rule.IsActive, rule.CreatedBy, rule.CreatedAt,
	).Scan(&rule.RuleID); err != nil {
		return fmt.Errorf("insert cost_routing_rule: %w", err)
	}
	return nil
}

// GetByID loads one rule.
func (r *CostRoutingRuleRepository) GetByID(ctx context.Context, id int32) (*costroutingrule.Rule, error) {
	q := `SELECT ` + crrCols + ` FROM cost_routing_rule WHERE crr_rule_id=$1`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanCrrRow(row)
}

// Update mutates editable fields.
func (r *CostRoutingRuleRepository) Update(ctx context.Context, rule *costroutingrule.Rule) error {
	const q = `
		UPDATE cost_routing_rule SET
			crr_priority=$2, crr_condition=$3::jsonb, crr_action_type=$4,
			crr_action_target=NULLIF($5,''), crr_is_active=$6
		WHERE crr_rule_id=$1`
	res, err := r.db.ExecContext(ctx, q,
		rule.RuleID, rule.Priority, rule.Condition, rule.ActionType, rule.ActionTarget, rule.IsActive,
	)
	if err != nil {
		return fmt.Errorf("update cost_routing_rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costroutingrule.ErrNotFound
	}
	return nil
}

// Delete removes a rule (hard delete is fine — rules are admin-managed and audit log captures changes).
func (r *CostRoutingRuleRepository) Delete(ctx context.Context, id int32) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cost_routing_rule WHERE crr_rule_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete cost_routing_rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costroutingrule.ErrNotFound
	}
	return nil
}

// List returns a paginated list ordered by priority ASC (first-match-wins evaluation order).
func (r *CostRoutingRuleRepository) List(ctx context.Context, f costroutingrule.Filter) ([]*costroutingrule.Rule, int64, error) {
	where := "FROM cost_routing_rule WHERE 1=1"
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND crr_is_active=TRUE`
	case filterInactive:
		where += ` AND crr_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_routing_rule: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + crrCols + ` ` + where + fmt.Sprintf(` ORDER BY crr_priority ASC, crr_rule_id ASC LIMIT $%d OFFSET $%d`, 1, 2)
	rows, err := r.db.QueryContext(ctx, q, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_routing_rule: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costroutingrule.Rule{}
	for rows.Next() {
		rule, sErr := scanCrrRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		out = append(out, rule)
	}
	return out, total, rows.Err()
}

// =============================================================================
// scanners
// =============================================================================

func scanCrrRow(row *sql.Row) (*costroutingrule.Rule, error) {
	r, err := scanCrr(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costroutingrule.ErrNotFound
	}
	return r, err
}

func scanCrrRows(rows *sql.Rows) (*costroutingrule.Rule, error) {
	return scanCrr(rows.Scan)
}

func scanCrr(scan func(...any) error) (*costroutingrule.Rule, error) {
	r := &costroutingrule.Rule{}
	var target sql.NullString
	var createdAt time.Time
	if err := scan(&r.RuleID, &r.Priority, &r.Condition, &r.ActionType, &target, &r.IsActive, &r.CreatedBy, &createdAt); err != nil {
		return nil, fmt.Errorf("scan cost_routing_rule: %w", err)
	}
	r.ActionTarget = target.String
	r.CreatedAt = createdAt
	return r, nil
}
