package oracle

import (
	"database/sql"
	"testing"
)

func TestIsActiveFromFrzFlag(t *testing.T) {
	tests := []struct {
		name string
		flag sql.NullInt64
		want bool
	}{
		{"frozen (1) is inactive", sql.NullInt64{Int64: 1, Valid: true}, false},
		{"zero is active", sql.NullInt64{Int64: 0, Valid: true}, true},
		{"NULL is active", sql.NullInt64{Valid: false}, true},
		// value 2: real production data (2 of 2320 rows), meaning unknown — see the
		// decision-gate comment on isActiveFromFrzFlag. Defaults to active; not a
		// verified fact, just the current (deliberately non-guessing) behavior.
		{"value 2 (2 real prod rows, meaning unknown) defaults to active", sql.NullInt64{Int64: 2, Valid: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isActiveFromFrzFlag(tt.flag); got != tt.want {
				t.Errorf("isActiveFromFrzFlag(%+v) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}
