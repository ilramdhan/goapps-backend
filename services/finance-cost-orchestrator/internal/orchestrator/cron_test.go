package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreviousPeriodYYYYMM(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"may 5 → april", time.Date(2026, time.May, 5, 2, 0, 0, 0, time.UTC), "202604"},
		{"jan 5 → dec prev year", time.Date(2026, time.January, 5, 2, 0, 0, 0, time.UTC), "202512"},
		{"feb 5 → january", time.Date(2026, time.February, 5, 2, 0, 0, 0, time.UTC), "202601"},
		{"dec 5 → november", time.Date(2025, time.December, 5, 2, 0, 0, 0, time.UTC), "202511"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, previousPeriodYYYYMM(tc.in))
		})
	}
}
