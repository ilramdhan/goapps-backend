package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErpDemandRepository implements erpintegration.DemandRepository over
// cst_erp_adj_demand (migration 000549; plan-04 P3-T2 step 3). Replace is
// DELETE + INSERT of the batch's rows in one PostgreSQL transaction, so a
// re-run with unchanged sources yields the same rows (AC-05). Nothing here
// touches Oracle.
type ErpDemandRepository struct{ db *DB }

// NewErpDemandRepository constructs the repository.
func NewErpDemandRepository(db *DB) *ErpDemandRepository {
	return &ErpDemandRepository{db: db}
}

var _ erpintegration.DemandRepository = (*ErpDemandRepository)(nil)

// Replace deletes and re-inserts the batch's demand rows in one transaction.
func (r *ErpDemandRepository) Replace(ctx context.Context, batchID int64, lines []erpintegration.DemandLine) (int64, error) {
	var n int64
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = replaceDemand(ctx, tx, batchID, lines)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// List returns the batch's demand rows ordered by the uk_ced key.
func (r *ErpDemandRepository) List(ctx context.Context, batchID int64) ([]erpintegration.DemandLine, error) {
	return listDemand(ctx, r.db, batchID)
}

// Count returns the number of demand rows of the batch.
func (r *ErpDemandRepository) Count(ctx context.Context, batchID int64) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM cst_erp_adj_demand WHERE ced_batch_id = $1`, batchID).Scan(&n); err != nil {
		return 0, fmt.Errorf("erp demand count: %w", err)
	}
	return n, nil
}

const erpDemandDeleteSQL = `DELETE FROM cst_erp_adj_demand WHERE ced_batch_id = $1`

// erpDemandInsertSQL inserts every line in one statement from parallel
// arrays (one element per line). Decimals travel as text and are cast to
// NUMERIC server-side: no float is involved at any point.
const erpDemandInsertSQL = `
	INSERT INTO cst_erp_adj_demand (
		ced_batch_id, ced_period, ced_txn_code, ced_item_kind, ced_item_code, ced_item_name,
		ced_grade_code, ced_shade_code, ced_item_count, ced_rate_variants, ced_qty_kg,
		ced_min_rate, ced_max_rate, ced_adj_val, ced_approved_items, ced_posted_items,
		ced_goapps_batch, ced_goapps_source, ced_loaded_at)
	SELECT $1, u.period, u.txn, u.kind, u.item, u.item_name,
	       u.grade, u.shade, u.item_count, u.rate_variants, u.qty,
	       u.min_rate, u.max_rate, u.adj_val, u.approved, u.posted,
	       u.gbatch, u.gsource, u.loaded_at
	  FROM unnest(
		$2::text[], $3::text[], $4::text[], $5::text[], $6::text[],
		$7::text[], $8::text[], $9::int[], $10::int[], $11::numeric[],
		$12::numeric[], $13::numeric[], $14::numeric[], $15::int[], $16::int[],
		$17::text[], $18::text[], $19::timestamptz[]
	  ) AS u(period, txn, kind, item, item_name,
	         grade, shade, item_count, rate_variants, qty,
	         min_rate, max_rate, adj_val, approved, posted,
	         gbatch, gsource, loaded_at)`

// replaceDemand is the tx-scoped DELETE + INSERT.
func replaceDemand(ctx context.Context, q erpQuerier, batchID int64, lines []erpintegration.DemandLine) (int64, error) {
	if batchID <= 0 {
		return 0, fmt.Errorf("erp demand replace: invalid batch id %d", batchID)
	}
	for i := range lines {
		if lines[i].BatchID != batchID {
			return 0, fmt.Errorf("erp demand replace: line %d belongs to batch %d, not %d", i, lines[i].BatchID, batchID)
		}
		if err := lines[i].Validate(); err != nil {
			return 0, fmt.Errorf("erp demand replace line %d: %w", i, err)
		}
	}
	if _, err := q.ExecContext(ctx, erpDemandDeleteSQL, batchID); err != nil {
		return 0, fmt.Errorf("erp demand delete: %w", err)
	}
	if len(lines) == 0 {
		return 0, nil
	}
	res, err := q.ExecContext(ctx, erpDemandInsertSQL, demandInsertArgs(batchID, lines)...)
	if err != nil {
		return 0, fmt.Errorf("erp demand insert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("erp demand rows affected: %w", err)
	}
	return n, nil
}

func demandInsertArgs(batchID int64, lines []erpintegration.DemandLine) []any {
	c := len(lines)
	var (
		period, txn, kind, item, name  = make([]string, c), make([]string, c), make([]string, c), make([]string, c), make([]sql.NullString, c)
		grade, shade, gbatch, gsource  = make([]string, c), make([]string, c), make([]sql.NullString, c), make([]sql.NullString, c)
		itemCount, rateVar, appr, post = make([]int64, c), make([]int64, c), make([]int64, c), make([]int64, c)
		qty                            = make([]string, c)
		minRate, maxRate, adjVal       = make([]sql.NullString, c), make([]sql.NullString, c), make([]sql.NullString, c)
		loadedAt                       = make([]string, c)
	)
	for i, l := range lines {
		period[i], txn[i], kind[i], item[i] = l.Period, l.TxnCode, string(l.Kind), l.ItemCode
		name[i] = nullString(l.ItemName)
		grade[i], shade[i] = l.GradeCode, l.ShadeCode
		itemCount[i], rateVar[i], appr[i], post[i] = l.ItemCount, l.RateVariants, l.ApprovedItems, l.PostedItems
		qty[i] = l.QtyKg.String()
		minRate[i], maxRate[i], adjVal[i] = nullDecimalText(l.MinRate), nullDecimalText(l.MaxRate), nullDecimalText(l.AdjVal)
		gbatch[i], gsource[i] = nullString(l.GoappsBatch), nullString(l.GoappsSource)
		loadedAt[i] = loadedAtText(l.LoadedAt)
	}
	return []any{
		batchID,
		pq.Array(period), pq.Array(txn), pq.Array(kind), pq.Array(item), pq.Array(name),
		pq.Array(grade), pq.Array(shade), pq.Array(itemCount), pq.Array(rateVar), pq.Array(qty),
		pq.Array(minRate), pq.Array(maxRate), pq.Array(adjVal), pq.Array(appr), pq.Array(post),
		pq.Array(gbatch), pq.Array(gsource), pq.Array(loadedAt),
	}
}

const erpDemandListSQL = `
	SELECT ced_batch_id, ced_period, ced_txn_code, ced_item_kind, ced_item_code,
	       COALESCE(ced_item_name, ''), ced_grade_code, ced_shade_code,
	       ced_item_count, ced_rate_variants, ced_qty_kg::text,
	       ced_min_rate::text, ced_max_rate::text, ced_adj_val::text,
	       ced_approved_items, ced_posted_items,
	       COALESCE(ced_goapps_batch, ''), COALESCE(ced_goapps_source, ''), ced_loaded_at
	  FROM cst_erp_adj_demand
	 WHERE ced_batch_id = $1
	 ORDER BY ced_txn_code, ced_item_code, ced_grade_code, ced_shade_code`

func listDemand(ctx context.Context, q erpQuerier, batchID int64) (out []erpintegration.DemandLine, err error) {
	rows, err := q.QueryContext(ctx, erpDemandListSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp demand list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp demand list")
	for rows.Next() {
		l, err := scanDemandLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp demand list rows: %w", err)
	}
	return out, nil
}

func scanDemandLine(s rowScanner) (erpintegration.DemandLine, error) {
	var (
		l                        erpintegration.DemandLine
		kind, qty                string
		minRate, maxRate, adjVal sql.NullString
	)
	if err := s.Scan(&l.BatchID, &l.Period, &l.TxnCode, &kind, &l.ItemCode,
		&l.ItemName, &l.GradeCode, &l.ShadeCode,
		&l.ItemCount, &l.RateVariants, &qty,
		&minRate, &maxRate, &adjVal,
		&l.ApprovedItems, &l.PostedItems,
		&l.GoappsBatch, &l.GoappsSource, &l.LoadedAt); err != nil {
		return erpintegration.DemandLine{}, fmt.Errorf("erp demand scan: %w", err)
	}
	k, err := erpintegration.ParseItemKind(kind)
	if err != nil {
		return erpintegration.DemandLine{}, err
	}
	l.Kind = k
	if l.QtyKg, err = erpintegration.ParseDecimal(qty); err != nil {
		return erpintegration.DemandLine{}, fmt.Errorf("erp demand qty: %w", err)
	}
	for _, p := range []struct {
		src sql.NullString
		dst *decimal.NullDecimal
	}{{minRate, &l.MinRate}, {maxRate, &l.MaxRate}, {adjVal, &l.AdjVal}} {
		if *p.dst, err = parseNullDecimalText(p.src); err != nil {
			return erpintegration.DemandLine{}, fmt.Errorf("erp demand decimal: %w", err)
		}
	}
	return l, nil
}

func nullDecimalText(d decimal.NullDecimal) sql.NullString {
	if !d.Valid {
		return sql.NullString{}
	}
	return sql.NullString{String: d.Decimal.String(), Valid: true}
}

func parseNullDecimalText(s sql.NullString) (decimal.NullDecimal, error) {
	if !s.Valid {
		return decimal.NullDecimal{}, nil
	}
	return erpintegration.ParseNullDecimal(&s.String)
}

// loadedAtText renders a load time for the timestamptz[] parameter. A zero
// time means "now" (the column default).
func loadedAtText(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(time.RFC3339Nano)
}
