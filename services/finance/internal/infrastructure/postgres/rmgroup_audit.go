// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// Audit action constants for aud_rm_group / aud_rm_group_detail.
const (
	auditActionCreate = "CREATE"
	auditActionUpdate = "UPDATE"
	auditActionDelete = "DELETE"
)

// sqlExecutor is the narrow contract shared by *sql.DB and *sql.Tx. Audit
// helpers accept this so they can run inside a transaction when the caller
// supplies one, or directly on the pool when no transaction is in scope.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertHeadAudit appends one row to aud_rm_group capturing the post-operation
// snapshot of the head. Callers pass the action type (CREATE/UPDATE/DELETE)
// and the actor who performed the operation.
func insertHeadAudit(ctx context.Context, exec sqlExecutor, head *rmgroup.Head, action, changedBy string) error {
	query := `
		INSERT INTO aud_rm_group (
			group_head_id, action,
			group_code, group_name, description, colourant, ci_name,
			cost_percentage, cost_per_kg,
			flag_valuation, flag_marketing, flag_simulation,
			init_val_valuation, init_val_marketing, init_val_simulation,
			is_active, changed_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`
	_, err := exec.ExecContext(ctx, query,
		head.ID(), action,
		head.Code().String(), head.Name(), nullableString(head.Description()),
		nullableString(head.Colourant()), nullableString(head.CIName()),
		head.CostPercentage(), head.CostPerKg(),
		head.FlagValuation().String(), head.FlagMarketing().String(), head.FlagSimulation().String(),
		head.InitValValuation(), head.InitValMarketing(), head.InitValSimulation(),
		head.IsActive(), changedBy,
	)
	if err != nil {
		return fmt.Errorf("insert aud_rm_group: %w", err)
	}
	return nil
}

// insertDetailAudit appends one row to aud_rm_group_detail.
func insertDetailAudit(ctx context.Context, exec sqlExecutor, detail *rmgroup.Detail, action, changedBy string) error {
	query := `
		INSERT INTO aud_rm_group_detail (
			group_detail_id, group_head_id, action,
			item_code, item_name, item_type_code, grade_code, item_grade, uom_code,
			market_percentage, market_value_rp,
			sort_order, is_active, is_dummy, changed_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`
	_, err := exec.ExecContext(ctx, query,
		detail.ID(), detail.HeadID(), action,
		detail.ItemCode().String(),
		nullableString(detail.ItemName()), nullableString(detail.ItemTypeCode()),
		nullableString(detail.GradeCode()), nullableString(detail.ItemGrade()), nullableString(detail.UOMCode()),
		detail.MarketPercentage(), detail.MarketValueRp(),
		detail.SortOrder(), detail.IsActive(), detail.IsDummy(),
		changedBy,
	)
	if err != nil {
		return fmt.Errorf("insert aud_rm_group_detail: %w", err)
	}
	return nil
}

// insertDetailAuditDelete writes a DELETE audit row using a partial snapshot
// (only the ids + actor). Used on soft-delete when the full entity may not be
// readily available — the existing row stays addressable via group_detail_id.
func insertDetailAuditDelete(ctx context.Context, exec sqlExecutor, detail *rmgroup.Detail, changedBy string) error {
	return insertDetailAudit(ctx, exec, detail, auditActionDelete, changedBy)
}
