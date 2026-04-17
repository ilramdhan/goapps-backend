package oraclesync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolvePeriod(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		expected string
	}{
		{
			name:     "day 1 returns previous month",
			date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			expected: "202601",
		},
		{
			name:     "day 3 returns previous month",
			date:     time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			expected: "202601",
		},
		{
			name:     "day 5 returns previous month",
			date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			expected: "202601",
		},
		{
			name:     "day 6 returns current month",
			date:     time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC),
			expected: "202602",
		},
		{
			name:     "day 15 returns current month",
			date:     time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			expected: "202602",
		},
		{
			name:     "january day 1 wraps to previous year december",
			date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "202512",
		},
		{
			name:     "december day 6 returns current month",
			date:     time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC),
			expected: "202612",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolvePeriod(tt.date)
			assert.Equal(t, tt.expected, result)
		})
	}
}
