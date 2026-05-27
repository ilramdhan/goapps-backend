package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
		       cpc_verified_at, COALESCE(cpc_verified_by, '')`

// UpsertWithSupersede atomically SUPERSEDEs any existing active row for the
// (product, period, calc_type) tuple, then inserts the new row with version
// = prev+1. Returns the new cost id plus the previous (if any) version, total,
// and id so the caller can write an audit-history row outside the transaction.
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

	prevVersion, prevTotal, prevCostID, err = supersedePrevious(ctx, tx, res.ProductSysID(), res.Period(), res.CalcType())
	if err != nil {
		return 0, 0, 0, 0, err
	}

	newCostID, err = insertNewResult(ctx, tx, res, prevVersion+1)
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
			cpc_input_hash, cpc_status, cpc_job_id, cpc_calculated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,0),$11,$12,$13,$14,$15,NULLIF($16,''),$17,NULLIF($18,0),$19)
		RETURNING cpc_cost_id`
	var id int64
	err := tx.QueryRowContext(ctx, q,
		r.ProductSysID(), r.Period(), string(r.CalcType()), r.RouteHeadID(),
		safeconv.IntToInt32(version), r.CostPerUnit(), r.TotalRMCost(), r.TotalConv(),
		r.TotalCost(), safeconv.IntToInt32(r.UomID()), r.Currency(),
		nullableJSON(r.CostByLevel()), nullableJSON(r.RMCostDetail()),
		nullableJSON(r.ParamSnapshot()), nullableJSON(r.FormulaTrace()),
		r.InputHash(), string(r.Status()), r.JobID(), r.CalculatedBy(),
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

// ListHistory returns paginated history for a product/calcType.
func (r *CostResultRepository) ListHistory(ctx context.Context, productSysID int64, calcType costcalc.CalculationType, page, pageSize int) ([]*costcalc.Result, int, error) {
	where := []string{"cpc_product_sys_id = $1", "cpc_calculation_type = $2"}
	args := []any{productSysID, string(calcType)}
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

// ListResults lists active cost results across products for a filter, joining
// cost_product_master for the resolved product code/name. When the filter
// Period is empty it resolves the latest period present in cst_product_cost.
func (r *CostResultRepository) ListResults(
	ctx context.Context, f costcalc.ResultListFilter,
) ([]*costcalc.ResultSummary, int, string, error) {
	period := f.Period
	if period == "" {
		if err := r.db.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(cpc_period), '') FROM cst_product_cost`).Scan(&period); err != nil {
			return nil, 0, "", fmt.Errorf("resolve latest period: %w", err)
		}
	}
	if period == "" {
		return []*costcalc.ResultSummary{}, 0, "", nil
	}

	where := []string{"cpc.cpc_period = $1"}
	args := []any{period}
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
	whereSQL := " WHERE " + strings.Join(where, " AND ")
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
			COALESCE(cpc.cpc_job_id, 0), cpc.cpc_calculated_at, cpc.cpc_calculated_by` +
		from + whereSQL +
		` ORDER BY cpc.cpc_calculated_at DESC, cpc.cpc_cost_id DESC` +
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
		); scanErr != nil {
			return nil, 0, "", fmt.Errorf("scan cost result row: %w", scanErr)
		}
		s.CalcType = costcalc.CalculationType(calcType)
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", fmt.Errorf("iterate cost results: %w", err)
	}
	return out, total, period, nil
}

// MarkVerified transitions a CALCULATED row to VERIFIED.
func (r *CostResultRepository) MarkVerified(ctx context.Context, costID int64, by string) error {
	return r.transitionStatus(ctx, costID, by, "CALCULATED", "VERIFIED")
}

// MarkApproved transitions a VERIFIED row to APPROVED.
func (r *CostResultRepository) MarkApproved(ctx context.Context, costID int64, by string) error {
	return r.transitionStatus(ctx, costID, by, "VERIFIED", "APPROVED")
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
	)
	if err := s.Scan(
		&id, &productSysID, &period, &calcType, &routeHeadID, &version,
		&costPerUnit, &totalRM, &totalConv, &totalCost, &uomID, &currency,
		&costByLevel, &rmDetail, &paramSnap, &formulaTrace,
		&inputHash, &status, &jobID, &calcAt, &calcBy, &verifiedAt, &verifiedBy,
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
	), nil
}
