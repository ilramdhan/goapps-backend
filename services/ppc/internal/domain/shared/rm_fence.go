// Package shared holds cross-domain PPC value objects and helper calculators
// (RM fence, rounding) that are pure and infra-free.
package shared

// RMFenceStatus is the outcome of an RM fence check.
type RMFenceStatus string

// RM fence statuses.
const (
	// RMFenceOK means allocated is comfortably within the limit.
	RMFenceOK RMFenceStatus = "OK"
	// RMFenceWarning means allocated has reached the warning threshold (85%).
	RMFenceWarning RMFenceStatus = "WARNING"
	// RMFenceBlocked means allocated exceeds limit + one doff (hard fence).
	RMFenceBlocked RMFenceStatus = "BLOCKED"
)

// RMFenceWarnRatio is the fraction of the limit at which a warning is raised.
const RMFenceWarnRatio = 0.85

// CheckRMFence evaluates an RM allocation against a hard fence. The fence blocks
// when allocated exceeds the limit plus one doff of tolerance (TXT); it warns at
// RMFenceWarnRatio of the limit. A non-positive limit yields OK (no fence set).
func CheckRMFence(allocated, limit, doffWeight float64) RMFenceStatus {
	if limit <= 0 {
		return RMFenceOK
	}
	if allocated > limit+doffWeight {
		return RMFenceBlocked
	}
	if allocated >= limit*RMFenceWarnRatio {
		return RMFenceWarning
	}
	return RMFenceOK
}
