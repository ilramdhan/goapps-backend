// Package chartdata contains the application-layer orchestration for serving
// BI viewer chart data: query planning, fact aggregate execution, KPI compute,
// formatting, and cache integration.
package chartdata

import (
	"fmt"
	"time"
)

// PeriodRange is an inclusive [From, To] date window resolved from a preset.
type PeriodRange struct {
	From time.Time
	To   time.Time
}

// Period calendars (HD-06). CALENDAR is the legacy calendar-month behavior;
// PayrollCalendar is the payroll period 26th(prev month)..25th used by HR facts.
const (
	CalendarCalendar = "CALENDAR"
	PayrollCalendar  = "PAYROLL_26_25"
)

// ResolvePeriod resolves a preset on the calendar-month calendar. It is kept for callers that
// have no dashboard context; dashboard-aware callers use ResolvePeriodCal.
func ResolvePeriod(preset string, customFrom, customTo time.Time, grain string, now time.Time) PeriodRange {
	return ResolvePeriodCal(preset, customFrom, customTo, grain, CalendarCalendar, now)
}

// ResolvePeriodCal converts a period preset (or CUSTOM) into a concrete date range.
//
// Grain affects what "L12M" means: for MONTHLY it's the first day of 12 months ago
// through the first day of the current month; for DAILY it's exact day arithmetic;
// for QUARTERLY/YEARLY it snaps to quarter/year boundaries.
//
// calendar = PAYROLL_26_25 re-bases THIS_MONTH / LAST_MONTH / L12M / L24M on payroll periods
// (K3): MONTHLY facts are dated on the 25th, WEEKLY facts on Monday and a week belongs to the
// period containing its Sunday (K10), so the WEEKLY window is shifted back 6 days. THIS_WEEK,
// LAST_WEEK and YESTERDAY are calendar-independent; THIS_QTR/THIS_YEAR stay calendar-based.
//
// CUSTOM uses customFrom/customTo unchanged. ALL returns a zero range that the
// query planner treats as "no period filter".
//
// `now` is the reference time (typically time.Now().UTC()); injectable for tests.
func ResolvePeriodCal(preset string, customFrom, customTo time.Time, grain, calendar string, now time.Time) PeriodRange {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payroll := calendar == PayrollCalendar
	switch preset {
	case "CUSTOM":
		return PeriodRange{From: customFrom, To: customTo}
	case "ALL":
		return PeriodRange{}
	case "YESTERDAY":
		y := dayOf(now).AddDate(0, 0, -1)
		return PeriodRange{From: y, To: y}
	case "THIS_MONTH":
		return monthWindow(0, payroll, grain, now)
	case "LAST_MONTH":
		return monthWindow(-1, payroll, grain, now)
	case "THIS_WEEK", "LAST_WEEK":
		return weekWindow(preset == "LAST_WEEK", now)
	case "THIS_QTR":
		first := firstOfQuarter(now)
		return PeriodRange{From: first, To: now}
	case "THIS_YEAR":
		first := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return PeriodRange{From: first, To: now}
	case "L24M":
		if payroll {
			return payrollTrailing(24, grain, now)
		}
		return PeriodRange{From: shiftByMonths(now, -24), To: now}
	case "", "L12M":
		if payroll {
			return payrollTrailing(12, grain, now)
		}
		return PeriodRange{From: shiftByMonths(now, -12), To: now}
	}
	if payroll {
		return payrollTrailing(12, grain, now)
	}
	return PeriodRange{From: shiftByMonths(now, -12), To: now}
}

// weekWindow returns this ISO week up to now, or the full previous week. WEEKLY snaps to ISO
// Monday (HD-05); "Kemarin" stays a DAILY concern.
func weekWindow(last bool, now time.Time) PeriodRange {
	mon := mondayOf(now)
	if last {
		return PeriodRange{From: mon.AddDate(0, 0, -7), To: mon.AddDate(0, 0, -1)}
	}
	return PeriodRange{From: mon, To: now}
}

// monthWindow returns the month offset months from now's month (0 = this, -1 = last): the payroll
// period 26..25 when payroll, else the calendar month (this month runs to now).
func monthWindow(offset int, payroll bool, grain string, now time.Time) PeriodRange {
	if payroll {
		end := payrollEnd(payrollRef(now, grain)).AddDate(0, offset, 0)
		return payrollWindow(payrollStart(end), end, grain, now)
	}
	first := firstOfMonth(now).AddDate(0, offset, 0)
	if offset == 0 {
		return PeriodRange{From: first, To: now}
	}
	return PeriodRange{From: first, To: first.AddDate(0, 1, -1)}
}

// payrollRef is the date that decides which payroll period "now" falls in. WEEKLY facts belong to
// the period containing their Sunday (K10), so the reference is the Sunday of now's ISO week;
// other grains use now itself.
func payrollRef(now time.Time, grain string) time.Time {
	if grain == grainWeekly {
		return mondayOf(now).AddDate(0, 0, 6)
	}
	return now
}

// payrollEnd returns the 25th that closes the payroll period containing t
// (26th..25th: from the 26th onward the period ends on next month's 25th).
func payrollEnd(t time.Time) time.Time {
	m := t.Month()
	if t.Day() > 25 {
		m++
	}
	return time.Date(t.Year(), m, 25, 0, 0, 0, 0, t.Location())
}

// payrollStart returns the 26th of the previous month for a payroll period ending on `end`.
func payrollStart(end time.Time) time.Time {
	return time.Date(end.Year(), end.Month()-1, 26, 0, 0, 0, 0, end.Location())
}

// payrollWindow maps a payroll period [start,end] to the fact-date window of the grain:
// MONTHLY facts sit on the 25th (inside [start,end]); WEEKLY facts sit on Monday and belong
// to the period containing their Sunday, so Monday ∈ [start-6, end-6] (K10); DAILY facts are
// dated per day and the window is capped at `now`.
func payrollWindow(start, end time.Time, grain string, now time.Time) PeriodRange {
	switch grain {
	case grainWeekly:
		return PeriodRange{From: start.AddDate(0, 0, -6), To: end.AddDate(0, 0, -6)}
	case grainDaily:
		if d := dayOf(now); d.Before(end) {
			end = d
		}
	}
	return PeriodRange{From: start, To: end}
}

// PayrollTrailingStart returns the first fact date of the last n payroll periods (sparklines).
func PayrollTrailingStart(n int, grain string, now time.Time) time.Time {
	return payrollTrailing(n, grain, now).From
}

// payrollTrailing returns the last n payroll periods including the current one.
func payrollTrailing(n int, grain string, now time.Time) PeriodRange {
	end := payrollEnd(payrollRef(now, grain))
	first := payrollStart(end.AddDate(0, -(n - 1), 0))
	return payrollWindow(first, end, grain, now)
}

// dayOf returns 00:00 of t's calendar day.
func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// mondayOf returns 00:00 of the ISO Monday of the week containing t.
func mondayOf(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	back := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -back)
}

// firstOfMonth returns YYYY-MM-01 00:00:00 in t's location.
func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// firstOfQuarter returns the first day of the quarter containing t.
func firstOfQuarter(t time.Time) time.Time {
	month := ((int(t.Month())-1)/3)*3 + 1
	return time.Date(t.Year(), time.Month(month), 1, 0, 0, 0, 0, t.Location())
}

// shiftByMonths returns first-of-month for t shifted by deltaMonths.
func shiftByMonths(t time.Time, deltaMonths int) time.Time {
	first := firstOfMonth(t)
	return first.AddDate(0, deltaMonths, 0)
}

// PeriodLabel formats a date as the canonical period_label string for a given grain.
func PeriodLabel(t time.Time, grain string) string {
	switch grain {
	case grainDaily:
		return t.Format("2006-01-02")
	case grainWeekly:
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	case "QUARTERLY":
		q := (int(t.Month())-1)/3 + 1
		return t.Format("2006") + "-Q" + string(rune('0'+q))
	case "YEARLY":
		return t.Format("2006")
	default: // MONTHLY
		return t.Format("200601")
	}
}
