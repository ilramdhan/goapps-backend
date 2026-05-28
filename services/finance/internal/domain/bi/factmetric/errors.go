// Package factmetric contains the BI fact metric (long-format warehouse row) value object
// and the read/ingest repository interface.
package factmetric

import "errors"

// Sentinel errors.
var (
	// ErrInvalidPlan is returned by query planning when filters are inconsistent.
	ErrInvalidPlan = errors.New("invalid fact-metric query plan")
	// ErrDrillTooDeep is returned when the requested drill path exceeds the dashboard's max level.
	ErrDrillTooDeep = errors.New("drill path exceeds max drill level")
)
