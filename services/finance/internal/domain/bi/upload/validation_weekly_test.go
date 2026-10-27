package upload

import (
	"testing"
	"time"
)

func TestParseWeekly_SnapsToMonday(t *testing.T) {
	d, lbl, ferr := parseWeekly("2026-10-15", "2026-10-15", 1) // Thursday
	if ferr != nil {
		t.Fatal(ferr)
	}
	if !d.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) || lbl != "2026-W42" {
		t.Errorf("got %v %s", d, lbl)
	}
	d, lbl, _ = parseWeekly("2026-10-18", "", 1) // Sunday → same ISO week
	if d.Day() != 12 || lbl != "2026-W42" {
		t.Errorf("sunday: %v %s", d, lbl)
	}
	if _, _, ferr = parseWeekly("202610", "202610", 1); ferr == nil {
		t.Error("want error for non-date")
	}
}
