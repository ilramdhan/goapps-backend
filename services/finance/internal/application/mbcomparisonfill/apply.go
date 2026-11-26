package mbcomparisonfill

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Result values of one applied row.
const (
	ResultApplied    = "APPLIED"
	ResultWouldApply = "WOULD_APPLY" // dry-run
	ResultFailed     = "FAILED"
	ResultReverted   = "REVERTED"
	ResultChanged    = "CHANGED_SINCE_APPLY"
	ResultAlready    = "ALREADY_REVERTED"
	// ResultAlreadyApplied: live spin already at the target (idempotent re-apply).
	ResultAlreadyApplied = "ALREADY_APPLIED"
)

// ActorPrefix is required on every actor (cpp_filled_by marker).
const ActorPrefix = "mbcmp-"

// RowResult is the outcome of one APPLY row (also the revert input).
type RowResult struct {
	PlanRow
	Result       string
	Detail       string
	CAPPInserted []string // CAPP rows really inserted by this run (revert deletes only those)
}

// Writer persists plan rows. With commit=false it rolls back after computing results (dry-run).
type Writer interface {
	// Preflight aborts unless migration 000616 rewrote the three formulas.
	Preflight(ctx context.Context) error
	// ApplyBatch writes the rows in ONE transaction (savepoint per row).
	ApplyBatch(ctx context.Context, actor string, rows []PlanRow, commit bool) ([]RowResult, error)
	// Revert restores one applied row (own tx); only params still holding the after-image.
	Revert(ctx context.Context, actor string, r RowResult) (RowResult, error)
}

// ValidateActor enforces the mbcmp-<env> marker.
func ValidateActor(a string) error {
	if !strings.HasPrefix(a, ActorPrefix) || len(a) == len(ActorPrefix) {
		return fmt.Errorf("actor must look like %s<env>, got %q", ActorPrefix, a)
	}
	return nil
}

// Apply runs the APPLY rows of a plan in batches. limit 0 = unlimited, batch 0 = one batch.
func Apply(ctx context.Context, w Writer, actor string, rows []PlanRow, limit, batch int, commit bool) ([]RowResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	if err := w.Preflight(ctx); err != nil {
		return nil, err
	}
	var todo []PlanRow
	for _, r := range rows {
		if r.Decision == DecisionApply {
			todo = append(todo, r)
		}
	}
	if limit > 0 && len(todo) > limit {
		todo = todo[:limit]
	}
	if batch <= 0 {
		batch = len(todo)
	}
	var out []RowResult
	for i := 0; i < len(todo); i += batch {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		end := min(i+batch, len(todo))
		res, err := w.ApplyBatch(ctx, actor, todo[i:end], commit)
		if err != nil {
			return out, err
		}
		out = append(out, res...)
	}
	return out, nil
}

// Revert restores APPLIED rows of earlier result CSVs, newest first.
func Revert(ctx context.Context, w Writer, actor string, items []RowResult) ([]RowResult, error) {
	if err := ValidateActor(actor); err != nil {
		return nil, err
	}
	var out []RowResult
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Result != ResultApplied {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		r, err := w.Revert(ctx, actor, items[i])
		if err != nil {
			r = items[i]
			r.Result, r.Detail = ResultFailed, err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

// ResultHeader is the result CSV header.
var ResultHeader = append(append([]string{}, PlanHeader...), "result", "detail", "capp_inserted_json")

// WriteResultCSV writes results.
func WriteResultCSV(w io.Writer, rs []RowResult) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ResultHeader); err != nil {
		return fmt.Errorf("write result header: %w", err)
	}
	for _, r := range rs {
		rec := append(planRecord(r.PlanRow), r.Result, r.Detail, mustJSON(r.CAPPInserted))
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write result row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadResultCSVs loads result CSVs in the given order.
func ReadResultCSVs(paths []string) ([]RowResult, error) {
	var out []RowResult
	for _, p := range paths {
		recs, err := readCSV(p)
		if err != nil {
			return nil, err
		}
		if len(recs) == 0 || strings.Join(recs[0], ",") != strings.Join(ResultHeader, ",") {
			return nil, fmt.Errorf("%s: not a mb-comparison-fill result csv (header mismatch)", p)
		}
		for _, rec := range recs[1:] {
			if len(rec) != len(ResultHeader) {
				return nil, errors.New("result row column count mismatch in " + p)
			}
			pr, err := ReadPlanCSVRecord(rec[:len(PlanHeader)])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			n := len(PlanHeader)
			rr := RowResult{PlanRow: pr, Result: rec[n], Detail: rec[n+1]}
			if err := unmarshalAll(rec[n+2:n+3], &rr.CAPPInserted); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, rr)
		}
	}
	return out, nil
}
