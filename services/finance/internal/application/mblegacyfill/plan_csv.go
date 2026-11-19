package mblegacyfill

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// PlanHeader is the plan CSV header: proposal columns + class, decision, reason.
var PlanHeader = append(append([]string{}, proposalHeader...), "class", "decision", "reason")

// WritePlanCSV writes the plan rows.
func WritePlanCSV(w io.Writer, plan *Plan) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(PlanHeader); err != nil {
		return fmt.Errorf("write plan header: %w", err)
	}
	for _, pr := range plan.Rows {
		r := pr.Row
		rec := []string{
			string(r.Entity), r.ID, r.SysID, r.Field, r.GoappsValue, r.LegacyValue,
			r.MatchMethod, r.Ambiguous, r.Usulan, r.Note, string(pr.Class), string(pr.Decision), pr.Reason,
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write plan row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// Counts aggregates the plan for the summary.
func (p *Plan) Counts() map[Decision]int {
	m := map[Decision]int{}
	for _, r := range p.Rows {
		m[r.Decision]++
	}
	return m
}

// WriteSummaryCSV writes section,key,count lines: decision totals,
// class/decision breakdown, and skipped usulan counts.
func WriteSummaryCSV(w io.Writer, plan *Plan) error {
	cw := csv.NewWriter(w)
	var recs [][]string
	add := func(section, key string, n int) {
		recs = append(recs, []string{section, key, strconv.Itoa(n)})
	}
	add("proposal", "total_rows", plan.Total)
	add("proposal", "planned_rows", len(plan.Rows))
	counts := plan.Counts()
	for _, d := range []Decision{DecisionApply, DecisionChanged, DecisionAlready, DecisionReject} {
		add("decision", string(d), counts[d])
	}
	byClass := map[string]int{}
	for _, r := range plan.Rows {
		byClass[string(r.Class)+"/"+string(r.Decision)]++
	}
	for _, k := range sortedKeys(byClass) {
		add("class_decision", k, byClass[k])
	}
	for _, k := range sortedKeys(plan.Skipped) {
		add("skipped_usulan", k, plan.Skipped[k])
	}
	if err := cw.Write([]string{"section", "key", "count"}); err != nil {
		return fmt.Errorf("write summary header: %w", err)
	}
	if err := cw.WriteAll(recs); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return cw.Error()
}

func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
