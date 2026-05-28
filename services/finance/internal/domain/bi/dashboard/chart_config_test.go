package dashboard_test

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func TestParseChartConfig_Waterfall_AppliesDefaults(t *testing.T) {
	ct, _ := dashboard.NewChartType("waterfall")
	raw := map[string]any{
		"x_axis_field": "group_2",
		"y_axis_field": "display_value",
	}
	cfg, err := dashboard.ParseChartConfig(ct, raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.XAxisField != "group_2" {
		t.Errorf("XAxisField: want group_2, got %q", cfg.XAxisField)
	}
	// Defaults from registry should apply
	if !cfg.ShowTotalBar {
		t.Error("show_total_bar should default to true for waterfall")
	}
	if cfg.PositiveColor != "#1d9e75" {
		t.Errorf("PositiveColor: want default #1d9e75, got %q", cfg.PositiveColor)
	}
	if cfg.NumberFormat != chart.NumberFormatCurrencyThousands {
		t.Errorf("NumberFormat: want currency_thousands, got %q", cfg.NumberFormat)
	}
	if cfg.Decimals != 1 {
		t.Errorf("Decimals: want 1 from default, got %d", cfg.Decimals)
	}
}

func TestParseChartConfig_Waterfall_OverrideStyle(t *testing.T) {
	ct, _ := dashboard.NewChartType("waterfall")
	raw := map[string]any{
		"x_axis_field":     "group_2",
		"y_axis_field":     "display_value",
		"positive_color":   "#aabbcc",
		"show_total_bar":   false,
		"show_data_labels": true,
		"decimals":         float64(2),
	}
	cfg, err := dashboard.ParseChartConfig(ct, raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PositiveColor != "#aabbcc" {
		t.Errorf("want override, got %q", cfg.PositiveColor)
	}
	if cfg.ShowTotalBar {
		t.Error("show_total_bar override should override default")
	}
	if !cfg.ShowDataLabels {
		t.Error("show_data_labels should be true")
	}
	if cfg.Decimals != 2 {
		t.Errorf("decimals: want 2, got %d", cfg.Decimals)
	}
}

func TestParseChartConfig_MissingRequired(t *testing.T) {
	ct, _ := dashboard.NewChartType("waterfall")
	_, err := dashboard.ParseChartConfig(ct, map[string]any{"y_axis_field": "value"})
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("expected ErrInvalidChartConfig, got %v", err)
	}
}

func TestParseChartConfig_UnknownNumberFormat(t *testing.T) {
	ct, _ := dashboard.NewChartType("bar")
	_, err := dashboard.ParseChartConfig(ct, map[string]any{
		"x_axis_field":  "group_1",
		"y_axis_field":  "value",
		"number_format": "klingon",
	})
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("expected ErrInvalidChartConfig, got %v", err)
	}
}

func TestParseChartConfig_MixedSeriesDefs(t *testing.T) {
	ct, _ := dashboard.NewChartType("mixed")
	raw := map[string]any{
		"x_axis_field": "period",
		"y_axis_field": "value",
		"series_defs": []any{
			map[string]any{"name": "Net Profit", "type": "bar", "field": "display_value"},
			map[string]any{"name": "YoY", "type": "line", "field": "yoy_value"},
		},
	}
	cfg, err := dashboard.ParseChartConfig(ct, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SeriesDefs) != 2 {
		t.Fatalf("want 2 series defs, got %d", len(cfg.SeriesDefs))
	}
	if cfg.SeriesDefs[0].Name != "Net Profit" || cfg.SeriesDefs[1].Type != "line" {
		t.Errorf("unexpected series defs: %+v", cfg.SeriesDefs)
	}
}

func TestParseChartConfig_BadSeriesDef(t *testing.T) {
	ct, _ := dashboard.NewChartType("mixed")
	raw := map[string]any{
		"x_axis_field": "period",
		"y_axis_field": "value",
		"series_defs": []any{
			map[string]any{"name": "Missing fields"},
		},
	}
	_, err := dashboard.ParseChartConfig(ct, raw)
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("expected ErrInvalidChartConfig, got %v", err)
	}
}

func TestChartConfig_MarshalToMap_RoundTrip(t *testing.T) {
	ct, _ := dashboard.NewChartType("waterfall")
	original := map[string]any{
		"x_axis_field":     "group_2",
		"y_axis_field":     "display_value",
		"positive_color":   "#aabbcc",
		"show_data_labels": true,
		"decimals":         float64(2),
	}
	cfg, err := dashboard.ParseChartConfig(ct, original)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := cfg.MarshalToMap()
	if roundTrip["x_axis_field"] != "group_2" {
		t.Errorf("x_axis_field lost: %v", roundTrip["x_axis_field"])
	}
	if roundTrip["positive_color"] != "#aabbcc" {
		t.Errorf("positive_color lost: %v", roundTrip["positive_color"])
	}
	if roundTrip["decimals"] != 2 {
		t.Errorf("decimals: want 2, got %v", roundTrip["decimals"])
	}
}
