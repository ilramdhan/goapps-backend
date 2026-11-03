package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// PackProdDailyRow is one row of ASPAK.VIEW_BI_PACK_PROD_DAILY (P003, ETL§A). Kgs are decimal,
// never float; nullable view columns stay nullable so staging keeps NULL vs empty.
type PackProdDailyRow struct {
	PackDate          time.Time
	WeekStart         time.Time
	MonthStart        time.Time
	Period            string
	WeekPeriod        string
	DeptID            string
	DeptName          string
	ProdGroup         string
	PalletCode        sql.NullString
	PrdTypeName       sql.NullString
	PrdTypeOrder      sql.NullInt64
	GrpRepName        sql.NullString
	TransactionType   sql.NullString
	TransactionTypeMC sql.NullString
	GroupType         sql.NullString
	GroupTypeOrder    sql.NullInt64
	MergeReporting    sql.NullString
	PolyType          sql.NullString
	McNo              sql.NullString
	MachGroup         sql.NullString
	MachOrderRep      sql.NullInt64
	MergeNo           sql.NullString
	ProdDescr         sql.NullString
	Shade             sql.NullString
	PoyLine           sql.NullString
	GradeEligible     string
	AxQty             decimal.Decimal
	FqQty             decimal.Decimal
	DgQty             decimal.Decimal
	JltQty            decimal.Decimal
	TotalQty          decimal.Decimal
}

// ASPAKPackProdRepository reads the packed-production view through the read-only BI_HR_RO
// connection (HRIS_ORACLE, PP-03).
type ASPAKPackProdRepository struct {
	client *Client
}

// NewASPAKPackProdRepository constructs the repository.
func NewASPAKPackProdRepository(client *Client) *ASPAKPackProdRepository {
	return &ASPAKPackProdRepository{client: client}
}

// packProdDailyQuery is ETL§A. The pack_date filter is mandatory: without it the view scans the
// whole history since 2003. Dates are bound as YYYY-MM-DD text so no session time zone can shift
// the window, and the half-open upper bound keeps d2 even if pack_date carries a time part.
const packProdDailyQuery = `SELECT
	PACK_DATE, WEEK_START, MONTH_START, PERIOD, WEEK_PERIOD,
	DEPT_ID, DEPT_NAME, PROD_GROUP, PALLET_CODE, PRD_TYPE_NAME, PRD_TYPE_ORDER,
	GRP_REP_NAME, TRANSACTION_TYPE, TRANSACTION_TYPE_MC, GROUP_TYPE, GROUP_TYPE_ORDER,
	MERGE_REPORTING, POLY_TYPE, MC_NO, MACH_GROUP, MACH_ORDER_REP,
	MERGE_NO, PROD_DESCR, SHADE, POY_LINE, GRADE_ELIGIBLE,
	TO_CHAR(AX_QTY,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(FQ_QTY,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''),
	TO_CHAR(DG_QTY,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(JLT_QTY,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''),
	TO_CHAR(TOTAL_QTY,'TM9','NLS_NUMERIC_CHARACTERS=''.,''')
FROM ASPAK.VIEW_BI_PACK_PROD_DAILY
WHERE PACK_DATE >= TO_DATE(:1,'YYYY-MM-DD') AND PACK_DATE < TO_DATE(:2,'YYYY-MM-DD') + 1`

// packProdColumns is the number of select items in packProdDailyQuery (= staging columns, ETL§C).
const packProdColumns = 31

// FetchPackProdDaily returns view rows with pack_date in [d1, d2] (dates come from the job).
func (r *ASPAKPackProdRepository) FetchPackProdDaily(ctx context.Context, d1, d2 time.Time) ([]PackProdDailyRow, error) {
	rows, err := r.client.db.QueryContext(ctx, packProdDailyQuery, d1.Format(time.DateOnly), d2.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("query VIEW_BI_PACK_PROD_DAILY: %w", err)
	}
	defer func() {
		if ce := rows.Close(); ce != nil {
			r.client.logger.Warn().Err(ce).Msg("close VIEW_BI_PACK_PROD_DAILY rows")
		}
	}()

	var out []PackProdDailyRow
	for rows.Next() {
		x, err := scanPackProdRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// scanPackProdRow scans one view row. NOT NULL staging columns are scanned as NullString so a NULL
// from the view fails with the column and pack_date named, before any staging write.
func scanPackProdRow(rows *sql.Rows) (PackProdDailyRow, error) {
	var (
		x                                                          PackProdDailyRow
		packDate                                                   sql.NullTime
		period, weekPeriod, deptID, deptName, prodGroup, gradeElig sql.NullString
		ax, fq, dg, jlt, total                                     sql.NullString
	)
	if err := rows.Scan(&packDate, &x.WeekStart, &x.MonthStart, &period, &weekPeriod,
		&deptID, &deptName, &prodGroup, &x.PalletCode, &x.PrdTypeName, &x.PrdTypeOrder,
		&x.GrpRepName, &x.TransactionType, &x.TransactionTypeMC, &x.GroupType, &x.GroupTypeOrder,
		&x.MergeReporting, &x.PolyType, &x.McNo, &x.MachGroup, &x.MachOrderRep,
		&x.MergeNo, &x.ProdDescr, &x.Shade, &x.PoyLine, &gradeElig,
		&ax, &fq, &dg, &jlt, &total); err != nil {
		return x, fmt.Errorf("scan VIEW_BI_PACK_PROD_DAILY (pack_date %v): %w", packDate.Time.Format(time.DateOnly), err)
	}
	x.PackDate = packDateUTC(packDate.Time)
	x.WeekStart, x.MonthStart = packDateUTC(x.WeekStart), packDateUTC(x.MonthStart)
	for _, p := range []struct {
		name string
		dst  *string
		src  sql.NullString
	}{{"PERIOD", &x.Period, period}, {"WEEK_PERIOD", &x.WeekPeriod, weekPeriod}, {"DEPT_ID", &x.DeptID, deptID},
		{"DEPT_NAME", &x.DeptName, deptName}, {"PROD_GROUP", &x.ProdGroup, prodGroup}, {"GRADE_ELIGIBLE", &x.GradeEligible, gradeElig}} {
		if !p.src.Valid {
			return x, fmt.Errorf("VIEW_BI_PACK_PROD_DAILY %s is NULL on pack_date %s", p.name, x.PackDate.Format(time.DateOnly))
		}
		*p.dst = p.src.String
	}
	for _, p := range []struct {
		dst *decimal.Decimal
		src sql.NullString
	}{{&x.AxQty, ax}, {&x.FqQty, fq}, {&x.DgQty, dg}, {&x.JltQty, jlt}, {&x.TotalQty, total}} {
		d, err := parsePackQty(p.src)
		if err != nil {
			return x, fmt.Errorf("pack_date %s: %w", x.PackDate.Format(time.DateOnly), err)
		}
		*p.dst = d
	}
	return x, nil
}

// parsePackQty converts an Oracle NUMBER (as TM9 text) to decimal; NULL → 0 (staging NOT NULL DEFAULT 0).
func parsePackQty(s sql.NullString) (decimal.Decimal, error) {
	if !s.Valid || s.String == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse pack qty %q: %w", s.String, err)
	}
	return d, nil
}

// packDateUTC keeps the calendar date and drops driver time-zone offsets (DATE columns have no zone).
func packDateUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
