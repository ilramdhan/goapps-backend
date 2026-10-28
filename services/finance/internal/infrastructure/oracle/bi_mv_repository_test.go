package oracle

import (
	"database/sql"
	"testing"
)

func TestResolveMVValues(t *testing.T) {
	tests := []struct {
		name        string
		value       sql.NullFloat64
		display     sql.NullFloat64
		wantValue   float64
		wantDisplay float64
		wantOK      bool
	}{
		{"both present", sql.NullFloat64{Float64: 12.5, Valid: true}, sql.NullFloat64{Float64: 12.5, Valid: true}, 12.5, 12.5, true},
		{"display differs from value", sql.NullFloat64{Float64: 1250, Valid: true}, sql.NullFloat64{Float64: 1.25, Valid: true}, 1250, 1.25, true},
		{"zero is a real value, not skipped", sql.NullFloat64{Float64: 0, Valid: true}, sql.NullFloat64{Float64: 0, Valid: true}, 0, 0, true},
		{"NULL display falls back to value", sql.NullFloat64{Float64: 7, Valid: true}, sql.NullFloat64{}, 7, 7, true},
		// Real production data (2026-10-28): 1 of 3708 MV_DASH_DELMAR_MGT rows, MARGIN for the
		// running month, had both columns NULL and failed the whole ETL_DELIVERY_MARGIN load.
		{"NULL value and display is skipped", sql.NullFloat64{}, sql.NullFloat64{}, 0, 0, false},
		{"NULL value with display is skipped", sql.NullFloat64{}, sql.NullFloat64{Float64: 3, Valid: true}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, d, ok := resolveMVValues(tt.value, tt.display)
			if v != tt.wantValue || d != tt.wantDisplay || ok != tt.wantOK {
				t.Fatalf("resolveMVValues() = (%v, %v, %v), want (%v, %v, %v)", v, d, ok, tt.wantValue, tt.wantDisplay, tt.wantOK)
			}
		})
	}
}
