package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// readPlanFile loads a plan CSV (mblegacyfill.PlanHeader) back into plan rows. Each row's
// field is re-resolved through the registry; unknown fields are an error (never guessed).
func readPlanFile(path string) (rows []mblegacyfill.PlanRow, err error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied input path for a one-shot CLI
	if err != nil {
		return nil, fmt.Errorf("open plan file: %w", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && err == nil {
			rows, err = nil, fmt.Errorf("close plan file: %w", cErr)
		}
	}()
	return parsePlan(f)
}

func parsePlan(r io.Reader) ([]mblegacyfill.PlanRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read plan header: %w", err)
	}
	if err := checkPlanHeader(head); err != nil {
		return nil, err
	}
	var out []mblegacyfill.PlanRow
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("plan line %d: %w", line, err)
		}
		pr, err := planRowFromRecord(line, rec)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
}

func checkPlanHeader(head []string) error {
	want := mblegacyfill.PlanHeader
	if len(head) < len(want) {
		return fmt.Errorf("plan header has %d columns, want %d", len(head), len(want))
	}
	for i, w := range want {
		if got := strings.TrimPrefix(strings.TrimSpace(head[i]), "\ufeff"); got != w {
			return fmt.Errorf("plan header column %d = %q, want %q", i+1, got, w)
		}
	}
	return nil
}

func planRowFromRecord(line int, rec []string) (mblegacyfill.PlanRow, error) {
	if len(rec) < len(mblegacyfill.PlanHeader) {
		return mblegacyfill.PlanRow{}, fmt.Errorf("plan line %d: %d columns, want %d", line, len(rec), len(mblegacyfill.PlanHeader))
	}
	row := mblegacyfill.Row{
		Line: line, Entity: mblegacyfill.Entity(strings.ToLower(strings.TrimSpace(rec[0]))), ID: strings.TrimSpace(rec[1]),
		SysID: strings.TrimSpace(rec[2]), Field: strings.ToLower(strings.TrimSpace(rec[3])),
		GoappsValue: rec[4], LegacyValue: rec[5], MatchMethod: rec[6], Ambiguous: rec[7],
		Usulan: strings.TrimSpace(rec[8]), Note: rec[9],
	}
	decision := mblegacyfill.Decision(strings.TrimSpace(rec[11]))
	// Only APPLY rows are written, so only they need a writable spec; REJECT/CHANGED/etc.
	// rows (class-X fields, parse-rejected rows) are kept with an empty Spec.
	if decision == mblegacyfill.DecisionApply {
		spec, ok := mblegacyfill.Lookup(row.Entity, row.Field)
		if !ok {
			return mblegacyfill.PlanRow{}, fmt.Errorf("plan line %d: field %s/%s is not writable", line, row.Entity, row.Field)
		}
		row.Spec = spec
	}
	return mblegacyfill.PlanRow{
		Row: row, Class: mblegacyfill.Class(strings.TrimSpace(rec[10])),
		Decision: decision, Reason: rec[12],
	}, nil
}

// selectApplyRows keeps APPLY rows (optionally of the given SYSIDs), then groups them by
// SYSID (rows without SYSID group by entity|id), head-bearing groups first, so one batch
// keeps a SYSID's head and spin rows together. limit caps the APPLY rows (0 = no cap);
// the cap is applied at group granularity from the front, never splitting a group.
func selectApplyRows(rows []mblegacyfill.PlanRow, onlySysID []string, limit int) [][]mblegacyfill.PlanRow {
	order, groups := groupApplyRows(rows, onlySysID)
	var heads, rest [][]mblegacyfill.PlanRow
	for _, k := range order {
		if hasHead(groups[k]) {
			heads = append(heads, groups[k])
		} else {
			rest = append(rest, groups[k])
		}
	}
	return capGroups(append(heads, rest...), limit)
}

func groupApplyRows(rows []mblegacyfill.PlanRow, onlySysID []string) ([]string, map[string][]mblegacyfill.PlanRow) {
	only := map[string]bool{}
	for _, s := range onlySysID {
		only[s] = true
	}
	var order []string
	groups := map[string][]mblegacyfill.PlanRow{}
	for _, r := range rows {
		if r.Decision != mblegacyfill.DecisionApply || (len(only) > 0 && !only[r.Row.SysID]) {
			continue
		}
		k := r.Row.SysID
		if k == "" {
			k = string(r.Row.Entity) + "|" + r.Row.ID
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	return order, groups
}

func capGroups(groups [][]mblegacyfill.PlanRow, limit int) [][]mblegacyfill.PlanRow {
	if limit <= 0 {
		return groups
	}
	n := 0
	for i, g := range groups {
		if n >= limit {
			return groups[:i]
		}
		n += len(g)
	}
	return groups
}

func hasHead(g []mblegacyfill.PlanRow) bool {
	for _, r := range g {
		if r.Row.Entity == mblegacyfill.EntityHead {
			return true
		}
	}
	return false
}

// chunkGroups flattens groups into batches of n groups (n <= 0: one batch).
func chunkGroups(groups [][]mblegacyfill.PlanRow, n int) [][]mblegacyfill.PlanRow {
	if n <= 0 {
		n = len(groups)
	}
	var out [][]mblegacyfill.PlanRow
	for i := 0; i < len(groups); i += n {
		var b []mblegacyfill.PlanRow
		for _, g := range groups[i:min(i+n, len(groups))] {
			b = append(b, g...)
		}
		out = append(out, b)
	}
	return out
}
