package chartdata

import (
	"testing"
	"time"
)

func TestPeriodLabel_Weekly(t *testing.T) {
	// ISO year differs from calendar year around new year.
	if got := PeriodLabel(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), "WEEKLY"); got != "2026-W42" {
		t.Errorf("got %s", got)
	}
	if got := PeriodLabel(time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC), "WEEKLY"); got != "2025-W01" {
		t.Errorf("got %s", got)
	}
}

func TestCompareRange_Weekly(t *testing.T) {
	cur := PeriodRange{From: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	r, lbl := compareRange(cur, "MoM", "WEEKLY", "")
	if !r.To.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) || lbl != "2026-W41" {
		t.Errorf("MoM: %v %s", r, lbl)
	}
	r, lbl = compareRange(cur, "YoY", "WEEKLY", "")
	if r.To.Weekday() != time.Monday || lbl != "2025-W42" {
		t.Errorf("YoY: %v %s", r, lbl)
	}
}
