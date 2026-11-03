package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// ppStagingColumns is the ETL§C column list, in ETL§A select order.
var ppStagingColumns = []string{
	"pack_date", "week_start", "month_start", "period", "week_period",
	"dept_id", "dept_name", "prod_group", "pallet_code", "prd_type_name", "prd_type_order",
	"grp_rep_name", "transaction_type", "transaction_type_mc", "group_type", "group_type_order",
	"merge_reporting", "poly_type", "mc_no", "mach_group", "mach_order_rep",
	"merge_no", "prod_descr", "shade", "poy_line", "grade_eligible",
	"ax_qty", "fq_qty", "dg_qty", "jlt_qty", "total_qty",
}

// ppInsertBatchRows bounds one multi-row INSERT (31 params per row, well under the 65535 limit).
const ppInsertBatchRows = 500

// BiPPStagingRepository writes the packed-production staging table bi_pp_stg_pack_prod_daily.
type BiPPStagingRepository struct {
	db *DB
}

// NewBiPPStagingRepository constructs the repository.
func NewBiPPStagingRepository(db *DB) *BiPPStagingRepository {
	return &BiPPStagingRepository{db: db}
}

// ReplacePackProdDaily deletes staging rows with pack_date in [d1, d2], batch-inserts rows (ETL§C),
// then re-aggregates the DAILY/WEEKLY/MONTHLY PP_PACK_* facts of the window and reconciles them (ETL§D/§F),
// all in one transaction, so combinations that vanished from the source vanish from staging and
// facts too. sourceID is stamped on the facts. Returns staging rows inserted.
func (r *BiPPStagingRepository) ReplacePackProdDaily(
	ctx context.Context, d1, d2 time.Time, rows []oracleinfra.PackProdDailyRow, sourceID uuid.UUID,
) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("bi_pp: rollback failed")
		}
	}()

	// Serialize concurrent replaces (07:00 cron vs manual run) so the window is not doubled.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "bi_pp_replace:pack_prod_daily"); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_pp_stg_pack_prod_daily WHERE pack_date BETWEEN $1 AND $2`, d1, d2); err != nil {
		return 0, fmt.Errorf("delete window: %w", err)
	}
	for i := 0; i < len(rows); i += ppInsertBatchRows {
		end := min(i+ppInsertBatchRows, len(rows))
		if err = insertPPBatch(ctx, tx, rows[i:end]); err != nil {
			return 0, fmt.Errorf("insert staging batch at offset %d: %w", i, err)
		}
	}
	if _, err = replacePPFacts(ctx, tx, d1, d2, sourceID); err != nil {
		return 0, fmt.Errorf("aggregate pack prod facts: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(rows), nil
}

// RebuildPackProdFacts re-aggregates the PP_PACK_* facts of [d1, d2] from the existing staging
// without touching Oracle or staging (P007: fills WEEKLY history for periods staged before the
// WEEKLY grain existed). Same transaction, lock and ETL§F reconciliation as ReplacePackProdDaily.
// Returns fact rows inserted.
func (r *BiPPStagingRepository) RebuildPackProdFacts(ctx context.Context, d1, d2 time.Time, sourceID uuid.UUID) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("bi_pp: rollback failed")
		}
	}()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "bi_pp_replace:pack_prod_daily"); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}
	n, err := replacePPFacts(ctx, tx, d1, d2, sourceID)
	if err != nil {
		return 0, fmt.Errorf("aggregate pack prod facts: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// insertPPBatch inserts rows with one parameterized multi-row INSERT.
func insertPPBatch(ctx context.Context, tx *sql.Tx, rows []oracleinfra.PackProdDailyRow) error {
	if len(rows) == 0 {
		return nil
	}
	nCols := len(ppStagingColumns)
	var sb strings.Builder
	sb.WriteString("INSERT INTO bi_pp_stg_pack_prod_daily (")
	sb.WriteString(strings.Join(ppStagingColumns, ", "))
	sb.WriteString(") VALUES ")
	args := make([]any, 0, len(rows)*nCols)
	for i, x := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(")
		for c := 0; c < nCols; c++ {
			if c > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "$%d", i*nCols+c+1)
		}
		sb.WriteString(")")
		args = append(args,
			x.PackDate, x.WeekStart, x.MonthStart, x.Period, x.WeekPeriod,
			x.DeptID, x.DeptName, x.ProdGroup, x.PalletCode, x.PrdTypeName, x.PrdTypeOrder,
			x.GrpRepName, x.TransactionType, x.TransactionTypeMC, x.GroupType, x.GroupTypeOrder,
			x.MergeReporting, x.PolyType, x.McNo, x.MachGroup, x.MachOrderRep,
			x.MergeNo, x.ProdDescr, x.Shade, x.PoyLine, x.GradeEligible,
			x.AxQty.String(), x.FqQty.String(), x.DgQty.String(), x.JltQty.String(), x.TotalQty.String(),
		)
	}
	_, err := tx.ExecContext(ctx, sb.String(), args...)
	return err
}
