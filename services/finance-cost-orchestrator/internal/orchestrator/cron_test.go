package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

// fakeCronPub records published job ids.
type fakeCronPub struct{ published []int64 }

func (f *fakeCronPub) PublishJobTriggered(_ context.Context, id int64) error {
	f.published = append(f.published, id)
	return nil
}

const (
	lockQueryRe   = `SELECT EXISTS \(\s*SELECT 1 FROM cst_period_lock`
	createQueryRe = `INSERT INTO cal_job`
)

func newMockCron(t *testing.T) (*CronScheduler, sqlmock.Sqlmock, *fakeCronPub) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	pub := &fakeCronPub{}
	s, err := NewCronScheduler(db, pub, "0 0 2 5 * *", "UTC")
	require.NoError(t, err)
	return s, mock, pub
}

// plan-02 P1-T5 / N-6: a locked ACTUAL period gives 0 CreateAutoJob calls and
// a nil error (no retry); the skip metric increments.
func TestTriggerJob_LockedPeriod_SkipsWithoutError(t *testing.T) {
	s, mock, pub := newMockCron(t)
	before := testutil.ToFloat64(skippedLockedTotal)
	mock.ExpectQuery(lockQueryRe).WithArgs("202608", "ACTUAL").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	published, err := s.triggerJob(context.Background(), "202608")
	require.NoError(t, err)
	require.False(t, published)
	require.Empty(t, pub.published)
	require.NoError(t, mock.ExpectationsWereMet(), "no INSERT INTO cal_job may run")
	require.InDelta(t, before+1, testutil.ToFloat64(skippedLockedTotal), 1e-9)
}

func TestTriggerJob_UnlockedPeriod_CreatesOneJob(t *testing.T) {
	s, mock, pub := newMockCron(t)
	before := testutil.ToFloat64(skippedLockedTotal)
	mock.ExpectQuery(lockQueryRe).WithArgs("202608", "ACTUAL").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(createQueryRe).WithArgs("202608", "ACTUAL", "ALL", "CRON", "system").
		WillReturnRows(sqlmock.NewRows([]string{"cj_job_id"}).AddRow(int64(42)))

	published, err := s.triggerJob(context.Background(), "202608")
	require.NoError(t, err)
	require.True(t, published)
	require.Equal(t, []int64{42}, pub.published)
	require.NoError(t, mock.ExpectationsWereMet())
	require.InDelta(t, before, testutil.ToFloat64(skippedLockedTotal), 1e-9)
}

func TestTriggerJob_LockCheckError_FailsClosed(t *testing.T) {
	s, mock, pub := newMockCron(t)
	mock.ExpectQuery(lockQueryRe).WillReturnError(errors.New("db down"))

	published, err := s.triggerJob(context.Background(), "202608")
	require.Error(t, err)
	require.False(t, published)
	require.Empty(t, pub.published)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFire_LockedPeriod_DoesNotPanicOrCreate(t *testing.T) {
	s, mock, pub := newMockCron(t)
	mock.ExpectQuery(lockQueryRe).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	s.fire()
	require.Empty(t, pub.published)
	require.NoError(t, mock.ExpectationsWereMet())
}
