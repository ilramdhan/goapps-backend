// Package dashboard contains the BI Dashboard aggregate root and its value objects.
//
// A Dashboard is the config-driven definition of a BI viewer: which slice of
// bi_fact_metric to query (filter_type/filter_group_1/periode_grain), which chart
// type to render, the field mapping into that chart, optional KPI definitions,
// and behavioral toggles (drill, compare modes, cache TTL, refresh interval).
//
// All value objects in this file are immutable: construct via the New* constructor,
// read via the String() (or value-typed) accessor, never via direct field access.
package dashboard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

// codeRegex matches the canonical dashboard / group code pattern.
var codeRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Code is a validated dashboard or group business code.
type Code struct{ value string }

// NewCode validates and constructs a Code.
//
// Rules:
//   - 2..60 characters
//   - matches ^[A-Z][A-Z0-9_]*$  (uppercase letter first, then uppercase/digit/underscore)
func NewCode(s string) (Code, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || len(s) > 60 {
		return Code{}, fmt.Errorf("%w: code must be 2..60 chars, got %d", ErrInvalidCode, len(s))
	}
	if !codeRegex.MatchString(s) {
		return Code{}, fmt.Errorf("%w: code must match ^[A-Z][A-Z0-9_]*$, got %q", ErrInvalidCode, s)
	}
	return Code{value: s}, nil
}

// String returns the underlying code string.
func (c Code) String() string { return c.value }

// IsZero reports whether this Code is the zero value (uninitialized).
func (c Code) IsZero() bool { return c.value == "" }

// ChartType wraps a chart-type string with registry-backed validation.
type ChartType struct{ value chart.Type }

// NewChartType validates that the given string is a known chart type and constructs a ChartType.
func NewChartType(s string) (ChartType, error) {
	if s == "" {
		return ChartType{}, fmt.Errorf("%w: chart_type must not be empty", ErrInvalidChartType)
	}
	if !chart.IsValid(s) {
		return ChartType{}, fmt.Errorf("%w: %q is not a registered chart type", ErrInvalidChartType, s)
	}
	return ChartType{value: chart.Type(s)}, nil
}

// String returns the canonical chart-type string.
func (t ChartType) String() string { return string(t.value) }

// Type returns the underlying chart.Type for registry lookups.
func (t ChartType) Type() chart.Type { return t.value }

// PeriodGrain wraps a period-grain string with validation.
type PeriodGrain struct{ value chart.PeriodGrain }

// NewPeriodGrain validates and constructs a PeriodGrain.
func NewPeriodGrain(s string) (PeriodGrain, error) {
	if !chart.IsValidPeriodGrain(s) {
		return PeriodGrain{}, fmt.Errorf("%w: %q is not a valid period grain (want DAILY|MONTHLY|QUARTERLY|YEARLY)", ErrInvalidGrain, s)
	}
	return PeriodGrain{value: chart.PeriodGrain(s)}, nil
}

// String returns the canonical period-grain string.
func (g PeriodGrain) String() string { return string(g.value) }

// CompareModes is a validated list of compare modes that a dashboard supports.
type CompareModes struct{ values []chart.CompareMode }

// NewCompareModes validates and constructs a CompareModes set.
//
// Empty input is allowed (a dashboard may opt out of compare overlays entirely).
// Duplicates are silently de-duplicated.
func NewCompareModes(modes []string) (CompareModes, error) {
	seen := make(map[chart.CompareMode]struct{}, len(modes))
	out := make([]chart.CompareMode, 0, len(modes))
	for _, m := range modes {
		if !chart.IsValidCompareMode(m) {
			return CompareModes{}, fmt.Errorf("%w: %q is not a valid compare mode", ErrInvalidCompareMode, m)
		}
		cm := chart.CompareMode(m)
		if _, dup := seen[cm]; dup {
			continue
		}
		seen[cm] = struct{}{}
		out = append(out, cm)
	}
	return CompareModes{values: out}, nil
}

// Values returns a copy of the underlying compare-mode slice.
func (c CompareModes) Values() []chart.CompareMode {
	clone := make([]chart.CompareMode, len(c.values))
	copy(clone, c.values)
	return clone
}

// Strings returns the compare modes as a fresh string slice.
func (c CompareModes) Strings() []string {
	out := make([]string, len(c.values))
	for i, m := range c.values {
		out[i] = string(m)
	}
	return out
}

// Contains reports whether the given mode is in the set.
func (c CompareModes) Contains(m chart.CompareMode) bool {
	return slices.Contains(c.values, m)
}

// DefaultPeriod is the enum of preset period choices stored in bi_dashboard.default_period.
type DefaultPeriod struct{ value string }

// allowedDefaultPeriods is the closed set of period preset keys.
var allowedDefaultPeriods = map[string]struct{}{
	"L12M":       {},
	"L24M":       {},
	"THIS_YEAR":  {},
	"THIS_QTR":   {},
	"THIS_MONTH": {},
	"ALL":        {},
	"CUSTOM":     {},
}

// NewDefaultPeriod validates and constructs a DefaultPeriod.
func NewDefaultPeriod(s string) (DefaultPeriod, error) {
	if _, ok := allowedDefaultPeriods[s]; !ok {
		return DefaultPeriod{}, fmt.Errorf("%w: %q is not a valid default period preset", ErrInvalidPeriod, s)
	}
	return DefaultPeriod{value: s}, nil
}

// String returns the canonical default-period key.
func (d DefaultPeriod) String() string { return d.value }

// MaxDrillLevel is a validated 1..3 integer.
type MaxDrillLevel struct{ value int }

// NewMaxDrillLevel validates and constructs a MaxDrillLevel.
func NewMaxDrillLevel(n int) (MaxDrillLevel, error) {
	if n < 1 || n > 3 {
		return MaxDrillLevel{}, fmt.Errorf("%w: max_drill_level must be 1..3, got %d", ErrInvalidDrillLevel, n)
	}
	return MaxDrillLevel{value: n}, nil
}

// Int returns the underlying drill level.
func (m MaxDrillLevel) Int() int { return m.value }

// CacheTTL is a validated cache TTL in seconds (0..86400).
type CacheTTL struct{ value int }

// NewCacheTTL validates and constructs a CacheTTL.
//
// 0 means "disable cache". Maximum 86400 seconds (24h).
func NewCacheTTL(seconds int) (CacheTTL, error) {
	if seconds < 0 || seconds > 86400 {
		return CacheTTL{}, fmt.Errorf("%w: cache_ttl_sec must be 0..86400, got %d", ErrInvalidCacheTTL, seconds)
	}
	return CacheTTL{value: seconds}, nil
}

// Seconds returns the TTL in seconds.
func (c CacheTTL) Seconds() int { return c.value }

// IsDisabled reports whether caching is opted out for this dashboard.
func (c CacheTTL) IsDisabled() bool { return c.value == 0 }

// RefreshInterval is a validated polling interval in seconds (0..3600).
type RefreshInterval struct{ value int }

// NewRefreshInterval validates and constructs a RefreshInterval.
//
// 0 means "no polling". Maximum 3600 seconds (1h).
func NewRefreshInterval(seconds int) (RefreshInterval, error) {
	if seconds < 0 || seconds > 3600 {
		return RefreshInterval{}, fmt.Errorf("%w: refresh_interval_sec must be 0..3600, got %d", ErrInvalidRefreshInterval, seconds)
	}
	return RefreshInterval{value: seconds}, nil
}

// Seconds returns the interval in seconds.
func (r RefreshInterval) Seconds() int { return r.value }

// IsDisabled reports whether auto-refresh polling is opted out.
func (r RefreshInterval) IsDisabled() bool { return r.value == 0 }
