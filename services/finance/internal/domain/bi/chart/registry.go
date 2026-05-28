// Package chart contains the BI chart-type registry — the single source of truth for
// what fields each chart type requires, which rendering library handles it, and what
// default options to apply when an admin doesn't set them.
//
// The frontend mirrors this exact structure in src/lib/bi/chart-registry.ts; both must
// stay in sync. Registry membership is intentionally tiny (one entry per chart type) so
// adding a new chart type is a one-line config change plus one new component file.
package chart

// Lib identifies which frontend rendering library owns a chart type.
type Lib string

const (
	// LibShadcn means the chart is rendered with shadcn/ui chart wrappers (Recharts).
	LibShadcn Lib = "shadcn"
	// LibECharts means the chart is rendered with Apache ECharts via echarts-for-react.
	LibECharts Lib = "echarts"
)

// Type is the canonical chart-type string used in bi_dashboard.chart_type.
type Type string

// All supported chart types. Values match the proto ChartType enum lowercase suffix.
const (
	TypeBar           Type = "bar"
	TypeHorizontalBar Type = "horizontal_bar"
	TypeStackedBar    Type = "stacked_bar"
	TypeLine          Type = "line"
	TypeArea          Type = "area"
	TypeWaterfall     Type = "waterfall"
	TypeDonut         Type = "donut"
	TypeKPICard       Type = "kpi_card"
	TypeTreemap       Type = "treemap"
	TypeHeatmap       Type = "heatmap"
	TypeScatter       Type = "scatter"
	TypeMixed         Type = "mixed"
	TypeDataTable     Type = "data_table"
)

// Registration describes one chart type's contract.
type Registration struct {
	// Type is the canonical chart-type string.
	Type Type
	// Lib is the frontend rendering library responsible for this chart.
	Lib Lib
	// RequiredFields must be present in chart_config for a Dashboard using this type.
	RequiredFields []string
	// OptionalFields are recognized but not mandatory.
	OptionalFields []string
	// SupportsDrill is true when clicking a data point can drill down a level.
	SupportsDrill bool
	// SupportsCompare is true when compare-mode (MoM/YoY/...) overlays make sense.
	SupportsCompare bool
	// DefaultConfig provides values used when the admin form omits them.
	DefaultConfig map[string]any
}

// DefaultRegistry returns the immutable in-memory chart registry.
//
// A fresh map is returned per call so callers can safely mutate it for tests.
func DefaultRegistry() map[Type]Registration {
	return map[Type]Registration{
		TypeBar: {
			Type:            TypeBar,
			Lib:             LibShadcn,
			RequiredFields:  []string{"x_axis_field", "y_axis_field"},
			OptionalFields:  []string{"color_field", "sort_order"},
			SupportsDrill:   true,
			SupportsCompare: true,
			DefaultConfig: map[string]any{
				"number_format":    "thousands",
				"decimals":         1,
				"show_data_labels": false,
				"legend_position":  "bottom",
			},
		},
		TypeHorizontalBar: {
			Type:            TypeHorizontalBar,
			Lib:             LibShadcn,
			RequiredFields:  []string{"x_axis_field", "y_axis_field"},
			OptionalFields:  []string{"sort_order"},
			SupportsDrill:   true,
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"number_format": "thousands", "decimals": 1},
		},
		TypeStackedBar: {
			Type:            TypeStackedBar,
			Lib:             LibECharts,
			RequiredFields:  []string{"x_axis_field", "y_axis_field", "stack_by_field"},
			SupportsDrill:   true,
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"number_format": "thousands", "legend_position": "bottom"},
		},
		TypeLine: {
			Type:            TypeLine,
			Lib:             LibShadcn,
			RequiredFields:  []string{"x_axis_field", "y_axis_field"},
			OptionalFields:  []string{"smooth"},
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"smooth": true, "number_format": "thousands"},
		},
		TypeArea: {
			Type:            TypeArea,
			Lib:             LibShadcn,
			RequiredFields:  []string{"x_axis_field", "y_axis_field"},
			OptionalFields:  []string{"smooth", "opacity"},
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"smooth": true, "opacity": 0.3, "number_format": "thousands"},
		},
		TypeWaterfall: {
			Type:           TypeWaterfall,
			Lib:            LibECharts,
			RequiredFields: []string{"x_axis_field", "y_axis_field"},
			OptionalFields: []string{"positive_color", "negative_color", "total_color", "show_total_bar"},
			SupportsDrill:  true,
			DefaultConfig: map[string]any{
				"show_total_bar":  true,
				"positive_color":  "#1d9e75",
				"negative_color":  "#a32d2d",
				"total_color":     "#534AB7",
				"number_format":   "currency_thousands",
				"decimals":        1,
				"legend_position": "none",
			},
		},
		TypeDonut: {
			Type:           TypeDonut,
			Lib:            LibShadcn,
			RequiredFields: []string{"label_field", "value_field"},
			OptionalFields: []string{"inner_radius", "label_position"},
			SupportsDrill:  true,
			DefaultConfig:  map[string]any{"inner_radius": 0.4, "legend_position": "right"},
		},
		TypeKPICard: {
			Type:            TypeKPICard,
			Lib:             LibShadcn,
			RequiredFields:  []string{"value_field"},
			OptionalFields:  []string{"compare", "sparkline"},
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"number_format": "currency_thousands"},
		},
		TypeTreemap: {
			Type:           TypeTreemap,
			Lib:            LibECharts,
			RequiredFields: []string{"label_field", "value_field", "parent_field"},
			SupportsDrill:  true,
			DefaultConfig:  map[string]any{"number_format": "thousands"},
		},
		TypeHeatmap: {
			Type:           TypeHeatmap,
			Lib:            LibECharts,
			RequiredFields: []string{"x_axis_field", "y_axis_field", "value_field"},
			OptionalFields: []string{"color_scale"},
			DefaultConfig:  map[string]any{"color_scale": "viridis"},
		},
		TypeScatter: {
			Type:           TypeScatter,
			Lib:            LibECharts,
			RequiredFields: []string{"x_axis_field", "y_axis_field"},
			OptionalFields: []string{"size_field", "color_field"},
			DefaultConfig:  map[string]any{},
		},
		TypeMixed: {
			Type:            TypeMixed,
			Lib:             LibECharts,
			RequiredFields:  []string{"x_axis_field", "y_axis_field", "series_defs"},
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"number_format": "thousands", "legend_position": "bottom"},
		},
		TypeDataTable: {
			Type:            TypeDataTable,
			Lib:             LibShadcn,
			OptionalFields:  []string{"columns", "sort", "paginate"},
			SupportsCompare: true,
			DefaultConfig:   map[string]any{"paginate": true},
		},
	}
}

// Lookup returns the registration for a chart type, or false when the type is unknown.
func Lookup(t Type) (Registration, bool) {
	r, ok := DefaultRegistry()[t]
	return r, ok
}

// IsValid reports whether the given chart-type string maps to a known registration.
func IsValid(t string) bool {
	_, ok := Lookup(Type(t))
	return ok
}
