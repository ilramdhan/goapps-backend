package oracle

// erp_reader_adj_snapshot.go holds the read-only ADJ snapshot reader (plan-06
// P5-T4 step 2; design Part 1 §3.2 G6/G7, §S-R8; Part 3 §13.2 layer 1). It
// SELECTs every OT_ADJ_ITEM row of the period's guarded ADJ heads together
// with the head approval / post status, so the valuation preview can split
// the eligible set from the deny list and store the PG snapshot before any
// writer call. The statement is a plain SELECT sent through
// ReadOnlyQuerier.QueryRO (it passes CheckReadOnly); it takes no row lock
// (no FOR UPDATE / NOWAIT) and writes nothing.
//
// Numbers are rendered Oracle-side with TO_CHAR 'TM9' and a fixed NLS
// decimal separator and parsed into shopspring decimals: no value ever
// passes through a binary float. ADJH_POST_STATUS is read with a plain
// TO_CHAR so the query works whether the column is VARCHAR2 or NUMBER.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erpAdjSnapshotQuery reads the period's guarded ADJ items with their head
// status. Binds :1 and :2 are both the YYYYMM period (sargable month range
// on ADJH_DT, the same range as the demand base query and the head probe).
const erpAdjSnapshotQuery = `SELECT h.ADJH_SYS_ID, i.ADJI_SYS_ID, h.ADJH_TXN_CODE, h.ADJH_APPR_STATUS,
       TO_CHAR(h.ADJH_POST_STATUS) ADJH_POST_STATUS,
       i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2,
       TO_CHAR(i.ADJI_QTY_BU, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') ADJI_QTY_BU,
       i.ADJI_ITEM_DESC,
       TO_CHAR(i.ADJI_RATE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') ADJI_RATE,
       TO_CHAR(i.ADJI_VAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') ADJI_VAL,
       i.ADJI_FLEX_01, i.ADJI_FLEX_02, i.ADJI_FLEX_03, i.ADJI_FLEX_04, i.ADJI_FLEX_05,
       i.ADJI_FLEX_06, i.ADJI_FLEX_07, i.ADJI_FLEX_08, i.ADJI_FLEX_09, i.ADJI_FLEX_10,
       i.ADJI_FLEX_11, i.ADJI_FLEX_12, i.ADJI_FLEX_13, i.ADJI_FLEX_14
  FROM MGTDAT.OT_ADJ_HEAD h
  JOIN MGTDAT.OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
 WHERE h.ADJH_TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')
   AND h.ADJH_DT >= TO_DATE(:1, 'YYYYMM')
   AND h.ADJH_DT < ADD_MONTHS(TO_DATE(:2, 'YYYYMM'), 1)
 ORDER BY i.ADJI_SYS_ID`

// erpAdjSnapshotFixedCols is the number of columns before FLEX_01.
const erpAdjSnapshotFixedCols = 12

// ErpAdjSnapshotReader implements erpintegration.AdjSnapshotReader.
type ErpAdjSnapshotReader struct {
	q ReadOnlyQuerier
}

var _ erpintegration.AdjSnapshotReader = (*ErpAdjSnapshotReader)(nil)

// NewErpAdjSnapshotReader builds the reader. q is normally Client.ReadOnly().
func NewErpAdjSnapshotReader(q ReadOnlyQuerier) *ErpAdjSnapshotReader {
	return &ErpAdjSnapshotReader{q: q}
}

// SnapshotAdjRows implements erpintegration.AdjSnapshotReader: every item of
// the period's INVADJ / MBINVADJ / MBINVADJRP heads, ordered by ADJI_SYS_ID.
// Posted and approved heads are included (the caller applies the deny list
// and reports them); codes and text are returned exactly as stored.
func (r *ErpAdjSnapshotReader) SnapshotAdjRows(ctx context.Context, period string) (out []erpintegration.AdjSnapshotRow, err error) {
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return nil, err
	}
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, erpAdjSnapshotQuery, period, period)
	if err != nil {
		return nil, fmt.Errorf("read ADJ snapshot: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		row, serr := scanAdjSnapshotRow(rows)
		if serr != nil {
			return nil, fmt.Errorf("read ADJ snapshot: %w", serr)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ADJ snapshot: iterate: %w", err)
	}
	return out, nil
}

// scanAdjSnapshotRow scans one row in the erpAdjSnapshotQuery column order.
func scanAdjSnapshotRow(rows Rows) (erpintegration.AdjSnapshotRow, error) {
	var (
		head, item, appr              sql.NullInt64
		txn, post, code, grade, shade sql.NullString
		qty, desc, rate, val          sql.NullString
		flex                          [erpintegration.AdjFlexCount]sql.NullString
		row                           erpintegration.AdjSnapshotRow
		dest                          = make([]any, 0, erpAdjSnapshotFixedCols+erpintegration.AdjFlexCount)
	)
	dest = append(dest, &head, &item, &txn, &appr, &post, &code, &grade, &shade, &qty, &desc, &rate, &val)
	for i := range flex {
		dest = append(dest, &flex[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return row, fmt.Errorf("scan: %w", err)
	}
	if !head.Valid || !item.Valid {
		return row, fmt.Errorf("snapshot row: NULL ADJH_SYS_ID / ADJI_SYS_ID")
	}
	row = erpintegration.AdjSnapshotRow{
		HeadSysID: head.Int64, ItemSysID: item.Int64,
		TxnCode: txn.String, ItemCode: code.String, GradeCode: grade.String, ShadeCode: shade.String,
		ItemDesc: desc.String,
	}
	if appr.Valid {
		v := appr.Int64
		row.HeadApprStatus = &v
	}
	if post.Valid {
		v := post.String
		row.HeadPostStatus = &v
	}
	var err error
	if row.QtyBu, err = nullDecimal(qty); err != nil {
		return row, fmt.Errorf("item %d ADJI_QTY_BU: %w", item.Int64, err)
	}
	if row.Rate, err = nullDecimal(rate); err != nil {
		return row, fmt.Errorf("item %d ADJI_RATE: %w", item.Int64, err)
	}
	if row.Val, err = nullDecimal(val); err != nil {
		return row, fmt.Errorf("item %d ADJI_VAL: %w", item.Int64, err)
	}
	for i, f := range flex {
		if f.Valid {
			v := f.String
			row.Flex[i] = &v
		}
	}
	return row, nil
}
