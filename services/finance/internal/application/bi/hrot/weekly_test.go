package hrot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeekRow_CountedHours(t *testing.T) {
	tests := []struct{ name, typ, calc, want string }{
		{"normal counts fully", "Normal", "7.5", "7.5"},
		{"off counts fully", "Off", "9", "9"},
		{"change off counts fully", OTTypeChangeOff, "3.25", "3.25"},
		{"holiday exactly 7.5 counts 0", "Holiday", "7.5", "0"},
		{"holiday below 7.5 counts 0", "Holiday", "4", "0"},
		{"holiday above 7.5 counts excess", "Holiday", "9.75", "2.25"},
		{"short holiday exactly 7.5 counts 0", "Short Holiday", "7.5", "0"},
		{"short holiday just above", "short holiday", "7.500001", "0.000001"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := WeekRow{OTType: tc.typ, CalcHours: dec(tc.calc)}.CountedHours()
			assert.True(t, got.Equal(dec(tc.want)), "got %s", got)
		})
	}
}

func TestWeekStatusAndBucket(t *testing.T) {
	tests := []struct{ h, status, bucket string }{
		{"0.5", StatusUnder, Bucket0to7},
		{"7", StatusUnder, Bucket0to7},
		{"7.0001", StatusUnder, Bucket7to14},
		{"9.9999", StatusUnder, Bucket7to14},
		{"10", StatusNear, Bucket7to14},
		{"14", StatusNear, Bucket7to14}, // exactly 14 is compliant (K1: over = > 14)
		{"14.000001", StatusOver, Bucket14to18},
		{"18", StatusOver, Bucket14to18},
		{"18.01", StatusOver, BucketOver18},
	}
	for _, tc := range tests {
		t.Run(tc.h, func(t *testing.T) {
			assert.Equal(t, tc.status, WeekStatus(dec(tc.h), DefaultNearLimit))
			assert.Equal(t, tc.bucket, WeekBucket(dec(tc.h)))
		})
	}
	assert.Equal(t, StatusUnder, WeekStatus(dec("10"), dec("12")), "configurable near threshold")
}

func TestMonday(t *testing.T) {
	for d, want := range map[string]string{
		"2026-10-12": "2026-10-12", // Monday
		"2026-10-18": "2026-10-12", // Sunday
		"2027-01-01": "2026-12-28", // Friday, ISO 2026-W53
	} {
		assert.Equal(t, want, Monday(day(d)).Format("2006-01-02"), d)
	}
}

// find returns the single fact with the given type, NIK and period date (or fails).
func find(t *testing.T, facts []Fact, typ, nik, date string) Fact {
	t.Helper()
	var out []Fact
	for _, f := range facts {
		if f.Type == typ && f.Group3 == nik && f.PeriodDate.Equal(day(date)) {
			out = append(out, f)
		}
	}
	require.Len(t, out, 1, "%s %s %s", typ, nik, date)
	return out[0]
}

func has(facts []Fact, typ, nik, date string) bool {
	for _, f := range facts {
		if f.Type == typ && f.Group3 == nik && f.PeriodDate.Equal(day(date)) {
			return true
		}
	}
	return false
}

func wr(d, nik, typ, calc string) WeekRow {
	return WeekRow{WorkDate: day(d), NIK: nik, StagingDept: "STG", OTType: typ, CalcHours: dec(calc)}
}

func TestAggregateWeekly_WeekAcrossPayrollCutoff(t *testing.T) {
	// Week Mon 2026-11-23 .. Sun 2026-11-29 crosses the 25th/26th: one WEEKLY row on the Monday;
	// its payroll period (Sunday 29 → 202612) is the viewer's filter concern (K10, HD-05).
	rows := []WeekRow{
		wr("2026-11-24", "A", "Normal", "8"),
		wr("2026-11-27", "A", "Off", "6"),
		wr("2026-11-28", "A", "Holiday", "7.5"), // counts 0
	}
	facts := AggregateWeekly(rows, map[string]string{"A": "PROD"}, WeeklyParams{
		From: day("2026-11-23"), To: day("2026-11-29"), LoadedFrom: day("2026-01-01"), NearLimit: DefaultNearLimit,
	})

	w := find(t, facts, TypeWeek, "A", "2026-11-23")
	assert.Equal(t, "PROD", w.Group1)
	assert.Equal(t, StatusNear, w.Group2) // exactly 14
	assert.Equal(t, GrainWeekly, w.Grain)
	assert.Equal(t, "2026-W48", w.PeriodLabel)
	assert.True(t, w.Value.Equal(dec("14")))
	assert.Equal(t, Bucket7to14, find(t, facts, TypeWeekBucket, "A", "2026-11-23").Group2)
	assert.Equal(t, StatusNear, find(t, facts, TypeWeekStatus, "A", "2026-11-23").Group2)
	assert.False(t, has(facts, TypeWeekExcess, "A", "2026-11-23"))
	assert.False(t, has(facts, TypeChronic, "A", "2026-11-23"))

	d := find(t, facts, TypeWeekDaily, "A", "2026-11-24")
	assert.Equal(t, GrainDaily, d.Grain)
	assert.Equal(t, "Normal", d.Group2)
	assert.False(t, has(facts, TypeWeekDaily, "A", "2026-11-28"), "zero counted hours → no daily fact")
}

func TestAggregateWeekly_ISOYearBoundaryAndStreak(t *testing.T) {
	// Weeks: 2026-12-14 over, 12-21 over, 12-28 (W53, spans new year) over, 2027-01-04 under,
	// 01-11 over. B starts with a carried streak of 3 from the week before the window.
	rows := []WeekRow{
		wr("2026-12-15", "B", "Normal", "15"),
		wr("2026-12-22", "B", "Normal", "14.5"),
		wr("2026-12-31", "B", "Normal", "10"),
		wr("2027-01-01", "B", "Holiday", "17.5"), // counts 10 → week = 20, bucket >18
		wr("2027-01-05", "B", "Normal", "3"),
		wr("2027-01-12", "B", "Off", "16"),
	}
	facts := AggregateWeekly(rows, nil, WeeklyParams{
		From: day("2026-12-14"), To: day("2027-01-17"), LoadedFrom: day("2026-01-01"),
		NearLimit: DefaultNearLimit, PrevStreak: map[string]int{"B": 3},
	})

	nye := find(t, facts, TypeWeek, "B", "2026-12-28")
	assert.Equal(t, "2026-W53", nye.PeriodLabel)
	assert.True(t, nye.Value.Equal(dec("20")), "got %s", nye.Value)
	assert.Equal(t, "STG", nye.Group1, "falls back to staging dept")
	assert.Equal(t, BucketOver18, find(t, facts, TypeWeekBucket, "B", "2026-12-28").Group2)
	assert.True(t, find(t, facts, TypeWeekExcess, "B", "2026-12-28").Value.Equal(dec("6")))
	assert.Equal(t, "2027-W01", find(t, facts, TypeWeek, "B", "2027-01-04").PeriodLabel)

	for date, want := range map[string]string{"2026-12-14": "4", "2026-12-21": "5", "2026-12-28": "6", "2027-01-11": "1"} {
		assert.True(t, find(t, facts, TypeChronic, "B", date).Value.Equal(dec(want)), date)
	}
	assert.False(t, has(facts, TypeChronic, "B", "2027-01-04"))
	assert.False(t, has(facts, TypeWeekStatus, "B", "2027-01-04"), "under-limit has no status fact")
}

func TestAggregateWeekly_GapWeekResetsStreak(t *testing.T) {
	rows := []WeekRow{
		wr("2026-10-05", "C", "Normal", "15"),
		wr("2026-10-19", "C", "Normal", "15"), // week of 10-12 has no OT at all
	}
	facts := AggregateWeekly(rows, nil, WeeklyParams{
		From: day("2026-10-05"), To: day("2026-10-25"), LoadedFrom: day("2026-01-01"), NearLimit: DefaultNearLimit,
	})
	assert.True(t, find(t, facts, TypeChronic, "C", "2026-10-19").Value.Equal(dec("1")))
	assert.False(t, has(facts, TypeWeek, "C", "2026-10-12"))
}

func TestAggregateWeekly_SkipsIncompleteWeek(t *testing.T) {
	// Staging starts Wednesday 2026-10-07: that week is partial and must not be written.
	rows := []WeekRow{wr("2026-10-07", "D", "Normal", "15"), wr("2026-10-13", "D", "Normal", "15")}
	facts := AggregateWeekly(rows, nil, WeeklyParams{
		From: day("2026-10-05"), To: day("2026-10-18"), LoadedFrom: day("2026-10-07"), NearLimit: DefaultNearLimit,
	})
	assert.False(t, has(facts, TypeWeek, "D", "2026-10-05"))
	assert.False(t, has(facts, TypeWeekDaily, "D", "2026-10-07"))
	assert.True(t, find(t, facts, TypeChronic, "D", "2026-10-12").Value.Equal(dec("1")), "streak restarts after skipped week")
}

func TestAggregateWeekly_SumThenRound(t *testing.T) {
	// 3 × 4.66666666 = 13.99999998 → stored 14 → near (compliant).
	rows := []WeekRow{
		wr("2026-10-12", "E", "Normal", "4.66666666"),
		wr("2026-10-13", "E", "Normal", "4.66666666"),
		wr("2026-10-14", "E", "Normal", "4.66666666"),
	}
	facts := AggregateWeekly(rows, nil, WeeklyParams{
		From: day("2026-10-12"), To: day("2026-10-18"), LoadedFrom: day("2026-01-01"), NearLimit: DefaultNearLimit,
	})
	w := find(t, facts, TypeWeek, "E", "2026-10-12")
	assert.Equal(t, StatusNear, w.Group2)
	assert.True(t, w.Value.Equal(dec("14")))
}

func TestAggregateWeekly_ClassifiesOnStoredValue(t *testing.T) {
	// 14.00004 is stored as 14: compliant, no over-limit/excess/chronic facts that would show "14 h over".
	facts := AggregateWeekly([]WeekRow{wr("2026-10-12", "F", "Normal", "14.00004")}, nil, WeeklyParams{
		From: day("2026-10-12"), To: day("2026-10-18"), LoadedFrom: day("2026-01-01"), NearLimit: DefaultNearLimit,
	})
	assert.Equal(t, StatusNear, find(t, facts, TypeWeek, "F", "2026-10-12").Group2)
	assert.False(t, has(facts, TypeWeekExcess, "F", "2026-10-12"))
	assert.False(t, has(facts, TypeChronic, "F", "2026-10-12"))

	facts = AggregateWeekly([]WeekRow{wr("2026-10-12", "G", "Normal", "14.00005")}, nil, WeeklyParams{
		From: day("2026-10-12"), To: day("2026-10-18"), LoadedFrom: day("2026-01-01"), NearLimit: DefaultNearLimit,
	})
	assert.Equal(t, StatusOver, find(t, facts, TypeWeek, "G", "2026-10-12").Group2)
	assert.True(t, find(t, facts, TypeWeekExcess, "G", "2026-10-12").Value.Equal(dec("0.0001")))
}
