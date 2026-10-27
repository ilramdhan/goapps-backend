package job_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/job"
	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

type countingRefresher struct {
	calls int
	err   error
}

func (r *countingRefresher) RefreshMVs(context.Context) error {
	r.calls++
	return r.err
}

type stubETL struct{ err error }

func (e stubETL) Load(context.Context, string, string, uuid.UUID) (int, error) { return 7, e.err }

func etlJob(refresh bool) *jobdomain.Job {
	cfg := map[string]any{"kind": "etl_hr_ot_staging", "target_type": "HR_OT_STAGING"}
	if refresh {
		cfg["refresh_mvs"] = true
	}
	return &jobdomain.Job{ID: uuid.New(), Name: "ETL_HR_OT_DAILY", Config: cfg}
}

// H013: an ETL job with config.refresh_mvs refreshes the BI MVs after a successful load only.
func TestCronTrigger_ETLRefreshMVs(t *testing.T) {
	tests := []struct {
		name      string
		refresh   bool
		loadErr   error
		wantCalls int
		wantOK    bool
	}{
		{"refresh after load", true, nil, 1, true},
		{"no flag no refresh", false, nil, 0, true},
		{"failed load skips refresh", true, errors.New("oracle down"), 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := etlJob(tc.refresh)
			repo := &stubRepo{jobs: []*jobdomain.Job{j}}
			mv := &countingRefresher{}
			h := job.NewTriggerHandler(repo, mv, stubETL{err: tc.loadErr}, nil)
			entry, err := h.CronTrigger(context.Background(), j.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCalls, mv.calls)
			assert.Equal(t, tc.wantOK, entry.Status == jobdomain.StatusSuccess)
		})
	}
}

func TestCronTrigger_ETLRefreshFailureFailsJob(t *testing.T) {
	j := etlJob(true)
	repo := &stubRepo{jobs: []*jobdomain.Job{j}}
	h := job.NewTriggerHandler(repo, &countingRefresher{err: errors.New("lock")}, stubETL{}, nil)
	entry, err := h.CronTrigger(context.Background(), j.ID)
	require.NoError(t, err)
	assert.NotEqual(t, jobdomain.StatusSuccess, entry.Status)
}

func TestValidateCronExpression_TimeZonePrefix(t *testing.T) {
	assert.NoError(t, jobdomain.ValidateCronExpression("CRON_TZ=Asia/Jakarta 0 5 * * *"))
	assert.NoError(t, jobdomain.ValidateCronExpression("0 5 * * *"))
	assert.Error(t, jobdomain.ValidateCronExpression("CRON_TZ=Asia/Jakarta"))
	assert.Error(t, jobdomain.ValidateCronExpression("CRON_TZ=Asia/Jakarta 0 5 * *"))
	assert.Error(t, jobdomain.ValidateCronExpression("CRON_TZ=Foo/Bar 0 5 * * *"))
}
