package oracle

// erp_reader_recon.go holds the read-only recon reader (plan-06 P5-T6;
// design Part 1 §2 step 12, Part 2 §9.1 "recon aggregate, CST_GOAPPS_*
// read-back"; PRD recon §4 / §4a / §7). It reads back:
//
//   - the GoApps batch header (CST_GOAPPS_STD_BATCH) and its cost rows
//     (CST_GOAPPS_STD_COST), so the control totals and md5 can be rebuilt
//     from what Oracle actually holds;
//   - the period's INVADJ / MBINVADJ / MBINVADJRP items aggregated per
//     (item, grade, shade): COUNT, COUNT(DISTINCT rate), MAX(rate),
//     SUM(qty)/1000, SUM(val), MAX(FLEX_13) and the items stamped with the
//     batch id.
//
// Every statement is a plain SELECT sent through ReadOnlyQuerier.QueryRO (it
// passes CheckReadOnly); none takes a row lock or writes anything. Numbers
// are rendered Oracle-side with TO_CHAR 'TM9' and a fixed NLS decimal
// separator and parsed into shopspring decimals (no binary float).

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

const (
	// erpReconBatchQuery reads the batch header. Bind :1 is the batch id.
	erpReconBatchQuery = `SELECT b.GSB_BATCH_ID, b.GSB_PERIOD, b.GSB_SEQ, b.GSB_STATUS, b.GSB_RULE_HASH,
       TO_CHAR(b.GSB_ROW_COUNT, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSB_ROW_COUNT,
       TO_CHAR(b.GSB_SUM_STD, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSB_SUM_STD,
       TO_CHAR(b.GSB_SUM_CONV, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSB_SUM_CONV,
       TO_CHAR(b.GSB_SUM_PVL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSB_SUM_PVL
  FROM MGTDAT.CST_GOAPPS_STD_BATCH b
 WHERE b.GSB_BATCH_ID = :1`

	// erpReconCostQuery reads the digest columns of the batch's cost rows
	// (the canonical text of erpintegration.CanonicalStdLine). Bind :1 is
	// the batch id.
	erpReconCostQuery = `SELECT c.GSC_ITEM_CODE, c.GSC_GRADE_CODE, c.GSC_SHADE_CODE, c.GSC_SOURCE, c.GSC_BASIS, c.GSC_FG_TYPE,
       TO_CHAR(c.GSC_CHP_COST, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_CHP_COST,
       TO_CHAR(c.GSC_AX_CONV_COST, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_AX_CONV_COST,
       TO_CHAR(c.GSC_CONV_COST, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_CONV_COST,
       TO_CHAR(c.GSC_SELLING_PRICE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_SELLING_PRICE,
       TO_CHAR(c.GSC_VALUE_LOSS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_VALUE_LOSS,
       TO_CHAR(c.GSC_AX_COST, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_AX_COST,
       TO_CHAR(c.GSC_STD_COST, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_STD_COST,
       TO_CHAR(c.GSC_PROD_VAL_LOSS, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') GSC_PROD_VAL_LOSS
  FROM MGTDAT.CST_GOAPPS_STD_COST c
 WHERE c.GSC_BATCH_ID = :1
 ORDER BY c.GSC_ITEM_CODE, c.GSC_GRADE_CODE, c.GSC_SHADE_CODE`

	// erpReconAdjQuery aggregates the period's guarded ADJ items per key.
	// Bind :1 is the batch id as text (the FLEX_13 stamp), :2 and :3 the
	// YYYYMM period (the same sargable month range as the snapshot query).
	erpReconAdjQuery = `SELECT i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2,
       COUNT(*) ITEMS,
       COUNT(DISTINCT i.ADJI_RATE) RATE_VARIANTS,
       TO_CHAR(MAX(i.ADJI_RATE), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MAX_RATE,
       TO_CHAR(SUM(i.ADJI_QTY_BU) / 1000, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') QTY_KG,
       TO_CHAR(SUM(i.ADJI_VAL), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') VAL,
       MAX(i.ADJI_FLEX_13) FLEX_13,
       SUM(CASE WHEN TRIM(i.ADJI_FLEX_13) = :1 THEN 1 ELSE 0 END) STAMPED
  FROM MGTDAT.OT_ADJ_HEAD h
  JOIN MGTDAT.OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
 WHERE h.ADJH_TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')
   AND h.ADJH_DT >= TO_DATE(:2, 'YYYYMM')
   AND h.ADJH_DT < ADD_MONTHS(TO_DATE(:3, 'YYYYMM'), 1)
 GROUP BY i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2
 ORDER BY i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2`
)

// ErpReconReader implements erpintegration.ErpReconReader.
type ErpReconReader struct {
	q ReadOnlyQuerier
}

var _ erpintegration.ErpReconReader = (*ErpReconReader)(nil)

// NewErpReconReader builds the reader. q is normally Client.ReadOnly().
func NewErpReconReader(q ReadOnlyQuerier) *ErpReconReader {
	return &ErpReconReader{q: q}
}

// ReadBackBatch implements erpintegration.ErpReconReader. A missing header
// returns Found=false (no error) and no cost rows are read.
func (r *ErpReconReader) ReadBackBatch(ctx context.Context, batchID int64) (erpintegration.OracleBatchReadBack, error) {
	out := erpintegration.OracleBatchReadBack{BatchID: batchID}
	if batchID <= 0 {
		return out, fmt.Errorf("read back batch: invalid batch id %d", batchID)
	}
	if r == nil || r.q == nil {
		return out, ErrNoReadConnection
	}
	found, err := r.readHeader(ctx, &out)
	if err != nil || !found {
		return out, err
	}
	out.Found = true
	rows, err := r.readCostRows(ctx, batchID)
	if err != nil {
		return out, err
	}
	out.CostRows = rows
	return out, nil
}

func (r *ErpReconReader) readHeader(ctx context.Context, out *erpintegration.OracleBatchReadBack) (found bool, err error) {
	rows, err := r.q.QueryRO(ctx, erpReconBatchQuery, out.BatchID)
	if err != nil {
		return false, fmt.Errorf("read back batch header: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, fmt.Errorf("read back batch header: %w", err)
		}
		return false, nil
	}
	var (
		id, seq                        sql.NullInt64
		period, status, hash           sql.NullString
		count, sumStd, sumConv, sumPvl sql.NullString
	)
	if err := rows.Scan(&id, &period, &seq, &status, &hash, &count, &sumStd, &sumConv, &sumPvl); err != nil {
		return false, fmt.Errorf("read back batch header: scan: %w", err)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read back batch header: iterate: %w", err)
	}
	totals, err := headerTotals(count, sumStd, sumConv, sumPvl)
	if err != nil {
		return false, fmt.Errorf("read back batch header: %w", err)
	}
	out.Period, out.Status, out.RuleHash = trimNull(period), trimNull(status), trimNull(hash)
	out.Seq, out.Header = int(seq.Int64), totals
	return true, nil
}

func headerTotals(count, sumStd, sumConv, sumPvl sql.NullString) (erpintegration.ControlTotals, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(count.String), 10, 64)
	if err != nil {
		return erpintegration.ControlTotals{}, fmt.Errorf("GSB_ROW_COUNT %q: %w", count.String, err)
	}
	var sums [3]decimal.Decimal
	for i, s := range [3]sql.NullString{sumStd, sumConv, sumPvl} {
		d, err := nullDecimal(s)
		if err != nil {
			return erpintegration.ControlTotals{}, fmt.Errorf("GSB_SUM_*: %w", err)
		}
		if d.Valid {
			sums[i] = d.Decimal
		}
	}
	return erpintegration.NewControlTotals(n, sums[0], sums[1], sums[2])
}

func (r *ErpReconReader) readCostRows(ctx context.Context, batchID int64) (out []erpintegration.StdRow, err error) {
	rows, err := r.q.QueryRO(ctx, erpReconCostQuery, batchID)
	if err != nil {
		return nil, fmt.Errorf("read back cost rows: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		row, serr := scanReconCostRow(rows)
		if serr != nil {
			return nil, fmt.Errorf("read back cost rows: %w", serr)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read back cost rows: iterate: %w", err)
	}
	return out, nil
}

// scanReconCostRow scans one row in the erpReconCostQuery column order.
func scanReconCostRow(rows Rows) (erpintegration.StdRow, error) {
	var (
		item, grade, shade, source, basis, fg sql.NullString
		nums                                  [8]sql.NullString
	)
	dest := []any{&item, &grade, &shade, &source, &basis, &fg}
	for i := range nums {
		dest = append(dest, &nums[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return erpintegration.StdRow{}, fmt.Errorf("scan: %w", err)
	}
	row := erpintegration.StdRow{
		Key:    erpintegration.ErpKey{ItemCode: trimNull(item), GradeCode: trimNull(grade), ShadeCode: trimNull(shade)},
		Source: erpintegration.StdSource(trimNull(source)), Basis: erprule.Basis(trimNull(basis)),
		FgType: trimNull(fg), Status: erpintegration.DeriveOK,
	}
	targets := []*decimal.NullDecimal{
		&row.ChpCost, &row.AxConvCost, &row.ConvCost, &row.SellingPrice,
		&row.ValueLoss, &row.AxCost, &row.StdCost, &row.ProdValLoss,
	}
	for i, t := range targets {
		d, err := nullDecimal(nums[i])
		if err != nil {
			return row, fmt.Errorf("%s: %w", row.Key, err)
		}
		*t = d
	}
	return row, nil
}

// ReadBackAdj implements erpintegration.ErpReconReader.
func (r *ErpReconReader) ReadBackAdj(ctx context.Context, period string, batchID int64) (out []erpintegration.AdjReadBackCombo, err error) {
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return nil, err
	}
	if batchID <= 0 {
		return nil, fmt.Errorf("read back ADJ: invalid batch id %d", batchID)
	}
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, erpReconAdjQuery, strconv.FormatInt(batchID, 10), period, period)
	if err != nil {
		return nil, fmt.Errorf("read back ADJ: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		c, serr := scanReconAdjRow(rows)
		if serr != nil {
			return nil, fmt.Errorf("read back ADJ: %w", serr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read back ADJ: iterate: %w", err)
	}
	return out, nil
}

// scanReconAdjRow scans one row in the erpReconAdjQuery column order.
func scanReconAdjRow(rows Rows) (erpintegration.AdjReadBackCombo, error) {
	var (
		item, grade, shade, rate, qty, val, flex sql.NullString
		items, variants, stamped                 sql.NullInt64
	)
	if err := rows.Scan(&item, &grade, &shade, &items, &variants, &rate, &qty, &val, &flex, &stamped); err != nil {
		return erpintegration.AdjReadBackCombo{}, fmt.Errorf("scan: %w", err)
	}
	c := erpintegration.AdjReadBackCombo{
		Key:   erpintegration.ErpKey{ItemCode: trimNull(item), GradeCode: trimNull(grade), ShadeCode: trimNull(shade)},
		Items: items.Int64, RateVariants: variants.Int64, Flex13: trimNull(flex), Stamped: stamped.Int64,
	}
	var err error
	if c.MaxRate, err = nullDecimal(rate); err != nil {
		return c, fmt.Errorf("%s MAX_RATE: %w", c.Key, err)
	}
	if c.QtyKg, err = nullDecimal(qty); err != nil {
		return c, fmt.Errorf("%s QTY_KG: %w", c.Key, err)
	}
	if c.Value, err = nullDecimal(val); err != nil {
		return c, fmt.Errorf("%s VAL: %w", c.Key, err)
	}
	return c, nil
}
