package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

// CostAuditLogRepository implements costauditlog.Repository.
type CostAuditLogRepository struct{ db *DB }

// NewCostAuditLogRepository constructs the repository.
func NewCostAuditLogRepository(db *DB) *CostAuditLogRepository {
	return &CostAuditLogRepository{db: db}
}

var _ costauditlog.Repository = (*CostAuditLogRepository)(nil)

// Emit appends a row. Validation happens here so callers can fire-and-forget;
// the DB triggers (000215) enforce immutability after insert.
func (r *CostAuditLogRepository) Emit(ctx context.Context, in costauditlog.NewInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	const q = `
		INSERT INTO cost_audit_log (
			cal_entity_type, cal_entity_id, cal_operation,
			cal_before_data, cal_after_data, cal_user_id, cal_performed_at
		) VALUES ($1, $2, $3, NULLIF($4,'')::jsonb, NULLIF($5,'')::jsonb, $6, NOW())`
	if _, err := r.db.ExecContext(ctx, q,
		in.EntityType, in.EntityID, in.Operation, in.BeforeData, in.AfterData, in.UserID,
	); err != nil {
		return fmt.Errorf("emit cost_audit_log: %w", err)
	}
	return nil
}

// List returns a filtered paginated list of audit rows.
func (r *CostAuditLogRepository) List(ctx context.Context, f costauditlog.Filter) ([]*costauditlog.Log, int64, error) {
	where := "FROM cost_audit_log WHERE 1=1"
	args := []any{}
	idx := 1
	if f.EntityType != "" {
		where += fmt.Sprintf(` AND cal_entity_type=$%d`, idx)
		args = append(args, f.EntityType)
		idx++
	}
	if f.EntityID > 0 {
		where += fmt.Sprintf(` AND cal_entity_id=$%d`, idx)
		args = append(args, f.EntityID)
		idx++
	}
	if f.UserID != "" {
		where += fmt.Sprintf(` AND cal_user_id=$%d`, idx)
		args = append(args, f.UserID)
		idx++
	}
	if f.Operation != "" {
		where += fmt.Sprintf(` AND cal_operation=$%d`, idx)
		args = append(args, f.Operation)
		idx++
	}
	if f.FromDate != "" {
		where += fmt.Sprintf(` AND cal_performed_at >= $%d::date`, idx)
		args = append(args, f.FromDate)
		idx++
	}
	if f.ToDate != "" {
		where += fmt.Sprintf(` AND cal_performed_at < ($%d::date + INTERVAL '1 day')`, idx)
		args = append(args, f.ToDate)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_audit_log: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT cal_log_id,cal_entity_type,cal_entity_id,cal_operation,
		COALESCE(cal_before_data::text,''),COALESCE(cal_after_data::text,''),
		cal_user_id,cal_performed_at
		` + where + fmt.Sprintf(` ORDER BY cal_performed_at DESC, cal_log_id DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_audit_log: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costauditlog.Log{}
	for rows.Next() {
		l := &costauditlog.Log{}
		var performedAt time.Time
		var before, after string
		if sErr := rows.Scan(&l.LogID, &l.EntityType, &l.EntityID, &l.Operation, &before, &after, &l.UserID, &performedAt); sErr != nil {
			return nil, 0, fmt.Errorf("scan cost_audit_log: %w", sErr)
		}
		l.BeforeData = strings.TrimSpace(before)
		l.AfterData = strings.TrimSpace(after)
		l.PerformedAt = performedAt
		out = append(out, l)
	}
	return out, total, rows.Err()
}
