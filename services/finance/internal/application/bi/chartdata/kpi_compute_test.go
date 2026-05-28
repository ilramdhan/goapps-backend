package chartdata_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// scriptedFactRepo returns a fixed scalar per QueryAggregate call from a queue.
type scriptedFactRepo struct {
	values  []float64
	idx     int
	calls   int
	queries []factmetric.PlannedQuery
}

func (r *scriptedFactRepo) GetDistincts(context.Context, factmetric.DistinctScope) (factmetric.DistinctValues, error) {
	return factmetric.DistinctValues{}, nil
}
func (r *scriptedFactRepo) QueryAggregate(_ context.Context, q factmetric.PlannedQuery) ([]factmetric.AggRow, error) {
	r.calls++
	r.queries = append(r.queries, q)
	if r.idx >= len(r.values) {
		return []factmetric.AggRow{{Category: "kpi", Value: 0}}, nil
	}
	v := r.values[r.idx]
	r.idx++
	return []factmetric.AggRow{{Category: "kpi", Value: v}}, nil
}
func (r *scriptedFactRepo) Upsert(context.Context, []factmetric.FactMetric) error { return nil }
func (r *scriptedFactRepo) LatestPeriod(context.Context, string, string, string) (time.Time, error) {
	return time.Time{}, nil
}

func dashboardWithKPIs(t *testing.T, kpis []map[string]any) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:           "KPI_DASH",
		Title:          "KPI",
		FilterType:     "MIS",
		FilterGroup1:   "EBITDA",
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      "kpi_card",
		ChartConfigRaw: map[string]any{"value_field": "display_value"},
		KpiConfigRaw:   kpis,
		MaxDrillLevel:  1,
		CacheTTLSec:    60,
		GroupID:        uuid.New(),
		IsActive:       true,
		CreatedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestComputeKPIs_NoCompare(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{"label": "Total", "value_field": "display_value", "agg": "sum", "compare": "none", "format": "currency_thousands"},
	})
	repo := &scriptedFactRepo{values: []float64{500}}
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 KPI, got %d", len(rows))
	}
	if rows[0].Value != 500 {
		t.Errorf("value: want 500, got %v", rows[0].Value)
	}
	if rows[0].CompareValue != 0 || rows[0].ComparePeriodLabel != "" {
		t.Errorf("no-compare KPI must leave compare empty, got %+v", rows[0])
	}
	if repo.calls != 1 {
		t.Errorf("no-compare should make 1 query, got %d", repo.calls)
	}
}

func TestComputeKPIs_MoM_DeltaComputed(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{"label": "Current", "value_field": "display_value", "agg": "sum", "compare": "MoM", "format": "currency_thousands"},
	})
	// First call = current period (120), second = compare period (100).
	repo := &scriptedFactRepo{values: []float64{120, 100}}
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("THIS_MONTH", time.Time{}, time.Time{}, "MONTHLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now)
	if err != nil {
		t.Fatal(err)
	}
	k := rows[0]
	if k.Value != 120 || k.CompareValue != 100 {
		t.Fatalf("value/compare: got %v/%v", k.Value, k.CompareValue)
	}
	if k.DeltaAbs != 20 {
		t.Errorf("deltaAbs: want 20, got %v", k.DeltaAbs)
	}
	if k.DeltaPct != 20 { // (120-100)/100*100
		t.Errorf("deltaPct: want 20, got %v", k.DeltaPct)
	}
	if repo.calls != 2 {
		t.Errorf("MoM should make 2 queries, got %d", repo.calls)
	}
}

func TestComputeKPIs_Sparkline(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{"label": "Trend", "value_field": "display_value", "agg": "sum", "compare": "none",
			"format": "thousands", "show_sparkline": true, "sparkline_periods": float64(3)},
	})
	repo := &scriptedFactRepo{values: []float64{10}} // current; sparkline returns the default single-row stub
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now)
	if err != nil {
		t.Fatal(err)
	}
	// 1 query for current value + 1 for sparkline = 2.
	if repo.calls != 2 {
		t.Errorf("sparkline KPI should make 2 queries, got %d", repo.calls)
	}
	if rows[0].Sparkline == nil {
		t.Error("expected sparkline populated")
	}
}

// firstDateArg returns the first time.Time argument of a captured query (the BETWEEN "from"
// bound), or the zero time if none is present.
func firstDateArg(q factmetric.PlannedQuery) time.Time {
	for _, a := range q.Args {
		if d, ok := a.(time.Time); ok {
			return d
		}
	}
	return time.Time{}
}

// Each KPI scopes its own window: a "current_month" KPI must query a narrower range than a
// "selected" (L12M) KPI even though both run on the same dashboard/period.
func TestComputeKPIs_PerKPIPeriodScoping(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{"label": "Current Month", "value_field": "display_value", "agg": "sum", "compare": "none", "period": "current_month", "format": "currency_thousands"},
		{"label": "YTD", "value_field": "display_value", "agg": "sum", "compare": "none", "period": "ytd", "format": "currency_thousands"},
		{"label": "L12M", "value_field": "display_value", "agg": "sum", "compare": "none", "period": "l12m", "format": "currency_thousands"},
	})
	repo := &scriptedFactRepo{values: []float64{1, 2, 3}}
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)

	if _, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now); err != nil {
		t.Fatal(err)
	}
	if len(repo.queries) != 3 {
		t.Fatalf("want 3 queries, got %d", len(repo.queries))
	}
	curFrom := firstDateArg(repo.queries[0])
	ytdFrom := firstDateArg(repo.queries[1])
	l12mFrom := firstDateArg(repo.queries[2])

	wantCur := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	wantYtd := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantL12m := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	if !curFrom.Equal(wantCur) {
		t.Errorf("current_month from: want %v, got %v", wantCur, curFrom)
	}
	if !ytdFrom.Equal(wantYtd) {
		t.Errorf("ytd from: want %v, got %v", wantYtd, ytdFrom)
	}
	if !l12mFrom.Equal(wantL12m) {
		t.Errorf("l12m from: want %v, got %v", wantL12m, l12mFrom)
	}
}

func TestComputeKPIs_Empty(t *testing.T) {
	d := dashboardWithKPIs(t, nil)
	rows, err := chartdata.ComputeKPIs(context.Background(), &scriptedFactRepo{}, d, chartdata.PeriodRange{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("no KPIs configured → empty result, got %d", len(rows))
	}
}
