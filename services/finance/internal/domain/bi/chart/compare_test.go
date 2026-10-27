package chart_test

import (
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

func TestShiftPeriod(t *testing.T) {
	base := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		mode  chart.CompareMode
		grain chart.PeriodGrain
		want  time.Time
	}{
		{"MoM monthly → -1 month", chart.CompareMoM, chart.GrainMonthly, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)},
		{"YoY monthly → -1 year", chart.CompareYoY, chart.GrainMonthly, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC)},
		{"QoQ monthly → -3 months", chart.CompareQoQ, chart.GrainMonthly, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)},
		{"R12 → -12 months", chart.CompareR12, chart.GrainMonthly, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC)},
		{"None → unchanged", chart.CompareNone, chart.GrainMonthly, base},
		{"YTD → unchanged", chart.CompareYTD, chart.GrainMonthly, base},
		{"MoM daily → -1 day", chart.CompareMoM, chart.GrainDaily, time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)},
		{"YoY yearly → -1 year", chart.CompareYoY, chart.GrainYearly, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chart.ShiftPeriod(base, tc.mode, tc.grain)
			if !got.Equal(tc.want) {
				t.Errorf("ShiftPeriod(%v, %s, %s): want %v, got %v", base, tc.mode, tc.grain, tc.want, got)
			}
		})
	}
}

func TestShiftPeriod_ZeroTime(t *testing.T) {
	var zero time.Time
	got := chart.ShiftPeriod(zero, chart.CompareYoY, chart.GrainMonthly)
	if !got.IsZero() {
		t.Errorf("zero time should remain zero, got %v", got)
	}
}

func TestIsValidCompareMode(t *testing.T) {
	for _, s := range []string{"none", "MoM", "QoQ", "YoY", "YTD", "R12"} {
		if !chart.IsValidCompareMode(s) {
			t.Errorf("expected %q valid", s)
		}
	}
	for _, s := range []string{"", "mom", "MOM", "unknown"} {
		if chart.IsValidCompareMode(s) {
			t.Errorf("expected %q invalid", s)
		}
	}
}

func TestIsValidPeriodGrain(t *testing.T) {
	for _, s := range []string{"DAILY", "WEEKLY", "MONTHLY", "QUARTERLY", "YEARLY"} {
		if !chart.IsValidPeriodGrain(s) {
			t.Errorf("expected %q valid", s)
		}
	}
	for _, s := range []string{"daily", "Monthly", "weekly", "HOURLY", ""} {
		if chart.IsValidPeriodGrain(s) {
			t.Errorf("expected %q invalid", s)
		}
	}
}

func TestShiftPeriod_Weekly(t *testing.T) {
	mon := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) // Monday
	cases := []struct {
		mode chart.CompareMode
		want time.Time
	}{
		{chart.CompareMoM, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{chart.CompareYoY, time.Date(2025, 10, 13, 0, 0, 0, 0, time.UTC)}, // −52 weeks, still Monday
	}
	for _, c := range cases {
		got := chart.ShiftPeriod(mon, c.mode, chart.GrainWeekly)
		if !got.Equal(c.want) || got.Weekday() != time.Monday {
			t.Errorf("%s: want %v, got %v", c.mode, c.want, got)
		}
	}
	if !chart.IsValidPeriodGrain("WEEKLY") {
		t.Error("WEEKLY must be a valid grain")
	}
}
