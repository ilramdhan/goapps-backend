package erpintegration

// dry_run_summary.go builds the validate dry-run summary (plan-06 P5-T2 step
// 3; design §7, §9.2; AC-04): counts per basis, the control sums, the top
// |Δ std| against the V-05 baseline and the rule diff. Pure and decimal-only.

import (
	"sort"

	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// DryRunTopN is the default number of top-|Δ| rows in the dry-run summary.
const DryRunTopN = 20

// basisNone labels rows without a basis in DryRunSummary.CountsByBasis.
const basisNone = "-"

var pct100 = decimal.NewFromInt(100)

// DryRunDelta is one row of the top-|Δ| list. DeltaPct is empty when the
// baseline std is zero (unbounded change).
type DryRunDelta struct {
	Key      string `json:"key"`
	Basis    string `json:"basis"`
	Prev     string `json:"prev_std"`
	Cur      string `json:"std"`
	Delta    string `json:"delta"`
	DeltaPct string `json:"delta_pct,omitempty"`
}

// DryRunSummary is what the approver reviews before acknowledging and
// pushing (ceib_summary.validate.dry_run).
type DryRunSummary struct {
	Rows            int64            `json:"rows"`
	OKRows          int64            `json:"ok_rows"`
	CountsByBasis   map[string]int64 `json:"counts_by_basis"`
	CountsBySource  map[string]int64 `json:"counts_by_source"`
	SumStd          string           `json:"sum_std"`
	SumConv         string           `json:"sum_conv"`
	SumPvl          string           `json:"sum_pvl"`
	BaselineBatchID *int64           `json:"baseline_batch_id,omitempty"`
	BaselinePeriod  string           `json:"baseline_period,omitempty"`
	Compared        int64            `json:"compared"`
	FirstSeen       int64            `json:"first_seen"`
	TopDelta        []DryRunDelta    `json:"top_delta"`
	RuleChanges     int              `json:"rule_changes"`
	RuleDiff        []string         `json:"rule_diff"`
	// RuleDiffError is set when a stored snapshot could not be parsed; the
	// diff is then empty (the dry run is still produced).
	RuleDiffError string `json:"rule_diff_error,omitempty"`
}

// BuildDryRunSummary summarizes the OK rows of a batch. Counts per basis and
// source and the Σ std / conv / pvl (5 dp, the control-total rule) are over
// OK rows; the top-|Δ| list compares OK rows with a std to the baseline std
// of the same key (ties by key), at most topN (<= 0 means DryRunTopN).
// baseline nil means no previous active batch: every OK row is first-seen.
func BuildDryRunSummary(rows []domain.StdRow, baseline *domain.ValidationBaseline, changes []erprule.RuleChange, topN int) DryRunSummary {
	if topN <= 0 {
		topN = DryRunTopN
	}
	sum := DryRunSummary{
		Rows: int64(len(rows)), CountsByBasis: map[string]int64{}, CountsBySource: map[string]int64{},
		TopDelta: []DryRunDelta{}, RuleChanges: len(changes), RuleDiff: make([]string, 0, len(changes)),
	}
	if baseline != nil {
		id := baseline.BatchID
		sum.BaselineBatchID, sum.BaselinePeriod = &id, baseline.Period
	}
	var std, conv, pvl decimal.Decimal
	deltas := make([]dryRunDeltaRow, 0, len(rows))
	for _, r := range rows {
		if r.Status != domain.DeriveOK {
			continue
		}
		sum.OKRows++
		sum.CountsByBasis[basisLabel(r.Basis)]++
		sum.CountsBySource[sourceLabel(r.Source)]++
		std, conv, pvl = std.Add(nullZero(r.StdCost)), conv.Add(nullZero(r.ConvCost)), pvl.Add(nullZero(r.ProdValLoss))
		if d, ok := baselineDelta(r, baseline); ok {
			sum.Compared++
			deltas = append(deltas, d)
		} else if r.StdCost.Valid {
			sum.FirstSeen++
		}
	}
	sum.SumStd = domain.Round5(std).StringFixed(domain.ScaleR5)
	sum.SumConv = domain.Round5(conv).StringFixed(domain.ScaleR5)
	sum.SumPvl = domain.Round5(pvl).StringFixed(domain.ScaleR5)
	sum.TopDelta = topDeltas(deltas, topN)
	for _, c := range changes {
		sum.RuleDiff = append(sum.RuleDiff, c.String())
	}
	return sum
}

type dryRunDeltaRow struct {
	row   domain.StdRow
	prev  decimal.Decimal
	delta decimal.Decimal
}

func baselineDelta(r domain.StdRow, baseline *domain.ValidationBaseline) (dryRunDeltaRow, bool) {
	if baseline == nil || !r.StdCost.Valid {
		return dryRunDeltaRow{}, false
	}
	prev, ok := baseline.Std[r.Key]
	if !ok {
		return dryRunDeltaRow{}, false
	}
	return dryRunDeltaRow{row: r, prev: prev, delta: r.StdCost.Decimal.Sub(prev)}, true
}

func topDeltas(ds []dryRunDeltaRow, n int) []DryRunDelta {
	sort.SliceStable(ds, func(i, j int) bool {
		if c := ds[i].delta.Abs().Cmp(ds[j].delta.Abs()); c != 0 {
			return c > 0
		}
		return lessKey(ds[i].row.Key, ds[j].row.Key)
	})
	if len(ds) > n {
		ds = ds[:n]
	}
	out := make([]DryRunDelta, len(ds))
	for i, d := range ds {
		out[i] = DryRunDelta{
			Key: d.row.Key.String(), Basis: basisLabel(d.row.Basis),
			Prev: d.prev.StringFixed(domain.ScaleR5), Cur: d.row.StdCost.Decimal.StringFixed(domain.ScaleR5),
			Delta: d.delta.StringFixed(domain.ScaleR5),
		}
		if !d.prev.IsZero() {
			out[i].DeltaPct = d.delta.Div(d.prev.Abs()).Mul(pct100).StringFixed(2)
		}
	}
	return out
}

func basisLabel(b erprule.Basis) string {
	if b == "" {
		return basisNone
	}
	return string(b)
}

func sourceLabel(s domain.StdSource) string {
	if s == "" {
		return basisNone
	}
	return string(s)
}

func nullZero(d decimal.NullDecimal) decimal.Decimal {
	if !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}
