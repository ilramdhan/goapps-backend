package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   Status
		terminal bool
	}{
		{StatusQueued, false},
		{StatusProcessing, false},
		{StatusSuccess, true},
		{StatusFailed, true},
		{StatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(tc.status.String(), func(t *testing.T) {
			assert.Equal(t, tc.terminal, tc.status.IsTerminal())
		})
	}
}

func TestStatus_IsActive(t *testing.T) {
	tests := []struct {
		status Status
		active bool
	}{
		{StatusQueued, true},
		{StatusProcessing, true},
		{StatusSuccess, false},
		{StatusFailed, false},
		{StatusCancelled, false},
	}

	for _, tc := range tests {
		t.Run(tc.status.String(), func(t *testing.T) {
			assert.Equal(t, tc.active, tc.status.IsActive())
		})
	}
}

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "SYNC-202601-001", false},
		{"empty", "", true},
		{"too long", "A123456789012345678901234567890X", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, err := NewCode(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.input, code.String())
			}
		})
	}
}

func TestGenerateCode(t *testing.T) {
	tests := []struct {
		name     string
		jobType  Type
		period   string
		seq      int
		expected string
	}{
		{"oracle sync", TypeOracleSync, "202601", 1, "ORACLE_SYN-202601-001"},
		{"calculation", TypeCalculation, "202603", 42, "CALCULATIO-202603-042"},
		{"no period", TypeExport, "", 5, "EXPORT-NOPERIOD-005"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code := GenerateCode(tc.jobType, tc.period, tc.seq)
			assert.Equal(t, tc.expected, code.String())
		})
	}
}
