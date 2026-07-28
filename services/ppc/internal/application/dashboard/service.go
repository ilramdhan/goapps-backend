// Package dashboard provides the application use cases for the PPC read
// dashboards: daily performance (KPI cards + MC-EFF heatmap) and morning review
// (actual-vs-plan, open issues, priorities, quick stats). It orchestrates the
// aggregation repository and applies the pure domain logic.
package dashboard

import (
	"context"
	"fmt"
	"time"

	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dashboard"
)

// Repository is the read surface the dashboard service needs. Implemented by
// postgres.DashboardRepository.
type Repository interface {
	AreaAggregate(ctx context.Context, area string, date time.Time, excluding bool) (domain.AreaAggregate, error)
	McEffGrid(ctx context.Context, area string, date time.Time) ([]domain.McEffCell, error)
	MachineActualVsPlan(ctx context.Context, area string, date time.Time) ([]domain.MachineRow, error)
	PendingApprovalWOs(ctx context.Context, area string) ([]domain.PendingWO, error)
	PrioritiesDueToday(ctx context.Context, area string, date time.Time) ([]domain.Priority, error)
	QuickStats(ctx context.Context, area string) (domain.QuickStats, error)
}

// Service builds the read-dashboard responses over its repository.
type Service struct {
	repo Repository
}

// NewService builds a dashboard service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// DailyPerformance is the assembled daily-performance dashboard.
type DailyPerformance struct {
	KPIs      []domain.KPI
	McEffGrid []domain.McEffCell
}

// DailyPerformanceQuery selects the daily-performance view.
type DailyPerformanceQuery struct {
	Area      string // "" = all areas
	Date      time.Time
	Excluding bool
}

// GetDailyPerformance assembles the KPI cards (Today + MTD) and the monthly
// MC-EFF heatmap for the requested area and date.
func (s *Service) GetDailyPerformance(ctx context.Context, q DailyPerformanceQuery) (DailyPerformance, error) {
	agg, err := s.repo.AreaAggregate(ctx, q.Area, q.Date, q.Excluding)
	if err != nil {
		return DailyPerformance{}, fmt.Errorf("daily performance aggregate: %w", err)
	}
	grid, err := s.repo.McEffGrid(ctx, q.Area, q.Date)
	if err != nil {
		return DailyPerformance{}, fmt.Errorf("daily performance mc-eff: %w", err)
	}
	return DailyPerformance{
		KPIs:      domain.BuildKPIs(agg),
		McEffGrid: grid,
	}, nil
}

// MorningReview is the assembled morning-review dashboard.
type MorningReview struct {
	ActualVsPlan []domain.MachineRow
	OpenIssues   []domain.Issue
	Priorities   []domain.Priority
	Stats        domain.QuickStats
}

// MorningReviewQuery selects the morning-review view.
type MorningReviewQuery struct {
	Area string // "" = all areas
	Date time.Time
}

// GetMorningReview assembles the morning-review dashboard: yesterday's actual vs
// plan per machine, open issues (pending-approval urgency), today's priorities,
// and headline quick stats. The date is the review day; actual-vs-plan compares
// the prior day (yesterday), priorities are due on/before the review day.
func (s *Service) GetMorningReview(ctx context.Context, q MorningReviewQuery) (MorningReview, error) {
	yesterday := q.Date.AddDate(0, 0, -1)

	rows, err := s.repo.MachineActualVsPlan(ctx, q.Area, yesterday)
	if err != nil {
		return MorningReview{}, fmt.Errorf("morning review actual-vs-plan: %w", err)
	}
	pending, err := s.repo.PendingApprovalWOs(ctx, q.Area)
	if err != nil {
		return MorningReview{}, fmt.Errorf("morning review pending approvals: %w", err)
	}
	priorities, err := s.repo.PrioritiesDueToday(ctx, q.Area, q.Date)
	if err != nil {
		return MorningReview{}, fmt.Errorf("morning review priorities: %w", err)
	}
	stats, err := s.repo.QuickStats(ctx, q.Area)
	if err != nil {
		return MorningReview{}, fmt.Errorf("morning review quick stats: %w", err)
	}

	return MorningReview{
		ActualVsPlan: rows,
		OpenIssues:   buildIssues(pending, time.Now()),
		Priorities:   priorities,
		Stats:        stats,
	}, nil
}

// buildIssues turns pending-approval WOs into open-issue lines graded by how
// close each WO is to auto-approval.
func buildIssues(pending []domain.PendingWO, now time.Time) []domain.Issue {
	issues := make([]domain.Issue, 0, len(pending))
	for _, w := range pending {
		severity, hoursLeft := domain.ApprovalSeverity(w.UpdatedAt, now)
		issues = append(issues, domain.Issue{
			IssueType: domain.IssuePendingApproval,
			RefID:     w.WoID,
			Title:     fmt.Sprintf("WO %s pending approval", w.WoNo),
			Detail:    fmt.Sprintf("status %s, ~%.1fh until auto-approve", w.Status, hoursLeft),
			Severity:  severity,
		})
	}
	return issues
}
