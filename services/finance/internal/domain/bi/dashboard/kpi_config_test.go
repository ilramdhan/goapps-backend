package dashboard_test

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func TestParseKpiConfig_Empty(t *testing.T) {
	cfg, err := dashboard.ParseKpiConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 0 {
		t.Errorf("want empty, got %d entries", len(cfg))
	}
}

func TestParseKpiConfig_HappyPath(t *testing.T) {
	raw := []map[string]any{
		{
			"label":             "Current Month",
			"value_field":       "display_value",
			"agg":               "sum",
			"compare":           "MoM",
			"format":            "currency_thousands",
			"show_sparkline":    true,
			"sparkline_periods": float64(12),
		},
		{
			"label":       "Margin",
			"value_field": "display_value",
			"agg":         "avg",
			"compare":     "none",
			"format":      "percent",
			"decimals":    float64(1),
		},
	}
	cfg, err := dashboard.ParseKpiConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 2 {
		t.Fatalf("want 2 entries, got %d", len(cfg))
	}
	if cfg[0].Compare != "MoM" || !cfg[0].ShowSparkline || cfg[0].SparklinePeriods != 12 {
		t.Errorf("entry 0 mismatch: %+v", cfg[0])
	}
	if cfg[1].Format != "percent" || cfg[1].Decimals != 1 {
		t.Errorf("entry 1 mismatch: %+v", cfg[1])
	}
}

func TestParseKpiConfig_DefaultsApplied(t *testing.T) {
	raw := []map[string]any{{"label": "X", "value_field": "value"}}
	cfg, err := dashboard.ParseKpiConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg[0].Agg != "sum" {
		t.Errorf("agg default: want sum, got %q", cfg[0].Agg)
	}
	if cfg[0].Compare != "none" {
		t.Errorf("compare default: want none, got %q", cfg[0].Compare)
	}
	if cfg[0].Format != "currency_thousands" {
		t.Errorf("format default: want currency_thousands, got %q", cfg[0].Format)
	}
}

func TestParseKpiConfig_SparklineAutoPeriod(t *testing.T) {
	raw := []map[string]any{{
		"label":          "X",
		"value_field":    "v",
		"show_sparkline": true,
	}}
	cfg, err := dashboard.ParseKpiConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg[0].SparklinePeriods != 12 {
		t.Errorf("auto sparkline periods: want 12, got %d", cfg[0].SparklinePeriods)
	}
}

func TestParseKpiConfig_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  []map[string]any
	}{
		{"missing label", []map[string]any{{"value_field": "v"}}},
		{"missing value_field", []map[string]any{{"label": "X"}}},
		{"unknown agg", []map[string]any{{"label": "X", "value_field": "v", "agg": "median"}}},
		{"unknown compare", []map[string]any{{"label": "X", "value_field": "v", "compare": "PoP"}}},
		{"unknown format", []map[string]any{{"label": "X", "value_field": "v", "format": "fancy"}}},
		{"decimals out of range", []map[string]any{{"label": "X", "value_field": "v", "decimals": float64(99)}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dashboard.ParseKpiConfig(tc.raw)
			if !errors.Is(err, dashboard.ErrInvalidKpiConfig) {
				t.Errorf("expected ErrInvalidKpiConfig, got %v", err)
			}
		})
	}
}

func TestParseKpiConfig_MaxEntries(t *testing.T) {
	raw := make([]map[string]any, 7)
	for i := range raw {
		raw[i] = map[string]any{"label": "X", "value_field": "v"}
	}
	_, err := dashboard.ParseKpiConfig(raw)
	if !errors.Is(err, dashboard.ErrInvalidKpiConfig) {
		t.Errorf("expected ErrInvalidKpiConfig for 7 entries, got %v", err)
	}
}

func TestKpiConfig_MarshalToList_RoundTrip(t *testing.T) {
	raw := []map[string]any{{
		"label":          "X",
		"value_field":    "v",
		"agg":            "sum",
		"compare":        "YoY",
		"format":         "currency_millions",
		"show_sparkline": true,
	}}
	cfg, err := dashboard.ParseKpiConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := cfg.MarshalToList()
	if out[0]["label"] != "X" || out[0]["compare"] != "YoY" {
		t.Errorf("round-trip lost data: %+v", out[0])
	}
	if out[0]["show_sparkline"] != true {
		t.Errorf("sparkline flag lost: %+v", out[0])
	}
	if out[0]["sparkline_periods"] != 12 {
		t.Errorf("auto sparkline periods lost: %+v", out[0])
	}
}
