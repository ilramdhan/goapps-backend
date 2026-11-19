package mblegacyfill

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
)

// Apply result values (aligned with mbactivesync).
const (
	ResultApplied = "APPLIED"
	ResultSkipped = "SKIPPED"
	ResultFailed  = "FAILED"
)

// Reasons produced by the writer.
const (
	ReasonClassDNotImplemented = "class_d_not_implemented"
	ReasonUniquenessPrefix     = "uniqueness: "
	ReasonChangedSince         = "changed_since_proposal"
	ReasonAlreadyInTarget      = "already_in_target"
	ReasonNotFound             = "entity_not_found_or_deleted"
)

// RowResult is the outcome of applying one plan row.
type RowResult struct {
	Row        Row
	Decision   string // APPLY, or CHANGED/ALREADY when the in-tx guard re-check decided differently
	Result     string
	Reason     string
	BeforeJSON string
	AfterJSON  string
	// SideEffects is a JSON blob of cascade effects (class D): cascaded spin ids, cost product id.
	SideEffects string
}

// Writer applies APPLY plan rows. Implementations group rows per head and use
// one transaction per head; non-APPLY rows are ignored.
type Writer interface {
	Apply(ctx context.Context, actor string, rows []PlanRow) ([]RowResult, error)
	// SpinHeads maps lower-case spin id -> lower-case head id for live spins in rows.
	SpinHeads(ctx context.Context, rows []PlanRow) (map[string]string, error)
}

// ResultHeader is the result CSV header.
var ResultHeader = append(append([]string{}, proposalHeader...), "decision", "result", "reason", "before_json", "after_json", "side_effects_json")

// WriteResultCSV writes the apply results.
func WriteResultCSV(w io.Writer, results []RowResult) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ResultHeader); err != nil {
		return fmt.Errorf("write result header: %w", err)
	}
	for _, rr := range results {
		r := rr.Row
		rec := []string{
			string(r.Entity), r.ID, r.SysID, r.Field, r.GoappsValue, r.LegacyValue,
			r.MatchMethod, r.Ambiguous, r.Usulan, r.Note, rr.Decision, rr.Result, rr.Reason, rr.BeforeJSON, rr.AfterJSON, rr.SideEffects,
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write result row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// ShadeNK is the shade-master dedupe key: UPPER with '-', spaces and tabs removed
// (mirrors lfShadeLookup's SQL expression).
func ShadeNK(code string) string {
	out := make([]rune, 0, len(code))
	for _, c := range code {
		switch c {
		case '-', ' ', '\t':
			continue
		}
		out = append(out, c)
	}
	return upper(string(out))
}
