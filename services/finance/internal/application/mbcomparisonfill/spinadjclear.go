package mbcomparisonfill

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Spin-adj-clear mode: every live MB spin whose LDR is not a clean ACTUAL gets run_ldr kept as is,
// adjustment cleared, calc = run_ldr, LDR type ACTUAL, locked. No input file: the plan reads the DB.

// AdjRow is one live spin (before-image of the LDR group) plus the planner decision.
type AdjRow struct {
	ID, Code, LDRType string
	RunLDR, Calc, Adj *float64 // nil = NULL
	IsActual          bool
	Target            float64 // = run_ldr (4dp)
	Decision          Decision
	Reason            string
}

// AdjResult is the outcome of one APPLY row (also the revert input).
type AdjResult struct {
	AdjRow
	Result, Detail, Before, After string
}

// AdjStore is the DB port of the spin-adj-clear mode.
type AdjStore interface {
	// Candidates lists the live spins matching the scope (read-only).
	Candidates(ctx context.Context) ([]AdjRow, error)
	// ApplyBatch writes in ONE tx (FOR UPDATE + guard + savepoint per row); commit=false rolls back.
	ApplyBatch(ctx context.Context, actor string, rows []AdjRow, commit bool) ([]AdjResult, error)
	Revert(ctx context.Context, actor string, r AdjResult) (AdjResult, error)
}

// AdjClean reports whether the LDR group is already the clean ACTUAL state.
func AdjClean(r AdjRow) bool {
	return r.Adj == nil && r.IsActual && r.LDRType == "ACTUAL" && r.RunLDR != nil && eqF4(r.Calc, r.RunLDR)
}

// PlanAdjClear decides each candidate. Pure.
func PlanAdjClear(rows []AdjRow) []AdjRow {
	out := make([]AdjRow, len(rows))
	for i, r := range rows {
		switch {
		case r.RunLDR == nil:
			r.Decision, r.Reason = DecisionReport, "run_ldr_null"
		case Fix4(*r.RunLDR) <= 0:
			r.Decision, r.Reason = DecisionReport, "run_ldr_not_positive"
		case AdjClean(r):
			r.Decision, r.Reason = DecisionSkipSame, "already_clean"
		default:
			r.Decision, r.Target = DecisionApply, Fix4(*r.RunLDR)
		}
		out[i] = r
	}
	return out
}

// adjConsistent: the plan CSV must not be able to widen what gets written.
func adjConsistent(r AdjRow) bool {
	return r.RunLDR != nil && Fix4(*r.RunLDR) > 0 && r.Target == Fix4(*r.RunLDR)
}

// ApplyAdjClear runs the APPLY rows in batches. limit 0 = all (counts spins), batch 0 = one batch.
func ApplyAdjClear(ctx context.Context, s AdjStore, actor string, rows []AdjRow, limit, batch int, commit bool) ([]AdjResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	var todo []AdjRow
	var out []AdjResult
	for _, r := range rows {
		if r.Decision != DecisionApply {
			continue
		}
		if !adjConsistent(r) {
			out = append(out, AdjResult{AdjRow: r, Result: ResultFailed, Detail: "plan_tampered"})
			continue
		}
		todo = append(todo, r)
	}
	if limit > 0 && len(todo) > limit {
		todo = todo[:limit]
	}
	if batch <= 0 {
		batch = len(todo)
	}
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

// RevertAdjClear restores APPLIED rows, newest first.
func RevertAdjClear(ctx context.Context, s AdjStore, actor string, items []AdjResult) ([]AdjResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	var out []AdjResult
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

var adjPlanHeader = []string{"mbs_id", "spin_code", "run_ldr", "calc", "adj", "ldr_type", "is_actual", "target", "decision", "reason"}

// AdjResultHeader is the result CSV header.
var AdjResultHeader = append(append([]string{}, adjPlanHeader...), "result", "detail", "before_json", "after_json")

func adjRecord(r AdjRow) []string {
	return []string{r.ID, r.Code, fmtOpt(r.RunLDR), fmtOpt(r.Calc), fmtOpt(r.Adj), r.LDRType, strconv.FormatBool(r.IsActual),
		strconv.FormatFloat(r.Target, 'f', -1, 64), string(r.Decision), r.Reason}
}

func adjFromRecord(c []string) (r AdjRow, err error) {
	r.ID, r.Code, r.LDRType = strings.TrimSpace(c[0]), c[1], c[5]
	for i, dst := range []**float64{&r.RunLDR, &r.Calc, &r.Adj} {
		if *dst, err = parseOpt(c[2+i]); err != nil {
			return r, fmt.Errorf("spin %s: %w", r.ID, err)
		}
	}
	r.IsActual = strings.EqualFold(c[6], "true")
	if r.Target, err = strconv.ParseFloat(strings.TrimSpace(c[7]), 64); err != nil {
		return r, fmt.Errorf("spin %s: bad target %q", r.ID, c[7])
	}
	r.Decision, r.Reason = Decision(c[8]), c[9]
	return r, nil
}

// ReadAdjCSV reads an adj-clear plan CSV.
func ReadAdjCSV(path string) ([]AdjRow, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || !slices.Equal(recs[0], adjPlanHeader) {
		return nil, fmt.Errorf("%s: not a spin-adj-clear plan csv (header mismatch)", path)
	}
	var out []AdjRow
	for _, c := range recs[1:] {
		if len(c) != len(adjPlanHeader) {
			return nil, errors.New("plan row column count mismatch")
		}
		r, err := adjFromRecord(c)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func writeAdjCSV(w io.Writer, header []string, n int, rec func(i int) []string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for i := 0; i < n; i++ {
		if err := cw.Write(rec(i)); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAdjPlanCSV writes the plan.
func WriteAdjPlanCSV(w io.Writer, rows []AdjRow) error {
	return writeAdjCSV(w, adjPlanHeader, len(rows), func(i int) []string { return adjRecord(rows[i]) })
}

// WriteAdjResultCSV writes results.
func WriteAdjResultCSV(w io.Writer, rs []AdjResult) error {
	return writeAdjCSV(w, AdjResultHeader, len(rs), func(i int) []string {
		r := rs[i]
		return append(adjRecord(r.AdjRow), r.Result, r.Detail, r.Before, r.After)
	})
}

// ReadAdjResultCSVs loads result CSVs in order.
func ReadAdjResultCSVs(paths []string) ([]AdjResult, error) {
	var out []AdjResult
	n := len(adjPlanHeader)
	for _, p := range paths {
		recs, err := readCSV(p)
		if err != nil {
			return nil, err
		}
		if len(recs) == 0 || !slices.Equal(recs[0], AdjResultHeader) {
			return nil, fmt.Errorf("%s: not a spin-adj-clear result csv (header mismatch)", p)
		}
		for _, c := range recs[1:] {
			if len(c) != len(AdjResultHeader) {
				return nil, errors.New("result row column count mismatch in " + p)
			}
			r, err := adjFromRecord(c[:n])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, AdjResult{AdjRow: r, Result: c[n], Detail: c[n+1], Before: c[n+2], After: c[n+3]})
		}
	}
	return out, nil
}
