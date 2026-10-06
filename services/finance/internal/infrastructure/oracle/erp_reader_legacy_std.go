package oracle

// erp_reader_legacy_std.go holds the read-only reader of the legacy standard
// cost table MGTDAT.OT_STD_COST_PRODUCTS_MGT (plan P3-T6, design Part 2
// §9.3, I-5). It is used by the one-off ERP attribute backfill only. Every
// statement is a plain SELECT sent through ReadOnlyQuerier.QueryRO (so it
// passes CheckReadOnly); nothing here writes Oracle and nothing uses
// FOR UPDATE.
//
// Column names follow design research-legacy §3b. FG_PRD_PER_DAY is listed
// there as "NUMBER?" (unconfirmed, never written by the legacy code seen), so
// it is only selected when the reader is built WithPrdPerDay(true); by default
// the query selects a NULL in its place and cannot fail on a missing column.
// FG_MS_BATCH_ITEM may be VARCHAR2 or NUMBER, so it is read with a plain
// TO_CHAR, which works for both. Numeric text uses TM9 with a fixed NLS
// decimal separator and is parsed into shopspring decimals, never floats.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

const (
	// legacyStdSelectPrefix is the column list shared by all variants.
	legacyStdSelectPrefix = `SELECT FG_ITEM_CODE, FG_ITEM_GRADE, FG_ITEM_SHADE, FG_ITEM_TYPE, FG_TYPE,
       FG_CHP_ITEM_CODE, TO_CHAR(FG_MS_BATCH_ITEM) FG_MS_BATCH_ITEM, `
	// legacyStdPrdColumn reads FG_PRD_PER_DAY as decimal text.
	legacyStdPrdColumn = `TO_CHAR(FG_PRD_PER_DAY, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') FG_PRD_PER_DAY`
	// legacyStdPrdNull stands in for FG_PRD_PER_DAY when it is not read.
	legacyStdPrdNull = `CAST(NULL AS VARCHAR2(64)) FG_PRD_PER_DAY`
	// legacyStdFrom is the table and the always-on filter.
	legacyStdFrom = `
  FROM MGTDAT.OT_STD_COST_PRODUCTS_MGT
 WHERE FG_ITEM_CODE IS NOT NULL`
	// legacyStdPeriodFilter keeps rows created up to the end of the YYYYMM
	// period bound to :1 (rows without a creation date are kept).
	legacyStdPeriodFilter = `
   AND (FG_ITEM_CR_DT IS NULL OR FG_ITEM_CR_DT < ADD_MONTHS(TO_DATE(:1, 'YYYYMM'), 1))`
	// legacyStdOrder makes the read deterministic.
	legacyStdOrder = `
 ORDER BY FG_ITEM_CODE, FG_ITEM_SHADE, FG_ITEM_GRADE`
)

// LegacyStdReader reads OT_STD_COST_PRODUCTS_MGT via the read-only querier.
type LegacyStdReader struct {
	q             ReadOnlyQuerier
	readPrdPerDay bool
}

var _ erpintegration.LegacyStdReader = (*LegacyStdReader)(nil)

// NewLegacyStdReader builds the reader. q is normally Client.ReadOnly().
func NewLegacyStdReader(q ReadOnlyQuerier) *LegacyStdReader {
	return &LegacyStdReader{q: q}
}

// WithPrdPerDay enables reading FG_PRD_PER_DAY. Enable it only after the
// column is confirmed with a user-run ALL_TAB_COLUMNS describe.
func (r *LegacyStdReader) WithPrdPerDay(on bool) *LegacyStdReader {
	r.readPrdPerDay = on
	return r
}

// query returns the SELECT for the configured variant.
func (r *LegacyStdReader) query(withPeriod bool) string {
	prd := legacyStdPrdNull
	if r.readPrdPerDay {
		prd = legacyStdPrdColumn
	}
	q := legacyStdSelectPrefix + prd + legacyStdFrom
	if withPeriod {
		q += legacyStdPeriodFilter
	}
	return q + legacyStdOrder
}

// List implements erpintegration.LegacyStdReader. period is YYYYMM or "" for
// the whole table.
func (r *LegacyStdReader) List(ctx context.Context, period string) (out []erpintegration.LegacyStdRow, err error) {
	period = strings.TrimSpace(period)
	if period != "" {
		if err := erpintegration.ValidateBatchPeriod(period); err != nil {
			return nil, err
		}
	}
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	var rows Rows
	if period != "" {
		rows, err = r.q.QueryRO(ctx, r.query(true), period)
	} else {
		rows, err = r.q.QueryRO(ctx, r.query(false))
	}
	if err != nil {
		return nil, fmt.Errorf("read OT_STD_COST_PRODUCTS_MGT: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		row, serr := scanLegacyStdRow(rows)
		if serr != nil {
			return nil, fmt.Errorf("read OT_STD_COST_PRODUCTS_MGT: %w", serr)
		}
		if row.ItemCode == "" {
			continue
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read OT_STD_COST_PRODUCTS_MGT: iterate: %w", err)
	}
	return out, nil
}

// scanLegacyStdRow scans one row in the query column order.
func scanLegacyStdRow(rows Rows) (erpintegration.LegacyStdRow, error) {
	var item, grade, shade, itemType, fgType, chp, ms, prd sql.NullString
	if err := rows.Scan(&item, &grade, &shade, &itemType, &fgType, &chp, &ms, &prd); err != nil {
		return erpintegration.LegacyStdRow{}, fmt.Errorf("scan: %w", err)
	}
	row := erpintegration.LegacyStdRow{
		ItemCode:    trimNull(item),
		GradeCode:   trimNull(grade),
		ShadeCode:   trimNull(shade),
		ItemType:    trimNull(itemType),
		FgType:      trimNull(fgType),
		ChpItemCode: trimNull(chp),
		MsBatchItem: trimNull(ms),
	}
	d, err := nullDecimal(prd)
	if err != nil {
		return row, fmt.Errorf("legacy std row %s/%s/%s FG_PRD_PER_DAY: %w", row.ItemCode, row.GradeCode, row.ShadeCode, err)
	}
	if d.Valid {
		row.PrdPerDay = d.Decimal.String()
	}
	return row, nil
}
