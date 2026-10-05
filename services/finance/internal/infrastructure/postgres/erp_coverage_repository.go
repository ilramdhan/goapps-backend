package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErpCoverageRepository implements erpintegration.CoverageRepository over
// cst_erp_coverage (migration 000549; design §4.2, C-5; plan-04 P3-T2).
// Replace is DELETE + INSERT of the batch's rows in one PG transaction.
type ErpCoverageRepository struct{ db *DB }

// NewErpCoverageRepository constructs the repository.
func NewErpCoverageRepository(db *DB) *ErpCoverageRepository {
	return &ErpCoverageRepository{db: db}
}

var _ erpintegration.CoverageRepository = (*ErpCoverageRepository)(nil)

// Replace deletes and re-inserts the batch's coverage rows in one transaction.
func (r *ErpCoverageRepository) Replace(ctx context.Context, batchID int64, lines []erpintegration.CoverageLine) (int64, error) {
	var n int64
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = replaceCoverage(ctx, tx, batchID, lines)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

const erpCoverageListSQL = `
	SELECT cec_batch_id, cec_item_kind, cec_item_code, cec_shade_code, cec_grade_codes,
	       cec_product_sys_id, cec_cost_id, cec_cost_version, cec_status,
	       COALESCE(cec_reason, ''), cec_qty_kg::text, cec_candidates::text, cec_id
	  FROM cst_erp_coverage
	 WHERE cec_batch_id = $1
	   AND (cardinality($2::text[]) = 0 OR cec_status = ANY($2::text[]))
	 ORDER BY cec_item_code, cec_shade_code`

// List returns the batch's coverage rows ordered by (item, shade).
func (r *ErpCoverageRepository) List(ctx context.Context, batchID int64, statuses ...erpintegration.CoverageStatus) (out []erpintegration.CoverageLine, err error) {
	filter := make([]string, 0, len(statuses))
	for _, s := range statuses {
		filter = append(filter, string(s))
	}
	rows, err := r.db.QueryContext(ctx, erpCoverageListSQL, batchID, pq.Array(filter))
	if err != nil {
		return nil, fmt.Errorf("erp coverage list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp coverage list")
	for rows.Next() {
		l, err := scanCoverageLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp coverage list rows: %w", err)
	}
	return out, nil
}

// Counts returns the per-status row count of the batch.
func (r *ErpCoverageRepository) Counts(ctx context.Context, batchID int64) (out erpintegration.CoverageCounts, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cec_status, count(*) FROM cst_erp_coverage WHERE cec_batch_id = $1 GROUP BY cec_status`, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp coverage counts: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp coverage counts")
	out = erpintegration.CoverageCounts{}
	for rows.Next() {
		var (
			status string
			n      int64
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("erp coverage counts scan: %w", err)
		}
		st, err := erpintegration.ParseCoverageStatus(status)
		if err != nil {
			return nil, err
		}
		out[st] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp coverage counts rows: %w", err)
	}
	return out, nil
}

const erpCoverageDeleteSQL = `DELETE FROM cst_erp_coverage WHERE cec_batch_id = $1`

// erpCoverageInsertSQL inserts one line; cec_grade_codes is a TEXT[] per row,
// which unnest cannot carry as a 2-D array of ragged rows, so coverage is
// inserted row by row inside the replace transaction (a few thousand rows per
// period at most).
const erpCoverageInsertSQL = `
	INSERT INTO cst_erp_coverage (
		cec_batch_id, cec_item_kind, cec_item_code, cec_shade_code, cec_grade_codes,
		cec_product_sys_id, cec_cost_id, cec_cost_version, cec_status, cec_reason,
		cec_qty_kg, cec_candidates)
	VALUES ($1, $2, $3, $4, $5::text[], $6, $7, $8, $9, $10, $11::numeric, $12::jsonb)`

// replaceCoverage is the tx-scoped DELETE + INSERT.
func replaceCoverage(ctx context.Context, q erpQuerier, batchID int64, lines []erpintegration.CoverageLine) (int64, error) {
	if batchID <= 0 {
		return 0, fmt.Errorf("erp coverage replace: invalid batch id %d", batchID)
	}
	for i := range lines {
		if lines[i].BatchID != batchID {
			return 0, fmt.Errorf("erp coverage replace: line %d belongs to batch %d, not %d", i, lines[i].BatchID, batchID)
		}
		if err := lines[i].Validate(); err != nil {
			return 0, fmt.Errorf("erp coverage replace line %d: %w", i, err)
		}
	}
	if _, err := q.ExecContext(ctx, erpCoverageDeleteSQL, batchID); err != nil {
		return 0, fmt.Errorf("erp coverage delete: %w", err)
	}
	var n int64
	for i := range lines {
		l := &lines[i]
		grades := l.GradeCodes
		if grades == nil {
			grades = []string{}
		}
		if _, err := q.ExecContext(ctx, erpCoverageInsertSQL,
			batchID, string(l.Kind), l.ItemCode, l.ShadeCode, pq.Array(grades),
			erpNullInt64(l.ProductSysID), erpNullInt64(l.CostID), erpNullInt32(l.CostVersion),
			string(l.Status), nullString(l.Reason), l.QtyKg.String(), nullJSONArg(l.Candidates)); err != nil {
			return 0, fmt.Errorf("erp coverage insert %s/%s: %w", l.ItemCode, l.ShadeCode, err)
		}
		n++
	}
	return n, nil
}

func scanCoverageLine(s rowScanner) (erpintegration.CoverageLine, error) {
	var (
		l                 erpintegration.CoverageLine
		kind, status, qty string
		grades            pq.StringArray
		product, cost     sql.NullInt64
		version           sql.NullInt32
		candidates        sql.NullString
	)
	if err := s.Scan(&l.BatchID, &kind, &l.ItemCode, &l.ShadeCode, &grades,
		&product, &cost, &version, &status, &l.Reason, &qty, &candidates, &l.ID); err != nil {
		return erpintegration.CoverageLine{}, fmt.Errorf("erp coverage scan: %w", err)
	}
	k, err := erpintegration.ParseItemKind(kind)
	if err != nil {
		return erpintegration.CoverageLine{}, err
	}
	st, err := erpintegration.ParseCoverageStatus(status)
	if err != nil {
		return erpintegration.CoverageLine{}, err
	}
	l.Kind, l.Status = k, st
	l.GradeCodes = []string(grades)
	if product.Valid {
		v := product.Int64
		l.ProductSysID = &v
	}
	if cost.Valid {
		v := cost.Int64
		l.CostID = &v
	}
	if version.Valid {
		v := version.Int32
		l.CostVersion = &v
	}
	if candidates.Valid {
		l.Candidates = []byte(candidates.String)
	}
	if l.QtyKg, err = erpintegration.ParseDecimal(qty); err != nil {
		return erpintegration.CoverageLine{}, fmt.Errorf("erp coverage qty: %w", err)
	}
	return l, nil
}

func erpNullInt64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func erpNullInt32(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}

// GetByID returns one coverage row of the batch by cec_id, or
// erpintegration.ErrCoverageLineNotFound.
func (r *ErpCoverageRepository) GetByID(ctx context.Context, batchID, cecID int64) (erpintegration.CoverageLine, error) {
	row := r.db.QueryRowContext(ctx, strings.Replace(erpCoverageListSQL,
		"AND (cardinality($2::text[]) = 0 OR cec_status = ANY($2::text[]))", "AND cec_id = $2", 1)+" LIMIT 1", batchID, cecID)
	l, err := scanCoverageLine(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return erpintegration.CoverageLine{}, erpintegration.ErrCoverageLineNotFound
		}
		return erpintegration.CoverageLine{}, err
	}
	return l, nil
}
