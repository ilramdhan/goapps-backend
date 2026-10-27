package chartdata

import (
	"testing"
	"time"
)

func ymd(t time.Time) string { return t.Format("2006-01-02") }
func dd(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 10, 0, 0, 0, time.UTC)
}

// MAJOR-1 (review H003): WEEKLY memakai hari Minggu minggu berjalan sebagai acuan periode (K10).
func TestPayrollWeekly_ReferenceIsSunday(t *testing.T) {
	// Sen 23 Nov 2026 .. Min 29 Nov 2026: Minggu 29 Nov → periode berakhir 25 Des.
	// Senin yang termasuk periode itu: [26 Nov-6 = 20 Nov, 25 Des-6 = 19 Des].
	// now = Sen 23 .. Min 29 Nov harus semuanya memberi jendela yang sama (tanpa fix, 23–25 Nov
	// jatuh ke periode berakhir 25 Nov).
	for day := 23; day <= 29; day++ {
		got := ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 11, day))
		if ymd(got.From) != "2026-11-20" || ymd(got.To) != "2026-12-19" {
			t.Errorf("now=%d Nov: THIS_MONTH WEEKLY = %s..%s, want 2026-11-20..2026-12-19", day, ymd(got.From), ymd(got.To))
		}
	}
	// Sen 16 Nov .. Min 22 Nov: Minggu 22 Nov → periode berakhir 25 Nov → Senin ∈ [20 Okt, 19 Nov].
	got := ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 11, 18))
	if ymd(got.From) != "2026-10-20" || ymd(got.To) != "2026-11-19" {
		t.Errorf("now=18 Nov WEEKLY = %s..%s", ymd(got.From), ymd(got.To))
	}
	// LAST_MONTH & L12M saat now = Sel 24 Nov (Minggu 29 Nov → periode Des).
	got = ResolvePeriodCal("LAST_MONTH", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 11, 24))
	if ymd(got.From) != "2026-10-20" || ymd(got.To) != "2026-11-19" {
		t.Errorf("LAST_MONTH now=24 Nov = %s..%s", ymd(got.From), ymd(got.To))
	}
	got = ResolvePeriodCal("L12M", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 11, 24))
	if ymd(got.To) != "2026-12-19" || ymd(got.From) != "2025-12-20" {
		t.Errorf("L12M WEEKLY now=24 Nov = %s..%s", ymd(got.From), ymd(got.To))
	}
	// Senin nyata di batas: Sen 19 Okt (Minggu 25 Okt) ∈ periode Okt; Sen 26 Okt (Minggu 1 Nov) ∈ Nov.
	oct := ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 10, 20))
	nov := ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "WEEKLY", PayrollCalendar, dd(2026, 10, 27))
	in := func(r PeriodRange, d time.Time) bool {
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		return !d.Before(r.From) && !d.After(r.To)
	}
	if !in(oct, dd(2026, 10, 19)) || in(oct, dd(2026, 10, 26)) || in(nov, dd(2026, 10, 19)) || !in(nov, dd(2026, 10, 26)) {
		t.Errorf("batas Senin 19/26 Okt salah: oct=%v..%v nov=%v..%v", oct.From, oct.To, nov.From, nov.To)
	}
}

// MAJOR-2: compare MoM/QoQ memakai kalender payroll (25-an), bukan tanggal 1.
func TestCompareRange_Payroll(t *testing.T) {
	cur := PeriodRange{From: dd(2026, 9, 26), To: dd(2026, 10, 25)}
	r, lbl := compareRange(cur, "MoM", "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2026-08-26" || ymd(r.To) != "2026-09-25" || lbl != "202609" {
		t.Errorf("MoM = %s..%s %s", ymd(r.From), ymd(r.To), lbl)
	}
	r, lbl = compareRange(cur, "QoQ", "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2026-06-26" || ymd(r.To) != "2026-07-25" || lbl != "202607" {
		t.Errorf("QoQ = %s..%s %s", ymd(r.From), ymd(r.To), lbl)
	}
	// Lintas tahun: periode Des–Jan → MoM = Nov–Des
	cur = PeriodRange{From: dd(2026, 12, 26), To: dd(2027, 1, 25)}
	r, lbl = compareRange(cur, "MoM", "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2026-11-26" || ymd(r.To) != "2026-12-25" || lbl != "202612" {
		t.Errorf("MoM lintas tahun = %s..%s %s", ymd(r.From), ymd(r.To), lbl)
	}
	// YoY tetap -1 tahun
	r, _ = compareRange(cur, "YoY", "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2025-12-26" || ymd(r.To) != "2026-01-25" {
		t.Errorf("YoY = %s..%s", ymd(r.From), ymd(r.To))
	}
	// Kalender biasa tidak berubah
	cal := PeriodRange{From: dd(2026, 10, 1), To: dd(2026, 10, 26)}
	r, _ = compareRange(cal, "MoM", "MONTHLY", "")
	if ymd(r.From) != "2026-09-01" {
		t.Errorf("CALENDAR MoM from = %s", ymd(r.From))
	}
}

// MAJOR-3: scope KPI current_month / l12m mengikuti kalender payroll.
func TestResolveKPIPeriod_Payroll(t *testing.T) {
	now := dd(2026, 10, 25)
	r := resolveKPIPeriod("current_month", PeriodRange{}, now, "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2026-09-26" || ymd(r.To) != "2026-10-25" {
		t.Errorf("current_month = %s..%s", ymd(r.From), ymd(r.To))
	}
	r = resolveKPIPeriod("l12m", PeriodRange{}, now, "MONTHLY", PayrollCalendar)
	if ymd(r.From) != "2025-10-26" || ymd(r.To) != "2026-10-25" {
		t.Errorf("l12m = %s..%s (harus 12 periode: 25 Nov 2025..25 Okt 2026)", ymd(r.From), ymd(r.To))
	}
	r = resolveKPIPeriod("current_month", PeriodRange{}, now, "MONTHLY", "CALENDAR")
	if ymd(r.From) != "2026-10-01" {
		t.Errorf("CALENDAR current_month from = %s", ymd(r.From))
	}
	if got := ymd(PayrollTrailingStart(6, "MONTHLY", now)); got != "2026-04-26" {
		t.Errorf("sparkline start = %s", got)
	}
}

// PR #236 review: sparkline_periods counts weeks on a WEEKLY dashboard (12 → 12 Mondays), not months.
func TestSparklineStart_WeeklyCountsWeeks(t *testing.T) {
	now := dd(2026, time.October, 15) // Thursday; week Monday = 2026-10-12
	if got := ymd(sparklineStart(12, grainWeekly, PayrollCalendar, now)); got != "2026-07-27" {
		t.Errorf("weekly start = %s, want 2026-07-27", got)
	}
	if got := ymd(sparklineStart(1, grainWeekly, "", now)); got != "2026-10-12" {
		t.Errorf("weekly n=1 start = %s, want this Monday", got)
	}
	if got := ymd(sparklineStart(12, "MONTHLY", "", now)); got != "2025-11-01" {
		t.Errorf("monthly start = %s, want 2025-11-01", got)
	}
}
