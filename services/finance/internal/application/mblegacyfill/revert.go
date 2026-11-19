package mblegacyfill

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Revert result values.
const (
	RevertReverted        = "REVERTED"
	RevertAlreadyReverted = "ALREADY_REVERTED"
	RevertChangedSince    = "CHANGED_SINCE_APPLY"
	RevertSkipped         = "SKIPPED"
	RevertFailed          = "FAILED"

	// ReasonCascadePartial: the item's own columns were handled but a cascaded
	// spin/cost-product target was changed after the apply or no longer exists.
	ReasonCascadePartial = "cascade_partial"
)

// Revert reasons.
const (
	ReasonAlreadyReverted = "already_reverted"
	ReasonChangedSinceApp = "changed_since_apply"
	ReasonNotApplied      = "not_applied"
	ReasonNoBeforeImage   = "no_before_image"
)

// ShadeMasterKey is the after-image key written when a shade master row was inserted.
const ShadeMasterKey = "shade_master_created"

// ResultRow is one row of an apply result CSV (input of revert).
type ResultRow struct {
	File        int // index of the input file, in command-line order
	Line        int
	Entity      Entity
	ID          string
	Field       string
	Result      string
	BeforeJSON  string
	AfterJSON   string
	SideEffects string
}

// RevertResult is the outcome of reverting one result row.
type RevertResult struct {
	Item   ResultRow
	Result string
	Reason string
	// Columns lists "column:action" entries (restored|already_reverted|changed_since_apply).
	Columns []string
	// Cascade summarizes the reversal of the head cascade (spins + cost product), JSON; empty when none.
	Cascade string
}

// Reverter restores before-images. Implementations use one transaction per head,
// restoring spins first and the head last, and must be idempotent.
type Reverter interface {
	Revert(ctx context.Context, actor string, items []ResultRow) ([]RevertResult, error)
}

// ReadResultCSVs reads apply result CSVs; file index follows the order of paths.
func ReadResultCSVs(paths []string) ([]ResultRow, error) {
	var out []ResultRow
	for i, p := range paths {
		rows, err := readResultFile(i, p)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func readResultFile(idx int, path string) (rows []ResultRow, err error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("open result csv: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	rows, err = ParseResultCSV(idx, f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

// ParseResultCSV parses one apply result CSV (columns located by header name).
func ParseResultCSV(file int, r io.Reader) ([]ResultRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read result header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"entity", "id", "field", "result", "before_json", "after_json"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("result csv missing column %q", need)
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return rec[i]
		}
		return ""
	}
	var out []ResultRow
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read result line %d: %w", line, err)
		}
		out = append(out, ResultRow{
			File: file, Line: line, Entity: Entity(strings.ToLower(strings.TrimSpace(get(rec, "entity")))),
			ID: strings.TrimSpace(get(rec, "id")), Field: get(rec, "field"), Result: get(rec, "result"),
			BeforeJSON: get(rec, "before_json"), AfterJSON: get(rec, "after_json"), SideEffects: get(rec, "side_effects_json"),
		})
	}
}

// CountShadeMastersCreated counts the distinct shade master rows inserted by the applied rows.
// They are not deleted by revert; the count is reported to the operator.
func CountShadeMastersCreated(items []ResultRow) int {
	seen := map[string]bool{}
	for _, it := range items {
		if it.Result != ResultApplied {
			continue
		}
		var m map[string]string
		if json.Unmarshal([]byte(it.AfterJSON), &m) == nil {
			if code, ok := m[ShadeMasterKey]; ok {
				seen[ShadeNK(code)] = true
			}
		}
	}
	return len(seen)
}

// RevertHeader is the revert result CSV header.
var RevertHeader = []string{"entity", "id", "field", "source_file", "source_line", "result", "reason", "columns", "cascade"}

// WriteRevertCSV writes the revert results.
func WriteRevertCSV(w io.Writer, results []RevertResult) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(RevertHeader); err != nil {
		return fmt.Errorf("write revert header: %w", err)
	}
	for _, rr := range results {
		it := rr.Item
		rec := []string{string(it.Entity), it.ID, it.Field, fmt.Sprint(it.File), fmt.Sprint(it.Line),
			rr.Result, rr.Reason, strings.Join(rr.Columns, ";"), rr.Cascade}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write revert row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// RevertSummary counts revert results by result value.
func RevertSummary(results []RevertResult) map[string]int {
	m := map[string]int{}
	for _, r := range results {
		m[r.Result]++
	}
	return m
}
