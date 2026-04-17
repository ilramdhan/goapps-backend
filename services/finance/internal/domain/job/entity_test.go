package job

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecution_Success(t *testing.T) {
	params := json.RawMessage(`{"period":"202601"}`)
	exec, err := NewExecution(TypeOracleSync, "item_cons_stk_po", "202601", "user:test@test.com", 5, params)

	require.NoError(t, err)
	assert.NotEmpty(t, exec.ID())
	assert.Equal(t, TypeOracleSync, exec.JobType())
	assert.Equal(t, "item_cons_stk_po", exec.Subtype())
	assert.Equal(t, "202601", exec.Period())
	assert.Equal(t, StatusQueued, exec.Status())
	assert.Equal(t, 5, exec.Priority())
	assert.Equal(t, 0, exec.Progress())
	assert.Equal(t, 0, exec.RetryCount())
	assert.Equal(t, 3, exec.MaxRetries())
	assert.Equal(t, "user:test@test.com", exec.CreatedBy())
	assert.Nil(t, exec.StartedAt())
	assert.Nil(t, exec.CompletedAt())
}

func TestNewExecution_Validation(t *testing.T) {
	tests := []struct {
		name      string
		jobType   Type
		createdBy string
		priority  int
		wantErr   error
	}{
		{"empty job type", "", "user:test", 5, ErrEmptyJobType},
		{"empty created by", TypeOracleSync, "", 5, ErrEmptyCreatedBy},
		{"priority too low", TypeOracleSync, "user:test", 0, ErrInvalidPriority},
		{"priority too high", TypeOracleSync, "user:test", 11, ErrInvalidPriority},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewExecution(tc.jobType, "", "", tc.createdBy, tc.priority, nil)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestExecution_StatusTransitions(t *testing.T) {
	exec := createTestExecution(t)

	// QUEUED -> PROCESSING.
	require.NoError(t, exec.Start())
	assert.Equal(t, StatusProcessing, exec.Status())
	assert.NotNil(t, exec.StartedAt())

	// Cannot start again.
	assert.ErrorIs(t, exec.Start(), ErrInvalidStatus)

	// PROCESSING -> SUCCESS.
	result := json.RawMessage(`{"total_rows":100}`)
	require.NoError(t, exec.Complete(result))
	assert.Equal(t, StatusSuccess, exec.Status())
	assert.Equal(t, 100, exec.Progress())
	assert.NotNil(t, exec.CompletedAt())
	assert.JSONEq(t, `{"total_rows":100}`, string(exec.ResultSummary()))
}

func TestExecution_Fail(t *testing.T) {
	exec := createTestExecution(t)
	require.NoError(t, exec.Start())

	require.NoError(t, exec.Fail("connection timeout"))
	assert.Equal(t, StatusFailed, exec.Status())
	assert.Equal(t, "connection timeout", exec.ErrorMessage())
	assert.Equal(t, 1, exec.RetryCount())
	assert.NotNil(t, exec.CompletedAt())

	// Cannot fail a terminal job.
	assert.ErrorIs(t, exec.Fail("again"), ErrAlreadyCompleted)
}

func TestExecution_Cancel(t *testing.T) {
	exec := createTestExecution(t)

	require.NoError(t, exec.Cancel("admin@test.com"))
	assert.Equal(t, StatusCancelled, exec.Status())
	assert.Equal(t, "admin@test.com", exec.CancelledBy())
	assert.NotNil(t, exec.CancelledAt())

	// Cannot cancel again.
	assert.ErrorIs(t, exec.Cancel("admin@test.com"), ErrAlreadyCancelled)
}

func TestExecution_CancelProcessing(t *testing.T) {
	exec := createTestExecution(t)
	require.NoError(t, exec.Start())

	require.NoError(t, exec.Cancel("admin@test.com"))
	assert.Equal(t, StatusCancelled, exec.Status())
}

func TestExecution_CancelCompleted(t *testing.T) {
	exec := createTestExecution(t)
	require.NoError(t, exec.Start())
	require.NoError(t, exec.Complete(nil))

	assert.ErrorIs(t, exec.Cancel("admin@test.com"), ErrNotCancellable)
}

func TestExecution_UpdateProgress(t *testing.T) {
	exec := createTestExecution(t)

	exec.UpdateProgress(50)
	assert.Equal(t, 50, exec.Progress())

	// Clamp to bounds.
	exec.UpdateProgress(-10)
	assert.Equal(t, 0, exec.Progress())

	exec.UpdateProgress(200)
	assert.Equal(t, 100, exec.Progress())
}

func TestExecution_CanRetry(t *testing.T) {
	exec := createTestExecution(t)
	require.NoError(t, exec.Start())

	// Before any failure, can retry.
	assert.True(t, exec.CanRetry())
	assert.Equal(t, 0, exec.RetryCount())

	// After 1 fail, retryCount=1, maxRetries=3 -> can still retry.
	require.NoError(t, exec.Fail("error"))
	assert.Equal(t, 1, exec.RetryCount())
	assert.True(t, exec.CanRetry())
}

func TestExecutionLog(t *testing.T) {
	exec := createTestExecution(t)
	log := NewExecutionLog(exec.ID(), "oracle_procedure", LogStarted, "Executing procedure...", nil)

	assert.NotEmpty(t, log.ID())
	assert.Equal(t, exec.ID(), log.JobID())
	assert.Equal(t, "oracle_procedure", log.Step())
	assert.Equal(t, LogStarted, log.Status())
	assert.Equal(t, "Executing procedure...", log.Message())
	assert.Nil(t, log.CompletedAt())
	assert.Nil(t, log.DurationMs())

	log.MarkCompleted(LogSuccess, "Done in 10s")
	assert.Equal(t, LogSuccess, log.Status())
	assert.Equal(t, "Done in 10s", log.Message())
	assert.NotNil(t, log.CompletedAt())
	assert.NotNil(t, log.DurationMs())
}

func createTestExecution(t *testing.T) *Execution {
	t.Helper()
	exec, err := NewExecution(TypeOracleSync, "item_cons_stk_po", "202601", "user:test@test.com", 5, nil)
	require.NoError(t, err)
	return exec
}
