package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRange_Validation(t *testing.T) {
	tests := []struct {
		name, d1, d2, wantErr string
	}{
		{"empty d1", "", "2026-10-29", "required"},
		{"empty d2", "2026-01-05", "", "required"},
		{"bad d1", "05-01-2026", "2026-10-29", "-d1"},
		{"bad d2", "2026-01-05", "2026/10/29", "-d2"},
		{"d2 before d1", "2026-10-29", "2026-01-05", "-d2 before -d1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseRange(tt.d1, tt.d2)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseRange_Valid_ReturnsDates(t *testing.T) {
	d1, d2, err := parseRange("2026-01-05", "2026-01-05")
	require.NoError(t, err)
	assert.Equal(t, time.Monday, d1.Weekday())
	assert.Equal(t, d1, d2)
}
