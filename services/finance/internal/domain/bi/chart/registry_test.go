package chart_test

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

func TestDefaultRegistry_HasAllThirteenTypes(t *testing.T) {
	reg := chart.DefaultRegistry()
	if got := len(reg); got != 13 {
		t.Fatalf("registry size: want 13, got %d", got)
	}
}

func TestRegistration_Waterfall(t *testing.T) {
	r, ok := chart.Lookup(chart.TypeWaterfall)
	if !ok {
		t.Fatal("waterfall must be registered")
	}
	if r.Lib != chart.LibECharts {
		t.Fatalf("waterfall lib: want echarts, got %s", r.Lib)
	}
	if !r.SupportsDrill {
		t.Error("waterfall must support drill")
	}
	if r.SupportsCompare {
		t.Error("waterfall must not support compare overlay")
	}
	wantRequired := []string{"x_axis_field", "y_axis_field"}
	if !equalStrings(r.RequiredFields, wantRequired) {
		t.Errorf("waterfall required fields: want %v, got %v", wantRequired, r.RequiredFields)
	}
}

func TestRegistration_Donut_RequiresLabelValue(t *testing.T) {
	r, ok := chart.Lookup(chart.TypeDonut)
	if !ok {
		t.Fatal("donut must be registered")
	}
	wantRequired := []string{"label_field", "value_field"}
	if !equalStrings(r.RequiredFields, wantRequired) {
		t.Errorf("donut required fields: want %v, got %v", wantRequired, r.RequiredFields)
	}
}

func TestIsValid_KnownAndUnknown(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"bar", true},
		{"waterfall", true},
		{"kpi_card", true},
		{"unknown", false},
		{"", false},
		{"BAR", false}, // case-sensitive
	}
	for _, tc := range tests {
		if got := chart.IsValid(tc.input); got != tc.want {
			t.Errorf("IsValid(%q): want %v, got %v", tc.input, tc.want, got)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
