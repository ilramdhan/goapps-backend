package bietl

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// TargetHROTStaging is the job target_type that loads VIEW_BI_OT_DAILY into bi_hr_stg_ot_daily.
const TargetHROTStaging = "HR_OT_STAGING"

// HRISWindowDays is the re-pull horizon (HRIS corrects data retroactively, HD-04).
const HRISWindowDays = 35

// OTSource fetches OT view rows from Oracle HRIS.
type OTSource interface {
	FetchOTDaily(ctx context.Context, d1, d2 time.Time) ([]oracleinfra.OTDailyRow, error)
}

// OTStaging replaces the staging window and re-aggregates its R1 facts in one transaction (H011).
type OTStaging interface {
	ReplaceOTDaily(ctx context.Context, d1, d2 time.Time, rows []oracleinfra.OTDailyRow, sourceID uuid.UUID) (int, error)
}

// TargetHRATTStaging is the job target_type that loads VIEW_BI_ATT_DAILY into bi_hr_stg_att_daily
// and aggregates HR_HEADCOUNT (H016, HD-12).
const TargetHRATTStaging = "HR_ATT_STAGING"

// ATTSource fetches attendance view rows from Oracle HRIS.
type ATTSource interface {
	FetchATTDaily(ctx context.Context, d1, d2 time.Time) ([]oracleinfra.ATTDailyRow, error)
}

// ATTStaging replaces the attendance staging window and re-aggregates headcount in one transaction.
type ATTStaging interface {
	ReplaceATTDaily(ctx context.Context, d1, d2 time.Time, rows []oracleinfra.ATTDailyRow, sourceID uuid.UUID) (int, error)
}

// HRISLoader loads HRIS views into staging (HD-03/HD-04). Job start/end/status are logged in
// bi_job_log by the job handler; this loader never logs employee names.
type HRISLoader struct {
	ot      OTSource
	staging OTStaging
	att     ATTSource
	attStg  ATTStaging
	now     func() time.Time
}

// NewHRISLoader constructs an HRISLoader.
func NewHRISLoader(ot OTSource, staging OTStaging) *HRISLoader {
	return &HRISLoader{ot: ot, staging: staging, now: time.Now}
}

// WithATT enables the HR_ATT_STAGING target (H016).
func (l *HRISLoader) WithATT(src ATTSource, stg ATTStaging) *HRISLoader {
	l.att, l.attStg = src, stg
	return l
}

// Load implements Runner.
func (l *HRISLoader) Load(ctx context.Context, targetType, _ string, sourceID uuid.UUID) (int, error) {
	switch targetType {
	case TargetHROTStaging:
		d1, d2 := HRISWindow(l.now())
		rows, err := l.ot.FetchOTDaily(ctx, d1, d2)
		if err != nil {
			return 0, fmt.Errorf("fetch OT daily: %w", err)
		}
		return l.staging.ReplaceOTDaily(ctx, d1, d2, rows, sourceID)
	case TargetHRATTStaging:
		if l.att == nil || l.attStg == nil {
			return 0, fmt.Errorf("HRIS target_type %q not configured", targetType)
		}
		d1, d2 := HRISWindow(l.now())
		rows, err := l.att.FetchATTDaily(ctx, d1, d2)
		if err != nil {
			return 0, fmt.Errorf("fetch ATT daily: %w", err)
		}
		return l.attStg.ReplaceATTDaily(ctx, d1, d2, rows, sourceID)
	default:
		return 0, fmt.Errorf("unsupported HRIS target_type %q", targetType)
	}
}

// hrisZone is the HRIS business time zone: "today"/"yesterday" are WIB dates whatever the server
// TZ is (the finance container runs in UTC; the 05:00 WIB job fires at 22:00 UTC the day before).
var hrisZone = loadHRISZone()

func loadHRISZone() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}

// HRISWindow returns [d1, d2] for the pull: d2 = yesterday (WIB); d1 = today−35 rounded down to the
// Monday of its week and to the start (day 26) of the payroll period it falls in, whichever is
// earlier, so weekly (R2) and monthly (26–25) aggregates are always recomputed over whole spans.
func HRISWindow(now time.Time) (d1, d2 time.Time) {
	now = now.In(hrisZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	d2 = today.AddDate(0, 0, -1)
	d1 = today.AddDate(0, 0, -HRISWindowDays)

	monday := d1.AddDate(0, 0, -((int(d1.Weekday()) + 6) % 7))
	periodStart := time.Date(d1.Year(), d1.Month(), 26, 0, 0, 0, 0, time.UTC)
	if d1.Day() < 26 {
		periodStart = periodStart.AddDate(0, -1, 0)
	}
	if periodStart.Before(monday) {
		return periodStart, d2
	}
	return monday, d2
}
