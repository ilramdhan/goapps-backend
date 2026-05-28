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
}

// SeriesDef defines one series in a mixed chart.
type SeriesDef struct {
	Name  string
	Type  string // 'bar' | 'line' | 'area'
	Field string
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

	return cfg, nil
}

// MarshalToMap converts a ChartConfig back to a map for JSONB storage. Empty/default
// values are omitted so the persisted JSON stays compact.
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
	return out
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
