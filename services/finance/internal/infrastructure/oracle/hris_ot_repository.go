package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// OTDailyRow is one row of MGTHRIS.VIEW_BI_OT_DAILY (hours kept as decimal, never float).
type OTDailyRow struct {
	WorkDate      time.Time
	WeekStart     time.Time
	AttPeriod     string
	WeekAttPeriod string
	EmpSysID      string
	NIK           string
	EmpName       string
	Division      string
	DeptID        string
	DeptName      string
	ScheduleCode  string
	OTType        string
	Tier1         decimal.Decimal
	Tier2         decimal.Decimal
	Tier3         decimal.Decimal
	ActualHours   decimal.Decimal
	CalcHours     decimal.Decimal
	LimitHours    decimal.Decimal
	CompID        string // HD-19: company code (HMEMD_HMCOMP_COMP_ID)
	CompName      string // HD-19: company name; empty when the master row is missing
}

// HRISOTRepository reads HRIS views through the read-only BI_HR_RO connection.
type HRISOTRepository struct {
	client *Client
}

// NewHRISOTRepository constructs the repository.
func NewHRISOTRepository(client *Client) *HRISOTRepository {
	return &HRISOTRepository{client: client}
}

const otDailyQuery = `SELECT
	WORK_DATE, WEEK_START, ATT_PERIOD, WEEK_ATT_PERIOD,
	TO_CHAR(EMP_SYS_ID), NIK, EMP_NAME, DIVISION, TO_CHAR(DEPT_ID), DEPT_NAME, SCHEDULE_CODE, OT_TYPE,
	TO_CHAR(OT_TIER_1,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(OT_TIER_2,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(OT_TIER_3,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''),
	TO_CHAR(OT_ACTUAL_HOURS,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(OT_CALC_HOURS,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(OT_LIMIT_HOURS,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''),
	COMP_ID, COMP_NAME
FROM MGTHRIS.VIEW_BI_OT_DAILY
WHERE WORK_DATE BETWEEN TO_DATE(:1, 'YYYY-MM-DD') AND TO_DATE(:2, 'YYYY-MM-DD')`

// oracleDate formats a job date as YYYY-MM-DD for TO_DATE(:n, 'YYYY-MM-DD'), like the ERP readers: binding
// time.Time to a DATE column carries the hour/zone, which can shift the range edge by a day.
func oracleDate(t time.Time) string { return t.Format("2006-01-02") }

// FetchOTDaily returns view rows with work_date in [d1, d2] (dates come from the job, never hardcoded).
func (r *HRISOTRepository) FetchOTDaily(ctx context.Context, d1, d2 time.Time) ([]OTDailyRow, error) {
	rows, err := r.client.db.QueryContext(ctx, otDailyQuery, oracleDate(d1), oracleDate(d2))
	if err != nil {
		return nil, fmt.Errorf("query VIEW_BI_OT_DAILY: %w", err)
	}
	defer func() {
		if ce := rows.Close(); ce != nil {
			r.client.logger.Warn().Err(ce).Msg("close VIEW_BI_OT_DAILY rows")
		}
	}()

	var out []OTDailyRow
	for rows.Next() {
		var (
			x                                                    OTDailyRow
			sysID, nik, name, div, dept, deptName, sched, otType sql.NullString
			t1, t2, t3, act, calc, lim                           sql.NullString
			compID, compName                                     sql.NullString
		)
		if err := rows.Scan(&x.WorkDate, &x.WeekStart, &x.AttPeriod, &x.WeekAttPeriod,
			&sysID, &nik, &name, &div, &dept, &deptName, &sched, &otType,
			&t1, &t2, &t3, &act, &calc, &lim, &compID, &compName); err != nil {
			return nil, fmt.Errorf("scan VIEW_BI_OT_DAILY: %w", err)
		}
		x.EmpSysID, x.NIK, x.EmpName, x.Division = sysID.String, nik.String, name.String, div.String
		x.DeptID, x.DeptName, x.ScheduleCode, x.OTType = dept.String, deptName.String, sched.String, otType.String
		x.CompID, x.CompName = compID.String, compName.String
		for _, p := range []struct {
			dst *decimal.Decimal
			src sql.NullString
		}{{&x.Tier1, t1}, {&x.Tier2, t2}, {&x.Tier3, t3}, {&x.ActualHours, act}, {&x.CalcHours, calc}, {&x.LimitHours, lim}} {
			d, err := parseHours(p.src)
			if err != nil {
				return nil, err
			}
			*p.dst = d
		}
		x.WorkDate, x.WeekStart = dateUTC(x.WorkDate), dateUTC(x.WeekStart)
		out = append(out, x)
	}
	return out, rows.Err()
}

// parseHours converts an Oracle NUMBER (as text) to decimal; NULL → 0.
func parseHours(s sql.NullString) (decimal.Decimal, error) {
	if !s.Valid || s.String == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse hours %q: %w", s.String, err)
	}
	return d, nil
}

// dateUTC keeps the calendar date and drops driver time-zone offsets (DATE columns have no zone).
func dateUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
