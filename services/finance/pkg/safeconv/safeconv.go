// Package safeconv provides safe type conversion functions.
package safeconv

import "math"

// Int64ToInt32 safely converts int64 to int32, capping at min/max values.
func Int64ToInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// IntToInt32 safely converts int to int32, capping at min/max values.
func IntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
