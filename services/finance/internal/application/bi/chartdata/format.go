package chartdata

import (
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// Result is the shaped payload returned to the gRPC delivery layer.
//
// All numeric values are accompanied by a pre-formatted string label using the
// dashboard's chart_config.number_format so the frontend renders without re-parsing.
type Result struct {
	Config       map[string]any
	Series       []Series
	Categories   []string
	KPIs         []KpiResult
	DrillContext DrillContext
	Meta         Meta
}

// Series is one series in a chart payload.
type Series struct {
	Name    string
	LibHint string // 'positive' | 'negative' | 'total' for waterfall
	Points  []DataPoint
}

// DataPoint is one data point in a series.
type DataPoint struct {
	Category string
	Value    float64
	Label    string
	Meta     map[string]any
}

// KpiResult is the formatted KPI card payload.
type KpiResult struct {
	Label              string
	Value              float64
	ValueFormatted     string
	CompareValue       float64
	DeltaAbs           float64
	DeltaPct           float64
	ComparePeriodLabel string
	Improving          bool
	Sparkline          []float64
}

// DrillContext tells the frontend whether the current view can drill deeper and what the next field is.
type DrillContext struct {
	CurrentPath []string
	NextField   string
	NextValues  []string
	CanDrill    bool
}

// Meta holds response-level diagnostics (cache hit, data freshness, etc.).
type Meta struct {
	AsOf      time.Time
	RowCount  int
	CacheHit  bool
	QueryHash string
}

// Shape turns raw aggregate rows + KPI results into the canonical viewer payload.
//
// The chart's primary series is named after the dashboard title. For compare modes,
// a second series ("MoM Previous", "YoY Previous", etc.) is added if AggRow.PrevValue
// values are non-zero.
//
// For categorical charts (waterfall/bar/donut etc.) categories come from AggRow.Category;
// for trend charts (line/area/mixed) they come from period labels.
func Shape(d *dashboarddomain.Dashboard, rows []factmetric.AggRow, kpis []factmetric.KpiRow, f ViewerFilters, drillContext DrillContext) Result {
	primary := Series{
		Name:   d.Title(),
		Points: make([]DataPoint, 0, len(rows)),
	}
	var comparePts []DataPoint
	categories := make([]string, 0, len(rows))
	numFmt := pickNumberFormat(d.ChartConfig())
	decimals := d.ChartConfig().Decimals

	for _, r := range rows {
		cat := r.Category
		if r.PeriodLabel != "" {
			cat = r.PeriodLabel
		}
		categories = append(categories, cat)
		primary.Points = append(primary.Points, DataPoint{
			Category: cat,
			Value:    r.Value,
			Label:    chart.Format(r.Value, numFmt, decimals),
		})
		if r.PrevValue != 0 {
			comparePts = append(comparePts, DataPoint{
				Category: cat,
				Value:    r.PrevValue,
				Label:    chart.Format(r.PrevValue, numFmt, decimals),
			})
		}
	}

	series := []Series{primary}
	if len(comparePts) > 0 && f.Compare != "" && f.Compare != "none" {
		series = append(series, Series{
			Name:   f.Compare + " Previous",
			Points: comparePts,
		})
	}

	// Format KPIs
	formattedKPIs := make([]KpiResult, 0, len(kpis))
	kpiCfg := d.KpiConfig()
	for i, k := range kpis {
		fmtKey := numFmt
		dec := decimals
		if i < len(kpiCfg) {
			if kpiCfg[i].Format != "" {
				fmtKey = kpiCfg[i].Format
			}
			if kpiCfg[i].Decimals != 0 {
				dec = kpiCfg[i].Decimals
			}
		}
		formattedKPIs = append(formattedKPIs, KpiResult{
			Label:              k.Label,
			Value:              k.Value,
			ValueFormatted:     chart.Format(k.Value, fmtKey, dec),
			CompareValue:       k.CompareValue,
			DeltaAbs:           k.DeltaAbs,
			DeltaPct:           k.DeltaPct,
			ComparePeriodLabel: k.ComparePeriodLabel,
			Improving:          isImproving(d, k.DeltaAbs),
			Sparkline:          k.Sparkline,
		})
	}

	return Result{
		Config:       d.ChartConfig().MarshalToMap(),
		Series:       series,
		Categories:   categories,
		KPIs:         formattedKPIs,
		DrillContext: drillContext,
		Meta:         Meta{AsOf: time.Now().UTC(), RowCount: len(rows)},
	}
}

// pickNumberFormat returns the chart_config's number_format or a sensible default.
func pickNumberFormat(c dashboarddomain.ChartConfig) chart.NumberFormat {
	if c.NumberFormat != "" {
		return c.NumberFormat
	}
	return chart.NumberFormatThousands
}

// isImproving determines whether a positive delta means "good" for the given dashboard.
//
// MIS/EBITDA/NET PROFIT: up = good. Cost-like metrics would be down = good but we cannot
// infer that purely from the dashboard struct; the KPI definition's value_field could
// disambiguate, but for MVP we treat "value increased" as improving.
func isImproving(_ *dashboarddomain.Dashboard, deltaAbs float64) bool {
	return deltaAbs >= 0
}

// BuildDrillContext computes the DrillContext block from current filters + dashboard depth.
//
//nolint:nestif // depth/group branching is naturally nested; extraction would obscure the drill-path logic
func BuildDrillContext(d *dashboarddomain.Dashboard, f ViewerFilters) DrillContext {
	current := append([]string{}, f.DrillPath...)
	depth := len(current)
	// The fact hierarchy is fixed at 3 group levels (group_1→group_2→group_3). The number of
	// drillable transitions is therefore 2 — or 1 when the dashboard pre-filters group_1, since
	// that level is consumed by the filter and the viewer only drills group_2→group_3. Cap the
	// configured max_drill_level by this hard limit so we never offer a drill past group_3.
	hierarchyMax := 2
	if d.FilterGroup1() != "" {
		hierarchyMax = 1
	}
	maxDepth := min(d.MaxDrillLevel().Int(), hierarchyMax)
	canDrill := d.DrillEnabled() && depth < maxDepth

	next := ""
	if canDrill {
		// Field name reflects what the next click will drill INTO.
		switch depth {
		case 0:
			if d.FilterGroup1() != "" {
				next = "group_3"
			} else {
				next = "group_2"
			}
		case 1:
			if d.FilterGroup1() != "" {
				next = "" // already at group_3
			} else {
				next = "group_3"
			}
		}
	}
	return DrillContext{
		CurrentPath: current,
		NextField:   next,
		NextValues:  nil, // populated server-side only when needed; not in MVP
		CanDrill:    canDrill,
	}
}
