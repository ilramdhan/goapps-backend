package erpintegration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

func newBackfillJob(t *testing.T, jobs *memJobs, period, params string) *job.Execution {
	t.Helper()
	var raw json.RawMessage
	if params != "" {
		raw = json.RawMessage(params)
	}
	exec, err := job.NewExecution(job.TypeErpIntegration, SubtypeAttrBackfill, period, "alice", 5, raw)
	require.NoError(t, err)
	require.NoError(t, jobs.Create(context.Background(), exec))
	return exec
}

func TestJobExecutor_AttrBackfill(t *testing.T) {
	ctx := context.Background()

	t.Run("default dry run, job period", func(t *testing.T) {
		jobs := newMemJobs()
		reader := &fakeLegacyReader{rows: legacyFixture()}
		repo := &fakeBackfillRepo{products: productsFixture()}
		ex := NewJobExecutor(jobs, nil, nil, nil).WithAttrBackfill(NewBackfillAttributesHandler(reader, repo, nil))
		exec := newBackfillJob(t, jobs, "202608", "")
		require.NoError(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, job.StatusSuccess, exec.Status())
		assert.Empty(t, repo.writes)
		assert.Equal(t, []string{"202608"}, reader.periods)
		var rep BackfillReport
		require.NoError(t, json.Unmarshal(exec.ResultSummary(), &rep))
		assert.Equal(t, BackfillModeDryRun, rep.Mode)
		assert.Equal(t, []int{progressStart, progressRead, progressPersist}, jobs.progress[exec.ID()])
	})

	t.Run("apply with params period", func(t *testing.T) {
		jobs := newMemJobs()
		reader := &fakeLegacyReader{rows: legacyFixture()}
		repo := &fakeBackfillRepo{products: productsFixture()}
		ex := NewJobExecutor(jobs, nil, nil, nil).WithAttrBackfill(NewBackfillAttributesHandler(reader, repo, nil))
		exec := newBackfillJob(t, jobs, "202608", `{"dry_run":false,"period":" 202607 "}`)
		require.NoError(t, ex.Execute(ctx, exec.ID()))
		assert.Equal(t, []string{"202607"}, reader.periods)
		assert.NotEmpty(t, repo.writes)
		var rep BackfillReport
		require.NoError(t, json.Unmarshal(exec.ResultSummary(), &rep))
		assert.Equal(t, BackfillModeApply, rep.Mode)
	})

	t.Run("not configured fails job", func(t *testing.T) {
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil)
		exec := newBackfillJob(t, jobs, "202608", `{}`)
		require.ErrorIs(t, ex.Execute(ctx, exec.ID()), domain.ErrAttrBackfillNotConfigured)
		assert.Equal(t, job.StatusFailed, exec.Status())
	})

	t.Run("nil reader fails closed", func(t *testing.T) {
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithAttrBackfill(NewBackfillAttributesHandler(nil, &fakeBackfillRepo{}, nil))
		exec := newBackfillJob(t, jobs, "", `null`)
		require.ErrorIs(t, ex.Execute(ctx, exec.ID()), domain.ErrAttrBackfillNotConfigured)
	})

	t.Run("bad params", func(t *testing.T) {
		jobs := newMemJobs()
		ex := NewJobExecutor(jobs, nil, nil, nil).WithAttrBackfill(NewBackfillAttributesHandler(&fakeLegacyReader{}, &fakeBackfillRepo{}, nil))
		exec := newBackfillJob(t, jobs, "202608", `{"dry_run":"yes"}`)
		require.ErrorIs(t, ex.Execute(ctx, exec.ID()), ErrInvalidJobParams)
	})
}
