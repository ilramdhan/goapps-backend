package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

type memBacktestRepo struct {
	batchID int64
	period  string
	rep     domain.BacktestReport
	err     error
}

func (m *memBacktestRepo) Replace(_ context.Context, id int64, period string, r domain.BacktestReport) error {
	m.batchID, m.period, m.rep = id, period, r
	return m.err
}

func (m *memBacktestRepo) Get(context.Context, int64) (domain.BacktestReport, error) {
	return m.rep, nil
}

func newBacktestJob(t *testing.T, jobs *memJobs, period, params string) *job.Execution {
	t.Helper()
	var raw json.RawMessage
	if params != "" {
		raw = json.RawMessage(params)
	}
	exec, err := job.NewExecution(job.TypeErpIntegration, SubtypeBacktest, period, "alice", 5, raw)
	require.NoError(t, err)
	require.NoError(t, jobs.Create(context.Background(), exec))
	return exec
}

func TestJobExecutor_Backtest(t *testing.T) {
	ctx := context.Background()

	t.Run("failed comparison still completes", func(t *testing.T) {
		var order []string
		h, _, _ := btHandler(t, &order, "")
		repo := &memBacktestRepo{}
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithBacktest(h, repo)
		exec := newBacktestJob(t, jobs, "202604", "")
		require.NoError(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, job.StatusSuccess, exec.Status())
		var res BacktestJobResult
		require.NoError(t, json.Unmarshal(exec.ResultSummary(), &res))
		assert.True(t, res.Failed)
		assert.Equal(t, "202604", res.Period)
		assert.NotZero(t, res.BatchID)
		assert.Equal(t, 1, res.Counts["DIFF"])
		assert.Equal(t, res.BatchID, repo.batchID)
		assert.Len(t, repo.rep.Lines, 1)
	})

	t.Run("params period wins", func(t *testing.T) {
		var order []string
		h, _, _ := btHandler(t, &order, "")
		repo := &memBacktestRepo{}
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithBacktest(h, repo)
		exec := newBacktestJob(t, jobs, "202604", `{"period":" 202606 "}`)
		require.NoError(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, "202606", repo.period)
	})

	t.Run("handler error fails job", func(t *testing.T) {
		var order []string
		h, _, _ := btHandler(t, &order, "derive")
		repo := &memBacktestRepo{}
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithBacktest(h, repo)
		exec := newBacktestJob(t, jobs, "202604", "")
		require.Error(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, job.StatusFailed, exec.Status())
		assert.Zero(t, repo.batchID)
	})

	t.Run("persist error fails job", func(t *testing.T) {
		var order []string
		h, _, _ := btHandler(t, &order, "")
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithBacktest(h, &memBacktestRepo{err: errors.New("db down")})
		exec := newBacktestJob(t, jobs, "202604", "")
		require.Error(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, job.StatusFailed, exec.Status())
	})

	t.Run("not configured fails job", func(t *testing.T) {
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil)
		exec := newBacktestJob(t, jobs, "202604", "")
		require.Error(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, job.StatusFailed, exec.Status())
	})
}
