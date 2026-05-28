package dashboard_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    string
	}{
		{"valid uppercase", "EBITDA", false, "EBITDA"},
		{"valid with digit", "EBITDA2", false, "EBITDA2"},
		{"valid with underscore", "NET_PROFIT", false, "NET_PROFIT"},
		{"trims whitespace", "  EBITDA  ", false, "EBITDA"},
		{"too short", "A", true, ""},
		{"lowercase reject", "ebitda", true, ""},
		{"hyphen reject", "EB-IT", true, ""},
		{"leading digit reject", "1EBITDA", true, ""},
		{"empty", "", true, ""},
		{"too long", strings.Repeat("A", 61), true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dashboard.NewCode(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !errors.Is(err, dashboard.ErrInvalidCode) {
					t.Errorf("expected ErrInvalidCode, got %v", err)
				}
				return
			}
			if got.String() != tc.want {
				t.Errorf("String(): want %q, got %q", tc.want, got.String())
			}
			if got.IsZero() {
				t.Error("non-zero code reported as zero")
			}
		})
	}
}

func TestCode_ZeroValue(t *testing.T) {
	var c dashboard.Code
	if !c.IsZero() {
		t.Error("zero Code should report IsZero=true")
	}
	if c.String() != "" {
		t.Errorf("zero Code String() should be empty, got %q", c.String())
	}
}

func TestNewChartType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"waterfall", "waterfall", false},
		{"line", "line", false},
		{"kpi_card", "kpi_card", false},
		{"unknown", "unknown_chart", true},
		{"empty", "", true},
		{"uppercase reject", "WATERFALL", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dashboard.NewChartType(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !errors.Is(err, dashboard.ErrInvalidChartType) {
					t.Errorf("expected ErrInvalidChartType, got %v", err)
				}
				return
			}
			if got.String() != tc.input {
				t.Errorf("String(): want %q, got %q", tc.input, got.String())
			}
		})
	}
}

func TestNewPeriodGrain(t *testing.T) {
	for _, s := range []string{"DAILY", "MONTHLY", "QUARTERLY", "YEARLY"} {
		if _, err := dashboard.NewPeriodGrain(s); err != nil {
			t.Errorf("want OK for %q, got %v", s, err)
		}
	}
	for _, s := range []string{"daily", "Monthly", "WEEKLY", ""} {
		if _, err := dashboard.NewPeriodGrain(s); err == nil {
			t.Errorf("want error for %q", s)
		} else if !errors.Is(err, dashboard.ErrInvalidGrain) {
			t.Errorf("expected ErrInvalidGrain, got %v", err)
		}
	}
}

func TestNewCompareModes(t *testing.T) {
	t.Run("valid set with dedup", func(t *testing.T) {
		got, err := dashboard.NewCompareModes([]string{"MoM", "YoY", "MoM"})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(got.Values()); n != 2 {
			t.Errorf("want 2 modes after dedup, got %d", n)
		}
		if !got.Contains(chart.CompareMoM) {
			t.Error("expected MoM present")
		}
		if got.Contains(chart.CompareR12) {
			t.Error("expected R12 absent")
		}
	})
	t.Run("empty list ok", func(t *testing.T) {
		got, err := dashboard.NewCompareModes(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Values()) != 0 {
			t.Error("expected empty modes")
		}
	})
	t.Run("invalid rejects", func(t *testing.T) {
		_, err := dashboard.NewCompareModes([]string{"MoM", "BAD"})
		if !errors.Is(err, dashboard.ErrInvalidCompareMode) {
			t.Errorf("expected ErrInvalidCompareMode, got %v", err)
		}
	})
	t.Run("strings round-trip", func(t *testing.T) {
		got, _ := dashboard.NewCompareModes([]string{"MoM", "YoY"})
		strs := got.Strings()
		if len(strs) != 2 || strs[0] != "MoM" || strs[1] != "YoY" {
			t.Errorf("unexpected strings: %v", strs)
		}
	})
}

func TestNewDefaultPeriod(t *testing.T) {
	for _, s := range []string{"L12M", "L24M", "THIS_YEAR", "THIS_QTR", "THIS_MONTH", "ALL", "CUSTOM"} {
		if _, err := dashboard.NewDefaultPeriod(s); err != nil {
			t.Errorf("want OK for %q, got %v", s, err)
		}
	}
	for _, s := range []string{"", "l12m", "LAST_12M", "L36M"} {
		if _, err := dashboard.NewDefaultPeriod(s); !errors.Is(err, dashboard.ErrInvalidPeriod) {
			t.Errorf("want ErrInvalidPeriod for %q, got %v", s, err)
		}
	}
}

func TestNewMaxDrillLevel(t *testing.T) {
	for _, n := range []int{1, 2, 3} {
		if _, err := dashboard.NewMaxDrillLevel(n); err != nil {
			t.Errorf("want OK for %d, got %v", n, err)
		}
	}
	for _, n := range []int{0, 4, -1, 100} {
		if _, err := dashboard.NewMaxDrillLevel(n); !errors.Is(err, dashboard.ErrInvalidDrillLevel) {
			t.Errorf("want ErrInvalidDrillLevel for %d, got %v", n, err)
		}
	}
}

func TestNewCacheTTL(t *testing.T) {
	t.Run("zero disabled", func(t *testing.T) {
		c, err := dashboard.NewCacheTTL(0)
		if err != nil {
			t.Fatal(err)
		}
		if !c.IsDisabled() {
			t.Error("0 TTL should be disabled")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		c, err := dashboard.NewCacheTTL(1800)
		if err != nil {
			t.Fatal(err)
		}
		if c.Seconds() != 1800 || c.IsDisabled() {
			t.Errorf("unexpected: %v", c)
		}
	})
	t.Run("out of bounds", func(t *testing.T) {
		for _, n := range []int{-1, 86401} {
			if _, err := dashboard.NewCacheTTL(n); !errors.Is(err, dashboard.ErrInvalidCacheTTL) {
				t.Errorf("want ErrInvalidCacheTTL for %d, got %v", n, err)
			}
		}
	})
}

func TestNewRefreshInterval(t *testing.T) {
	if r, err := dashboard.NewRefreshInterval(0); err != nil || !r.IsDisabled() {
		t.Errorf("0 should construct as disabled, got r=%v err=%v", r, err)
	}
	if _, err := dashboard.NewRefreshInterval(3601); !errors.Is(err, dashboard.ErrInvalidRefreshInterval) {
		t.Errorf("want ErrInvalidRefreshInterval for 3601, got %v", err)
	}
}
