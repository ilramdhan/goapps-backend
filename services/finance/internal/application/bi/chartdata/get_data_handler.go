package chartdata

import (
	"context"
	"fmt"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// Cache is the minimal interface this handler needs.
type Cache interface {
	Get(ctx context.Context, dashboardCode, filtersHash string, out any) (bool, error)
	Set(ctx context.Context, dashboardCode, filtersHash string, value any, ttl time.Duration) error
}

// HashFilters is injected by the wiring layer (typically redisinfra.HashFilters).
type HashFilters func(any) string

// GetDataQuery is the payload for GetDataHandler.
type GetDataQuery struct {
	DashboardCode string
	Filters       ViewerFilters
	UserRoles     []string
	IsSuperAdmin  bool
	// CanViewEmployee = caller holds PermWorkforceEmployee (HD-07); gates group_3 of HR_* types.
	CanViewEmployee bool
	// CanViewWorkforce = caller holds PermWorkforceView; gates every HR_* dashboard (HD-10).
	CanViewWorkforce bool
}

// GetDataHandler is the viewer-side orchestrator: loads dashboard, applies access check,
// reads cache, executes query, computes KPIs, formats result, writes cache, returns.
type GetDataHandler struct {
	repo  dashboarddomain.Repository
	fact  factmetric.Repository
	cache Cache
	hash  HashFilters
	now   func() time.Time // injectable for tests
}

// NewGetDataHandler constructs a GetDataHandler.
// cache and hash may be nil for tests (cache becomes pass-through).
func NewGetDataHandler(
	repo dashboarddomain.Repository,
	fact factmetric.Repository,
	cache Cache,
	hash HashFilters,
) *GetDataHandler {
	return &GetDataHandler{
		repo:  repo,
		fact:  fact,
		cache: cache,
		hash:  hash,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithNow overrides the clock source. Used by tests for deterministic period resolution.
func (h *GetDataHandler) WithNow(now func() time.Time) *GetDataHandler {
	if now != nil {
		h.now = now
	}
	return h
}

// resolveAsOf returns the "data as of" anchor for the dashboard. Warehouse data lags wall-clock
// (the current calendar month is usually not loaded yet), so period presets and KPI windows are
// anchored to the latest loaded period for this dashboard's scope; falls back to now when empty.
//
// WEEKLY (HR R2, HD-05) is the exception: "this week / last week" mean the calendar week, and a
// week with nobody over/near the limit has no fact rows, so the anchor is today's WIB date (H013).
func (h *GetDataHandler) resolveAsOf(ctx context.Context, d *dashboarddomain.Dashboard) (time.Time, error) {
	if d.PeriodGrain().String() == grainWeekly {
		return wibDate(h.now()), nil
	}
	latest, err := h.fact.LatestPeriod(ctx, d.FilterType(), d.FilterGroup1(), d.PeriodGrain().String())
	if err != nil {
		return time.Time{}, fmt.Errorf("latest period: %w", err)
	}
	if latest.IsZero() {
		return h.now(), nil
	}
	return latest, nil
}

// Handle executes the full viewer pipeline.
func (h *GetDataHandler) Handle(ctx context.Context, q GetDataQuery) (*Result, error) {
	// 1. Load dashboard
	d, err := h.repo.GetByCode(ctx, q.DashboardCode)
	if err != nil {
		return nil, err
	}
	// 2. Access checks (role whitelist, HD-10 workforce, HD-07 employee level)
	if err := authorizeView(d, q); err != nil {
		return nil, err
	}

	// 3. Try cache
	var hashKey string
	if h.cache != nil && h.hash != nil {
		hashKey = h.hash(q.Filters)
		var cached Result
		hit, err := h.cache.Get(ctx, q.DashboardCode, hashKey, &cached)
		if err != nil {
			_ = err //nolint:errcheck // best-effort cache read; miss is treated as uncached
		}
		if hit {
			cached.Meta.CacheHit = true
			cached.Meta.QueryHash = hashKey
			return &cached, nil
		}
	}

	// 4. Resolve the "as of" anchor (latest loaded period; falls back to now).
	asOf, err := h.resolveAsOf(ctx, d)
	if err != nil {
		return nil, err
	}

	// 5. Plan + execute aggregate query
	plan, err := Plan(d, q.Filters, asOf)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	rows, err := h.fact.QueryAggregate(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("execute aggregate: %w", err)
	}

	// 6. Compute KPIs
	period := ResolvePeriodCal(q.Filters.PeriodPreset, q.Filters.PeriodFrom, q.Filters.PeriodTo,
		d.PeriodGrain().String(), d.PeriodCalendar().String(), asOf)
	kpis, err := ComputeKPIs(ctx, h.fact, d, period, asOf, q.Filters.Group1Filter, q.Filters.Group2Filter)
	if err != nil {
		return nil, fmt.Errorf("compute kpis: %w", err)
	}

	// 7. Shape + format
	drillCtx := BuildDrillContext(d, q.Filters)
	result := Shape(d, rows, kpis, q.Filters, drillCtx)
	addEmployeeOTRanges(&result, d, q.Filters, asOf)
	result.Meta.QueryHash = hashKey

	// 8. Cache (best-effort)
	ttl := time.Duration(d.CacheTTL().Seconds()) * time.Second
	if h.cache != nil && h.hash != nil && ttl > 0 {
		if err := h.cache.Set(ctx, q.DashboardCode, hashKey, result, ttl); err != nil {
			_ = err //nolint:errcheck // best-effort cache write; failure is non-fatal
		}
	}

	return &result, nil
}

// authorizeView applies the dashboard access checks of Handle. The employee-level check runs
// before the cache so a result cached for a permitted user is never served to one without it.
func authorizeView(d *dashboarddomain.Dashboard, q GetDataQuery) error {
	if !d.IsAccessibleBy(q.UserRoles, q.IsSuperAdmin) {
		return dashboarddomain.ErrForbidden
	}
	// HR_* dashboards need workforce.view on top of the role whitelist (HD-10).
	if !q.IsSuperAdmin && !q.CanViewWorkforce && TouchesHRType(d.FilterType(), d.KpiConfig()) {
		return fmt.Errorf("%w: workforce data requires %s", dashboarddomain.ErrForbidden, PermWorkforceView)
	}
	// Employee-level (group_3) of HR_* needs its own permission (HD-07).
	if !q.IsSuperAdmin && !q.CanViewEmployee && RequestsEmployeeLevel(d, q.Filters) {
		return fmt.Errorf("%w: employee-level data requires %s", dashboarddomain.ErrForbidden, PermWorkforceEmployee)
	}
	return nil
}
