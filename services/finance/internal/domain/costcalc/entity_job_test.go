package costcalc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewJob_InvalidPeriod(t *testing.T) {
	_, err := NewJob("2026", CalcTypeActual, ScopeAll, nil, "MANUAL", "tester")
	require.ErrorIs(t, err, ErrInvalidPeriod)
}

func TestJob_HappyPathTransitions(t *testing.T) {
	j, err := NewJob("202604", CalcTypeActual, ScopeSingleProduct, nil, "MANUAL", "tester")
	require.NoError(t, err)
	require.Equal(t, JobStatusQueued, j.Status())

	require.NoError(t, j.MarkPlanning())
	require.Equal(t, JobStatusPlanning, j.Status())

	j.SetTotals(10, 1, 1)
	require.Equal(t, 10, j.TotalProducts())

	require.NoError(t, j.MarkProcessing())
	require.NotNil(t, j.StartedAt())

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, j.MarkComplete(10, 0, 0))
	require.Equal(t, JobStatusSuccess, j.Status())
	require.NotNil(t, j.CompletedAt())
	require.Greater(t, j.DurationMs(), int64(0))
}

func TestJob_MarkComplete(t *testing.T) {
	tests := []struct {
		name        string
		startStatus JobStatus
		succ        int
		fail        int
		blocked     int
		want        JobStatus
		wantErr     bool
	}{
		{"all success then SUCCESS", JobStatusProcessing, 100, 0, 0, JobStatusSuccess, false},
		{"partial fail then PARTIAL_FAILED", JobStatusProcessing, 50, 50, 0, JobStatusPartialFailed, false},
		{"partial blocked then PARTIAL_FAILED", JobStatusProcessing, 50, 0, 50, JobStatusPartialFailed, false},
		{"all fail then FAILED", JobStatusProcessing, 0, 100, 0, JobStatusFailed, false},
		{"from queued then invalid", JobStatusQueued, 100, 0, 0, "", true},
		{"from planning then invalid", JobStatusPlanning, 100, 0, 0, "", true},
		{"from cancelled then invalid", JobStatusCancelled, 100, 0, 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{status: tt.startStatus}
			now := time.Now()
			j.startedAt = &now
			err := j.MarkComplete(tt.succ, tt.fail, tt.blocked)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrJobInvalidStatus)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, j.Status())
		})
	}
}

func TestJob_Cancel(t *testing.T) {
	for _, from := range []JobStatus{JobStatusQueued, JobStatusPlanning, JobStatusProcessing} {
		t.Run("from "+string(from), func(t *testing.T) {
			j := &Job{status: from}
			require.NoError(t, j.Cancel())
			require.Equal(t, JobStatusCancelled, j.Status())
		})
	}
	for _, from := range []JobStatus{JobStatusSuccess, JobStatusFailed, JobStatusPartialFailed, JobStatusCancelled} {
		t.Run("from "+string(from)+" invalid", func(t *testing.T) {
			j := &Job{status: from}
			require.ErrorIs(t, j.Cancel(), ErrJobInvalidStatus)
		})
	}
}
