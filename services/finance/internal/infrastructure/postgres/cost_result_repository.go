package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// CostResultRepository persists Result aggregates against `cst_product_cost`.
type CostResultRepository struct {
	db *DB
}

// NewCostResultRepository constructs a CostResultRepository.
func NewCostResultRepository(db *DB) *CostResultRepository {
	return &CostResultRepository{db: db}
}

var _ costcalc.ResultRepository = (*CostResultRepository)(nil)

const resultColumns = `cpc_cost_id, cpc_product_sys_id, cpc_period, cpc_calculation_type,
		       cpc_route_head_id, cpc_version, cpc_cost_per_unit,
		       COALESCE(cpc_total_rm_cost, 0), COALESCE(cpc_total_conversion, 0),
		       COALESCE(cpc_total_cost, 0), COALESCE(cpc_uom_id, 0),
		       cpc_currency_code, cpc_cost_by_level, cpc_rm_cost_detail,
		       cpc_param_snapshot, cpc_formula_trace,
		       COALESCE(cpc_input_hash, ''), cpc_status,
		       COALESCE(cpc_job_id, 0), cpc_calculated_at, cpc_calculated_by,
		       cpc_verified_at, COALESCE(cpc_verified_by, ''),
		       COALESCE(cpc_captive_cost, 0), COALESCE(cpc_delivery_cost, 0),
		       COALESCE(cpc_vb1_del_cost, 0), COALESCE(cpc_vb2_del_cost, 0),
		       COALESCE(cpc_vb3_del_cost, 0), COALESCE(cpc_vb4_del_cost, 0),
		       COALESCE(cpc_vb5_del_cost, 0)`

// upsertMaxRetries caps the retry loop when a concurrent transaction inserts a
// conflicting active row between our supersede and insert.
const upsertMaxRetries = 3

// UpsertWithSupersede atomically SUPERSEDEs any existing active row for the
// (product, period, calc_type) tuple, then inserts the new row with version
// = prev+1. Returns the new cost id plus the previous (if any) version, total,
// and id so the caller can write an audit-history row outside the transaction.
//
// When two concurrent jobs compute the same product+period+calc_type and no
// prior active row exists, the second INSERT hits uk_cpc_active. The retry
// loop rolls back, re-opens a fresh transaction (where the winner's row is now
// visible), supersedes it, and inserts again.
func (r *CostResultRepository) UpsertWithSupersede(
	ctx context.Context, res *costcalc.Result,
) (newCostID int64, prevVersion int, prevTotal float64, prevCostID int64, err error) {
	start := time.Now()
	defer func() {
		metrics.DBTxSeconds.WithLabelValues("upsert").Observe(time.Since(start).Seconds())
	}()
	if res == nil {
		return 0, 0, 0, 0, fmt.Errorf("upsert result: nil result")
	}

	for attempt := range upsertMaxRetries {
		newCostID, prevVersion, prevTotal, prevCostID, err = r.tryUpsert(ctx, res)
		if err == nil {
			return newCostID, prevVersion, prevTotal, prevCostID, nil
		}
		if !isUniqueViolation(err) {
			return 0, 0, 0, 0, err
		}
		metrics.UpsertRetryTotal.Inc()
		if attempt < upsertMaxRetries-1 {
			continue
		}
	}
	return 0, 0, 0, 0, fmt.Errorf("upsert result: unique violation after %d retries: %w", upsertMaxRetries, err)
}

// tryUpsert runs one supersede+insert attempt inside a single transaction.
func (r *CostResultRepository) tryUpsert(ctx context.Context, res *costcalc.Result) (int64, int, float64, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("begin upsert tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	prevVersion, prevTotal, prevCostID, err := supersedePrevious(ctx, tx, res.ProductSysID(), res.Period(), res.CalcType())
	if err != nil {
		return 0, 0, 0, 0, err
	}

	newCostID, err := insertNewResult(ctx, tx, res, prevVersion+1)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("commit upsert tx: %w", err)
	}
	committed = true
	res.AssignID(newCostID)
	if prevVersion > 0 {
		metrics.RecomputeTotal.Inc()
	}
	return newCostID, prevVersion, prevTotal, prevCostID, nil
}

// UpsertWithSupersedeTx is the transaction-scoped variant of UpsertWithSupersede, used by
// mbbatch.RunMBBatch so that superseding + inserting all 3 calc-type rows for one MB share a
// single commit/rollback boundary (design addendum §10.3 step 7) instead of each type's write
// committing independently. Caller owns the transaction lifecycle (begin/commit/rollback).
func (r *CostResultRepository) UpsertWithSupersedeTx(
	ctx context.Context, tx *sql.Tx, res *costcalc.Result,
) (newCostID int64, prevVersion int, prevTotal float64, prevCostID int64, err error) {
	if res == nil {
		return 0, 0, 0, 0, fmt.Errorf("upsert result tx: nil result")
	}
	prevVersion, prevTotal, prevCostID, err = supersedePrevious(ctx, tx, res.ProductSysID(), res.Period(), res.CalcType())
	if err != nil {
		return 0, 0, 0, 0, err
	}
	newCostID, err = insertNewResult(ctx, tx, res, prevVersion+1)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	res.AssignID(newCostID)
	if prevVersion > 0 {
		metrics.RecomputeTotal.Inc()
	}
	return newCostID, prevVersion, prevTotal, prevCostID, nil
}

// supersedePrevious marks the previous active row (if any) as SUPERSEDED.
func supersedePrevious(
	ctx context.Context, tx *sql.Tx, productSysID int64, period string, calcType costcalc.CalculationType,
) (int, float64, int64, error) {
	const q = `
		UPDATE cst_product_cost
		   SET cpc_status = 'SUPERSEDED'
		 WHERE cpc_product_sys_id = $1
		   AND cpc_period = $2
		   AND cpc_calculation_type = $3
		   AND cpc_status != 'SUPERSEDED'
		RETURNING cpc_version, COALESCE(cpc_total_cost, cpc_cost_per_unit), cpc_cost_id`
	var (
		prevVersion int
		prevTotal   float64
		prevCostID  int64
	)
	if err := tx.QueryRowContext(ctx, q, productSysID, period, string(calcType)).
		Scan(&prevVersion, &prevTotal, &prevCostID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, nil
		}
		return 0, 0, 0, fmt.Errorf("supersede previous cost: %w", err)
	}
	return prevVersion, prevTotal, prevCostID, nil
}

// insertNewResult inserts a new cst_product_cost row at the given version.
func insertNewResult(ctx context.Context, tx *sql.Tx, r *costcalc.Result, version int) (int64, error) {
	const q = `
		INSERT INTO cst_product_cost (
			cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id,
			cpc_version, cpc_cost_per_unit, cpc_total_rm_cost, cpc_total_conversion,
			cpc_total_cost, cpc_uom_id, cpc_currency_code,
			cpc_cost_by_level, cpc_rm_cost_detail, cpc_param_snapshot, cpc_formula_trace,
			cpc_input_hash, cpc_status, cpc_job_id, cpc_calculated_by,
			cpc_captive_cost, cpc_delivery_cost,
			cpc_vb1_del_cost, cpc_vb2_del_cost, cpc_vb3_del_cost,
			cpc_vb4_del_cost, cpc_vb5_del_cost
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,0),$11,$12,$13,$14,$15,NULLIF($16,''),$17,NULLIF($18,0),$19,
		          NULLIF($20,0),$21,NULLIF($22,0),NULLIF($23,0),NULLIF($24,0),NULLIF($25,0),NULLIF($26,0))
		RETURNING cpc_cost_id`
	var id int64
	err := tx.QueryRowContext(ctx, q,
		r.ProductSysID(), r.Period(), string(r.CalcType()), r.RouteHeadID(),
		safeconv.IntToInt32(version), r.CostPerUnit(), r.TotalRMCost(), r.TotalConv(),
		r.TotalCost(), safeconv.IntToInt32(r.UomID()), r.Currency(),
		nullableJSON(r.CostByLevel()), nullableJSON(r.RMCostDetail()),
		nullableJSON(r.ParamSnapshot()), nullableJSON(r.FormulaTrace()),
		r.InputHash(), string(r.Status()), r.JobID(), r.CalculatedBy(),
		r.CaptiveCost(), r.DeliveryCost(),
		r.VB1DelCost(), r.VB2DelCost(), r.VB3DelCost(),
		r.VB4DelCost(), r.VB5DelCost(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert new result: %w", err)
	}
	return id, nil
}

// GetActive returns the non-SUPERSEDED row for the tuple, or ErrCostNotFound.
func (r *CostResultRepository) GetActive(ctx context.Context, productSysID int64, period string, calcType costcalc.CalculationType) (*costcalc.Result, error) {
	q := `SELECT ` + resultColumns + ` FROM cst_product_cost
		   WHERE cpc_product_sys_id = $1 AND cpc_period = $2
		     AND cpc_calculation_type = $3 AND cpc_status != 'SUPERSEDED'
		   LIMIT 1`
	res, err := scanResult(r.db.QueryRowContext(ctx, q, productSysID, period, string(calcType)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costcalc.ErrCostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active cost: %w", err)
	}
	return res, nil
}

// GetActiveTx is the transaction-scoped variant of GetActive, used by MB Push-to-Head execute so
// the read participates in the same transaction as the subsequent status-flip write.
func (r *CostResultRepository) GetActiveTx(ctx context.Context, tx *sql.Tx, productSysID int64, period string, calcType costcalc.CalculationType) (*costcalc.Result, error) {
	q := `SELECT ` + resultColumns + ` FROM cst_product_cost
		   WHERE cpc_product_sys_id = $1 AND cpc_period = $2
		     AND cpc_calculation_type = $3 AND cpc_status != 'SUPERSEDED'
		   LIMIT 1`
	res, err := scanResult(tx.QueryRowContext(ctx, q, productSysID, period, string(calcType)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costcalc.ErrCostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active cost tx: %w", err)
	}
	return res, nil
}

// GetByID returns a single row by surrogate key.
func (r *CostResultRepository) GetByID(ctx context.Context, id int64) (*costcalc.Result, error) {
	q := `SELECT ` + resultColumns + ` FROM cst_product_cost WHERE cpc_cost_id = $1`
	res, err := scanResult(r.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costcalc.ErrCostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cost by id: %w", err)
	}
	return res, nil
}

// ListByProductIDsPeriodType returns the active (non-SUPERSEDED) result for each
// of the given products in one round-trip, keyed by product_sys_id. Products
// with no result for the tuple are simply absent from the map — that is a normal
// state for a route stage that has never been calculated, not an error.
func (r *CostResultRepository) ListByProductIDsPeriodType(
	ctx context.Context, productSysIDs []int64, period string, calcType costcalc.CalculationType,
) (map[int64]*costcalc.Result, error) {
	out := map[int64]*costcalc.Result{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	q := `SELECT ` + resultColumns + ` FROM cst_product_cost
		   WHERE cpc_product_sys_id = ANY($1) AND cpc_period = $2
		     AND cpc_calculation_type = $3 AND cpc_status != 'SUPERSEDED'`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(productSysIDs), period, string(calcType))
	if err != nil {
		return nil, fmt.Errorf("list costs by products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		res, scanErr := scanResult(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan cost row: %w", scanErr)
		}
		out[res.ProductSysID()] = res
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost rows: %w", err)
	}
	return out, nil
}

// ListHistory returns paginated history for a product, optionally filtered by calcType.
func (r *CostResultRepository) ListHistory(ctx context.Context, productSysID int64, calcType costcalc.CalculationType, page, pageSize int) ([]*costcalc.Result, int, error) {
	where := []string{"cpc_product_sys_id = $1"}
	args := []any{productSysID}
	if calcType != "" {
		args = append(args, string(calcType))
		where = append(where, fmt.Sprintf("cpc_calculation_type = $%d", len(args)))
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM cst_product_cost`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost history: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	offset := (page - 1) * pageSize

	listSQL := `SELECT ` + resultColumns + ` FROM cst_product_cost` + whereSQL +
		` ORDER BY cpc_calculated_at DESC, cpc_version DESC` +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost history: %w", err)
	}
	defer closeRows(rows)

	out := []*costcalc.Result{}
	for rows.Next() {
		res, scanErr := scanResult(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan cost history row: %w", scanErr)
		}
		out = append(out, res)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost history: %w", err)
	}
	return out, total, nil
}

// cpcSortColumn maps an API sort key to its ORDER BY expression. The keys must
// match the `sort_by` allow-list pinned in finance/v1/cost_calc.proto exactly —
// a mismatch silently drops the sort. Values come from this fixed map only,
// never from user input, so they are safe to interpolate into the query.
func cpcSortColumn(sortBy string) string {
	switch sortBy {
	case "productCode":
		return "cpm.cpm_product_code"
	case "productName":
		return "cpm.cpm_product_name"
	case "period":
		return "cpc.cpc_period"
	case "calculationType":
		return "cpc.cpc_calculation_type"
	case "costPerUnit":
		return "cpc.cpc_cost_per_unit"
	case "totalCost":
		return "COALESCE(cpc.cpc_total_cost, 0)"
	case "status":
		return "cpc.cpc_status"
	case "calculatedAt":
		return "cpc.cpc_calculated_at"
	default:
		return ""
	}
}

// cpcOrderBy builds the ORDER BY body. An unknown/empty sort key falls back to
// the default newest-first ordering; a known key always gets cpc_cost_id DESC
// appended so paging stays stable across ties.
func cpcOrderBy(sortBy, sortOrder string) string {
	col := cpcSortColumn(sortBy)
	if col == "" {
		return "cpc.cpc_calculated_at DESC, cpc.cpc_cost_id DESC"
	}
	dir := sortASC
	if strings.EqualFold(sortOrder, "desc") {
		dir = sortDESC
	}
	return col + " " + dir + ", cpc.cpc_cost_id DESC"
}

// cpcListWhere builds the WHERE body and positional args shared by the count
// and page queries in ListResults. Exactly one of period/yearFilter is used:
// an empty filter Period falls back to "all periods in the current year".
func cpcListWhere(f costcalc.ResultListFilter, period, yearFilter string) (string, []any) {
	where := []string{}
	args := []any{}
	if period != "" {
		args = append(args, period)
		where = append(where, fmt.Sprintf("cpc.cpc_period = $%d", len(args)))
	} else {
		args = append(args, yearFilter)
		where = append(where, fmt.Sprintf("LEFT(cpc.cpc_period, 4) = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("cpc.cpc_status = $%d", len(args)))
	} else {
		where = append(where, "cpc.cpc_status != 'SUPERSEDED'")
	}
	if f.CalcType != "" {
		args = append(args, string(f.CalcType))
		where = append(where, fmt.Sprintf("cpc.cpc_calculation_type = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		where = append(where, fmt.Sprintf(
			"(cpm.cpm_product_code ILIKE $%d OR cpm.cpm_product_name ILIKE $%d)", len(args), len(args)))
	}
	if len(f.ProductTypeIDs) > 0 {
		args = append(args, pq.Array(f.ProductTypeIDs))
		where = append(where, fmt.Sprintf("cpm.cpm_product_type_id = ANY($%d)", len(args)))
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// ListResults lists active cost results across products for a filter, joining
// cost_product_master for the resolved product code/name. When the filter
// Period is empty it resolves the latest period present in cst_product_cost.
func (r *CostResultRepository) ListResults(
	ctx context.Context, f costcalc.ResultListFilter,
) ([]*costcalc.ResultSummary, int, string, error) {
	period := f.Period
	// yearFilter is set when no exact period is given — we show all periods in the current year.
	yearFilter := ""
	if period == "" {
		yearFilter = fmt.Sprintf("%d", currentYear())
	}

	whereSQL, args := cpcListWhere(f, period, yearFilter)
	from := ` FROM cst_product_cost cpc
		LEFT JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpc.cpc_product_sys_id`

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*)`+from+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, "", fmt.Errorf("count cost results: %w", err)
	}

	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	offset := (page - 1) * pageSize

	listSQL := `SELECT cpc.cpc_cost_id, cpc.cpc_product_sys_id,
			COALESCE(cpm.cpm_product_code, ''), COALESCE(cpm.cpm_product_name, ''),
			cpc.cpc_period, cpc.cpc_calculation_type, cpc.cpc_route_head_id, cpc.cpc_version,
			cpc.cpc_cost_per_unit, COALESCE(cpc.cpc_total_rm_cost, 0),
			COALESCE(cpc.cpc_total_conversion, 0), COALESCE(cpc.cpc_total_cost, 0),
			COALESCE(cpc.cpc_uom_id, 0), cpc.cpc_currency_code, cpc.cpc_status,
			COALESCE(cpc.cpc_job_id, 0), cpc.cpc_calculated_at, cpc.cpc_calculated_by,
			COALESCE(cpm.cpm_product_type_id, 0),
			COALESCE((SELECT cpt_type_code FROM cost_product_type
			           WHERE cpt_type_id = cpm.cpm_product_type_id), '')` +
		from + whereSQL +
		` ORDER BY ` + cpcOrderBy(f.SortBy, f.SortOrder) +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, "", fmt.Errorf("list cost results: %w", err)
	}
	defer closeRows(rows)

	out := []*costcalc.ResultSummary{}
	for rows.Next() {
		var s costcalc.ResultSummary
		var calcType string
		if scanErr := rows.Scan(
			&s.CostID, &s.ProductSysID, &s.ProductCode, &s.ProductName,
			&s.Period, &calcType, &s.RouteHeadID, &s.Version,
			&s.CostPerUnit, &s.TotalRMCost, &s.TotalConv, &s.TotalCost,
			&s.UOMID, &s.CurrencyCode, &s.Status, &s.JobID, &s.CalculatedAt, &s.CalculatedBy,
			&s.ProductTypeID, &s.ProductTypeCode,
		); scanErr != nil {
			return nil, 0, "", fmt.Errorf("scan cost result row: %w", scanErr)
		}
		s.CalcType = costcalc.CalculationType(calcType)
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", fmt.Errorf("iterate cost results: %w", err)
	}
	resolvedPeriod := period
	if resolvedPeriod == "" {
		resolvedPeriod = yearFilter
	}
	return out, total, resolvedPeriod, nil
}

// currentYear returns the 4-digit current calendar year.
func currentYear() int {
	return time.Now().UTC().Year()
}

// MarkVerified transitions a CALCULATED row to VERIFIED.
func (r *CostResultRepository) MarkVerified(ctx context.Context, costID int64, by string) error {
	return r.transitionStatus(ctx, costID, by, "CALCULATED", "VERIFIED")
}

// MarkApproved transitions a VERIFIED row to APPROVED.
func (r *CostResultRepository) MarkApproved(ctx context.Context, costID int64, by string) error {
	return r.transitionStatus(ctx, costID, by, "VERIFIED", "APPROVED")
}

// ListDistinctPeriods returns the distinct periods (YYYYMM) that have cost
// results, ordered newest first.
//
// The WHERE clause excludes synthetic "9999xx"-style periods left behind by
// integration tests that write fixture rows directly into cst_product_cost
// in the shared dev DB without cleaning up (see handlers_test.go,
// trigger_handler_test.go, process_chunk_test.go). Those fixture periods were
// otherwise leaking into the Cost Results page's period-filter dropdown,
// confusing users with entries like "999992" next to real periods like
// "202604". Only plausible business periods (year 2000-2099, month 01-12)
// are returned.
func (r *CostResultRepository) ListDistinctPeriods(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT cpc_period FROM cst_product_cost
		  WHERE cpc_period ~ '^20[0-9]{2}(0[1-9]|1[0-2])$'
		  ORDER BY cpc_period DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct cost result periods: %w", err)
	}
	defer closeRows(rows)
	var out []string
	for rows.Next() {
		var p string
		if scanErr := rows.Scan(&p); scanErr != nil {
			return nil, fmt.Errorf("scan period: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost result periods: %w", err)
	}
	return out, nil
}

// MarkApprovedFromCalculatedTx transitions a CALCULATED row directly to APPROVED, inside the
// caller's transaction, used by MB Push-to-Head execute (which does not go through the standalone
// two-step CALCULATED->VERIFIED->APPROVED verify/approve workflow) so the cst_mb_cost upsert and
// this status flip commit or roll back together.
func (r *CostResultRepository) MarkApprovedFromCalculatedTx(ctx context.Context, tx *sql.Tx, costID int64, by string) error {
	const q = `
		UPDATE cst_product_cost
		   SET cpc_status = 'APPROVED',
		       cpc_verified_at = now(),
		       cpc_verified_by = $2
		 WHERE cpc_cost_id = $1 AND cpc_status = 'CALCULATED'`
	res, err := tx.ExecContext(ctx, q, costID, by)
	if err != nil {
		return fmt.Errorf("transition cost status tx: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("transition cost status tx rows: %w", err)
	}
	if n == 0 {
		return costcalc.ErrCostInvalidStatus
	}
	return nil
}

// transitionStatus guards the state machine and updates the verifier columns.
func (r *CostResultRepository) transitionStatus(ctx context.Context, costID int64, by, fromStatus, toStatus string) error {
	const q = `
		UPDATE cst_product_cost
		   SET cpc_status = $3,
		       cpc_verified_at = now(),
		       cpc_verified_by = $4
		 WHERE cpc_cost_id = $1 AND cpc_status = $2`
	res, err := r.db.ExecContext(ctx, q, costID, fromStatus, toStatus, by)
	if err != nil {
		return fmt.Errorf("transition cost status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("transition cost status rows: %w", err)
	}
	if n == 0 {
		// Either the row doesn't exist or it's not in the expected status.
		return costcalc.ErrCostInvalidStatus
	}
	return nil
}

// scanResult reads one cst_product_cost row.
func scanResult(s rowScanner) (*costcalc.Result, error) {
	var (
		id, productSysID, routeHeadID int64
		period, calcType, currency    string
		version                       int32
		costPerUnit, totalRM          float64
		totalConv, totalCost          float64
		uomID                         int32
		costByLevel, rmDetail         []byte
		paramSnap, formulaTrace       []byte
		inputHash, status             string
		jobID                         int64
		calcAt                        time.Time
		calcBy                        string
		verifiedAt                    sql.NullTime
		verifiedBy                    string
		captiveCost, deliveryCost     float64
		vb1DelCost, vb2DelCost        float64
		vb3DelCost, vb4DelCost        float64
		vb5DelCost                    float64
	)
	if err := s.Scan(
		&id, &productSysID, &period, &calcType, &routeHeadID, &version,
		&costPerUnit, &totalRM, &totalConv, &totalCost, &uomID, &currency,
		&costByLevel, &rmDetail, &paramSnap, &formulaTrace,
		&inputHash, &status, &jobID, &calcAt, &calcBy, &verifiedAt, &verifiedBy,
		&captiveCost, &deliveryCost, &vb1DelCost, &vb2DelCost, &vb3DelCost,
		&vb4DelCost, &vb5DelCost,
	); err != nil {
		return nil, err
	}
	var verifiedPtr *time.Time
	if verifiedAt.Valid {
		t := verifiedAt.Time
		verifiedPtr = &t
	}
	return costcalc.HydrateResult(
		id, productSysID, period, costcalc.CalculationType(calcType), routeHeadID, int(version),
		costPerUnit, totalRM, totalConv, totalCost, int(uomID), currency,
		costByLevel, rmDetail, paramSnap, formulaTrace,
		inputHash, costcalc.ResultStatus(status), jobID, calcAt, calcBy,
		verifiedPtr, verifiedBy,
		captiveCost, deliveryCost, vb1DelCost, vb2DelCost, vb3DelCost,
		vb4DelCost, vb5DelCost,
	), nil
}
