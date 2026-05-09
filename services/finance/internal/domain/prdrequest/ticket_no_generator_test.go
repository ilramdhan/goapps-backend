// Package prdrequest_test contains domain-layer tests for ticket-number utilities.
package prdrequest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// =============================================================================
// FormatTicketNo
// =============================================================================

func TestFormatTicketNo(t *testing.T) {
	tests := []struct {
		name    string
		period  string
		seq     int
		want    string
		wantErr error
	}{
		{
			name:   "seq 1 pads to 001",
			period: "202504", seq: 1,
			want: "PR-202504-001",
		},
		{
			name:   "seq 100 formats correctly",
			period: "202504", seq: 100,
			want: "PR-202504-100",
		},
		{
			name:   "seq 1000 exceeds 3 digits",
			period: "202504", seq: 1000,
			want: "PR-202504-1000",
		},
		{
			name:   "different valid period",
			period: "199912", seq: 42,
			want: "PR-199912-042",
		},
		{
			name:    "seq 0 is invalid",
			period:  "202504", seq: 0,
			wantErr: prdrequest.ErrInvalidTicketNo,
		},
		{
			name:    "negative seq is invalid",
			period:  "202504", seq: -1,
			wantErr: prdrequest.ErrInvalidTicketNo,
		},
		{
			name:    "period too short is invalid",
			period:  "20250", seq: 1,
			wantErr: prdrequest.ErrInvalidTicketNo,
		},
		{
			name:    "period too long is invalid",
			period:  "2025041", seq: 1,
			wantErr: prdrequest.ErrInvalidTicketNo,
		},
		{
			name:    "period with letters is invalid",
			period:  "2025AB", seq: 1,
			wantErr: prdrequest.ErrInvalidTicketNo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tn, err := prdrequest.FormatTicketNo(tc.period, tc.seq)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "expected %v but got %v", tc.wantErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tn.String())
		})
	}
}

// =============================================================================
// NewTicketNo / ParseTicketNo
// =============================================================================

func TestNewTicketNo_ValidFormats(t *testing.T) {
	valid := []string{
		"PR-202504-001",
		"PR-202504-100",
		"PR-202504-1000",
		"PR-199912-999",
	}
	for _, s := range valid {
		t.Run(s, func(t *testing.T) {
			tn, err := prdrequest.NewTicketNo(s)
			require.NoError(t, err)
			assert.Equal(t, s, tn.String())
		})
	}
}

func TestNewTicketNo_InvalidFormats(t *testing.T) {
	invalid := []string{
		"",
		"PR-20250-001",  // period only 5 digits
		"PR-2025041-001", // period 7 digits
		"PR-202504-01",  // seq only 2 digits
		"pr-202504-001", // lowercase prefix
		"PQ-202504-001", // wrong prefix
		"202504-001",    // missing PR-
	}
	for _, s := range invalid {
		t.Run(s, func(t *testing.T) {
			_, err := prdrequest.NewTicketNo(s)
			require.Error(t, err)
			assert.True(t, errors.Is(err, prdrequest.ErrInvalidTicketNo))
		})
	}
}

func TestParseTicketNo(t *testing.T) {
	period, seq, err := prdrequest.ParseTicketNo("PR-202504-042")
	require.NoError(t, err)
	assert.Equal(t, "202504", period)
	assert.Equal(t, 42, seq)
}

func TestParseTicketNo_LargeSeq(t *testing.T) {
	period, seq, err := prdrequest.ParseTicketNo("PR-202504-1000")
	require.NoError(t, err)
	assert.Equal(t, "202504", period)
	assert.Equal(t, 1000, seq)
}

func TestParseTicketNo_Invalid(t *testing.T) {
	_, _, err := prdrequest.ParseTicketNo("INVALID")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prdrequest.ErrInvalidTicketNo))
}

// =============================================================================
// PeriodForNow
// =============================================================================

func TestPeriodForNow_Format(t *testing.T) {
	// Use a fixed time in UTC: 2025-04-15 10:00:00 UTC.
	// Asia/Jakarta is UTC+7, so local time is 2025-04-15 17:00:00 WIB.
	// Expected period: "202504".
	fixed := time.Date(2025, 4, 15, 10, 0, 0, 0, time.UTC)
	period, err := prdrequest.PeriodForNow(fixed)
	require.NoError(t, err)
	assert.Equal(t, "202504", period)
}

func TestPeriodForNow_MonthBoundary(t *testing.T) {
	// 2025-04-30 23:59:59 UTC = 2025-05-01 06:59:59 WIB → period 202505.
	crossMidnight := time.Date(2025, 4, 30, 23, 59, 59, 0, time.UTC)
	period, err := prdrequest.PeriodForNow(crossMidnight)
	require.NoError(t, err)
	assert.Equal(t, "202505", period)
}

func TestPeriodForNow_ResultIsYYYYMM(t *testing.T) {
	period, err := prdrequest.PeriodForNow(time.Now())
	require.NoError(t, err)
	assert.Len(t, period, 6, "period must be exactly 6 digits (YYYYMM)")
	for _, ch := range period {
		assert.True(t, ch >= '0' && ch <= '9', "period must be all digits")
	}
}
