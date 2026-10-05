package postgres

// erp_adj_snapshot_repository.go persists the immutable PG backup of the ADJ
// rows a valuation preview targets (plan-06 P5-T4 step 2; design Part 1
// §3.5 G7, §4.7). cst_erp_adj_snapshot is insert-only: the 000554 trigger
// trg_ceas_immutable raises on every UPDATE or DELETE, so this file only
// inserts and reads. Decimals travel as text and are cast to NUMERIC
// server-side; FLEX_01..14 are stored as a JSONB array (null = NULL).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erpAdjSnapshotInsertChunk bounds the rows per INSERT statement.
const erpAdjSnapshotInsertChunk = 500

const erpAdjSnapshotInsertSQL = `
	INSERT INTO cst_erp_adj_snapshot (
		ceas_preview_id, ceas_batch_id, ceas_period,
		ceas_adjh_sys_id, ceas_adji_sys_id, ceas_txn_code,
		ceas_head_appr_status, ceas_head_post_status,
		ceas_item_code, ceas_grade_code, ceas_shade_code,
		ceas_qty_bu, ceas_item_desc, ceas_rate, ceas_val, ceas_flex,
		ceas_new_rate, ceas_new_val, ceas_new_flex, ceas_row_hash)
	SELECT $1::uuid, $2, $3,
	       u.head, u.item, u.txn, u.appr, u.post,
	       u.code, u.grade, u.shade,
	       u.qty, u.descr, u.rate, u.val, u.flex,
	       u.new_rate, u.new_val, u.new_flex, u.row_hash
	  FROM unnest(
		$4::bigint[], $5::bigint[], $6::text[], $7::int[], $8::text[],
		$9::text[], $10::text[], $11::text[],
		$12::numeric[], $13::text[], $14::numeric[], $15::numeric[], $16::jsonb[],
		$17::numeric[], $18::numeric[], $19::jsonb[], $20::text[]
	  ) AS u(head, item, txn, appr, post, code, grade, shade,
	         qty, descr, rate, val, flex, new_rate, new_val, new_flex, row_hash)`

const erpAdjSnapshotListSQL = `
	SELECT ceas_adjh_sys_id, ceas_adji_sys_id, ceas_txn_code,
	       ceas_head_appr_status, ceas_head_post_status,
	       COALESCE(ceas_item_code, ''), COALESCE(ceas_grade_code, ''), COALESCE(ceas_shade_code, ''),
	       ceas_qty_bu::text, COALESCE(ceas_item_desc, ''), ceas_rate::text, ceas_val::text,
	       ceas_flex::text, ceas_new_rate::text, ceas_new_val::text, ceas_new_flex::text,
	       ceas_row_hash
	  FROM cst_erp_adj_snapshot
	 WHERE ceas_preview_id = $1::uuid
	 ORDER BY ceas_adji_sys_id`

// ErpAdjSnapshotRepository reads the snapshot rows of a preview. Rows are
// written only by ErpValuationPreviewRepository.CreateOpen, inside the
// preview transaction.
type ErpAdjSnapshotRepository struct{ db *DB }

// NewErpAdjSnapshotRepository constructs the repository.
func NewErpAdjSnapshotRepository(db *DB) *ErpAdjSnapshotRepository {
	return &ErpAdjSnapshotRepository{db: db}
}

// AdjSnapshotRecord is one stored snapshot row with its stored row hash.
type AdjSnapshotRecord struct {
	Row     erpintegration.AdjSnapshotRow
	RowHash string
}

// ListByPreview returns the preview's snapshot rows ordered by ADJI_SYS_ID.
func (r *ErpAdjSnapshotRepository) ListByPreview(ctx context.Context, previewID string) (out []AdjSnapshotRecord, err error) {
	rows, err := r.db.QueryContext(ctx, erpAdjSnapshotListSQL, previewID)
	if err != nil {
		return nil, fmt.Errorf("erp adj snapshot list: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("erp adj snapshot list close: %w", cerr)
		}
	}()
	for rows.Next() {
		rec, serr := scanAdjSnapshot(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp adj snapshot list iterate: %w", err)
	}
	return out, nil
}

func scanAdjSnapshot(s rowScanner) (AdjSnapshotRecord, error) {
	var (
		rec                             AdjSnapshotRecord
		appr                            sql.NullInt64
		post                            sql.NullString
		qty, rate, val, newRate, newVal sql.NullString
		flexJSON                        string
		newFlexJSON                     sql.NullString
	)
	row := &rec.Row
	if err := s.Scan(&row.HeadSysID, &row.ItemSysID, &row.TxnCode, &appr, &post,
		&row.ItemCode, &row.GradeCode, &row.ShadeCode,
		&qty, &row.ItemDesc, &rate, &val,
		&flexJSON, &newRate, &newVal, &newFlexJSON, &rec.RowHash); err != nil {
		return rec, fmt.Errorf("erp adj snapshot scan: %w", err)
	}
	if appr.Valid {
		v := appr.Int64
		row.HeadApprStatus = &v
	}
	if post.Valid {
		v := post.String
		row.HeadPostStatus = &v
	}
	for _, p := range []struct {
		dst *decimal.NullDecimal
		src sql.NullString
	}{{&row.QtyBu, qty}, {&row.Rate, rate}, {&row.Val, val}, {&row.NewRate, newRate}, {&row.NewVal, newVal}} {
		d, err := parseNullDecimalText(p.src)
		if err != nil {
			return rec, fmt.Errorf("erp adj snapshot item %d decimal: %w", row.ItemSysID, err)
		}
		*p.dst = d
	}
	var flex []*string
	if err := json.Unmarshal([]byte(flexJSON), &flex); err != nil {
		return rec, fmt.Errorf("erp adj snapshot item %d flex: %w", row.ItemSysID, err)
	}
	copy(row.Flex[:], flex)
	if newFlexJSON.Valid {
		var nf []string
		if err := json.Unmarshal([]byte(newFlexJSON.String), &nf); err != nil {
			return rec, fmt.Errorf("erp adj snapshot item %d new flex: %w", row.ItemSysID, err)
		}
		var arr [erpintegration.AdjFlexCount]string
		copy(arr[:], nf)
		row.NewFlex = &arr
	}
	return rec, nil
}

// adjSnapshotCols holds one insert chunk as parallel arrays.
type adjSnapshotCols struct {
	head, item                      []int64
	txn, rowHash, flex              []string
	appr                            []sql.NullInt64
	post, code, grade, shade, descr []sql.NullString
	qty, rate, val, newRate, newVal []sql.NullString
	newFlex                         []sql.NullString
}

func newAdjSnapshotCols(c int) *adjSnapshotCols {
	ns := func() []sql.NullString { return make([]sql.NullString, c) }
	return &adjSnapshotCols{
		head: make([]int64, c), item: make([]int64, c),
		txn: make([]string, c), rowHash: make([]string, c), flex: make([]string, c),
		appr: make([]sql.NullInt64, c),
		post: ns(), code: ns(), grade: ns(), shade: ns(), descr: ns(),
		qty: ns(), rate: ns(), val: ns(), newRate: ns(), newVal: ns(), newFlex: ns(),
	}
}

func (c *adjSnapshotCols) set(i int, r *erpintegration.AdjSnapshotRow) error {
	c.head[i], c.item[i], c.txn[i], c.rowHash[i] = r.HeadSysID, r.ItemSysID, r.TxnCode, r.RowHash()
	c.appr[i] = erpNullInt64(r.HeadApprStatus)
	if r.HeadPostStatus != nil {
		c.post[i] = sql.NullString{String: *r.HeadPostStatus, Valid: true}
	}
	c.code[i], c.grade[i], c.shade[i], c.descr[i] = nullString(r.ItemCode), nullString(r.GradeCode), nullString(r.ShadeCode), nullString(r.ItemDesc)
	c.qty[i], c.rate[i], c.val[i] = nullDecimalText(r.QtyBu), nullDecimalText(r.Rate), nullDecimalText(r.Val)
	c.newRate[i], c.newVal[i] = nullDecimalText(r.NewRate), nullDecimalText(r.NewVal)
	f, err := json.Marshal(r.Flex[:])
	if err != nil {
		return fmt.Errorf("erp adj snapshot item %d flex: %w", r.ItemSysID, err)
	}
	c.flex[i] = string(f)
	if r.NewFlex != nil {
		nf, err := json.Marshal(r.NewFlex[:])
		if err != nil {
			return fmt.Errorf("erp adj snapshot item %d new flex: %w", r.ItemSysID, err)
		}
		c.newFlex[i] = sql.NullString{String: string(nf), Valid: true}
	}
	return nil
}

func (c *adjSnapshotCols) args(previewID string, batchID int64, period string) []any {
	return []any{
		previewID, batchID, period,
		pq.Array(c.head), pq.Array(c.item), pq.Array(c.txn), pq.Array(c.appr), pq.Array(c.post),
		pq.Array(c.code), pq.Array(c.grade), pq.Array(c.shade),
		pq.Array(c.qty), pq.Array(c.descr), pq.Array(c.rate), pq.Array(c.val), pq.Array(c.flex),
		pq.Array(c.newRate), pq.Array(c.newVal), pq.Array(c.newFlex), pq.Array(c.rowHash),
	}
}

// insertAdjSnapshot inserts rows for previewID in chunks (tx-scoped).
func insertAdjSnapshot(ctx context.Context, q erpQuerier, previewID string, batchID int64, period string, rows []erpintegration.AdjSnapshotRow) (int64, error) {
	var n int64
	for start := 0; start < len(rows); start += erpAdjSnapshotInsertChunk {
		end := min(start+erpAdjSnapshotInsertChunk, len(rows))
		c := newAdjSnapshotCols(end - start)
		for i := start; i < end; i++ {
			if err := c.set(i-start, &rows[i]); err != nil {
				return 0, err
			}
		}
		res, err := q.ExecContext(ctx, erpAdjSnapshotInsertSQL, c.args(previewID, batchID, period)...)
		if err != nil {
			return 0, fmt.Errorf("erp adj snapshot insert rows %d..%d: %w", start, end-1, err)
		}
		k, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("erp adj snapshot rows affected: %w", err)
		}
		n += k
	}
	return n, nil
}
