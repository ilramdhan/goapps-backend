package workorder

import (
	"math"
	"strings"
	"time"
)

// NaturalShades is the set of shade codes treated as "natural" (undyed). Two
// plan items merge on shade when their codes are equal, or when BOTH sit in
// this set — an upstream (tty/pty/poy) level is natural regardless of which
// finished-goods color it eventually becomes, which is exactly what makes
// four differently-colored contracts producible as one work order.
//
// This lives here, in one place, so the set can be widened without a migration
// and without hunting for SQL literals.
var NaturalShades = []string{"NL", "NATURAL", ""}

// DefaultMergeWindowDays is the deadline tolerance applied when the caller
// does not supply one.
const DefaultMergeWindowDays = 7

// MaxMergeWindowDays bounds the deadline tolerance a caller may ask for.
const MaxMergeWindowDays = 30

// normalizeShade folds a shade code to its comparison form. The same folding
// is applied in SQL (UPPER(TRIM(COALESCE(...,”)))) so Go and Postgres agree on
// what "compatible" means.
func normalizeShade(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsNaturalShade reports whether a shade code is natural (undyed).
func IsNaturalShade(code string) bool {
	n := normalizeShade(code)
	for _, s := range NaturalShades {
		if n == s {
			return true
		}
	}
	return false
}

// ShadesCompatible reports whether two shade codes may share a work order:
// identical after folding, or both natural.
func ShadesCompatible(a, b string) bool {
	if normalizeShade(a) == normalizeShade(b) {
		return true
	}
	return IsNaturalShade(a) && IsNaturalShade(b)
}

// MergeSubject is the projection of a plan item the merge predicate needs. The
// work-order domain deliberately does not import the plan-item aggregate — it
// only needs these five fields, and keeping them flat lets the repository
// answer straight from SQL.
type MergeSubject struct {
	PlanItemID     int64
	ProductSysID   int64
	MachineGroupID int64
	ShadeCode      string
	Deadline       time.Time
	QtyTarget      float64
	Status         string
}

// MergeableStatuses are the plan-item statuses a merge candidate may hold.
//
// NOTE: the spec writes these as ('DRAFT','ACTIVE'), but the plan-item domain
// has no ACTIVE status — the proto enum PLAN_ITEM_STATUS_ACTIVE maps to the
// domain string CONFIRMED. DRAFT/CONFIRMED is the faithful translation; using
// the spec's literal would match nothing.
var MergeableStatuses = []string{"DRAFT", "CONFIRMED"}

// isMergeableStatus reports whether a plan-item status permits merging.
func isMergeableStatus(status string) bool {
	for _, s := range MergeableStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// CanMerge reports whether candidate may join anchor's work order: same
// product, same machine group, compatible shade, deadline within windowDays,
// and a status that still permits planning. A candidate is never its own
// anchor.
//
// The server applies this to every client-supplied plan item — the candidate
// list is a convenience, not a trust boundary.
func CanMerge(anchor, candidate MergeSubject, windowDays int32) bool {
	if windowDays <= 0 {
		windowDays = DefaultMergeWindowDays
	}
	if candidate.PlanItemID == anchor.PlanItemID {
		return false
	}
	if candidate.ProductSysID != anchor.ProductSysID {
		return false
	}
	if candidate.MachineGroupID != anchor.MachineGroupID {
		return false
	}
	if !ShadesCompatible(anchor.ShadeCode, candidate.ShadeCode) {
		return false
	}
	if !isMergeableStatus(candidate.Status) {
		return false
	}
	return deadlineWithin(anchor.Deadline, candidate.Deadline, windowDays)
}

// deadlineWithin reports whether two deadlines sit within windowDays of each
// other, compared on the date (the column is a DATE).
func deadlineWithin(a, b time.Time, windowDays int32) bool {
	diff := a.Truncate(24*time.Hour).Sub(b.Truncate(24*time.Hour)).Hours() / 24
	return math.Abs(diff) <= float64(windowDays)
}
