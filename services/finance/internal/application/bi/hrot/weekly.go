package hrot

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Fact types written by the R2 aggregation (PRD §3 R2, §6, HD-09). All are per employee with the
// NIK in group_3 (HD-07), so employee-level access is gated like R1 and the dept restate (HD-15)
// applies; dept-level counts are SUMs of the value-1 rows.
const (
	TypeWeek       = "HR_OT_WEEK"        // WEEKLY; g2 status; jam hitung batas minggu itu
	TypeWeekStatus = "HR_OT_WEEK_STATUS" // WEEKLY; g2 OVER_LIMIT/NEAR_LIMIT; 1 per orang-minggu
	TypeWeekBucket = "HR_OT_WEEK_BUCKET" // WEEKLY; g2 kelompok jam; 1 per orang-minggu (histogram)
	TypeWeekExcess = "HR_OT_WEEK_EXCESS" // WEEKLY; jam di atas batas, hanya orang-minggu over-limit
	TypeChronic    = "HR_OT_CHRONIC"     // WEEKLY; minggu over-limit berturut-turut s.d. minggu ini
	TypeWeekDaily  = "HR_OT_WEEK_DAILY"  // DAILY; g2 jenis OT; jam hitung batas per hari (drill hari)

	// Over-limit per payroll month (H038, HD-20): g1 dept, no group_2/group_3, MONTHLY. Derived in SQL from
	// the stored HR_OT_WEEK_STATUS facts; a week counts in the payroll period of its Sunday (K10).
	TypeOverWeeks = "HR_OT_OVER_WEEKS" // orang-minggu over-limit
	TypeOverEmp   = "HR_OT_OVER_EMP"   // karyawan unik over-limit (tidak boleh dijumlah antar dept lintas bulan)
	TypeOverNik   = "HR_OT_OVER_NIK"   // H042, HD-22: g1 dept, g3 NIK; jumlah minggu over-limit NIK itu di bulan itu

	GrainWeekly = "WEEKLY"

	otTypeHoliday      = "holiday"
	otTypeShortHoliday = "short holiday"

	StatusOver  = "OVER_LIMIT"
	StatusNear  = "NEAR_LIMIT"
	StatusUnder = "UNDER_LIMIT"

	Bucket0to7   = "0-7"
	Bucket7to14  = "7-14"
	Bucket14to18 = "14-18"
	BucketOver18 = ">18"
)

var (
	// WeeklyLimit is the controlled weekly OT limit (K1); over-limit means strictly above it.
	WeeklyLimit = decimal.NewFromInt(14)
	// DefaultNearLimit is the default early-warning threshold (≥, PRD R2); overridable per job.
	DefaultNearLimit = decimal.NewFromInt(10)

	holidayFreeHours = decimal.RequireFromString("7.5")

	bucket7  = decimal.NewFromInt(7)
	bucket18 = decimal.NewFromInt(18)
)

// WeekRow is one staging row for R2 (one employee, one day, one OT type).
type WeekRow struct {
	WorkDate    time.Time
	NIK         string
	StagingDept string
	OTType      string
	CalcHours   decimal.Decimal // HATA_OT_CALCULATION, all approval states (K6, K7)
}

// CountedHours is the R2 measure of a row (K6, as VIEW_OT_WEEKLY_HM): the calculation hours, except
// Holiday/Short Holiday which count only the part above 7.5 h. Mirrors ot_limit_hours of
// VIEW_BI_OT_DAILY, kept here so the rule is tested; that column stays for reconciliation.
func (r WeekRow) CountedHours() decimal.Decimal {
	switch strings.ToLower(strings.TrimSpace(r.OTType)) {
	case otTypeHoliday, otTypeShortHoliday:
		return decimal.Max(r.CalcHours.Sub(holidayFreeHours), decimal.Zero)
	default:
		return r.CalcHours
	}
}

// WeeklyParams configures AggregateWeekly.
type WeeklyParams struct {
	From, To   time.Time // DAILY facts for WorkDate in [From, To]; weeks Monday(From)..Monday(To)
	LoadedFrom time.Time // first date staging holds: weeks starting earlier are incomplete, skipped
	NearLimit  decimal.Decimal
	// PrevStreak = HR_OT_CHRONIC of the week before Monday(From) per NIK, to continue streaks.
	PrevStreak map[string]int
}

// Monday returns the ISO week start (Monday) of d (K10, HD-05).
func Monday(d time.Time) time.Time {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// WeekStatus classifies a week's counted hours: > 14 over (K1), ≥ near threshold near, else under.
func WeekStatus(h, nearLimit decimal.Decimal) string {
	switch {
	case h.GreaterThan(WeeklyLimit):
		return StatusOver
	case h.GreaterThanOrEqual(nearLimit):
		return StatusNear
	default:
		return StatusUnder
	}
}

// WeekBucket returns the histogram group of a week's hours: 0–7, (7–14], (14–18], > 18. Upper
// bounds are inclusive so exactly 14 h (compliant) sits in 7-14 and exactly 18 h in 14-18.
func WeekBucket(h decimal.Decimal) string {
	switch {
	case h.GreaterThan(bucket18):
		return BucketOver18
	case h.GreaterThan(WeeklyLimit):
		return Bucket14to18
	case h.GreaterThan(bucket7):
		return Bucket7to14
	default:
		return Bucket0to7
	}
}

type empWeek struct {
	nik  string
	week time.Time
}

// AggregateWeekly builds R2 facts from staging rows covering Monday(p.From)..p.To.
//
// The week is Monday–Sunday regardless of payroll period: a week crossing the 25th/26th is one row
// dated on its Monday; which payroll period it belongs to (its Sunday, K10) is decided by the viewer
// filter (HD-05). Department = current dept (K15). Employee-weeks with zero counted hours produce no
// facts. Streaks continue from p.PrevStreak and reset on any week that is not over-limit.
func AggregateWeekly(rows []WeekRow, currentDept map[string]string, p WeeklyParams) []Fact {
	firstWeek, lastWeek := Monday(p.From), Monday(p.To)
	sums := map[empWeek]decimal.Decimal{}
	depts := map[string]string{}
	acc := accumulator{sums: map[key]decimal.Decimal{}}
	for _, r := range rows {
		w := Monday(r.WorkDate)
		if w.Before(firstWeek) || r.WorkDate.After(p.To) || w.Before(p.LoadedFrom) {
			continue
		}
		dept := deptOf(Row{NIK: r.NIK, StagingDept: r.StagingDept}, currentDept)
		depts[r.NIK] = dept
		acc.add(key{TypeWeekDaily, dept, r.OTType, r.NIK, GrainDaily, r.WorkDate}, r.CountedHours())
		sums[empWeek{r.NIK, w}] = sums[empWeek{r.NIK, w}].Add(r.CountedHours())
	}

	facts := acc.facts()
	for nik, dept := range depts {
		streak := p.PrevStreak[nik]
		for w := firstWeek; !w.After(lastWeek); w = w.AddDate(0, 0, 7) {
			if w.Before(p.LoadedFrom) {
				streak = 0
				continue
			}
			h := sums[empWeek{nik, w}]
			var wf []Fact
			wf, streak = weekFacts(nik, dept, w, h, p.NearLimit, streak)
			facts = append(facts, wf...)
		}
	}
	SortFacts(facts)
	return facts
}

// weekFacts returns the facts of one employee-week and the updated over-limit streak. The week is
// classified on its stored (4-decimal) value so a week shown as 14 h is never over-limit.
func weekFacts(nik, dept string, w time.Time, h, nearLimit decimal.Decimal, streak int) ([]Fact, int) {
	h = h.Round(factScale)
	status := WeekStatus(h, nearLimit)
	if status != StatusOver {
		streak = 0
	} else {
		streak++
	}
	if !h.IsPositive() {
		return nil, streak
	}
	one := decimal.NewFromInt(1)
	mk := func(typ, g2 string, v decimal.Decimal) Fact {
		return Fact{Type: typ, Group1: dept, Group2: g2, Group3: nik, Grain: GrainWeekly,
			PeriodDate: w, PeriodLabel: periodLabel(GrainWeekly, w), Value: v.Round(factScale)}
	}
	out := []Fact{mk(TypeWeek, status, h), mk(TypeWeekBucket, WeekBucket(h), one)}
	if status != StatusUnder {
		out = append(out, mk(TypeWeekStatus, status, one))
	}
	if status == StatusOver {
		out = append(out, mk(TypeWeekExcess, "", h.Sub(WeeklyLimit)))
		out = append(out, mk(TypeChronic, "", decimal.NewFromInt(int64(streak))))
	}
	return out, streak
}

// facts converts the accumulated sums to non-zero facts rounded half-up to 4 decimals.
func (a *accumulator) facts() []Fact {
	out := make([]Fact, 0, len(a.sums))
	for k, v := range a.sums {
		v = v.Round(factScale)
		if v.IsZero() {
			continue
		}
		out = append(out, Fact{
			Type: k.typ, Group1: k.g1, Group2: k.g2, Group3: k.g3, Grain: k.grain,
			PeriodDate: k.date, PeriodLabel: periodLabel(k.grain, k.date), Value: v,
		})
	}
	return out
}

func isoWeekLabel(d time.Time) string {
	y, w := d.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}
