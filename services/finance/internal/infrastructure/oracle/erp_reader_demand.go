package oracle

// erp_reader_demand.go holds the read-only ERP ADJ demand reader and the ADJ
// head probes (plan P3-T3, design §2 steps 1-2, §9.3, C-14, I-4). Every
// statement is a plain SELECT sent through ReadOnlyQuerier.QueryRO, so it
// passes CheckReadOnly; none uses FOR UPDATE.
//
// Numeric aggregates are converted to text Oracle-side with TO_CHAR 'TM9'
// and a fixed NLS decimal separator, then parsed into shopspring decimals:
// no value ever passes through a binary float.
//
// Column contract (pending lead confirmation D-D1): V_GOAPPS_ADJ_DEMAND
// exposes PERIOD (YYYYMM), TXN_CODE, ITEM_CODE, ITEM_NAME, GRADE_CODE_1,
// GRADE_CODE_2, HEAD_COUNT, ITEM_COUNT, RATE_VARIANTS, QTY_KG, MIN_RATE,
// MAX_RATE, ADJ_VAL, APPROVED_ITEMS, POSTED_ITEMS, GOAPPS_BATCH,
// GOAPPS_SOURCE. The base-table query returns the same columns (I-4).

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

const (
	// erpDemandViewQuery reads the demand contract view (C-14). The PERIOD
	// filter is mandatory (design Part 3 L-1).
	erpDemandViewQuery = `SELECT PERIOD, TXN_CODE, ITEM_CODE, ITEM_NAME, GRADE_CODE_1, GRADE_CODE_2,
       HEAD_COUNT, ITEM_COUNT, RATE_VARIANTS,
       TO_CHAR(QTY_KG, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') QTY_KG,
       TO_CHAR(MIN_RATE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MIN_RATE,
       TO_CHAR(MAX_RATE, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MAX_RATE,
       TO_CHAR(ADJ_VAL, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') ADJ_VAL,
       APPROVED_ITEMS, POSTED_ITEMS, GOAPPS_BATCH, GOAPPS_SOURCE
  FROM MGTDAT.V_GOAPPS_ADJ_DEMAND
 WHERE PERIOD = :1
   AND TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')`

	// erpDemandBaseQuery is the DEV-only equivalent over the base tables
	// (DATA_MAPPING §2.1, I-4). Binds :1 and :2 are both the YYYYMM period
	// (a sargable month range on ADJH_DT).
	erpDemandBaseQuery = `SELECT TO_CHAR(h.ADJH_DT, 'YYYYMM') PERIOD, h.ADJH_TXN_CODE TXN_CODE,
       i.ADJI_ITEM_CODE ITEM_CODE, MAX(it.ITEM_NAME) ITEM_NAME,
       i.ADJI_GRADE_CODE_1 GRADE_CODE_1, i.ADJI_GRADE_CODE_2 GRADE_CODE_2,
       COUNT(DISTINCT h.ADJH_SYS_ID) HEAD_COUNT, COUNT(*) ITEM_COUNT,
       COUNT(DISTINCT i.ADJI_RATE) RATE_VARIANTS,
       TO_CHAR(SUM(i.ADJI_QTY_BU) / 1000, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') QTY_KG,
       TO_CHAR(MIN(i.ADJI_RATE), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MIN_RATE,
       TO_CHAR(MAX(i.ADJI_RATE), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MAX_RATE,
       TO_CHAR(SUM(i.ADJI_VAL), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') ADJ_VAL,
       SUM(CASE WHEN h.ADJH_APPR_STATUS = 3 THEN 1 ELSE 0 END) APPROVED_ITEMS,
       SUM(CASE WHEN h.ADJH_POST_STATUS IS NOT NULL THEN 1 ELSE 0 END) POSTED_ITEMS,
       MAX(i.ADJI_FLEX_13) GOAPPS_BATCH, MAX(i.ADJI_FLEX_14) GOAPPS_SOURCE
  FROM MGTDAT.OT_ADJ_HEAD h
  JOIN MGTDAT.OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
  LEFT JOIN MGTDAT.OM_ITEM it ON it.ITEM_CODE = i.ADJI_ITEM_CODE
 WHERE h.ADJH_TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')
   AND h.ADJH_DT >= TO_DATE(:1, 'YYYYMM')
   AND h.ADJH_DT < ADD_MONTHS(TO_DATE(:2, 'YYYYMM'), 1)
 GROUP BY TO_CHAR(h.ADJH_DT, 'YYYYMM'), h.ADJH_TXN_CODE, i.ADJI_ITEM_CODE,
          i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2`

	// erpAdjHeadProbeQuery counts the period's heads (V-10, unlock, §S-R7).
	// Binds :1 and :2 are both the YYYYMM period.
	erpAdjHeadProbeQuery = `SELECT COUNT(*) HEADS,
       SUM(CASE WHEN h.ADJH_POST_STATUS IS NOT NULL THEN 1 ELSE 0 END) POSTED,
       SUM(CASE WHEN h.ADJH_APPR_STATUS = 3 THEN 1 ELSE 0 END) APPROVED,
       SUM(CASE WHEN h.ADJH_APPR_STATUS IS NULL THEN 1 ELSE 0 END) NULL_STATUS,
       SUM(CASE WHEN NVL(h.ADJH_APPR_STATUS, 0) != 3 AND h.ADJH_POST_STATUS IS NULL THEN 1 ELSE 0 END) ELIGIBLE
  FROM MGTDAT.OT_ADJ_HEAD h
 WHERE h.ADJH_TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')
   AND h.ADJH_DT >= TO_DATE(:1, 'YYYYMM')
   AND h.ADJH_DT < ADD_MONTHS(TO_DATE(:2, 'YYYYMM'), 1)`
)

// ErpDemandReader reads the ADJ demand of a period via the read-only querier.
type ErpDemandReader struct {
	q      ReadOnlyQuerier
	source erpintegration.DemandSource
}

var _ erpintegration.ErpDemandReader = (*ErpDemandReader)(nil)

// NewErpDemandReader builds the demand reader for the configured source. It
// fails closed: an unknown source, or base_tables when appEnv is production,
// returns an error and no reader.
func NewErpDemandReader(q ReadOnlyQuerier, source, appEnv string) (*ErpDemandReader, error) {
	src, err := erpintegration.ParseDemandSource(source)
	if err != nil {
		return nil, err
	}
	if src == erpintegration.DemandSourceBaseTables && productionEnvs[strings.ToLower(strings.TrimSpace(appEnv))] {
		return nil, erpintegration.ErrBaseTablesInProduction
	}
	return &ErpDemandReader{q: q, source: src}, nil
}

// Source returns the demand source in effect.
func (r *ErpDemandReader) Source() erpintegration.DemandSource { return r.source }

// LoadAdjDemand implements erpintegration.ErpDemandReader.
func (r *ErpDemandReader) LoadAdjDemand(ctx context.Context, period string) (out []erpintegration.ErpDemandRow, err error) {
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return nil, err
	}
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	var rows Rows
	if r.source == erpintegration.DemandSourceBaseTables {
		rows, err = r.q.QueryRO(ctx, erpDemandBaseQuery, period, period)
	} else {
		rows, err = r.q.QueryRO(ctx, erpDemandViewQuery, period)
	}
	if err != nil {
		return nil, fmt.Errorf("read ADJ demand (%s): %w", r.source, err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		row, serr := scanDemandRow(rows)
		if serr != nil {
			return nil, fmt.Errorf("read ADJ demand (%s): %w", r.source, serr)
		}
		if row.Period != period {
			return nil, fmt.Errorf("read ADJ demand (%s): row period %q does not match requested %q", r.source, row.Period, period)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ADJ demand (%s): iterate: %w", r.source, err)
	}
	return out, nil
}

// scanDemandRow scans one demand row in the column-contract order.
func scanDemandRow(rows Rows) (erpintegration.ErpDemandRow, error) {
	var (
		period, txn, item, name, grade, shade, batch, src sql.NullString
		heads, items, variants, approved, posted          sql.NullInt64
		qty, minRate, maxRate, adjVal                     sql.NullString
	)
	if err := rows.Scan(&period, &txn, &item, &name, &grade, &shade,
		&heads, &items, &variants, &qty, &minRate, &maxRate, &adjVal,
		&approved, &posted, &batch, &src); err != nil {
		return erpintegration.ErpDemandRow{}, fmt.Errorf("scan: %w", err)
	}
	row := erpintegration.ErpDemandRow{
		Period:        trimNull(period),
		TxnCode:       trimNull(txn),
		ItemCode:      trimNull(item),
		ItemName:      trimNull(name),
		GradeCode:     trimNull(grade),
		ShadeCode:     trimNull(shade),
		HeadCount:     heads.Int64,
		ItemCount:     items.Int64,
		RateVariants:  variants.Int64,
		ApprovedItems: approved.Int64,
		PostedItems:   posted.Int64,
		GoappsBatch:   trimNull(batch),
		GoappsSource:  trimNull(src),
	}
	key := fmt.Sprintf("%s/%s/%s/%s", row.TxnCode, row.ItemCode, row.GradeCode, row.ShadeCode)
	if row.ItemCode == "" {
		return row, fmt.Errorf("demand row %s: blank item code", key)
	}
	q, err := nullDecimal(qty)
	if err != nil {
		return row, fmt.Errorf("demand row %s QTY_KG: %w", key, err)
	}
	if !q.Valid {
		return row, fmt.Errorf("demand row %s: QTY_KG is NULL", key)
	}
	row.QtyKg = q.Decimal
	if row.MinRate, err = nullDecimal(minRate); err != nil {
		return row, fmt.Errorf("demand row %s MIN_RATE: %w", key, err)
	}
	if row.MaxRate, err = nullDecimal(maxRate); err != nil {
		return row, fmt.Errorf("demand row %s MAX_RATE: %w", key, err)
	}
	if row.AdjVal, err = nullDecimal(adjVal); err != nil {
		return row, fmt.Errorf("demand row %s ADJ_VAL: %w", key, err)
	}
	return row, nil
}

// ErpAdjReader answers the read-only ADJ head probes.
type ErpAdjReader struct {
	q ReadOnlyQuerier
}

var _ erpintegration.ErpAdjHeadProber = (*ErpAdjReader)(nil)

// NewErpAdjReader builds the head prober. q is normally Client.ReadOnly().
func NewErpAdjReader(q ReadOnlyQuerier) *ErpAdjReader {
	return &ErpAdjReader{q: q}
}

// ProbeHeads implements erpintegration.ErpAdjHeadProber.
func (r *ErpAdjReader) ProbeHeads(ctx context.Context, period string) (c erpintegration.AdjHeadCounts, err error) {
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return c, err
	}
	if r == nil || r.q == nil {
		return c, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, erpAdjHeadProbeQuery, period, period)
	if err != nil {
		return c, fmt.Errorf("probe ADJ heads: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return c, fmt.Errorf("probe ADJ heads: %w", err)
		}
		// COUNT(*) always yields one row; none means the probe is unusable.
		return c, fmt.Errorf("probe ADJ heads: no result row")
	}
	var heads, posted, approved, nullStatus, eligible sql.NullInt64
	if err := rows.Scan(&heads, &posted, &approved, &nullStatus, &eligible); err != nil {
		return c, fmt.Errorf("probe ADJ heads: scan: %w", err)
	}
	if err := rows.Err(); err != nil {
		return c, fmt.Errorf("probe ADJ heads: iterate: %w", err)
	}
	// SUM over zero heads is NULL in Oracle; that is a genuine zero.
	return erpintegration.AdjHeadCounts{
		Heads: heads.Int64, Posted: posted.Int64, Approved: approved.Int64,
		NullStatus: nullStatus.Int64, Eligible: eligible.Int64,
	}, nil
}

// ProbePosted implements erpintegration.ErpAdjHeadProber: the number of
// posted heads for the period (V-10, unlock).
func (r *ErpAdjReader) ProbePosted(ctx context.Context, period string) (int64, error) {
	c, err := r.ProbeHeads(ctx, period)
	if err != nil {
		return 0, err
	}
	return c.Posted, nil
}

// AdjPostedProbe is the real periodlock.AdjPostedProbe backed by the
// read-only head probe. It replaces periodlock.StaticAdjPostedProbe. Any
// probe error is returned, so the unlock handler fails closed.
type AdjPostedProbe struct {
	r *ErpAdjReader
}

var _ periodlock.AdjPostedProbe = (*AdjPostedProbe)(nil)

// NewAdjPostedProbe wraps an ErpAdjReader as a periodlock.AdjPostedProbe.
func NewAdjPostedProbe(r *ErpAdjReader) *AdjPostedProbe {
	return &AdjPostedProbe{r: r}
}

// IsAdjPosted implements periodlock.AdjPostedProbe.
func (p *AdjPostedProbe) IsAdjPosted(ctx context.Context, period string) (bool, error) {
	if p == nil || p.r == nil {
		return false, ErrNoReadConnection
	}
	n, err := p.r.ProbePosted(ctx, period)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func trimNull(s sql.NullString) string { return strings.TrimSpace(s.String) }

// nullDecimal parses an Oracle TO_CHAR numeric text into a NullDecimal.
func nullDecimal(s sql.NullString) (decimal.NullDecimal, error) {
	if !s.Valid {
		return decimal.NullDecimal{}, nil
	}
	return erpintegration.ParseNullDecimal(&s.String)
}
