package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpBacktestReportRepository implements erpintegration.BacktestReportRepository
// over cst_erp_backtest_line (migration 000559; plan-06 P5-T10b).
type ErpBacktestReportRepository struct{ db *DB }

// NewErpBacktestReportRepository constructs the repository.
func NewErpBacktestReportRepository(db *DB) *ErpBacktestReportRepository {
	return &ErpBacktestReportRepository{db: db}
}

var _ erpintegration.BacktestReportRepository = (*ErpBacktestReportRepository)(nil)

const insertErpBacktestLineSQL = `
	INSERT INTO cst_erp_backtest_line
		(cebl_batch_id, cebl_period, cebl_item_code, cebl_grade_code, cebl_shade_code, cebl_class, cebl_basis,
		 cebl_goapps, cebl_legacy, cebl_delta, cebl_delta_pct, cebl_fail)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

const selectErpBacktestLinesSQL = `
	SELECT cebl_item_code, cebl_grade_code, cebl_shade_code, cebl_class, cebl_basis,
	       cebl_goapps, cebl_legacy, cebl_delta, cebl_delta_pct, cebl_fail
	FROM cst_erp_backtest_line WHERE cebl_batch_id = $1 ORDER BY cebl_id`

// Replace deletes the batch's lines and inserts the report's in one tx.
func (r *ErpBacktestReportRepository) Replace(ctx context.Context, batchID int64, period string, report erpintegration.BacktestReport) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin backtest report tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback backtest report: %w", rbErr)
		}
	}()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cst_erp_backtest_line WHERE cebl_batch_id = $1`, batchID); err != nil {
		return fmt.Errorf("delete backtest lines: %w", err)
	}
	for _, l := range report.Lines {
		if _, err := tx.ExecContext(ctx, insertErpBacktestLineSQL, batchID, period,
			l.Key.ItemCode, l.Key.GradeCode, l.Key.ShadeCode, string(l.Class), string(l.Basis),
			l.GoApps, l.Legacy, l.Delta, l.DeltaPct, l.Fail); err != nil {
			return fmt.Errorf("insert backtest line %s: %w", l.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backtest report: %w", err)
	}
	return nil
}

// Get rebuilds the report of the batch; Counts and Failed derive from lines.
func (r *ErpBacktestReportRepository) Get(ctx context.Context, batchID int64) (rep erpintegration.BacktestReport, err error) {
	rep = erpintegration.BacktestReport{Counts: map[erpintegration.BacktestClass]int{}}
	rows, err := r.db.QueryContext(ctx, selectErpBacktestLinesSQL, batchID)
	if err != nil {
		return rep, fmt.Errorf("query backtest lines: %w", err)
	}
	defer closeRowsInto(rows, &err, "backtest lines")
	for rows.Next() {
		var (
			l                          erpintegration.BacktestLine
			class, basis               string
			goapps, legacy, delta, pct decimal.NullDecimal
		)
		if err := rows.Scan(&l.Key.ItemCode, &l.Key.GradeCode, &l.Key.ShadeCode, &class, &basis,
			&goapps, &legacy, &delta, &pct, &l.Fail); err != nil {
			return rep, fmt.Errorf("scan backtest line: %w", err)
		}
		l.Class, l.Basis = erpintegration.BacktestClass(class), erprule.Basis(basis)
		l.GoApps, l.Legacy, l.Delta, l.DeltaPct = goapps, legacy, delta, pct
		rep.Lines = append(rep.Lines, l)
		rep.Counts[l.Class]++
		if l.Fail {
			rep.Failed = true
		}
	}
	if err := rows.Err(); err != nil {
		return rep, fmt.Errorf("iterate backtest lines: %w", err)
	}
	return rep, nil
}
