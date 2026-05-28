// Package chartdata contains the application-layer orchestration for serving
// BI viewer chart data: query planning, fact aggregate execution, KPI compute,
// formatting, and cache integration.
package chartdata

import "time"

// PeriodRange is an inclusive [From, To] date window resolved from a preset.
type PeriodRange struct {
	From time.Time
	To   time.Time
}

// ResolvePeriod converts a period preset (or CUSTOM) into a concrete date range.
//
// Grain affects what "L12M" means: for MONTHLY it's the first day of 12 months ago
// through the first day of the current month; for DAILY it's exact day arithmetic;
// for QUARTERLY/YEARLY it snaps to quarter/year boundaries.
//
// CUSTOM uses customFrom/customTo unchanged. ALL returns a zero range that the
// query planner treats as "no period filter".
//
// `now` is the reference time (typically time.Now().UTC()); injectable for tests.
func ResolvePeriod(preset string, customFrom, customTo time.Time, _ string, now time.Time) PeriodRange {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	switch preset {
	case "CUSTOM":
		return PeriodRange{From: customFrom, To: customTo}
	case "ALL":
		return PeriodRange{}
	case "THIS_MONTH":
		first := firstOfMonth(now)
		return PeriodRange{From: first, To: now}
	case "THIS_QTR":
		first := firstOfQuarter(now)
		return PeriodRange{From: first, To: now}
	case "THIS_YEAR":
		first := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return PeriodRange{From: first, To: now}
	case "L24M":
		return PeriodRange{From: shiftByMonths(now, -24), To: now}
	case "", "L12M":
		return PeriodRange{From: shiftByMonths(now, -12), To: now}
	}
	return PeriodRange{From: shiftByMonths(now, -12), To: now}
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
	case "DAILY":
		return t.Format("2006-01-02")
	case "QUARTERLY":
		q := (int(t.Month())-1)/3 + 1
		return t.Format("2006") + "-Q" + string(rune('0'+q))
	case "YEARLY":
		return t.Format("2006")
	default: // MONTHLY
		return t.Format("200601")
	}
}
