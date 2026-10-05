package erpintegration

// backtest.go compares a historical SHADOW derivation with the legacy ADJ
// rates (plan P5-T10a; design Part 2 §10.1, user decision U-4). Pure domain:
// no I/O besides the LegacyAdjRateReader port.

import (
	"context"
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// BacktestClass is the comparison outcome of one key.
type BacktestClass string

// BacktestClass values.
const (
	BacktestMatch      BacktestClass = "MATCH"
	BacktestDiff       BacktestClass = "DIFF"
	BacktestOnlyGoApps BacktestClass = "ONLY_GOAPPS"
	BacktestOnlyLegacy BacktestClass = "ONLY_LEGACY"
)

// LegacyAdjRate is the legacy ADJ rate aggregated per (item, grade, shade)
// for the backtest period.
type LegacyAdjRate struct {
	Key          ErpKey
	MaxRate      decimal.NullDecimal
	RateVariants int64
	Items        int64
}

// LegacyAdjRateReader reads the legacy ADJ rates of a YYYYMM period
// (SELECT only; implemented by the oracle reader).
type LegacyAdjRateReader interface {
	ReadLegacyAdjRates(ctx context.Context, period string) ([]LegacyAdjRate, error)
}

// BacktestLine is one compared key.
type BacktestLine struct {
	Key      ErpKey
	Class    BacktestClass
	Basis    erprule.Basis // empty for ONLY_LEGACY
	GoApps   decimal.NullDecimal
	Legacy   decimal.NullDecimal
	Delta    decimal.NullDecimal // goapps - legacy
	DeltaPct decimal.NullDecimal // delta / legacy * 100; NULL when legacy is 0/NULL
	Fail     bool
}

// BacktestReport is the full comparison, ordered by key.
type BacktestReport struct {
	Lines  []BacktestLine
	Counts map[BacktestClass]int
	// Failed is true when any SP* row differs from legacy.
	Failed bool
}

func isSPBasis(b erprule.Basis) bool {
	return b == erprule.BasisSPPTY || b == erprule.BasisSPITY || b == erprule.BasisSPBSD
}

func trimKey(k ErpKey) ErpKey {
	return ErpKey{ItemCode: strings.TrimSpace(k.ItemCode), GradeCode: strings.TrimSpace(k.GradeCode), ShadeCode: strings.TrimSpace(k.ShadeCode)}
}

func lessKey(a, b ErpKey) bool {
	if a.ItemCode != b.ItemCode {
		return a.ItemCode < b.ItemCode
	}
	if a.GradeCode != b.GradeCode {
		return a.GradeCode < b.GradeCode
	}
	return a.ShadeCode < b.ShadeCode
}

// CompareBacktest joins the derived std rows with the legacy rates on the
// trimmed ErpKey and compares StdCost with the legacy MaxRate at Round5. An
// SP* basis row that is not an exact MATCH fails the report; COST/AX/MB rows
// only report the delta.
func CompareBacktest(std []StdRow, legacy []LegacyAdjRate) BacktestReport {
	rep := BacktestReport{Counts: map[BacktestClass]int{
		BacktestMatch: 0, BacktestDiff: 0, BacktestOnlyGoApps: 0, BacktestOnlyLegacy: 0,
	}}
	leg := make(map[ErpKey]LegacyAdjRate, len(legacy))
	for _, l := range legacy {
		k := trimKey(l.Key)
		cur, ok := leg[k]
		if !ok || (l.MaxRate.Valid && (!cur.MaxRate.Valid || l.MaxRate.Decimal.GreaterThan(cur.MaxRate.Decimal))) {
			l.Key = k
			leg[k] = l
		}
	}
	seen := make(map[ErpKey]bool, len(std))
	for _, r := range std {
		k := trimKey(r.Key)
		seen[k] = true
		line := BacktestLine{Key: k, Basis: r.Basis, GoApps: roundNull(r.StdCost)}
		l, ok := leg[k]
		if !ok {
			line.Class = BacktestOnlyGoApps
		} else {
			line.Legacy = roundNull(l.MaxRate)
			line.Delta, line.DeltaPct = backtestDelta(line.GoApps, line.Legacy)
			if equalNull(line.GoApps, line.Legacy) {
				line.Class = BacktestMatch
			} else {
				line.Class = BacktestDiff
			}
		}
		line.Fail = line.Class != BacktestMatch && isSPBasis(r.Basis)
		rep.add(line)
	}
	for k, l := range leg {
		if seen[k] {
			continue
		}
		rep.add(BacktestLine{Key: k, Class: BacktestOnlyLegacy, Legacy: roundNull(l.MaxRate)})
	}
	sort.SliceStable(rep.Lines, func(i, j int) bool { return lessKey(rep.Lines[i].Key, rep.Lines[j].Key) })
	return rep
}

func (r *BacktestReport) add(l BacktestLine) {
	r.Lines = append(r.Lines, l)
	r.Counts[l.Class]++
	if l.Fail {
		r.Failed = true
	}
}

func roundNull(n decimal.NullDecimal) decimal.NullDecimal {
	if !n.Valid {
		return n
	}
	return decimal.NullDecimal{Decimal: Round5(n.Decimal), Valid: true}
}

func equalNull(a, b decimal.NullDecimal) bool {
	if a.Valid != b.Valid {
		return false
	}
	return !a.Valid || a.Decimal.Equal(b.Decimal)
}

// backtestDelta returns goapps-legacy and Δ% (NULL when either side is NULL
// or legacy is 0), rounded half away from zero.
func backtestDelta(goapps, legacy decimal.NullDecimal) (delta, pct decimal.NullDecimal) {
	if !goapps.Valid || !legacy.Valid {
		return delta, pct
	}
	d := Round5(goapps.Decimal.Sub(legacy.Decimal))
	delta = decimal.NullDecimal{Decimal: d, Valid: true}
	if legacy.Decimal.IsZero() {
		return delta, pct
	}
	// Divide at high precision, then round once.
	p := goapps.Decimal.Sub(legacy.Decimal).Div(legacy.Decimal).Mul(decimal.NewFromInt(100))
	return delta, decimal.NullDecimal{Decimal: Round5(p), Valid: true}
}

// BacktestReportRepository persists the backtest report lines of a SHADOW
// batch (plan P5-T10b).
type BacktestReportRepository interface {
	// Replace atomically swaps the stored lines of the batch for the report.
	Replace(ctx context.Context, batchID int64, period string, report BacktestReport) error
	// Get rebuilds the report (lines, counts, failed) of the batch.
	Get(ctx context.Context, batchID int64) (BacktestReport, error)
}

// BacktestCSVHeader is the CSV export header.
var BacktestCSVHeader = []string{"period", "item", "grade", "shade", "class", "basis", "goapps", "legacy", "delta", "delta_pct", "fail"}

func nullStr(n decimal.NullDecimal) string {
	if !n.Valid {
		return ""
	}
	return n.Decimal.String()
}

// WriteCSV writes the report as CSV (header + one row per line); NULL values
// are empty cells.
func (r BacktestReport) WriteCSV(w io.Writer, period string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(BacktestCSVHeader); err != nil {
		return err
	}
	for _, l := range r.Lines {
		rec := []string{period, l.Key.ItemCode, l.Key.GradeCode, l.Key.ShadeCode, string(l.Class), string(l.Basis),
			nullStr(l.GoApps), nullStr(l.Legacy), nullStr(l.Delta), nullStr(l.DeltaPct), strconv.FormatBool(l.Fail)}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
