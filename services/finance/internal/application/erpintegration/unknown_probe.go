package erpintegration

// unknown_probe.go resolves an UNKNOWN W2 outcome (plan-06 P5-T5; design
// Part 2 §9.4; Part 1 §S-R11). When a package call times out or the
// connection breaks, GoApps cannot know whether Oracle committed. The probe
// re-reads the ADJ rows through the read-only snapshot reader (SELECT only,
// never a write) and checks the observable effect of the call on every row
// the call was expected to touch: ADJI_FLEX_13 = batch id after VALUATE,
// the head approval after APPROVE, the stamp gone after RESTORE.

import (
	"strconv"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Probe outcomes.
const (
	// UnknownResolvedOK means every expected row shows the effect: the call
	// committed.
	UnknownResolvedOK = "OK"
	// UnknownResolvedFailed means no expected row shows the effect: the call
	// did not commit (the batch stays where it was).
	UnknownResolvedFailed = "FAILED"
	// UnknownPartial means some rows show the effect and some do not: the
	// batch is held and an alert is raised (manual resolution).
	UnknownPartial = "UNKNOWN_PARTIAL"
)

// adjFlexBatchIdx is the index of ADJI_FLEX_13 (the GoApps batch id stamp).
const adjFlexBatchIdx = 12

// UnknownResolution is the result of the read-only re-probe.
type UnknownResolution struct {
	Outcome string `json:"outcome"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
}

// rowDoneFunc reports whether a re-read row shows the effect of the call.
type rowDoneFunc func(r domain.AdjSnapshotRow) bool

// ResolveUnknown counts how many of the expected items show the effect of
// the call in the re-read rows. An expected item missing from the re-read
// counts as not done. With no expected item there is no proof either way
// and the outcome is FAILED (fail closed: nothing is finalized in PG).
func ResolveUnknown(rows []domain.AdjSnapshotRow, expected []int64, done rowDoneFunc) UnknownResolution {
	byItem := make(map[int64]domain.AdjSnapshotRow, len(rows))
	for _, r := range rows {
		byItem[r.ItemSysID] = r
	}
	res := UnknownResolution{Total: len(expected)}
	for _, id := range expected {
		if r, ok := byItem[id]; ok && done(r) {
			res.Done++
		}
	}
	switch {
	case res.Total > 0 && res.Done == res.Total:
		res.Outcome = UnknownResolvedOK
	case res.Done == 0:
		res.Outcome = UnknownResolvedFailed
	default:
		res.Outcome = UnknownPartial
	}
	return res
}

// stampedBy reports whether ADJI_FLEX_13 carries batchID.
func stampedBy(r domain.AdjSnapshotRow, batchID int64) bool {
	f := r.Flex[adjFlexBatchIdx]
	return f != nil && strings.TrimSpace(*f) == strconv.FormatInt(batchID, 10)
}

// stampedItems lists the items whose ADJI_FLEX_13 carries batchID.
func stampedItems(rows []domain.AdjSnapshotRow, batchID int64) []int64 {
	var out []int64
	for _, r := range rows {
		if stampedBy(r, batchID) {
			out = append(out, r.ItemSysID)
		}
	}
	return out
}

// projectedItems lists the items the valuation changes (a std row matched).
func projectedItems(rows []domain.AdjSnapshotRow) []int64 {
	var out []int64
	for _, r := range rows {
		if r.IsProjected() {
			out = append(out, r.ItemSysID)
		}
	}
	return out
}
