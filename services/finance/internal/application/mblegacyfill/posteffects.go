package mblegacyfill

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Post-commit result values.
const (
	RecalcOK     = "RECALC_OK"
	RecalcFailed = "recalc_failed"
)

// recalcCols are the spin columns whose change makes a parent spin's R/D children stale.
var recalcCols = map[string]bool{"mbs_denier": true, "mbs_filament": true, "mbs_dozing": true}

// RecalcTarget is a parent spin that needs a child recalc after its head tx committed.
type RecalcTarget struct {
	SpinID    string
	OldDozing *float64 // dozing before the legacy-fill change (nil when unknown/empty)
	// LegacyDozing is the set of spin ids whose mbs_dozing was written by this apply
	// (direct spin row or head cascade). Shared by all targets of one run.
	LegacyDozing map[string]bool
}

// RecalcOutcome is the per-spin result of a recalc pass.
type RecalcOutcome struct {
	SpinID       string
	Result       string // RecalcOK | RecalcFailed
	Reason       string
	Recalculated int
	Incomplete   int
	OrionCode    string // parent spin orion item code (for affected-products)
	// ChildIDs are the R and D children whose dozing/LDR were rewritten by this recalc.
	ChildIDs []string
	// OverwroteLegacy are children carrying a legacy mbs_dozing written in this
	// apply that the recalc then rewrote (spin legacy value lost; revert will see changed_since_apply).
	OverwroteLegacy []string
}

// Recalculator runs mbspin RecalcService.Apply for one parent spin (port).
type Recalculator interface {
	RecalcSpin(ctx context.Context, t RecalcTarget, actor string) (RecalcOutcome, error)
}

type fxJSON struct {
	SpinChanges map[string]struct {
		Before map[string]string `json:"before"`
		After  map[string]string `json:"after"`
	} `json:"spin_changes"`
}

// SelectRecalcTargets picks, from APPLIED results only, the spins whose den/fil/doz
// changed: spin rows directly, plus spins touched by a head class-D cascade
// (side_effects spin_changes). Deterministic order (sorted by spin id).
func SelectRecalcTargets(results []RowResult) []RecalcTarget {
	set := map[string]*RecalcTarget{}
	add := func(id, doz string, hasDoz bool) {
		t, ok := set[id]
		if !ok {
			t = &RecalcTarget{SpinID: id}
			set[id] = t
		}
		if hasDoz && t.OldDozing == nil {
			if f, err := strconv.ParseFloat(doz, 64); err == nil {
				t.OldDozing = &f
			}
		}
	}
	for _, rr := range results {
		if rr.Result != ResultApplied {
			continue
		}
		if rr.Row.Entity == EntitySpin && recalcCols[rr.Row.Spec.Column] {
			v, ok := dozBefore(rr.BeforeJSON, rr.Row.Spec.Column)
			add(rr.Row.ID, v, ok)
		}
		if rr.SideEffects != "" {
			addCascaded(rr.SideEffects, add)
		}
	}
	legacy := LegacyDozingSpins(results)
	out := make([]RecalcTarget, 0, len(set))
	for _, t := range set {
		t.LegacyDozing = legacy
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SpinID < out[j].SpinID })
	return out
}

// LegacyDozingSpins returns spin ids whose mbs_dozing was APPLIED by legacy-fill,
// directly or through a head class-D cascade.
func LegacyDozingSpins(results []RowResult) map[string]bool {
	m := map[string]bool{}
	for _, rr := range results {
		if rr.Result != ResultApplied {
			continue
		}
		if rr.Row.Entity == EntitySpin && rr.Row.Spec.Column == "mbs_dozing" {
			m[rr.Row.ID] = true
		}
		var fx fxJSON
		if rr.SideEffects != "" && json.Unmarshal([]byte(rr.SideEffects), &fx) == nil {
			for id, ch := range fx.SpinChanges {
				if _, ok := ch.After["mbs_dozing"]; ok {
					m[id] = true
				}
			}
		}
	}
	return m
}

// OverwrittenLegacy returns the rewritten children that carry a legacy dozing from this apply.
func OverwrittenLegacy(childIDs []string, legacy map[string]bool) []string {
	var out []string
	for _, id := range childIDs {
		if legacy[id] {
			out = append(out, id)
		}
	}
	return out
}

func dozBefore(beforeJSON, col string) (string, bool) {
	if col != "mbs_dozing" {
		return "", false
	}
	var m map[string]string
	if json.Unmarshal([]byte(beforeJSON), &m) != nil {
		return "", false
	}
	v, ok := m[col]
	return v, ok && v != ""
}

func addCascaded(raw string, add func(id, doz string, hasDoz bool)) {
	var fx fxJSON
	if json.Unmarshal([]byte(raw), &fx) != nil {
		return
	}
	for id, ch := range fx.SpinChanges {
		hit := false
		for c := range ch.After {
			if recalcCols[c] {
				hit = true
			}
		}
		if !hit {
			continue
		}
		d, ok := ch.Before["mbs_dozing"]
		add(id, d, ok && d != "")
	}
}

// RunPostEffects recalcs each target; a failure is recorded, never fatal.
func RunPostEffects(ctx context.Context, rc Recalculator, actor string, targets []RecalcTarget) []RecalcOutcome {
	out := make([]RecalcOutcome, 0, len(targets))
	for _, t := range targets {
		o, err := rc.RecalcSpin(ctx, t, actor)
		o.SpinID = t.SpinID
		if err != nil {
			o.Result, o.Reason = RecalcFailed, err.Error()
		} else if o.Result == "" {
			o.Result = RecalcOK
		}
		out = append(out, o)
	}
	return out
}

// AffectedProduct is one yarn product possibly needing a lookup sync.
type AffectedProduct struct {
	ProductID int64
	ItemCode  string
	Reason    string
}

// WriteAffectedProductsCSV writes affected-products.csv (product id, item code, reason).
// Deviation: lookup sync needs the running worker/queue, so it is listed for the operator.
func WriteAffectedProductsCSV(w io.Writer, rows []AffectedProduct) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"product_id", "item_code", "reason"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write([]string{strconv.FormatInt(r.ProductID, 10), r.ItemCode, r.Reason}); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteRecalcCSV writes recalc-results.csv.
func WriteRecalcCSV(w io.Writer, outs []RecalcOutcome) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"spin_id", "result", "reason", "recalculated", "incomplete", "recalc_overwrote_legacy"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, o := range outs {
		if err := cw.Write([]string{o.SpinID, o.Result, o.Reason, strconv.Itoa(o.Recalculated), strconv.Itoa(o.Incomplete), strings.Join(o.OverwroteLegacy, ";")}); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}
