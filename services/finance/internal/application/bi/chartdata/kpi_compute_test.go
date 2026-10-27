package chartdata_test

import (
	"context"
	"strings"
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

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil)
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

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil)
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

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil)
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

	if _, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil); err != nil {
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

func TestComputeKPIs_CrossRatio_ComputesRatio(t *testing.T) {
	// Build a dashboard with a cross_ratio KPI.
	d := dashboardWithKPIs(t, []map[string]any{
		{
			"label":               "NP Margin",
			"agg":                 "cross_ratio",
			"numerator_group_1":   "NET PROFIT",
			"denominator_group_1": "EBITDA",
			"scale":               float64(100),
			"format":              "percent",
			"period":              "l12m",
		},
	})
	// First call returns numerator (NET PROFIT sum), second returns denominator (EBITDA sum).
	repo := &scriptedFactRepo{values: []float64{300_000, 1_000_000}}
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 KPI, got %d", len(rows))
	}
	// 300_000 / 1_000_000 * 100 = 30.0
	if rows[0].Value != 30.0 {
		t.Errorf("cross ratio: want 30.0, got %v", rows[0].Value)
	}
	// Should have made exactly 2 QueryAggregate calls (numerator + denominator).
	if repo.calls != 2 {
		t.Errorf("cross ratio should make 2 queries, got %d", repo.calls)
	}
}

func TestComputeKPIs_CrossRatio_MoMCompare(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{
			"label":               "NP Margin",
			"agg":                 "cross_ratio",
			"numerator_group_1":   "NET PROFIT",
			"denominator_group_1": "EBITDA",
			"scale":               float64(100),
			"format":              "percent",
			"period":              "l12m",
			"compare":             "MoM",
		},
	})
	// Current: 300/1000 = 30; compare period: 200/1000 = 20.
	repo := &scriptedFactRepo{values: []float64{300, 1000, 200, 1000}}
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "MONTHLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repo.calls != 4 {
		t.Fatalf("want 4 queries (2 per period), got %d", repo.calls)
	}
	r := rows[0]
	if r.Value != 30 || r.CompareValue != 20 || r.DeltaAbs != 10 || r.DeltaPct != 50 {
		t.Errorf("got value=%v compare=%v abs=%v pct=%v", r.Value, r.CompareValue, r.DeltaAbs, r.DeltaPct)
	}
}

func TestComputeKPIs_CrossRatio_ZeroDenominator_ReturnsZero(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{
			"label":               "Margin",
			"agg":                 "cross_ratio",
			"numerator_group_1":   "A",
			"denominator_group_1": "B",
			"scale":               float64(100),
			"format":              "percent",
		},
	})
	repo := &scriptedFactRepo{values: []float64{500, 0}} // denominator is 0
	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, chartdata.PeriodRange{}, time.Now(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 KPI, got %d", len(rows))
	}
	// No divide-by-zero panic — returns 0.
	if rows[0].Value != 0 {
		t.Errorf("zero denominator: want 0, got %v", rows[0].Value)
	}
}

func TestComputeKPIs_Empty(t *testing.T) {
	d := dashboardWithKPIs(t, nil)
	rows, err := chartdata.ComputeKPIs(context.Background(), &scriptedFactRepo{}, d, chartdata.PeriodRange{}, time.Now(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("no KPIs configured → empty result, got %d", len(rows))
	}
}

// HD-08: numerator/denominator may come from different fact types, and the viewer's dept
// (group_1) filter applies to sides that have no fixed group_1.
func TestComputeKPIs_CrossRatio_CrossTypeWithDeptFilter(t *testing.T) {
	d := dashboardWithKPIs(t, []map[string]any{
		{
			"label":                        "Sick Rate",
			"agg":                          "cross_ratio",
			"cross_ratio_numerator_type":   "HR_SICK_DAYS",
			"cross_ratio_denominator_type": "HR_SCHED_DAYS",
			"scale":                        float64(100),
			"format":                       "percent",
		},
	})
	repo := &scriptedFactRepo{values: []float64{5, 200}}
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	period := chartdata.ResolvePeriod("L12M", time.Time{}, time.Time{}, "WEEKLY", now)

	rows, err := chartdata.ComputeKPIs(context.Background(), repo, d, period, now, []string{"WEAVING", "SPINNING"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Value != 2.5 {
		t.Errorf("want 2.5, got %v", rows[0].Value)
	}
	if len(repo.queries) != 2 {
		t.Fatalf("want 2 queries, got %d", len(repo.queries))
	}
	for i, wantType := range []string{"HR_SICK_DAYS", "HR_SCHED_DAYS"} {
		q := repo.queries[i]
		if q.Args[0] != wantType {
			t.Errorf("query %d type: want %s, got %v", i, wantType, q.Args[0])
		}
		if !strings.Contains(q.SQL, "group_1 IN ($5,$6)") || q.Args[4] != "WEAVING" || q.Args[5] != "SPINNING" {
			t.Errorf("query %d: dept filter not applied: %s %v", i, q.SQL, q.Args)
		}
	}
}

// H013: filter_group_2 pins group_2 and reads mv_bi_metric_g2; this_week/last_week scope a
// WEEKLY KPI to the latest week and the one before it.
func TestComputeKPIs_FilterGroup2AndWeekScopes(t *testing.T) {
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "HR_WEEK", Title: "R2", FilterType: "HR_OT_WEEK_STATUS", PeriodGrain: "WEEKLY",
		DefaultPeriod: "L12M", ChartType: "kpi_card", ChartConfigRaw: map[string]any{"value_field": "value"},
		KpiConfigRaw: []map[string]any{
			{"label": "Over minggu lalu", "value_field": "value", "agg": "sum", "period": "last_week", "filter_group_2": "OVER_LIMIT", "format": "raw"},
			{"label": "Mendekati minggu ini", "value_field": "value", "agg": "sum", "period": "this_week", "filter_group_2": "NEAR_LIMIT", "format": "raw"},
		},
		MaxDrillLevel: 2, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &scriptedFactRepo{values: []float64{3, 5}}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC) // Wednesday
	if _, err = chartdata.ComputeKPIs(context.Background(), repo, d, chartdata.PeriodRange{}, now, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.queries) != 2 {
		t.Fatalf("want 2 queries, got %d", len(repo.queries))
	}
	for i, want := range []string{"OVER_LIMIT", "NEAR_LIMIT"} {
		q := repo.queries[i]
		if q.TargetTable != "mv_bi_metric_g2" || q.Args[1] != want {
			t.Errorf("kpi %d: source %s args %v", i, q.TargetTable, q.Args)
		}
	}
	if got, want := firstDateArg(repo.queries[0]), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("last_week from = %v, want %v", got, want)
	}
	if got, want := firstDateArg(repo.queries[1]), time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("this_week from = %v, want %v", got, want)
	}
}
