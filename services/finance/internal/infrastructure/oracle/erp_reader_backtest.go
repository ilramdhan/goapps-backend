package oracle

// erp_reader_backtest.go reads the legacy ADJ rates of a historical period
// for the SHADOW backtest (plan P5-T10a; design Part 2 §10.1). SELECT only,
// through ReadOnlyQuerier.QueryRO. The statement is the recon ADJ aggregate
// (erpReconAdjQuery) without the FLEX_13 stamp columns.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erpBacktestAdjQuery aggregates the period's ADJ items per key. Binds :1 and
// :2 are the YYYYMM period (same sargable month range as the recon query).
const erpBacktestAdjQuery = `SELECT i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2,
       COUNT(*) ITEMS,
       COUNT(DISTINCT i.ADJI_RATE) RATE_VARIANTS,
       TO_CHAR(MAX(i.ADJI_RATE), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') MAX_RATE
  FROM MGTDAT.OT_ADJ_HEAD h
  JOIN MGTDAT.OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
 WHERE h.ADJH_TXN_CODE IN ('INVADJ', 'MBINVADJ', 'MBINVADJRP')
   AND h.ADJH_DT >= TO_DATE(:1, 'YYYYMM')
   AND h.ADJH_DT < ADD_MONTHS(TO_DATE(:2, 'YYYYMM'), 1)
 GROUP BY i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2
 ORDER BY i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2`

// LegacyAdjRateReader implements erpintegration.LegacyAdjRateReader.
type LegacyAdjRateReader struct {
	q ReadOnlyQuerier
}

var _ erpintegration.LegacyAdjRateReader = (*LegacyAdjRateReader)(nil)

// NewLegacyAdjRateReader builds the reader. q is normally Client.ReadOnly().
func NewLegacyAdjRateReader(q ReadOnlyQuerier) *LegacyAdjRateReader {
	return &LegacyAdjRateReader{q: q}
}

// ReadLegacyAdjRates implements erpintegration.LegacyAdjRateReader.
func (r *LegacyAdjRateReader) ReadLegacyAdjRates(ctx context.Context, period string) (out []erpintegration.LegacyAdjRate, err error) {
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return nil, fmt.Errorf("read legacy ADJ rates: %w", err)
	}
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, erpBacktestAdjQuery, period, period)
	if err != nil {
		return nil, fmt.Errorf("read legacy ADJ rates: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		var (
			item, grade, shade, rate sql.NullString
			items, variants          sql.NullInt64
		)
		if err := rows.Scan(&item, &grade, &shade, &items, &variants, &rate); err != nil {
			return nil, fmt.Errorf("read legacy ADJ rates: scan: %w", err)
		}
		l := erpintegration.LegacyAdjRate{
			Key:   erpintegration.ErpKey{ItemCode: trimNull(item), GradeCode: trimNull(grade), ShadeCode: trimNull(shade)},
			Items: items.Int64, RateVariants: variants.Int64,
		}
		if l.MaxRate, err = nullDecimal(rate); err != nil {
			return nil, fmt.Errorf("read legacy ADJ rates: %s MAX_RATE: %w", l.Key, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy ADJ rates: iterate: %w", err)
	}
	return out, nil
}
