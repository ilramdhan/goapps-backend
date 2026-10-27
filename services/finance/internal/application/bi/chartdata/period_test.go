package chartdata_test

import (
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
)

func TestResolvePeriod_WeeklyPresets(t *testing.T) {
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC) // Thursday
	got := chartdata.ResolvePeriod("THIS_WEEK", time.Time{}, time.Time{}, "WEEKLY", now)
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !got.From.Equal(want) || !got.To.Equal(now) {
		t.Fatalf("THIS_WEEK = %v..%v, want from %v", got.From, got.To, want)
	}
	got = chartdata.ResolvePeriod("LAST_WEEK", time.Time{}, time.Time{}, "WEEKLY", now)
	if from, to := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC); !got.From.Equal(from) || !got.To.Equal(to) {
		t.Fatalf("LAST_WEEK = %v..%v", got.From, got.To)
	}
	// Sunday belongs to the week that started the previous Monday.
	sun := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	got = chartdata.ResolvePeriod("THIS_WEEK", time.Time{}, time.Time{}, "WEEKLY", sun)
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !got.From.Equal(want) {
		t.Fatalf("THIS_WEEK on Sunday from = %v, want %v", got.From, want)
	}
}

func d(y int, m time.Month, day int) time.Time  { return time.Date(y, m, day, 10, 0, 0, 0, time.UTC) }
func day(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC) }

// HD-06 / K3 / K10: preset tabel untuk kalender payroll 26–25.
func TestResolvePeriodCal_Payroll(t *testing.T) {
	const pay = chartdata.PayrollCalendar
	cases := []struct {
		name          string
		preset, grain string
		now           time.Time
		from, to      time.Time
	}{
		// tanggal 24 & 25 masih periode yang berakhir 25 bulan itu
		{"THIS_MONTH tgl 24", "THIS_MONTH", "MONTHLY", d(2026, 10, 24), day(2026, 9, 26), day(2026, 10, 25)},
		{"THIS_MONTH tgl 25", "THIS_MONTH", "MONTHLY", d(2026, 10, 25), day(2026, 9, 26), day(2026, 10, 25)},
		// tgl 26 membuka periode baru
		{"THIS_MONTH tgl 26", "THIS_MONTH", "MONTHLY", d(2026, 10, 26), day(2026, 10, 26), day(2026, 11, 25)},
		{"LAST_MONTH tgl 26", "LAST_MONTH", "MONTHLY", d(2026, 10, 26), day(2026, 9, 26), day(2026, 10, 25)},
		{"LAST_MONTH tgl 25", "LAST_MONTH", "MONTHLY", d(2026, 10, 25), day(2026, 8, 26), day(2026, 9, 25)},
		// 31 Des → periode 26 Des – 25 Jan (label YYYYMM = 202701)
		{"THIS_MONTH 31 Des", "THIS_MONTH", "MONTHLY", d(2026, 12, 31), day(2026, 12, 26), day(2027, 1, 25)},
		{"LAST_MONTH 31 Des", "LAST_MONTH", "MONTHLY", d(2026, 12, 31), day(2026, 11, 26), day(2026, 12, 25)},
		{"LAST_MONTH 5 Jan", "LAST_MONTH", "MONTHLY", d(2027, 1, 5), day(2026, 11, 26), day(2026, 12, 25)},
		// 12 periode terakhir termasuk periode berjalan
		{"L12M tgl 24", "L12M", "MONTHLY", d(2026, 10, 24), day(2025, 10, 26), day(2026, 10, 25)},
		{"L12M tgl 26", "L12M", "MONTHLY", d(2026, 10, 26), day(2025, 11, 26), day(2026, 11, 25)},
		{"L12M 31 Des", "L12M", "MONTHLY", d(2026, 12, 31), day(2026, 1, 26), day(2027, 1, 25)},
		{"L24M tgl 25", "L24M", "MONTHLY", d(2026, 10, 25), day(2024, 10, 26), day(2026, 10, 25)},
		// WEEKLY: Senin ∈ [mulai-6, akhir-6] supaya minggu ikut periode tempat Minggu-nya jatuh
		// Sen 19 Okt..Min 25 Okt → periode Okt; Sen 26 Okt..Min 1 Nov → Nov.
		{"WEEKLY THIS_MONTH tgl 24", "THIS_MONTH", "WEEKLY", d(2026, 10, 24), day(2026, 9, 20), day(2026, 10, 19)},
		{"WEEKLY THIS_MONTH tgl 26", "THIS_MONTH", "WEEKLY", d(2026, 10, 26), day(2026, 10, 20), day(2026, 11, 19)},
		{"WEEKLY LAST_MONTH tgl 26", "LAST_MONTH", "WEEKLY", d(2026, 10, 26), day(2026, 9, 20), day(2026, 10, 19)},
		// DAILY: dipotong di hari ini
		{"DAILY THIS_MONTH tgl 15", "THIS_MONTH", "DAILY", d(2026, 10, 15), day(2026, 9, 26), day(2026, 10, 15)},
		{"DAILY THIS_MONTH tgl 26", "THIS_MONTH", "DAILY", d(2026, 10, 26), day(2026, 10, 26), day(2026, 10, 26)},
		// Kemarin & minggu tidak bergantung kalender
		{"YESTERDAY awal bulan", "YESTERDAY", "DAILY", d(2026, 11, 1), day(2026, 10, 31), day(2026, 10, 31)},
		{"YESTERDAY 1 Jan", "YESTERDAY", "DAILY", d(2027, 1, 1), day(2026, 12, 31), day(2026, 12, 31)},
	}
	for _, c := range cases {
		got := chartdata.ResolvePeriodCal(c.preset, time.Time{}, time.Time{}, c.grain, pay, c.now)
		if !got.From.Equal(c.from) || !got.To.Equal(c.to) {
			t.Errorf("%s: got %s..%s, want %s..%s", c.name,
				got.From.Format("2006-01-02"), got.To.Format("2006-01-02"),
				c.from.Format("2006-01-02"), c.to.Format("2006-01-02"))
		}
	}
}

// Minggu yang melintasi 25/26 masuk periode tempat Minggu-nya jatuh (K10).
// Senin nyata: 19 Okt 2026 (Minggu 25 Okt → periode Okt), 26 Okt 2026 (Minggu 1 Nov → periode Nov).
func TestResolvePeriodCal_PayrollWeekCrossing(t *testing.T) {
	// Minggu Sen 26 Okt – Min 1 Nov 2026: Minggu = 1 Nov → periode Nov (26 Okt–25 Nov).
	mon := day(2026, 10, 26)
	cur := chartdata.ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "WEEKLY", chartdata.PayrollCalendar, d(2026, 10, 26))
	prev := chartdata.ResolvePeriodCal("LAST_MONTH", time.Time{}, time.Time{}, "WEEKLY", chartdata.PayrollCalendar, d(2026, 10, 26))
	in := func(r chartdata.PeriodRange) bool { return !mon.Before(r.From) && !mon.After(r.To) }
	if !in(cur) || in(prev) {
		t.Fatalf("week of %s: in current=%v in previous=%v (want true/false)", mon.Format("2006-01-02"), in(cur), in(prev))
	}
	// Minggu Sen 19 Okt – Min 25 Okt: Minggu = 25 Okt → periode Okt (sebelumnya).
	mon = day(2026, 10, 19)
	if in(cur) || !in(prev) {
		t.Fatalf("week of %s: in current=%v in previous=%v (want false/true)", mon.Format("2006-01-02"), in(cur), in(prev))
	}
}

// Kalender CALENDAR tidak berubah (perilaku lama) + preset baru.
func TestResolvePeriodCal_CalendarUnchanged(t *testing.T) {
	now := d(2026, 10, 26)
	got := chartdata.ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, "MONTHLY", chartdata.CalendarCalendar, now)
	if !got.From.Equal(day(2026, 10, 1)) || !got.To.Equal(now) {
		t.Fatalf("THIS_MONTH calendar = %v..%v", got.From, got.To)
	}
	got = chartdata.ResolvePeriodCal("LAST_MONTH", time.Time{}, time.Time{}, "MONTHLY", "", now)
	if !got.From.Equal(day(2026, 9, 1)) || !got.To.Equal(day(2026, 9, 30)) {
		t.Fatalf("LAST_MONTH calendar = %v..%v", got.From, got.To)
	}
	got = chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)
	if !got.From.Equal(day(2025, 10, 1)) {
		t.Fatalf("L12M legacy = %v", got.From)
	}
}
