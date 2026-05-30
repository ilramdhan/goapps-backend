package chartdata

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// PreviewQuery carries an unsaved dashboard config for live admin preview.
//
// Used by the admin wizard's right-pane to render chart against real fact data
// before the user clicks Save. No cache, no role check (admin form is permission-gated).
type PreviewQuery struct {
	FilterType   string
	FilterGroup1 string
	PeriodGrain  string
	ChartType    string
	ChartConfig  map[string]any
	KpiConfig    []map[string]any
	CompareModes []string
}

// PreviewHandler renders an unsaved dashboard configuration.
type PreviewHandler struct {
	fact factmetric.Repository
	now  func() time.Time
}

// NewPreviewHandler constructs a PreviewHandler.
func NewPreviewHandler(fact factmetric.Repository) *PreviewHandler {
	return &PreviewHandler{
		fact: fact,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// WithNow overrides the clock source.
func (h *PreviewHandler) WithNow(now func() time.Time) *PreviewHandler {
	if now != nil {
		h.now = now
	}
	return h
}

// Handle constructs a transient Dashboard from the preview query and runs the
// chart pipeline without caching. Default period is L12M.
func (h *PreviewHandler) Handle(ctx context.Context, q PreviewQuery) (*Result, error) {
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:               "PREVIEW",
		Title:              "Preview",
		FilterType:         q.FilterType,
		FilterGroup1:       q.FilterGroup1,
		PeriodGrain:        q.PeriodGrain,
		DefaultPeriod:      "L12M",
		ChartType:          q.ChartType,
		ChartConfigRaw:     q.ChartConfig,
		KpiConfigRaw:       q.KpiConfig,
		CompareModes:       q.CompareModes,
		DrillEnabled:       true,
		MaxDrillLevel:      3,
		CacheTTLSec:        0,
		RefreshIntervalSec: 0,
		GroupID:            uuid.New(),
		IsActive:           true,
		CreatedBy:          uuid.Nil,
	})
	if err != nil {
		return nil, fmt.Errorf("validate preview dashboard: %w", err)
	}

	filters := ViewerFilters{PeriodPreset: "L12M"}
	now := h.now()
	plan, err := Plan(d, filters, now)
	if err != nil {
		return nil, err
	}
	rows, err := h.fact.QueryAggregate(ctx, plan)
	if err != nil {
		return nil, err
	}

	period := ResolvePeriod(filters.PeriodPreset, time.Time{}, time.Time{}, d.PeriodGrain().String(), now)
	kpis, err := ComputeKPIs(ctx, h.fact, d, period, now, nil, nil)
	if err != nil {
		return nil, err
	}

	drillCtx := BuildDrillContext(d, filters)
	result := Shape(d, rows, kpis, filters, drillCtx)
	return &result, nil
}
