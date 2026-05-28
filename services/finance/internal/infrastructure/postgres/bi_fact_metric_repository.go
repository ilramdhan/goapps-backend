package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// BiFactMetricRepository implements factmetric.Repository.
type BiFactMetricRepository struct {
	db *DB
}

// NewBiFactMetricRepository constructs a BiFactMetricRepository.
func NewBiFactMetricRepository(db *DB) *BiFactMetricRepository {
	return &BiFactMetricRepository{db: db}
}

var _ factmetric.Repository = (*BiFactMetricRepository)(nil)

// GetDistincts returns distinct type/group_1/group_2/group_3 values for admin form dropdowns.
func (r *BiFactMetricRepository) GetDistincts(ctx context.Context, scope factmetric.DistinctScope) (factmetric.DistinctValues, error) {
	out := factmetric.DistinctValues{}

	if err := r.collectDistinct(ctx, "SELECT DISTINCT type FROM bi_fact_metric WHERE is_active ORDER BY type", nil, &out.Types); err != nil {
		return out, err
	}
	if scope.Type == "" {
		return out, nil
	}
	if err := r.collectDistinct(ctx,
		"SELECT DISTINCT group_1 FROM bi_fact_metric WHERE is_active AND type = $1 ORDER BY group_1",
		[]any{scope.Type}, &out.Group1s); err != nil {
		return out, err
	}
	if err := r.collectDistinct(ctx,
		"SELECT DISTINCT group_2 FROM bi_fact_metric WHERE is_active AND type = $1 AND group_2 IS NOT NULL ORDER BY group_2",
		[]any{scope.Type}, &out.Group2s); err != nil {
		return out, err
	}
	if err := r.collectDistinct(ctx,
		"SELECT DISTINCT group_3 FROM bi_fact_metric WHERE is_active AND type = $1 AND group_3 IS NOT NULL ORDER BY group_3",
		[]any{scope.Type}, &out.Group3s); err != nil {
		return out, err
	}
	if err := r.collectDistinct(ctx,
		"SELECT DISTINCT dimension_key FROM bi_fact_metric WHERE is_active AND type = $1 AND dimension_key <> '' ORDER BY dimension_key",
		[]any{scope.Type}, &out.DimensionKeys); err != nil {
		return out, err
	}
	return out, nil
}

// collectDistinct fills *dst with single-column string scan results.
func (r *BiFactMetricRepository) collectDistinct(ctx context.Context, q string, args []any, dst *[]string) error {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("query distincts: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return fmt.Errorf("scan distinct: %w", err)
		}
		*dst = append(*dst, s)
	}
	return rows.Err()
}

// LatestPeriod returns MAX(periode_date) for the scope, or a zero time when no rows match.
func (r *BiFactMetricRepository) LatestPeriod(ctx context.Context, factType, group1, grain string) (time.Time, error) {
	const q = `SELECT MAX(periode_date) FROM bi_fact_metric
WHERE is_active AND type = $1 AND periode_grain = $3 AND ($2 = '' OR group_1 = $2)`
	var t sql.NullTime
	if err := r.db.QueryRowContext(ctx, q, factType, group1, grain).Scan(&t); err != nil {
		return time.Time{}, fmt.Errorf("query latest period: %w", err)
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, nil
}

// QueryAggregate executes a planned SQL+args bundle and scans into AggRow slice.
//
// The planner is responsible for emitting a SELECT that yields columns in this
// canonical order: category (TEXT), period (DATE or NULL), period_label (TEXT or NULL),
// value (NUMERIC), prev_value (NUMERIC or 0), order (INT or 0).
func (r *BiFactMetricRepository) QueryAggregate(ctx context.Context, plan factmetric.PlannedQuery) ([]factmetric.AggRow, error) {
	rows, err := r.db.QueryContext(ctx, plan.SQL, plan.Args...)
	if err != nil {
		return nil, fmt.Errorf("query aggregate: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []factmetric.AggRow
	for rows.Next() {
		var (
			row    factmetric.AggRow
			period sql.NullTime
			label  sql.NullString
			prev   sql.NullFloat64
			order  sql.NullInt64
		)
		if err := rows.Scan(&row.Category, &period, &label, &row.Value, &prev, &order); err != nil {
			return nil, fmt.Errorf("scan aggregate row: %w", err)
		}
		if period.Valid {
			row.Period = period.Time
		}
		if label.Valid {
			row.PeriodLabel = label.String
		}
		if prev.Valid {
			row.PrevValue = prev.Float64
		}
		if order.Valid {
			row.Order = int(order.Int64)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Upsert ingests rows via INSERT ... ON CONFLICT (business_key) DO UPDATE.
//
// Chunks of 1000 are processed per statement. display_value is computed at the
// caller (Excel commit / ETL worker) since the SQL function bi_compute_display_value
// can also be invoked but caller usually has it pre-computed.
func (r *BiFactMetricRepository) Upsert(ctx context.Context, rowsIn []factmetric.FactMetric) error {
	const chunk = 1000
	for start := 0; start < len(rowsIn); start += chunk {
		end := min(start+chunk, len(rowsIn))
		if err := r.upsertChunk(ctx, rowsIn[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// upsertChunk runs a single INSERT with N-row VALUES list inside a transaction.
func (r *BiFactMetricRepository) upsertChunk(ctx context.Context, rowsIn []factmetric.FactMetric) error {
	if len(rowsIn) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			_ = err
		}
	}()

	const q = `
INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3,
    group_1_order, group_2_order, group_3_order,
    periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, source_id, dimension_key, uploaded_by, is_active
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
ON CONFLICT (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key)
DO UPDATE SET
    value = EXCLUDED.value,
    display_value = EXCLUDED.display_value,
    uom = EXCLUDED.uom,
    source_id = EXCLUDED.source_id,
    uploaded_by = EXCLUDED.uploaded_by,
    loaded_at = NOW(),
    is_active = TRUE`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			_ = err
		}
	}()
	for _, row := range rowsIn {
		if _, err := stmt.ExecContext(ctx,
			row.Type, row.Group1, biNullableString(row.Group2), biNullableString(row.Group3),
			nullableInt(row.Group1Order), nullableInt(row.Group2Order), nullableInt(row.Group3Order),
			row.PeriodGrain, row.PeriodDate, row.PeriodLabel,
			row.Value, row.DisplayValue, biNullableString(row.UOM), row.Scenario, row.SourceID, row.DimensionKey,
			nullableUUID(row.UploadedBy), row.IsActive,
		); err != nil {
			return fmt.Errorf("upsert row: %w", err)
		}
	}
	return tx.Commit()
}

// nullableInt returns nil for zero values (caller signals "unknown order" with 0).
func nullableInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}
