package dashboard

import "errors"

// Sentinel errors returned by the dashboard aggregate and its repository.
//
// Application and infrastructure layers must classify against these via errors.Is.
// The gRPC delivery layer maps them to status codes in error_response.go.
var (
	// ErrNotFound is returned when no dashboard matches the lookup.
	ErrNotFound = errors.New("dashboard not found")

	// ErrAlreadyExists is returned on dashboard_code uniqueness violation.
	ErrAlreadyExists = errors.New("dashboard code already exists")

	// ErrForbidden is returned when a user lacks access to a specific dashboard
	// (per bi_dashboard_role whitelist + finance.bi.dashboard.view permission).
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidCode is returned by NewCode on validation failure.
	ErrInvalidCode = errors.New("invalid dashboard code")

	// ErrInvalidChartType is returned by NewChartType for unknown chart types.
	ErrInvalidChartType = errors.New("invalid chart type")

	// ErrInvalidGrain is returned by NewPeriodGrain for unknown grains.
	ErrInvalidGrain = errors.New("invalid period grain")

	// ErrInvalidCompareMode is returned by NewCompareModes for unknown modes.
	ErrInvalidCompareMode = errors.New("invalid compare mode")

	// ErrInvalidPeriod is returned by NewDefaultPeriod for unknown preset keys.
	ErrInvalidPeriod = errors.New("invalid default period preset")

	// ErrInvalidDrillLevel is returned when max_drill_level is outside 1..3.
	ErrInvalidDrillLevel = errors.New("invalid max drill level")

	// ErrInvalidCacheTTL is returned when cache_ttl_sec is outside 0..86400.
	ErrInvalidCacheTTL = errors.New("invalid cache ttl")

	// ErrInvalidRefreshInterval is returned when refresh_interval_sec is outside 0..3600.
	ErrInvalidRefreshInterval = errors.New("invalid refresh interval")

	// ErrInvalidChartConfig is returned when chart_config fails required-field validation
	// against the chart registry.
	ErrInvalidChartConfig = errors.New("invalid chart_config")

	// ErrInvalidKpiConfig is returned when a kpi_config entry is malformed.
	ErrInvalidKpiConfig = errors.New("invalid kpi_config")

	// ErrInvalidTitle is returned when the dashboard title fails length/format checks.
	ErrInvalidTitle = errors.New("invalid dashboard title")
)
