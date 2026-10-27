package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ATTDailyRow is one row of MGTHRIS.VIEW_BI_ATT_DAILY (hours/minutes kept as decimal, never float).
// Nullable timestamps are zero when NULL. Names are not read (HD-07: NIK only in staging).
type ATTDailyRow struct {
	WorkDate          time.Time
	WeekStart         time.Time
	AttPeriod         string
	EmpSysID          string
	NIK               string
	Division          string
	DeptID            string
	DeptName          string
	EmpStatus         string
	EmpGroup          string
	ScheduleCode      string
	SchedStatus       string
	SchedType         string
	SchedIn           time.Time
	SchedOut          time.Time
	ActualIn          time.Time
	ActualOut         time.Time
	ActualHour        decimal.Decimal
	DayCatRekap       string
	DayCat            string
	IsScheduledWork   int
	LateMinutes       decimal.Decimal
	EarlyLeaveMinutes decimal.Decimal
}

// HRISATTRepository reads VIEW_BI_ATT_DAILY through the read-only BI_HR_RO connection.
type HRISATTRepository struct {
	client *Client
}

// NewHRISATTRepository constructs the repository.
func NewHRISATTRepository(client *Client) *HRISATTRepository {
	return &HRISATTRepository{client: client}
}

const numText = `'TM9','NLS_NUMERIC_CHARACTERS=''.,'''`

const attDailyQuery = `SELECT
	WORK_DATE, WEEK_START, ATT_PERIOD,
	TO_CHAR(EMP_SYS_ID), NIK, DIVISION, TO_CHAR(DEPT_ID), DEPT_NAME, EMP_STATUS, EMP_GROUP,
	SCHEDULE_CODE, SCHED_STATUS, SCHED_TYPE, SCHED_IN, SCHED_OUT, ACTUAL_IN, ACTUAL_OUT,
	TO_CHAR(ACTUAL_HOUR, ` + numText + `), DAY_CAT_REKAP, DAY_CAT, IS_SCHEDULED_WORK,
	TO_CHAR(LATE_MINUTES, ` + numText + `), TO_CHAR(EARLY_LEAVE_MINUTES, ` + numText + `)
FROM MGTHRIS.VIEW_BI_ATT_DAILY
WHERE WORK_DATE BETWEEN TO_DATE(:1, 'YYYY-MM-DD') AND TO_DATE(:2, 'YYYY-MM-DD')`

// FetchATTDaily returns view rows with work_date in [d1, d2] (dates come from the job, never hardcoded).
func (r *HRISATTRepository) FetchATTDaily(ctx context.Context, d1, d2 time.Time) ([]ATTDailyRow, error) {
	rows, err := r.client.db.QueryContext(ctx, attDailyQuery, oracleDate(d1), oracleDate(d2))
	if err != nil {
		return nil, fmt.Errorf("query VIEW_BI_ATT_DAILY: %w", err)
	}
	defer func() {
		if ce := rows.Close(); ce != nil {
			r.client.logger.Warn().Err(ce).Msg("close VIEW_BI_ATT_DAILY rows")
		}
	}()

	var out []ATTDailyRow
	for rows.Next() {
		var (
			x                                            ATTDailyRow
			sysID, nik, div, dept, deptName, status, grp sql.NullString
			sched, schedStatus, schedType, catRekap, cat sql.NullString
			schedIn, schedOut, actIn, actOut             sql.NullTime
			hour, late, early                            sql.NullString
			isSched                                      sql.NullInt64
		)
		if err := rows.Scan(&x.WorkDate, &x.WeekStart, &x.AttPeriod,
			&sysID, &nik, &div, &dept, &deptName, &status, &grp,
			&sched, &schedStatus, &schedType, &schedIn, &schedOut, &actIn, &actOut,
			&hour, &catRekap, &cat, &isSched, &late, &early); err != nil {
			return nil, fmt.Errorf("scan VIEW_BI_ATT_DAILY: %w", err)
		}
		x.EmpSysID, x.NIK, x.Division, x.DeptID, x.DeptName = sysID.String, nik.String, div.String, dept.String, deptName.String
		x.EmpStatus, x.EmpGroup = status.String, grp.String
		x.ScheduleCode, x.SchedStatus, x.SchedType = sched.String, schedStatus.String, schedType.String
		x.DayCatRekap, x.DayCat, x.IsScheduledWork = catRekap.String, cat.String, int(isSched.Int64)
		x.SchedIn, x.SchedOut, x.ActualIn, x.ActualOut = wallClock(schedIn), wallClock(schedOut), wallClock(actIn), wallClock(actOut)
		for _, p := range []struct {
			dst *decimal.Decimal
			src sql.NullString
		}{{&x.ActualHour, hour}, {&x.LateMinutes, late}, {&x.EarlyLeaveMinutes, early}} {
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

// wallClock drops the driver zone of an Oracle DATE (no zone) and returns zero for NULL.
func wallClock(t sql.NullTime) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	v := t.Time
	return time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), 0, time.UTC)
}
