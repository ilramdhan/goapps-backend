package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// SyncDataRepository implements syncdata.PostgresTargetRepository.
type SyncDataRepository struct {
	db *DB
}

// NewSyncDataRepository creates a new SyncDataRepository instance.
func NewSyncDataRepository(db *DB) *SyncDataRepository {
	return &SyncDataRepository{db: db}
}

// Verify interface compliance at compile time.
var _ syncdata.PostgresTargetRepository = (*SyncDataRepository)(nil)

const batchSize = 500

// UpsertItemConsStockPO batch upserts records into PostgreSQL.
func (r *SyncDataRepository) UpsertItemConsStockPO(
	ctx context.Context,
	items []*syncdata.ItemConsStockPO,
	syncedByJob uuid.UUID,
) (*syncdata.UpsertResult, error) {
	if len(items) == 0 {
		return &syncdata.UpsertResult{}, nil
	}

	now := time.Now()
	result := &syncdata.UpsertResult{TotalRows: len(items)}

	for i := 0; i < len(items); i += batchSize {
		end := min(i+batchSize, len(items))
		batch := items[i:end]

		affected, err := r.upsertBatch(ctx, batch, syncedByJob, now)
		if err != nil {
			return nil, fmt.Errorf("upsert batch %d-%d: %w", i, end, err)
		}
		result.Inserted += affected
	}

	result.Updated = result.TotalRows - result.Inserted
	return result, nil
}

func (r *SyncDataRepository) upsertBatch(
	ctx context.Context,
	items []*syncdata.ItemConsStockPO,
	syncedByJob uuid.UUID,
	syncedAt time.Time,
) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	// Build batch INSERT with ON CONFLICT DO UPDATE.
	const colsPerRow = 29 // placeholders per row (matches INSERT column count)
	var sb strings.Builder
	args := make([]any, 0, len(items)*colsPerRow)

	sb.WriteString(`
		INSERT INTO cst_item_cons_stk_po (
			period, item_code, grade_code, grade_name, item_name, uom,
			cons_qty, cons_val, cons_rate,
			stores_qty, stores_val, stores_rate,
			dept_qty, dept_val, dept_rate,
			last_po_qty1, last_po_val1, last_po_rate1, last_po_dt1,
			last_po_qty2, last_po_val2, last_po_rate2, last_po_dt2,
			last_po_qty3, last_po_val3, last_po_rate3, last_po_dt3,
			synced_at, synced_by_job
		) VALUES `)

	for i, item := range items {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i*29 + 1 // 29 placeholders per row
		sb.WriteString(fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base, base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8,
			base+9, base+10, base+11,
			base+12, base+13, base+14,
			base+15, base+16, base+17, base+18,
			base+19, base+20, base+21, base+22,
			base+23, base+24, base+25, base+26,
			base+27, base+28,
		))

		args = append(args,
			item.Period, item.ItemCode, item.GradeCode,
			nullStr(item.GradeName), nullStr(item.ItemName), nullStr(item.UOM),
			item.ConsQty, item.ConsVal, item.ConsRate,
			item.StoresQty, item.StoresVal, item.StoresRate,
			item.DeptQty, item.DeptVal, item.DeptRate,
			item.LastPOQty1, item.LastPOVal1, item.LastPORate1, item.LastPODt1,
			item.LastPOQty2, item.LastPOVal2, item.LastPORate2, item.LastPODt2,
			item.LastPOQty3, item.LastPOVal3, item.LastPORate3, item.LastPODt3,
			syncedAt, syncedByJob,
		)
	}

	sb.WriteString(` ON CONFLICT (period, item_code, grade_code) DO UPDATE SET
		grade_name = EXCLUDED.grade_name,
		item_name = EXCLUDED.item_name,
		uom = EXCLUDED.uom,
		cons_qty = EXCLUDED.cons_qty,
		cons_val = EXCLUDED.cons_val,
		cons_rate = EXCLUDED.cons_rate,
		stores_qty = EXCLUDED.stores_qty,
		stores_val = EXCLUDED.stores_val,
		stores_rate = EXCLUDED.stores_rate,
		dept_qty = EXCLUDED.dept_qty,
		dept_val = EXCLUDED.dept_val,
		dept_rate = EXCLUDED.dept_rate,
		last_po_qty1 = EXCLUDED.last_po_qty1,
		last_po_val1 = EXCLUDED.last_po_val1,
		last_po_rate1 = EXCLUDED.last_po_rate1,
		last_po_dt1 = EXCLUDED.last_po_dt1,
		last_po_qty2 = EXCLUDED.last_po_qty2,
		last_po_val2 = EXCLUDED.last_po_val2,
		last_po_rate2 = EXCLUDED.last_po_rate2,
		last_po_dt2 = EXCLUDED.last_po_dt2,
		last_po_qty3 = EXCLUDED.last_po_qty3,
		last_po_val3 = EXCLUDED.last_po_val3,
		last_po_rate3 = EXCLUDED.last_po_rate3,
		last_po_dt3 = EXCLUDED.last_po_dt3,
		synced_at = EXCLUDED.synced_at,
		synced_by_job = EXCLUDED.synced_by_job
	`)

	res, err := r.db.ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("execute upsert: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}

	return int(affected), nil //nolint:gosec // RowsAffected is bounded by batch size
}

// ListItemConsStockPO retrieves a paginated list of synced records.
func (r *SyncDataRepository) ListItemConsStockPO(
	ctx context.Context,
	filter syncdata.ListFilter,
) ([]*syncdata.ItemConsStockPO, int64, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Period != "" {
		conditions = append(conditions, fmt.Sprintf("period = $%d", argIdx))
		args = append(args, filter.Period)
		argIdx++
	}
	if filter.ItemCode != "" {
		conditions = append(conditions, fmt.Sprintf("item_code = $%d", argIdx))
		args = append(args, filter.ItemCode)
		argIdx++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(item_code ILIKE $%d OR item_name ILIKE $%d OR grade_name ILIKE $%d OR grade_code ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx,
		))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count.
	var total int64
	countQuery := "SELECT COUNT(*) FROM cst_item_cons_stk_po " + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count synced data: %w", err)
	}

	// Fetch page.
	page := max(filter.Page, 1)
	pageSize := min(max(filter.PageSize, 1), 100)
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`
		SELECT period, item_code, grade_code, grade_name, item_name, uom,
			   cons_qty, cons_val, cons_rate,
			   stores_qty, stores_val, stores_rate,
			   dept_qty, dept_val, dept_rate,
			   last_po_qty1, last_po_val1, last_po_rate1, last_po_dt1,
			   last_po_qty2, last_po_val2, last_po_rate2, last_po_dt2,
			   last_po_qty3, last_po_val3, last_po_rate3, last_po_dt3,
			   synced_at, synced_by_job
		FROM cst_item_cons_stk_po %s
		ORDER BY period DESC, item_code, grade_code
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list synced data: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var items []*syncdata.ItemConsStockPO
	for rows.Next() {
		item, scanErr := r.scanSyncedRow(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan synced row: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rows: %w", err)
	}

	return items, total, nil
}

// GetDistinctPeriods returns all distinct periods.
func (r *SyncDataRepository) GetDistinctPeriods(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT period FROM cst_item_cons_stk_po ORDER BY period DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get distinct periods: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var periods []string
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate period rows: %w", err)
	}

	return periods, nil
}

func (r *SyncDataRepository) scanSyncedRow(rows interface{ Scan(dest ...any) error }) (*syncdata.ItemConsStockPO, error) {
	var item syncdata.ItemConsStockPO
	var (
		gradeName                                             *string
		itemName                                              *string
		uom                                                   *string
		consQty, consVal, consRate                            sql.NullFloat64
		storesQty, storesVal, storesRate                      sql.NullFloat64
		deptQty, deptVal, deptRate                            sql.NullFloat64
		lastPOQty1, lastPOVal1, lastPORate1                   sql.NullFloat64
		lastPOQty2, lastPOVal2, lastPORate2                   sql.NullFloat64
		lastPOQty3, lastPOVal3, lastPORate3                   sql.NullFloat64
		lastPODt1, lastPODt2, lastPODt3                       sql.NullTime
		syncedAt                                              time.Time
		syncedByJob                                           *uuid.UUID
	)

	err := rows.Scan(
		&item.Period, &item.ItemCode, &item.GradeCode,
		&gradeName, &itemName, &uom,
		&consQty, &consVal, &consRate,
		&storesQty, &storesVal, &storesRate,
		&deptQty, &deptVal, &deptRate,
		&lastPOQty1, &lastPOVal1, &lastPORate1, &lastPODt1,
		&lastPOQty2, &lastPOVal2, &lastPORate2, &lastPODt2,
		&lastPOQty3, &lastPOVal3, &lastPORate3, &lastPODt3,
		&syncedAt, &syncedByJob,
	)
	if err != nil {
		return nil, err
	}

	if gradeName != nil {
		item.GradeName = *gradeName
	}
	if itemName != nil {
		item.ItemName = *itemName
	}
	if uom != nil {
		item.UOM = *uom
	}
	item.ConsQty = nullFloat(consQty)
	item.ConsVal = nullFloat(consVal)
	item.ConsRate = nullFloat(consRate)
	item.StoresQty = nullFloat(storesQty)
	item.StoresVal = nullFloat(storesVal)
	item.StoresRate = nullFloat(storesRate)
	item.DeptQty = nullFloat(deptQty)
	item.DeptVal = nullFloat(deptVal)
	item.DeptRate = nullFloat(deptRate)
	item.LastPOQty1 = nullFloat(lastPOQty1)
	item.LastPOVal1 = nullFloat(lastPOVal1)
	item.LastPORate1 = nullFloat(lastPORate1)
	item.LastPOQty2 = nullFloat(lastPOQty2)
	item.LastPOVal2 = nullFloat(lastPOVal2)
	item.LastPORate2 = nullFloat(lastPORate2)
	item.LastPOQty3 = nullFloat(lastPOQty3)
	item.LastPOVal3 = nullFloat(lastPOVal3)
	item.LastPORate3 = nullFloat(lastPORate3)
	item.LastPODt1 = nullTime(lastPODt1)
	item.LastPODt2 = nullTime(lastPODt2)
	item.LastPODt3 = nullTime(lastPODt3)
	item.SyncedAt = &syncedAt
	item.SyncedByJob = syncedByJob

	return &item, nil
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func nullTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
