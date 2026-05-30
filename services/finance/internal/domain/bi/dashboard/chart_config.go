package dashboard

import (
	"fmt"
	"maps"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

// ChartConfig is the typed representation of bi_dashboard.chart_config JSONB.
//
// The admin form always builds this via the field-mapping wizard; users never type JSON.
// Unknown keys in raw input are silently dropped (forward-compat with frontend additions).
type ChartConfig struct {
	// Field mapping
	XAxisField   string
	YAxisField   string
	LabelField   string
	ValueField   string
	StackByField string
	ParentField  string
	ColorField   string
	SeriesDefs   []SeriesDef

	// Style
	PrimaryColor   string
	PositiveColor  string
	NegativeColor  string
	TotalColor     string
	NumberFormat   chart.NumberFormat
	Decimals       int
	ShowDataLabels bool
	ShowTotalBar   bool
	AxisLabelX     string
	AxisLabelY     string
	LegendPosition string
	GridLines      string
	TooltipFormat  string
	EmptyMessage   string

	// Drill
	DrillTo string

	// Misc
	SortOrder     string
	InnerRadius   float64
	Opacity       float64
	Smooth        bool
	ColorScale    string
	SizeField     string
	LabelPosition string

	// AvailableChartTypes lists additional chart types the viewer may switch to.
	// Stored in chart_config.available_chart_types JSONB. Managed by the admin wizard.
	AvailableChartTypes []string

	// ViewConfigs holds per-view-type display settings keyed by chart type string.
	// Stored in chart_config.view_configs JSONB. Falls back to sensible defaults via Dashboard.ViewConfigFor.
	ViewConfigs map[string]ViewModeConfig

	// FilterChipsGroup1 lists the static group_1 values shown as filter chips in the viewer
	// (e.g. ["Export","Local"] for DELIVERY_MARGIN). Stored as filter_chips_group1 in JSONB.
	FilterChipsGroup1 []string

	// FilterChipsGroup2 lists the static group_2 values shown as filter chips in the viewer
	// (e.g. ["ACY","FG"] for DELIVERY_MARGIN). Stored as filter_chips_group2 in JSONB.
	FilterChipsGroup2 []string

	// MetricFilter holds the optional list of metric_names for multi-series dashboards
	// (e.g. SALES dashboards with GROSS_SALES / NETT_SALES / MARGIN on the same chart).
	// When non-empty the query planner bypasses MVs and queries bi_fact_metric directly.
	MetricFilter MetricFilterConfig

	// ComputedRatio, when non-nil, instructs the query planner to use planComputedRatio
	// instead of the standard MV/multi-metric path. Used for secondary charts such as
	// "Margin %" that derive a ratio from two existing metric columns.
	ComputedRatio *ComputedRatioConfig
}

// MetricFilterConfig carries the include_metrics list from chart_config.metric_filter.
type MetricFilterConfig struct {
	IncludeMetrics []string
}

// ComputedRatioConfig describes a ratio computation: SUM(numerator)/NULLIF(SUM(denominator),0)*scale.
// When set in chart_config.computed_ratio, the query planner switches to planComputedRatio.
type ComputedRatioConfig struct {
	// Numerator is the metric_name for the dividend (e.g. "MARGIN").
	Numerator string `json:"numerator"`
	// Denominator is the metric_name for the divisor (e.g. "NETT_SALES").
	Denominator string `json:"denominator"`
	// Scale multiplies the ratio result (100 for percent, 1 for raw fraction).
	Scale float64 `json:"scale"`
	// GroupBy is the column to group results by ("group_1" or "group_2").
	GroupBy string `json:"group_by"`
}

// SeriesDef defines one series in a mixed chart.
type SeriesDef struct {
	Name  string
	Type  string // 'bar' | 'line' | 'area'
	Field string
}

// ViewModeConfig holds per-view-type display settings for a dashboard chart.
// Stored in chart_config.view_configs JSONB, keyed by chart type string.
type ViewModeConfig struct {
	// TitleTemplate is shown in the chart card header. Use {period} as placeholder for YYYYMM period label.
	TitleTemplate string
	// DrillEnabled controls whether clicking a bar/segment triggers drill-down navigation.
	// Should be false for time-series charts (line, area) where x-axis is time, not a drillable category.
	DrillEnabled bool
	// Hint is the sub-title hint text shown below the chart card title.
	Hint string
}

// ParseChartConfig validates the raw config map against the chart registry's required-field
// list, applies defaults, and returns a typed ChartConfig.
//
// Returns ErrInvalidChartConfig (wrapping a field-level message) on validation failure.
//
//nolint:gocognit // cohesive field-mapping function; splitting by field group would obscure the schema
func ParseChartConfig(t ChartType, raw map[string]any) (ChartConfig, error) {
	reg, ok := chart.Lookup(t.Type())
	if !ok {
		return ChartConfig{}, fmt.Errorf("%w: chart type %q is not in registry", ErrInvalidChartConfig, t)
	}

	if raw == nil {
		raw = map[string]any{}
	}

	// Merge defaults under user-supplied values
	merged := make(map[string]any, len(reg.DefaultConfig)+len(raw))
	maps.Copy(merged, reg.DefaultConfig)
	maps.Copy(merged, raw)

	// Validate required fields
	for _, req := range reg.RequiredFields {
		if err := validateRequiredField(merged, req, t); err != nil {
			return ChartConfig{}, err
		}
	}

	cfg := ChartConfig{}
	applyString(merged, "x_axis_field", &cfg.XAxisField)
	applyString(merged, "y_axis_field", &cfg.YAxisField)
	applyString(merged, "label_field", &cfg.LabelField)
	applyString(merged, "value_field", &cfg.ValueField)
	applyString(merged, "stack_by_field", &cfg.StackByField)
	applyString(merged, "parent_field", &cfg.ParentField)
	applyString(merged, "color_field", &cfg.ColorField)
	applyString(merged, "primary_color", &cfg.PrimaryColor)
	applyString(merged, "positive_color", &cfg.PositiveColor)
	applyString(merged, "negative_color", &cfg.NegativeColor)
	applyString(merged, "total_color", &cfg.TotalColor)

	if v, ok := merged["number_format"].(string); ok && v != "" {
		if !chart.IsValidNumberFormat(v) {
			return ChartConfig{}, fmt.Errorf("%w: number_format %q is not recognized", ErrInvalidChartConfig, v)
		}
		cfg.NumberFormat = chart.NumberFormat(v)
	}

	if v, ok := mergedAsInt(merged, "decimals"); ok {
		cfg.Decimals = v
	}
	applyBool(merged, "show_data_labels", &cfg.ShowDataLabels)
	applyBool(merged, "show_total_bar", &cfg.ShowTotalBar)
	applyString(merged, "axis_label_x", &cfg.AxisLabelX)
	applyString(merged, "axis_label_y", &cfg.AxisLabelY)
	applyString(merged, "legend_position", &cfg.LegendPosition)
	applyString(merged, "grid_lines", &cfg.GridLines)
	applyString(merged, "tooltip_format", &cfg.TooltipFormat)
	applyString(merged, "empty_message", &cfg.EmptyMessage)
	applyString(merged, "drill_to", &cfg.DrillTo)
	applyString(merged, "sort_order", &cfg.SortOrder)

	if v, ok := mergedAsFloat(merged, "inner_radius"); ok {
		cfg.InnerRadius = v
	}
	if v, ok := mergedAsFloat(merged, "opacity"); ok {
		cfg.Opacity = v
	}
	applyBool(merged, "smooth", &cfg.Smooth)
	applyString(merged, "color_scale", &cfg.ColorScale)
	applyString(merged, "size_field", &cfg.SizeField)
	applyString(merged, "label_position", &cfg.LabelPosition)

	// series_defs (mixed-chart sub-config)
	if v, ok := merged["series_defs"]; ok {
		defs, err := parseSeriesDefs(v)
		if err != nil {
			return ChartConfig{}, fmt.Errorf("%w: %w", ErrInvalidChartConfig, err)
		}
		cfg.SeriesDefs = defs
	}

	// filter_chips_group1/2 — optional; static chip values for viewer filter chips.
	cfg.FilterChipsGroup1 = parseStringSlice(merged, "filter_chips_group1")
	cfg.FilterChipsGroup2 = parseStringSlice(merged, "filter_chips_group2")

	// metric_filter.include_metrics — optional; drives the multi-metric query planner path.
	cfg.MetricFilter = parseMetricFilter(merged)

	// computed_ratio — optional; drives planComputedRatio for secondary computed charts.
	cfg.ComputedRatio = parseComputedRatio(merged)

	// available_chart_types — optional; list of chart type strings the viewer may switch to.
	cfg.AvailableChartTypes = parseStringSlice(merged, "available_chart_types")

	// view_configs — optional; per-view-type display overrides keyed by chart type string.
	cfg.ViewConfigs = parseViewConfigs(merged)

	return cfg, nil
}

// parseMetricFilter extracts MetricFilterConfig from a raw chart-config map.
// Returns a zero MetricFilterConfig when the key is absent or malformed.
func parseMetricFilter(merged map[string]any) MetricFilterConfig {
	mf, ok := merged["metric_filter"].(map[string]any)
	if !ok {
		return MetricFilterConfig{}
	}
	raw, ok := mf["include_metrics"].([]any)
	if !ok {
		return MetricFilterConfig{}
	}
	metrics := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			metrics = append(metrics, s)
		}
	}
	return MetricFilterConfig{IncludeMetrics: metrics}
}

// parseComputedRatio extracts a ComputedRatioConfig from a raw chart-config map.
// Returns nil when the key is absent or numerator is empty.
// Denominator may be empty — in that case planComputedRatio emits SUM(numerator) per group
// instead of a ratio. This supports single-metric group aggregation (e.g. Net Sales by Type).
func parseComputedRatio(merged map[string]any) *ComputedRatioConfig {
	cr, ok := merged["computed_ratio"].(map[string]any)
	if !ok {
		return nil
	}
	num := mapStringVal(cr, "numerator")
	if num == "" {
		return nil
	}
	den := mapStringVal(cr, "denominator")
	scale := 1.0
	if s, ok2 := mergedAsFloat(cr, "scale"); ok2 && s != 0 {
		scale = s
	}
	groupBy := mapStringVal(cr, "group_by")
	if groupBy == "" {
		groupBy = "group_2"
	}
	return &ComputedRatioConfig{
		Numerator:   num,
		Denominator: den,
		Scale:       scale,
		GroupBy:     groupBy,
	}
}

// MarshalToMap converts a ChartConfig back to a map for JSONB storage. Empty/default
// values are omitted so the persisted JSON stays compact.
//
//nolint:gocyclo // one branch per optional field; splitting by field group would obscure the schema
func (c ChartConfig) MarshalToMap() map[string]any {
	out := map[string]any{}
	putString(out, "x_axis_field", c.XAxisField)
	putString(out, "y_axis_field", c.YAxisField)
	putString(out, "label_field", c.LabelField)
	putString(out, "value_field", c.ValueField)
	putString(out, "stack_by_field", c.StackByField)
	putString(out, "parent_field", c.ParentField)
	putString(out, "color_field", c.ColorField)
	putString(out, "primary_color", c.PrimaryColor)
	putString(out, "positive_color", c.PositiveColor)
	putString(out, "negative_color", c.NegativeColor)
	putString(out, "total_color", c.TotalColor)
	if c.NumberFormat != "" {
		out["number_format"] = string(c.NumberFormat)
	}
	if c.Decimals != 0 {
		out["decimals"] = c.Decimals
	}
	if c.ShowDataLabels {
		out["show_data_labels"] = true
	}
	if c.ShowTotalBar {
		out["show_total_bar"] = true
	}
	putString(out, "axis_label_x", c.AxisLabelX)
	putString(out, "axis_label_y", c.AxisLabelY)
	putString(out, "legend_position", c.LegendPosition)
	putString(out, "grid_lines", c.GridLines)
	putString(out, "tooltip_format", c.TooltipFormat)
	putString(out, "empty_message", c.EmptyMessage)
	putString(out, "drill_to", c.DrillTo)
	putString(out, "sort_order", c.SortOrder)
	if c.InnerRadius != 0 {
		out["inner_radius"] = c.InnerRadius
	}
	if c.Opacity != 0 {
		out["opacity"] = c.Opacity
	}
	if c.Smooth {
		out["smooth"] = true
	}
	putString(out, "color_scale", c.ColorScale)
	putString(out, "size_field", c.SizeField)
	putString(out, "label_position", c.LabelPosition)
	if len(c.SeriesDefs) > 0 {
		defs := make([]map[string]any, len(c.SeriesDefs))
		for i, sd := range c.SeriesDefs {
			defs[i] = map[string]any{"name": sd.Name, "type": sd.Type, "field": sd.Field}
		}
		out["series_defs"] = defs
	}
	if len(c.FilterChipsGroup1) > 0 {
		raw := make([]any, len(c.FilterChipsGroup1))
		for i, v := range c.FilterChipsGroup1 {
			raw[i] = v
		}
		out["filter_chips_group1"] = raw
	}
	if len(c.FilterChipsGroup2) > 0 {
		raw := make([]any, len(c.FilterChipsGroup2))
		for i, v := range c.FilterChipsGroup2 {
			raw[i] = v
		}
		out["filter_chips_group2"] = raw
	}
	if len(c.MetricFilter.IncludeMetrics) > 0 {
		raw := make([]any, len(c.MetricFilter.IncludeMetrics))
		for i, m := range c.MetricFilter.IncludeMetrics {
			raw[i] = m
		}
		out["metric_filter"] = map[string]any{"include_metrics": raw}
	}
	if c.ComputedRatio != nil {
		out["computed_ratio"] = map[string]any{
			"numerator":   c.ComputedRatio.Numerator,
			"denominator": c.ComputedRatio.Denominator,
			"scale":       c.ComputedRatio.Scale,
			"group_by":    c.ComputedRatio.GroupBy,
		}
	}
	if len(c.AvailableChartTypes) > 0 {
		raw := make([]any, len(c.AvailableChartTypes))
		for i, t := range c.AvailableChartTypes {
			raw[i] = t
		}
		out["available_chart_types"] = raw
	}
	if len(c.ViewConfigs) > 0 {
		vcMap := make(map[string]any, len(c.ViewConfigs))
		for k, v := range c.ViewConfigs {
			vcMap[k] = map[string]any{
				"title_template": v.TitleTemplate,
				"drill_enabled":  v.DrillEnabled,
				"hint":           v.Hint,
			}
		}
		out["view_configs"] = vcMap
	}
	return out
}

// parseViewConfigs extracts a map of ViewModeConfig from a raw chart-config map.
// Returns nil when the key is absent or malformed.
func parseViewConfigs(merged map[string]any) map[string]ViewModeConfig {
	vcRaw, ok := merged["view_configs"].(map[string]any)
	if !ok {
		return nil
	}
	viewCfgs := make(map[string]ViewModeConfig, len(vcRaw))
	for chartType, vcEntry := range vcRaw {
		m, ok2 := vcEntry.(map[string]any)
		if !ok2 {
			continue
		}
		viewCfgs[chartType] = ViewModeConfig{
			TitleTemplate: mapStringVal(m, "title_template"),
			DrillEnabled:  mapBoolVal(m, "drill_enabled"),
			Hint:          mapStringVal(m, "hint"),
		}
	}
	return viewCfgs
}

// parseSeriesDefs handles both []any and []map[string]any shapes (JSON decoded vs Go-native).
func parseSeriesDefs(v any) ([]SeriesDef, error) {
	switch t := v.(type) {
	case []any:
		out := make([]SeriesDef, 0, len(t))
		for i, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("series_defs[%d] is not an object", i)
			}
			sd, err := seriesDefFromMap(m, i)
			if err != nil {
				return nil, err
			}
			out = append(out, sd)
		}
		return out, nil
	case []map[string]any:
		out := make([]SeriesDef, 0, len(t))
		for i, m := range t {
			sd, err := seriesDefFromMap(m, i)
			if err != nil {
				return nil, err
			}
			out = append(out, sd)
		}
		return out, nil
	}
	return nil, fmt.Errorf("series_defs must be a list of objects")
}

func seriesDefFromMap(m map[string]any, idx int) (SeriesDef, error) {
	name := mapStringVal(m, "name")
	typ := mapStringVal(m, "type")
	field := mapStringVal(m, "field")
	if name == "" || typ == "" || field == "" {
		return SeriesDef{}, fmt.Errorf("series_defs[%d] missing required name/type/field", idx)
	}
	return SeriesDef{Name: name, Type: typ, Field: field}, nil
}

// mapStringVal extracts a string value from a map[string]any, returning "" if absent or wrong type.
func mapStringVal(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

// parseStringSlice extracts a []string from a map[string]any under the given key.
// Accepts both []any (JSON-decoded) and []string. Returns nil when absent or malformed.
func parseStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok2 := item.(string); ok2 && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// validateRequiredField checks that a required key exists in merged and has a valid value.
func validateRequiredField(merged map[string]any, req string, t ChartType) error {
	v, present := merged[req]
	if !present {
		return fmt.Errorf("%w: %q is required for chart type %s", ErrInvalidChartConfig, req, t)
	}
	s, ok := v.(string)
	if ok && s != "" {
		return nil
	}
	// series_defs is the one required field that's a list, not a string
	if req == "series_defs" {
		if _, listOK := v.([]any); listOK {
			return nil
		}
		if _, listOK := v.([]map[string]any); listOK {
			return nil
		}
	}
	return fmt.Errorf("%w: %q must be a non-empty string for chart type %s", ErrInvalidChartConfig, req, t)
}

// applyString sets *dst when the map key is a non-empty string.
func applyString(m map[string]any, key string, dst *string) {
	if v, ok := m[key].(string); ok && v != "" {
		*dst = v
	}
}

// applyBool sets *dst when the map key is a bool.
func applyBool(m map[string]any, key string, dst *bool) {
	if v, ok := m[key].(bool); ok {
		*dst = v
	}
}

// mergedAsInt extracts an int from a JSON-decoded map, accepting float64 (the JSON default) or int.
func mergedAsInt(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

func mergedAsFloat(m map[string]any, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func putString(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}
