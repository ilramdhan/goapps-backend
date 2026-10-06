package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/metrics"
)

// erp_metrics_repository.go reads the read-only aggregates behind the ERP
// observability gauges (plan-06 P5-T8; design Part 3 §15). SELECT only, PG
// only: nothing here writes or talks to Oracle.

// ErpMetricsRepository implements metrics.ErpStatsSource.
type ErpMetricsRepository struct{ db *DB }

// NewErpMetricsRepository constructs the repository.
func NewErpMetricsRepository(db *DB) *ErpMetricsRepository {
	return &ErpMetricsRepository{db: db}
}

var _ metrics.ErpStatsSource = (*ErpMetricsRepository)(nil)

const erpMetricsBatchSQL = `
	SELECT ceib_status, ceib_mode, COUNT(*)
	  FROM cst_erp_int_batch
	 GROUP BY ceib_status, ceib_mode`

const erpMetricsCallSQL = `
	SELECT ceocl_statement_key, ceocl_status, COUNT(*)
	  FROM cst_erp_oracle_call_log
	 GROUP BY ceocl_statement_key, ceocl_status`

const erpMetricsStartedAgeSQL = `
	SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(ceocl_started_at))), 0)::float8
	  FROM cst_erp_oracle_call_log
	 WHERE ceocl_status = 'STARTED'`

const erpMetricsStalePreviewSQL = `
	SELECT cevp_operation, COUNT(*)
	  FROM cst_erp_valuation_preview
	 WHERE cevp_status = 'OPEN' AND cevp_expires_at < NOW()
	 GROUP BY cevp_operation`

const erpMetricsOpenPreviewAgeSQL = `
	SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(cevp_created_at))), 0)::float8
	  FROM cst_erp_valuation_preview
	 WHERE cevp_status = 'OPEN'`

// ErpStats reads one sample.
func (r *ErpMetricsRepository) ErpStats(ctx context.Context) (metrics.ErpStats, error) {
	var st metrics.ErpStats

	if err := r.scanTriples(ctx, erpMetricsBatchSQL, func(a, b string, n int64) {
		st.Batches = append(st.Batches, metrics.BatchStatusCount{Status: a, Mode: b, Count: n})
	}); err != nil {
		return st, fmt.Errorf("erp metrics batches: %w", err)
	}
	if err := r.scanTriples(ctx, erpMetricsCallSQL, func(a, b string, n int64) {
		st.Calls = append(st.Calls, metrics.CallLogCount{Key: a, Status: b, Count: n})
	}); err != nil {
		return st, fmt.Errorf("erp metrics calls: %w", err)
	}

	var secs float64
	if err := r.db.QueryRowContext(ctx, erpMetricsStartedAgeSQL).Scan(&secs); err != nil {
		return st, fmt.Errorf("erp metrics started age: %w", err)
	}
	st.OldestStartedAge = secondsToDuration(secs)

	st.StaleOpenPreviews = map[string]int64{}
	rows, err := r.db.QueryContext(ctx, erpMetricsStalePreviewSQL)
	if err != nil {
		return st, fmt.Errorf("erp metrics stale previews: %w", err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // read-only cursor
	for rows.Next() {
		var op string
		var n int64
		if err := rows.Scan(&op, &n); err != nil {
			return st, fmt.Errorf("erp metrics stale previews scan: %w", err)
		}
		st.StaleOpenPreviews[op] = n
	}
	if err := rows.Err(); err != nil {
		return st, fmt.Errorf("erp metrics stale previews rows: %w", err)
	}

	if err := r.db.QueryRowContext(ctx, erpMetricsOpenPreviewAgeSQL).Scan(&secs); err != nil {
		return st, fmt.Errorf("erp metrics open preview age: %w", err)
	}
	st.OldestOpenPreviewAge = secondsToDuration(secs)
	return st, nil
}

func (r *ErpMetricsRepository) scanTriples(ctx context.Context, q string, add func(a, b string, n int64)) error {
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // read-only cursor
	for rows.Next() {
		var a, b string
		var n int64
		if err := rows.Scan(&a, &b, &n); err != nil {
			return err
		}
		add(a, b, n)
	}
	return rows.Err()
}

func secondsToDuration(s float64) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}
