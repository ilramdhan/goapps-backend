package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dashapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dashboard"
	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dashboard"
)

// fakeRepo is a hand-rolled dashboard.Repository stub capturing the args it saw.
type fakeRepo struct {
	agg          domain.AreaAggregate
	grid         []domain.McEffCell
	rows         []domain.MachineRow
	pending      []domain.PendingWO
	priorities   []domain.Priority
	stats        domain.QuickStats
	gotAvpDate   time.Time
	gotPrioDate  time.Time
	gotAggDate   time.Time
	gotExcluding bool
}

func (f *fakeRepo) AreaAggregate(_ context.Context, _ string, date time.Time, excluding bool) (domain.AreaAggregate, error) {
	f.gotAggDate = date
	f.gotExcluding = excluding
	return f.agg, nil
}

func (f *fakeRepo) McEffGrid(_ context.Context, _ string, _ time.Time) ([]domain.McEffCell, error) {
	return f.grid, nil
}

func (f *fakeRepo) MachineActualVsPlan(_ context.Context, _ string, date time.Time) ([]domain.MachineRow, error) {
	f.gotAvpDate = date
	return f.rows, nil
}

func (f *fakeRepo) PendingApprovalWOs(_ context.Context, _ string) ([]domain.PendingWO, error) {
	return f.pending, nil
}

func (f *fakeRepo) PrioritiesDueToday(_ context.Context, _ string, date time.Time) ([]domain.Priority, error) {
	f.gotPrioDate = date
	return f.priorities, nil
}

func (f *fakeRepo) QuickStats(_ context.Context, _ string) (domain.QuickStats, error) {
	return f.stats, nil
}

func TestGetDailyPerformance_BuildsKPIsAndGrid(t *testing.T) {
	repo := &fakeRepo{
		agg:  domain.AreaAggregate{ActualToday: 500, TheoToday: 1000},
		grid: []domain.McEffCell{{MachineID: 1, Shift: "1", EffPct: 88}},
	}
	svc := dashapp.NewService(repo)
	date := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	view, err := svc.GetDailyPerformance(context.Background(), dashapp.DailyPerformanceQuery{
		Date: date, Excluding: true,
	})
	require.NoError(t, err)
	assert.Len(t, view.KPIs, 5)
	assert.Len(t, view.McEffGrid, 1)
	assert.True(t, repo.gotExcluding)
	assert.Equal(t, date, repo.gotAggDate)
}

func TestGetMorningReview_UsesYesterdayForActualVsPlan(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{
		rows: []domain.MachineRow{{MachineID: 1, QtyTarget: 100, QtyActual: 120}},
		pending: []domain.PendingWO{
			{WoID: 7, WoNo: "WO-7", Status: "SUBMITTED", UpdatedAt: now.Add(-23 * time.Hour)},  // ~1h left => HIGH
			{WoID: 8, WoNo: "WO-8", Status: "PC_APPROVED", UpdatedAt: now.Add(-1 * time.Hour)}, // ~23h left => LOW
		},
		priorities: []domain.Priority{{WoID: 9, WoNo: "WO-9"}},
		stats:      domain.QuickStats{MachinesRunning: 2, MachinesTotal: 5, WOsPendingApproval: 2, UnmatchedSOCount: 3},
	}
	svc := dashapp.NewService(repo)
	date := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	view, err := svc.GetMorningReview(context.Background(), dashapp.MorningReviewQuery{Date: date})
	require.NoError(t, err)

	// actual-vs-plan must query yesterday, priorities the review day
	assert.Equal(t, date.AddDate(0, 0, -1), repo.gotAvpDate)
	assert.Equal(t, date, repo.gotPrioDate)

	require.Len(t, view.OpenIssues, 2)
	assert.Equal(t, domain.IssuePendingApproval, view.OpenIssues[0].IssueType)
	assert.Equal(t, int64(7), view.OpenIssues[0].RefID)
	assert.Equal(t, domain.SeverityHigh, view.OpenIssues[0].Severity)
	assert.Equal(t, domain.SeverityLow, view.OpenIssues[1].Severity)

	assert.Equal(t, dashboardFlag(view.ActualVsPlan[0]), domain.FlagOver)
	assert.Equal(t, int32(2), view.Stats.MachinesRunning)
	assert.Len(t, view.Priorities, 1)
}

func dashboardFlag(r domain.MachineRow) string { return r.Flag() }
