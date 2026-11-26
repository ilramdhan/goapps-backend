package mbcomparisonfill

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Spin-LDR mode: set mst_mb_spin LDR = Excel col 68 (sp68), ACTUAL, adjustment cleared.
// Spec: docs/superpowers/specs/2026-11-26-mb-comparison-p2-item2-data.md.

// Spin-LDR decisions.
const (
	SpinApplySet      Decision = "APPLY_SET"       // set run_ldr = target, lock ACTUAL, adj NULL
	SpinApplyClearAdj Decision = "APPLY_CLEAR_ADJ" // run_ldr already target: clear stale adjustment
	SpinSkipConflict  Decision = "SKIP_CONFLICT"
)

// Verdicts of sql/06 that the tool understands.
const (
	VerdictSame     = "SAMA"
	VerdictAdjBasi  = "SAMA_ADJ_BASI"
	VerdictSetLDR   = "BEDA_SET_RUN_LDR"
	VerdictDipakai  = "BEDA_SPIN_DIPAKAI_PRODUK_LAIN"
	VerdictConflict = "CONFLICT_EXCEL_BEDA"
	spinFixScale    = 1e4
)

// SpinRow is one spin of the 06-q1.csv snapshot plus the planner decision.
type SpinRow struct {
	ID, Code, Verdict string
	RunLDR, Calc, Adj *float64 // CSV snapshot (nil = NULL)
	IsActual          bool
	Distinct          int
	Min, Max          *float64
	Target            float64
	Decision          Decision
	Reason            string
}

// SpinLive is the current DB state of a spin.
type SpinLive struct {
	Found    bool
	RunLDR   *float64
	Adj      *float64
	IsActual bool
}

// SpinResult is the outcome of one APPLY row (also the revert input).
type SpinResult struct {
	SpinRow
	Result, Detail, Before, After string
}

// SpinStore is the DB port of the spin-ldr mode.
type SpinStore interface {
	Live(ctx context.Context, ids []string) (map[string]SpinLive, error)
	// ApplyBatch writes in ONE tx (FOR UPDATE + guard + savepoint per row); commit=false rolls back.
	ApplyBatch(ctx context.Context, actor string, rows []SpinRow, commit bool) ([]SpinResult, error)
	Revert(ctx context.Context, actor string, r SpinResult) (SpinResult, error)
}

// Fix4 rounds to the 4dp scale of mbs_run_ldr_pct.
func Fix4(x float64) float64 { return math.Round(x*spinFixScale) / spinFixScale }

func eqF4(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return Fix4(*a) == Fix4(*b)
}

// PlanSpinLDR decides each CSV row against the live state. Pure.
func PlanSpinLDR(rows []SpinRow, live map[string]SpinLive) []SpinRow {
	out := make([]SpinRow, len(rows))
	for i, r := range rows {
		r.Decision, r.Reason = decideSpin(&r, live[r.ID])
		out[i] = r
	}
	return out
}

func decideSpin(r *SpinRow, l SpinLive) (Decision, string) {
	if d, why := staticSpinDecision(r); d != "" {
		return d, why
	}
	r.Target = Fix4(*r.Min)
	switch {
	case !l.Found:
		return DecisionReport, "spin_not_found"
	case !eqF4(l.RunLDR, r.RunLDR) || !eqF4(l.Adj, r.Adj) || l.IsActual != r.IsActual:
		return DecisionReport, "stale_vs_csv_snapshot"
	case l.Adj == nil && l.IsActual && l.RunLDR != nil && Fix4(*l.RunLDR) == r.Target:
		return DecisionSkipSame, "already_target"
	case r.Verdict == VerdictAdjBasi && (l.RunLDR == nil || Fix4(*l.RunLDR) != r.Target):
		return DecisionReport, "adj_basi_but_run_ldr_differs"
	case r.Verdict == VerdictAdjBasi:
		return SpinApplyClearAdj, ""
	}
	return SpinApplySet, ""
}

// staticSpinDecision decides from the CSV row alone; "" = continue with the live checks.
func staticSpinDecision(r *SpinRow) (Decision, string) {
	switch {
	case r.Verdict == VerdictSame:
		return DecisionSkipSame, "csv_verdict_same"
	case r.Verdict == VerdictConflict || r.Distinct > 1:
		return SpinSkipConflict, "excel_values_differ"
	case r.Verdict != VerdictAdjBasi && r.Verdict != VerdictSetLDR && r.Verdict != VerdictDipakai:
		return DecisionReport, "unknown_verdict:" + r.Verdict
	case r.Min == nil || r.Max == nil || Fix4(*r.Min) != Fix4(*r.Max):
		return DecisionReport, "target_missing_or_ambiguous"
	case Fix4(*r.Min) <= 0:
		return DecisionReport, "target_not_positive"
	}
	return "", ""
}

// SpinPlanIDs lists the ids of rows.
func SpinPlanIDs(rows []SpinRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// ApplySpinLDR runs the APPLY_* rows in batches. limit 0 = all, batch 0 = one batch.
func ApplySpinLDR(ctx context.Context, s SpinStore, actor string, rows []SpinRow, limit, batch int, commit bool) ([]SpinResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	var todo []SpinRow
	for _, r := range rows {
		if r.Decision == SpinApplySet || r.Decision == SpinApplyClearAdj {
			todo = append(todo, r)
		}
	}
	if limit > 0 && len(todo) > limit {
		todo = todo[:limit]
	}
	if batch <= 0 {
		batch = len(todo)
	}
	var out []SpinResult
	for i := 0; i < len(todo); i += batch {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		res, err := s.ApplyBatch(ctx, actor, todo[i:min(i+batch, len(todo))], commit)
		if err != nil {
			return out, err
		}
		out = append(out, res...)
	}
	return out, nil
}

// RevertSpinLDR restores APPLIED rows, newest first.
func RevertSpinLDR(ctx context.Context, s SpinStore, actor string, items []SpinResult) ([]SpinResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	var out []SpinResult
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Result != ResultApplied {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		r, err := s.Revert(ctx, actor, items[i])
		if err != nil {
			r = items[i]
			r.Result, r.Detail = ResultFailed, err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

// ---- CSV ----

var spinPlanHeader = []string{"mbs_id", "spin_code", "verdict", "run_ldr", "calc", "adj", "is_actual", "distinct_sp68", "sp68_min", "sp68_max", "target", "decision", "reason"}

// SpinResultHeader is the result CSV header.
var SpinResultHeader = append(append([]string{}, spinPlanHeader...), "result", "detail", "before_json", "after_json")

func fmtOpt(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func parseOpt(s string) (*float64, error) {
	if s = strings.TrimSpace(s); s == "" {
		return nil, nil //nolint:nilnil // empty cell = NULL
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("bad number %q", s)
	}
	return &v, nil
}

func spinRecord(r SpinRow) []string {
	return []string{r.ID, r.Code, r.Verdict, fmtOpt(r.RunLDR), fmtOpt(r.Calc), fmtOpt(r.Adj), strconv.FormatBool(r.IsActual),
		strconv.Itoa(r.Distinct), fmtOpt(r.Min), fmtOpt(r.Max), strconv.FormatFloat(r.Target, 'f', -1, 64), string(r.Decision), r.Reason}
}

func spinFromRecord(c []string) (r SpinRow, err error) {
	// c: mbs_id,spin_code,verdict,run_ldr,calc,adj,is_actual,distinct,min,max[,target,decision,reason]
	r.ID, r.Code, r.Verdict = strings.TrimSpace(c[0]), c[1], strings.TrimSpace(c[2])
	for i, dst := range []**float64{&r.RunLDR, &r.Calc, &r.Adj} {
		if *dst, err = parseOpt(c[3+i]); err != nil {
			return r, fmt.Errorf("spin %s: %w", r.ID, err)
		}
	}
	r.IsActual = strings.EqualFold(c[6], "true") || c[6] == "t"
	if r.Distinct, err = strconv.Atoi(strings.TrimSpace(c[7])); err != nil {
		return r, fmt.Errorf("spin %s: bad distinct_sp68 %q", r.ID, c[7])
	}
	if r.Min, err = parseOpt(c[8]); err != nil {
		return r, fmt.Errorf("spin %s: %w", r.ID, err)
	}
	if r.Max, err = parseOpt(c[9]); err != nil {
		return r, fmt.Errorf("spin %s: %w", r.ID, err)
	}
	if len(c) >= 13 {
		if t, perr := parseOpt(c[10]); perr == nil && t != nil {
			r.Target = *t
		}
		r.Decision, r.Reason = Decision(c[11]), c[12]
	}
	return r, nil
}

// ReadSpinCSV reads 06-q1.csv (10+ columns by header) or a plan CSV (13 columns).
func ReadSpinCSV(path string) ([]SpinRow, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || len(recs[0]) < 10 || recs[0][0] != "mbs_id" {
		return nil, fmt.Errorf("%s: not a spin-ldr csv (header mbs_id,... expected)", path)
	}
	// 06-q1 has all_linked_products + verdict at 10,11; reorder to the plan layout.
	q1 := recs[0][len(recs[0])-1] == "verdict" && len(recs[0]) == 12
	var out []SpinRow
	for _, c := range recs[1:] {
		if q1 {
			if len(c) != 12 {
				return nil, errors.New("06-q1 row column count mismatch")
			}
			// q1: id,code,run,calc,adj,is_actual,excel_products,distinct,min,max,linked,verdict
			c = []string{c[0], c[1], c[11], c[2], c[3], c[4], c[5], c[7], c[8], c[9]}
		} else if len(c) != len(spinPlanHeader) {
			return nil, errors.New("plan row column count mismatch")
		}
		r, err := spinFromRecord(c)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// WriteSpinPlanCSV writes the plan.
func WriteSpinPlanCSV(w io.Writer, rows []SpinRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(spinPlanHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(spinRecord(r)); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteSpinResultCSV writes results.
func WriteSpinResultCSV(w io.Writer, rs []SpinResult) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(SpinResultHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rs {
		if err := cw.Write(append(spinRecord(r.SpinRow), r.Result, r.Detail, r.Before, r.After)); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadSpinResultCSVs loads result CSVs in order.
func ReadSpinResultCSVs(paths []string) ([]SpinResult, error) {
	var out []SpinResult
	n := len(spinPlanHeader)
	for _, p := range paths {
		recs, err := readCSV(p)
		if err != nil {
			return nil, err
		}
		if len(recs) == 0 || strings.Join(recs[0], ",") != strings.Join(SpinResultHeader, ",") {
			return nil, fmt.Errorf("%s: not a spin-ldr result csv (header mismatch)", p)
		}
		for _, c := range recs[1:] {
			if len(c) != len(SpinResultHeader) {
				return nil, errors.New("result row column count mismatch in " + p)
			}
			r, err := spinFromRecord(c[:n])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, SpinResult{SpinRow: r, Result: c[n], Detail: c[n+1], Before: c[n+2], After: c[n+3]})
		}
	}
	return out, nil
}
