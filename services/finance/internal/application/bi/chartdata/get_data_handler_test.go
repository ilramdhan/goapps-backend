package chartdata_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ---- mock dashboard repository ----

type mockDashboardRepo struct {
	getByCode func(ctx context.Context, code string) (*dashboarddomain.Dashboard, error)
}

func (m *mockDashboardRepo) Create(context.Context, *dashboarddomain.Dashboard) error { return nil }
func (m *mockDashboardRepo) GetByID(context.Context, uuid.UUID) (*dashboarddomain.Dashboard, error) {
	return nil, dashboarddomain.ErrNotFound
}
func (m *mockDashboardRepo) GetByCode(ctx context.Context, code string) (*dashboarddomain.Dashboard, error) {
	return m.getByCode(ctx, code)
}
func (m *mockDashboardRepo) List(context.Context, dashboarddomain.ListFilter) ([]*dashboarddomain.Dashboard, int64, error) {
	return nil, 0, nil
}
func (m *mockDashboardRepo) Update(context.Context, *dashboarddomain.Dashboard) error    { return nil }
func (m *mockDashboardRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error       { return nil }
func (m *mockDashboardRepo) Duplicate(context.Context, uuid.UUID, string, string, uuid.UUID) (*dashboarddomain.Dashboard, error) {
	return nil, nil
}
func (m *mockDashboardRepo) SetRoles(context.Context, uuid.UUID, []string, uuid.UUID) error { return nil }
func (m *mockDashboardRepo) GetRoles(context.Context, uuid.UUID) ([]string, error)          { return nil, nil }
func (m *mockDashboardRepo) ListAccessible(context.Context, []string, bool) ([]*dashboarddomain.Dashboard, error) {
	return nil, nil
}

// ---- mock fact repository ----

type mockFactRepo struct {
	aggRows  []factmetric.AggRow
	aggErr   error
	queries  int
}

func (m *mockFactRepo) GetDistincts(context.Context, factmetric.DistinctScope) (factmetric.DistinctValues, error) {
	return factmetric.DistinctValues{}, nil
}
func (m *mockFactRepo) QueryAggregate(context.Context, factmetric.PlannedQuery) ([]factmetric.AggRow, error) {
	m.queries++
	return m.aggRows, m.aggErr
}
func (m *mockFactRepo) Upsert(context.Context, []factmetric.FactMetric) error { return nil }

// ---- mock cache ----

type mockCache struct {
	store map[string][]byte
	sets  int
}

func newMockCache() *mockCache { return &mockCache{store: map[string][]byte{}} }
func (c *mockCache) Get(_ context.Context, code, hash string, _ any) (bool, error) {
	_, ok := c.store[code+hash]
	return ok, nil // we don't deserialize; presence indicates a hit but we return false-shaped miss for simplicity
}
func (c *mockCache) Set(_ context.Context, code, hash string, _ any, _ time.Duration) error {
	c.sets++
	c.store[code+hash] = []byte("x")
	return nil
}

func buildViewerDashboard(t *testing.T, roles []string) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:           "EBITDA",
		Title:          "EBITDA",
		FilterType:     "MIS",
		FilterGroup1:   "EBITDA",
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      "waterfall",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
		CompareModes:   []string{"YoY"},
		MaxDrillLevel:  3,
		CacheTTLSec:    1800,
		GroupID:        uuid.New(),
		IsActive:       true,
		AllowedRoleCodes: roles,
		CreatedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGetDataHandler_HappyPath_QueriesAndCaches(t *testing.T) {
	dash := buildViewerDashboard(t, nil) // no role whitelist = open
	repo := &mockDashboardRepo{getByCode: func(_ context.Context, _ string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	fact := &mockFactRepo{aggRows: []factmetric.AggRow{{Category: "INCOME", Value: 100}}}
	cache := newMockCache()

	h := chartdata.NewGetDataHandler(repo, fact, cache, func(any) string { return "h1" }).
		WithNow(func() time.Time { return time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC) })

	res, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "EBITDA",
		Filters:       chartdata.ViewerFilters{PeriodPreset: "L12M"},
		UserRoles:     []string{"ANY"},
		IsSuperAdmin:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) == 0 || len(res.Series[0].Points) != 1 {
		t.Fatalf("expected 1 data point, got %+v", res.Series)
	}
	if res.Series[0].Points[0].Category != "INCOME" {
		t.Errorf("unexpected category: %v", res.Series[0].Points[0].Category)
	}
	if cache.sets != 1 {
		t.Errorf("expected cache Set called once, got %d", cache.sets)
	}
}

func TestGetDataHandler_Forbidden(t *testing.T) {
	dash := buildViewerDashboard(t, []string{"CFO"}) // whitelist excludes our user
	repo := &mockDashboardRepo{getByCode: func(_ context.Context, _ string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	fact := &mockFactRepo{}
	h := chartdata.NewGetDataHandler(repo, fact, newMockCache(), func(any) string { return "h" })

	_, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "EBITDA",
		UserRoles:     []string{"INTERN"},
		IsSuperAdmin:  false,
	})
	if !errors.Is(err, dashboarddomain.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
	if fact.queries != 0 {
		t.Error("forbidden request must not hit the fact repo")
	}
}

func TestGetDataHandler_SuperAdminBypass(t *testing.T) {
	dash := buildViewerDashboard(t, []string{"CFO"})
	repo := &mockDashboardRepo{getByCode: func(_ context.Context, _ string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	fact := &mockFactRepo{aggRows: []factmetric.AggRow{{Category: "INCOME", Value: 1}}}
	h := chartdata.NewGetDataHandler(repo, fact, newMockCache(), func(any) string { return "h" }).
		WithNow(func() time.Time { return time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC) })

	_, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "EBITDA",
		Filters:       chartdata.ViewerFilters{PeriodPreset: "L12M"},
		UserRoles:     []string{"INTERN"},
		IsSuperAdmin:  true,
	})
	if err != nil {
		t.Errorf("super admin should bypass role check, got %v", err)
	}
}

func TestGetDataHandler_NotFound(t *testing.T) {
	repo := &mockDashboardRepo{getByCode: func(_ context.Context, _ string) (*dashboarddomain.Dashboard, error) {
		return nil, dashboarddomain.ErrNotFound
	}}
	h := chartdata.NewGetDataHandler(repo, &mockFactRepo{}, newMockCache(), func(any) string { return "h" })
	_, err := h.Handle(context.Background(), chartdata.GetDataQuery{DashboardCode: "NOPE"})
	if !errors.Is(err, dashboarddomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
