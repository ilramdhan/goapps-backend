package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
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

// insertNewResultQuery is the INSERT used by insertNewResult. It is a package-level
// constant (not a function-local one) so cost_result_repository_internal_test.go can
// assert on its text without opening a database connection.
//
// WHY THE `::numeric` CASTS ON $20-$26 ARE MANDATORY
//
// The seven fast-query columns (cpc_captive_cost, cpc_delivery_cost, cpc_vb1..vb5_del_cost)
// are NUMERIC(20,6) — see migrations/postgres/000383_add_cpc_fast_query_cols.up.sql:9-15 —
// and the Go values bound to them are float64.
//
// Six of them were originally written as `NULLIF($n,0)` with no cast. The literal `0` is an
// integer, so PostgreSQL resolved the NULLIF argument — and therefore the placeholder's
// parameter type — to int4 rather than numeric. The production driver is pgx
// (internal/infrastructure/postgres/connection.go:11 imports jackc/pgx/v5/stdlib), which
// encodes a float64 according to the server-inferred OID: for an integer OID it routes
// through float64Wrapper.Int64Value(), i.e. `int64(w)` — a plain TRUNCATION, no rounding,
// no error (github.com/jackc/pgx/v5@v5.8.0/pgtype/builtin_wrappers.go:359-365). Every
// fractional part was silently discarded on write.
//
// cpc_delivery_cost ($21) was the one placeholder with no NULLIF wrapper, so it inferred
// numeric from the target column and stayed correct — which is exactly why it was the only
// clean column in production while the other six diverged.
//
// The bug is NOT NULLIF (NULLIF rounds nothing); it is the inferred parameter type. The
// casts pin every one of the seven placeholders to numeric explicitly, including $21 which
// currently happens to infer correctly. DELETING ANY OF THESE CASTS WILL SILENTLY TRUNCATE
// VALUES AGAIN — there is no error, only wrong data. cost_result_repository_internal_test.go
// guards this.
const insertNewResultQuery = `
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
		          NULLIF($20::numeric,0),$21::numeric,NULLIF($22::numeric,0),NULLIF($23::numeric,0),
		          NULLIF($24::numeric,0),NULLIF($25::numeric,0),NULLIF($26::numeric,0))
		RETURNING cpc_cost_id`

// insertNewResult inserts a new cst_product_cost row at the given version.
func insertNewResult(ctx context.Context, tx *sql.Tx, r *costcalc.Result, version int) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, insertNewResultQuery,
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

// cpcListWhere builds the WHERE body and positional args shared by the page
// query in ListResults. period is always an exact cpc_period value by this
// point — when the caller's filter Period was empty, ListResults has already
// resolved it to the latest period present in cst_product_cost — so the
// predicate is always an equality match that can use idx_cpc_period_type_status.
// rmDetailExistsClause builds an EXISTS(...) predicate matching placeholder
// $n against any cpc_rm_cost_detail line's ref_code OR its resolved display
// name (RM group name for GROUP lines, ERP item name for ITEM lines, product
// code/name for PRODUCT lines).
//
// The JSON keys here MUST match the actual shape written by
// aggregateRMCost/RMCostDetail (services/finance/internal/application/costcalc/compute.go):
// snake_case `ref_code`/`rm_type`, not the camelCase `refCode`/`refLabel` this
// query used before — those keys never existed in the persisted JSON, so this
// EXISTS clause (and the primary-RM extraction below) silently matched
// nothing. There is also no `refLabel`/label field at all in the persisted
// JSON: a human-readable name has to be resolved separately per RM kind,
// which is what the joins below do. A PRODUCT-type line's ref_code is the
// literal string "product:<product_sys_id>" (see rmRefCode in compute.go),
// so resolving its code/name requires parsing that id back out and joining
// cost_product_master.
func rmDetailExistsClause(n int) string {
	return fmt.Sprintf(`EXISTS (
		     SELECT 1
		       FROM jsonb_array_elements(COALESCE(cpc.cpc_rm_cost_detail, '[]'::jsonb)) elem
		       LEFT JOIN cst_rm_group_head rmg_s
		              ON elem->>'rm_type' = 'GROUP' AND rmg_s.group_code = elem->>'ref_code' AND rmg_s.deleted_at IS NULL
		       LEFT JOIN cost_erp_item cei_s
		              ON elem->>'rm_type' = 'ITEM' AND cei_s.cei_item_code = elem->>'ref_code'
		       LEFT JOIN cost_product_master cpm_s
		              ON elem->>'rm_type' = 'PRODUCT'
		             AND cpm_s.cpm_product_sys_id = NULLIF(substring(elem->>'ref_code' FROM 'product:(\d+)'), '')::bigint
		      WHERE elem->>'ref_code' ILIKE $%[1]d
		         OR rmg_s.group_name ILIKE $%[1]d
		         OR cei_s.cei_item_name ILIKE $%[1]d
		         OR cpm_s.cpm_product_code ILIKE $%[1]d
		         OR cpm_s.cpm_product_name ILIKE $%[1]d
		   )`, n)
}

// jsonStringEscape escapes s for embedding inside a hand-built JSON string
// literal (backslash and double-quote only — RM group codes never contain
// control characters). Used by rmGroupCodeContainsClause instead of
// json.Marshal so building the containment literal never needs an ignored
// error return.
func jsonStringEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// rmGroupCodeContainsClause builds an OR-chain of jsonb containment (`@>`)
// predicates, one per RM group code starting at placeholder offset+1, each
// binding a `[{"rm_type": "GROUP", "ref_code": "<code>"}]` JSON literal.
// Returns the SQL fragment plus the values to append to the query's args.
//
// This replaces the previous `jsonb_array_elements(cpc_rm_cost_detail) elem
// WHERE elem->>'rm_type' = 'GROUP' AND elem->>'ref_code' = ANY($n::text[])`
// shape, which unnests and re-scans the whole JSONB array for every
// candidate row with no index support at all. `@>` containment on the whole
// column, by contrast, is exactly what a GIN(jsonb_path_ops) index accelerates
// — see migrations/postgres/000527_add_cpc_rm_cost_detail_gin_index.up.sql —
// so Postgres can use a Bitmap Index Scan on cst_product_cost instead of a
// per-row jsonb_array_elements scan. Scoped to GROUP-type entries only, per
// the filter's intent: some product routes reference an RM group (e.g.
// "BRT"), others reference a prior intermediate product — this filter
// targets the former specifically, not arbitrary RM text (that stays
// covered by the broader ILIKE/EXISTS match in the `search` param via
// rmDetailExistsClause, which cannot use this index — see that function's
// comment).
func rmGroupCodeContainsClause(codes []string, offset int) (string, []any) {
	parts := make([]string, 0, len(codes))
	vals := make([]any, 0, len(codes))
	for i, code := range codes {
		payload := fmt.Sprintf(`[{"rm_type": "GROUP", "ref_code": "%s"}]`, jsonStringEscape(code))
		vals = append(vals, payload)
		parts = append(parts, fmt.Sprintf("cpc.cpc_rm_cost_detail @> $%d::jsonb", offset+i+1))
	}
	return "(" + strings.Join(parts, " OR ") + ")", vals
}

func cpcListWhere(f costcalc.ResultListFilter, period string) (string, []any) {
	where := []string{}
	args := []any{}
	args = append(args, period)
	where = append(where, fmt.Sprintf("cpc.cpc_period = $%d", len(args)))
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
		n := len(args)
		// Item-code search bug: this branch used to search cei.cei_item_code
		// ONLY. cei is reached via `LEFT JOIN cost_erp_item cei ON cei.cei_item_code =
		// cpm.cpm_erp_item_code`, which fails to match whenever
		// cost_product_master.cpm_erp_item_code carries content cost_erp_item
		// doesn't have byte-for-byte — e.g. the descriptive-suffix values migration
		// 000409 widened the column for ("POY0000350-for selling"). When that join
		// misses, cei.* is NULL for the row, so the OLD `cei.cei_item_code ILIKE`
		// predicate could never match even though the item code IS visible in the
		// UI (via COALESCE(cei.cei_item_code, cpm.cpm_erp_item_code, '') in the
		// SELECT list below). Adding cpm.cpm_erp_item_code ILIKE here makes search
		// match on exactly what is displayed, independent of whether the cei join
		// happened to resolve.
		where = append(where, fmt.Sprintf(
			`(cpm.cpm_product_code ILIKE $%[1]d OR cpm.cpm_product_name ILIKE $%[1]d
			  OR cei.cei_item_code ILIKE $%[1]d OR cei.cei_item_name ILIKE $%[1]d
			  OR cpm.cpm_erp_item_code ILIKE $%[1]d
			  OR cpm.cpm_shade_code ILIKE $%[1]d OR cpm.cpm_shade_name ILIKE $%[1]d
			  OR `+rmDetailExistsClause(n)+`)`, n))
	}
	if len(f.ProductTypeIDs) > 0 {
		args = append(args, pq.Array(f.ProductTypeIDs))
		where = append(where, fmt.Sprintf("cpm.cpm_product_type_id = ANY($%d)", len(args)))
	}
	if len(f.ShadeCodes) > 0 {
		args = append(args, pq.Array(f.ShadeCodes))
		where = append(where, fmt.Sprintf("cpm.cpm_shade_code = ANY($%d)", len(args)))
	}
	if len(f.RMGroupCodes) > 0 {
		clause, vals := rmGroupCodeContainsClause(f.RMGroupCodes, len(args))
		args = append(args, vals...)
		where = append(where, clause)
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// ListResults lists active cost results across products for a filter, joining
// cost_product_master for the resolved product code/name. When the filter
// Period is empty it resolves the latest period present in cst_product_cost
// and filters on that single period only (an exact match on cpc_period, able
// to use idx_cpc_period_type_status) rather than the whole current year.
func (r *CostResultRepository) ListResults(
	ctx context.Context, f costcalc.ResultListFilter,
) ([]*costcalc.ResultSummary, int, string, error) {
	period := f.Period
	if period == "" {
		resolved, err := r.latestPeriod(ctx)
		if err != nil {
			return nil, 0, "", err
		}
		period = resolved
	}

	whereSQL, args := cpcListWhere(f, period)

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

	orderBy := cpcOrderBy(f.SortBy, f.SortOrder)

	// filtered resolves which page of cpc_cost_ids match the filter, ordered
	// and limited, using ONLY the cheap joins the WHERE/ORDER BY actually
	// need (cpm for product/shade/type columns, cei for the item-code
	// search fallback) plus the two RM filter predicates
	// (rmDetailExistsClause / rmGroupCodeContainsClause), which are
	// self-contained EXISTS/@> checks against cpc.cpc_rm_cost_detail
	// directly — neither needs the expensive per-row RM name-resolution
	// LATERAL joins reattached below.
	//
	// COUNT(*) OVER() forces Postgres to materialize every row matching
	// whereSQL before ORDER BY + LIMIT can be applied (a window function
	// can't be evaluated without seeing its whole partition first). Before
	// this CTE split, that same COUNT(*) OVER() sat in a SELECT whose FROM
	// also carried the top_rm/rm_all LATERAL joins below — rm_all alone
	// does jsonb_array_elements(cpc_rm_cost_detail) + 3 more LEFT JOINs +
	// jsonb_agg PER ROW — so that O(rows × RM-lines-per-row) work ran for
	// the ENTIRE filtered set (every row for the period/status, not just
	// the requested page) on every single list request, regardless of
	// which filter was applied. That is the root cause of the list staying
	// slow no matter which filter changes: period/status alone are rarely
	// selective enough on a large production table to keep the
	// pre-LIMIT row count small, and once it isn't, the per-row LATERAL
	// cost dominates. Splitting the window function into this CTE (no
	// LATERAL) bounds the materialize-before-LIMIT cost to the filter's
	// selectivity on plain/indexed columns only; the LATERAL name
	// resolution is re-attached in the outer SELECT, where it only ever
	// runs for the `pageSize` rows `filtered` already picked out.
	filteredSQL := `WITH filtered AS (
		SELECT cpc.cpc_cost_id, COUNT(*) OVER() AS full_count
		  FROM cst_product_cost cpc
		  LEFT JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpc.cpc_product_sys_id
		  LEFT JOIN cost_erp_item cei ON cei.cei_item_code = cpm.cpm_erp_item_code` +
		whereSQL +
		` ORDER BY ` + orderBy +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2) + `
	)
	`
	args = append(args, pageSize, offset)

	// top_rm resolves the single highest-contribution cpc_rm_cost_detail line
	// (LATERAL + LIMIT 1, so it can never multiply outer rows despite the
	// LEFT JOIN). rm_group/rm_item/rm_product resolve that line's display
	// name depending on its rm_type discriminator — see rmDetailExistsClause's
	// comment for why the JSON keys are ref_code/rm_type (not refCode/refLabel)
	// and why a PRODUCT-type ref_code needs the "product:<id>" parse. Both
	// LATERAL joins here now run only against `filtered`'s page of rows, not
	// every row matching whereSQL — see filteredSQL's comment above.
	from := ` FROM filtered
		JOIN cst_product_cost cpc ON cpc.cpc_cost_id = filtered.cpc_cost_id
		LEFT JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpc.cpc_product_sys_id
		LEFT JOIN cost_erp_item cei ON cei.cei_item_code = cpm.cpm_erp_item_code
		LEFT JOIN LATERAL (
			SELECT elem->>'ref_code' AS ref_code, elem->>'rm_type' AS rm_type
			  FROM jsonb_array_elements(COALESCE(cpc.cpc_rm_cost_detail, '[]'::jsonb)) elem
			 ORDER BY NULLIF(elem->>'contribution','')::numeric DESC NULLS LAST
			 LIMIT 1
		) top_rm ON true
		LEFT JOIN cst_rm_group_head top_rm_group
		       ON top_rm.rm_type = 'GROUP' AND top_rm_group.group_code = top_rm.ref_code AND top_rm_group.deleted_at IS NULL
		LEFT JOIN cost_erp_item top_rm_item
		       ON top_rm.rm_type = 'ITEM' AND top_rm_item.cei_item_code = top_rm.ref_code
		LEFT JOIN cost_product_master top_rm_product
		       ON top_rm.rm_type = 'PRODUCT'
		      AND top_rm_product.cpm_product_sys_id = NULLIF(substring(top_rm.ref_code FROM 'product:(\d+)'), '')::bigint
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
			         jsonb_build_object(
			           'route_level', e.route_level,
			           'rm_type', e.rm_type,
			           'ref_code', CASE WHEN e.rm_type = 'PRODUCT' THEN COALESCE(pm2.cpm_product_code, e.ref_code) ELSE e.ref_code END,
			           'ref_name', COALESCE(g2.group_name, i2.cei_item_name, pm2.cpm_product_name, ''),
			           'shade_code', COALESCE(e.shade_code, ''),
			           'unit_cost', e.unit_cost,
			           'ratio', e.ratio,
			           'contribution', e.contribution
			         ) ORDER BY e.contribution DESC NULLS LAST
			       ) AS details_json
			  FROM (
			    SELECT elem->>'rm_type' AS rm_type, elem->>'ref_code' AS ref_code,
			           elem->>'shade_code' AS shade_code,
			           NULLIF(elem->>'unit_cost','')::numeric AS unit_cost,
			           NULLIF(elem->>'ratio','')::numeric AS ratio,
			           NULLIF(elem->>'contribution','')::numeric AS contribution,
			           NULLIF(elem->>'route_level','')::int AS route_level
			      FROM jsonb_array_elements(COALESCE(cpc.cpc_rm_cost_detail, '[]'::jsonb)) elem
			  ) e
			  LEFT JOIN cst_rm_group_head g2 ON e.rm_type = 'GROUP' AND g2.group_code = e.ref_code AND g2.deleted_at IS NULL
			  LEFT JOIN cost_erp_item i2 ON e.rm_type = 'ITEM' AND i2.cei_item_code = e.ref_code
			  LEFT JOIN cost_product_master pm2
			         ON e.rm_type = 'PRODUCT' AND pm2.cpm_product_sys_id = NULLIF(substring(e.ref_code FROM 'product:(\d+)'), '')::bigint
		) rm_all ON true`

	// full_count now comes from filtered.full_count (computed once, cheaply,
	// inside the CTE) instead of a second COUNT(*) OVER() out here — the
	// outer query only ever sees `filtered`'s page-sized row set, so a
	// window function here would just repeat the same number.
	listSQL := filteredSQL + `SELECT cpc.cpc_cost_id, cpc.cpc_product_sys_id,
			COALESCE(cpm.cpm_product_code, ''), COALESCE(cpm.cpm_product_name, ''),
			cpc.cpc_period, cpc.cpc_calculation_type, cpc.cpc_route_head_id, cpc.cpc_version,
			cpc.cpc_cost_per_unit, COALESCE(cpc.cpc_total_rm_cost, 0),
			COALESCE(cpc.cpc_total_conversion, 0), COALESCE(cpc.cpc_total_cost, 0),
			COALESCE(cpc.cpc_uom_id, 0), cpc.cpc_currency_code, cpc.cpc_status,
			COALESCE(cpc.cpc_job_id, 0), cpc.cpc_calculated_at, cpc.cpc_calculated_by,
			COALESCE(cpm.cpm_product_type_id, 0),
			COALESCE((SELECT cpt_type_code FROM cost_product_type
			           WHERE cpt_type_id = cpm.cpm_product_type_id), ''),
			COALESCE(cei.cei_item_code, cpm.cpm_erp_item_code, ''),
			COALESCE(cei.cei_item_name, ''),
			COALESCE(cpm.cpm_shade_code, ''), COALESCE(cpm.cpm_shade_name, ''),
			CASE WHEN top_rm.rm_type = 'PRODUCT' THEN COALESCE(top_rm_product.cpm_product_code, top_rm.ref_code)
			     ELSE top_rm.ref_code END,
			COALESCE(top_rm_group.group_name, top_rm_item.cei_item_name, top_rm_product.cpm_product_name, ''),
			jsonb_array_length(COALESCE(cpc.cpc_rm_cost_detail, '[]'::jsonb)),
			COALESCE(rm_all.details_json, '[]'::jsonb),
			filtered.full_count` +
		from +
		` ORDER BY ` + orderBy

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, "", fmt.Errorf("list cost results: %w", err)
	}
	defer closeRows(rows)

	out := []*costcalc.ResultSummary{}
	total := 0
	for rows.Next() {
		var s costcalc.ResultSummary
		var calcType string
		var primaryRMCode, primaryRMName sql.NullString
		var rmCount int32
		var rmDetailsJSON []byte
		if scanErr := rows.Scan(
			&s.CostID, &s.ProductSysID, &s.ProductCode, &s.ProductName,
			&s.Period, &calcType, &s.RouteHeadID, &s.Version,
			&s.CostPerUnit, &s.TotalRMCost, &s.TotalConv, &s.TotalCost,
			&s.UOMID, &s.CurrencyCode, &s.Status, &s.JobID, &s.CalculatedAt, &s.CalculatedBy,
			&s.ProductTypeID, &s.ProductTypeCode,
			&s.ItemCode, &s.ItemName, &s.ShadeCode, &s.ShadeName,
			&primaryRMCode, &primaryRMName, &rmCount, &rmDetailsJSON, &total,
		); scanErr != nil {
			return nil, 0, "", fmt.Errorf("scan cost result row: %w", scanErr)
		}
		s.CalcType = costcalc.CalculationType(calcType)
		s.PrimaryRMCode = primaryRMCode.String
		s.PrimaryRMName = primaryRMName.String
		s.RMCount = rmCount
		if len(rmDetailsJSON) > 0 {
			var details []costcalc.RMDetailSummary
			if err := json.Unmarshal(rmDetailsJSON, &details); err != nil {
				return nil, 0, "", fmt.Errorf("unmarshal rm details: %w", err)
			}
			s.RMDetails = details
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", fmt.Errorf("iterate cost results: %w", err)
	}
	return out, total, period, nil
}

// latestPeriod resolves the single most recent period present in
// cst_product_cost. It reuses ListDistinctPeriods' "plausible business
// period" filter (year 2000-2099, month 01-12) so synthetic fixture periods
// (e.g. "999992", left behind by integration tests — see ListDistinctPeriods)
// never win just because they sort higher as strings than a real YYYYMM
// value. Returns "" when no plausible period exists yet.
func (r *CostResultRepository) latestPeriod(ctx context.Context) (string, error) {
	var period string
	err := r.db.QueryRowContext(ctx,
		`SELECT cpc_period FROM cst_product_cost
		  WHERE cpc_period ~ '^20[0-9]{2}(0[1-9]|1[0-2])$'
		  ORDER BY cpc_period DESC LIMIT 1`,
	).Scan(&period)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve latest cost result period: %w", err)
	}
	return period, nil
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
