package mblegacyfill

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

const cascadeRestored = "restored"

// revertedSpins returns the spin ids whose own columns were restored by the revert and
// whose restored set includes mbs_denier/mbs_filament/mbs_dozing, mapped to the dozing
// value the spin held right after the apply (the "old" dozing for the recalc, if any).
func revertedSpins(res []RevertResult) map[string]*float64 {
	set := map[string]*float64{}
	for _, r := range res {
		if r.Result == RevertFailed {
			continue
		}
		if r.Item.Entity == EntitySpin && r.Result == RevertReverted && restoredRecalcCol(r.Columns) {
			keepFirstDozing(set, r.Item.ID, afterDozing(r.Item.AfterJSON))
		}
		if r.Item.SideEffects != "" && r.Cascade != "" {
			addRevertedCascade(r, set)
		}
	}
	return set
}

func restoredRecalcCol(cols []string) bool {
	for _, c := range cols {
		name, action, ok := strings.Cut(c, ":")
		if ok && action == cascadeRestored && recalcCols[name] {
			return true
		}
	}
	return false
}

func afterDozing(raw string) *float64 {
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil || m["mbs_dozing"] == "" {
		return nil
	}
	f, err := strconv.ParseFloat(m["mbs_dozing"], 64)
	if err != nil {
		return nil
	}
	return &f
}

func addRevertedCascade(r RevertResult, set map[string]*float64) {
	var summary map[string]string
	fx, ok := parseFx(r.Item.SideEffects)
	if !ok || json.Unmarshal([]byte(r.Cascade), &summary) != nil {
		return
	}
	for id, ch := range fx.SpinChanges {
		if summary["spin:"+id] != cascadeRestored || !touchesProductColsRecalc(ch.After) {
			continue
		}
		var d *float64
		if v := ch.After["mbs_dozing"]; v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				d = &f
			}
		}
		keepFirstDozing(set, id, d)
	}
}

// keepFirstDozing records a spin, keeping the first non-nil "old" dozing seen for it so the
// result does not depend on the order of the revert items.
func keepFirstDozing(set map[string]*float64, id string, d *float64) {
	if cur, ok := set[id]; ok && cur != nil {
		return
	}
	set[id] = d
}

func touchesProductColsRecalc(after map[string]string) bool {
	for c := range after {
		if recalcCols[c] {
			return true
		}
	}
	return false
}

// SelectRevertRecalcTargets picks the spins (directly reverted spin rows and spins restored
// by a head cascade reversal) whose mbs_denier/mbs_filament/mbs_dozing were restored, so
// their R/D children can be recalculated from the restored parent values. Sorted by spin id.
func SelectRevertRecalcTargets(res []RevertResult) []RecalcTarget {
	set := revertedSpins(res)
	out := make([]RecalcTarget, 0, len(set))
	for id, d := range set {
		out = append(out, RecalcTarget{SpinID: id, OldDozing: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SpinID < out[j].SpinID })
	return out
}

// RevertedRowResults converts reverted items back to applied-style RowResults so
// CollectAffectedProducts can list the products bound to the reverted spins.
func RevertedRowResults(res []RevertResult) []RowResult {
	var items []ResultRow
	for _, r := range res {
		if r.Result == RevertReverted || (r.Result == RevertChangedSince && r.Reason == ReasonCascadePartial) {
			it := r.Item
			it.Result = ResultApplied
			items = append(items, it)
		}
	}
	return ResultRowsToRowResults(items)
}
