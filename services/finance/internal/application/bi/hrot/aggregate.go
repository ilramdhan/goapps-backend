// Package hrot aggregates HRIS overtime staging rows (bi_hr_stg_ot_daily) into bi_fact_metric
// facts for report R1 (H011). Pure functions over decimal hours: no I/O, no float.
package hrot

import (
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Fact types written by the R1 aggregation (PRD §6, HD-09).
const (
	TypeHours  = "HR_OT_HOURS"  // group_1 dept, group_2 OT type, group_3 NIK; jam aktual
	TypeDQ     = "HR_OT_DQ"     // group_1 dept, group_2 OT type, group_3 NIK; |calc − tier 1+2+3|
	TypePareto = "HR_OT_PARETO" // group_1 TOP_10PCT / TOTAL; MONTHLY only, ratio at query time (HD-08)
	// TypeEmpCount = distinct NIK with OT hours > 0 per dept; MONTHLY only. Numerator of the R1 KPI
	// "% karyawan lembur" (÷ HR_HEADCOUNT at query time, HD-08). Each NIK counts in one dept per
	// period, so summing across depts gives the company count (H032).
	TypeEmpCount = "HR_OT_EMP_COUNT"
	// TypeDQEmp = distinct NIK with a calc-vs-tier gap > 0 per dept; MONTHLY only (H039). Change Off has
	// no gap (HD-14), so it never counts.
	TypeDQEmp = "HR_OT_DQ_EMP"

	GrainDaily   = "DAILY"
	GrainMonthly = "MONTHLY"

	ParetoTop   = "TOP_10PCT"
	ParetoTotal = "TOTAL"

	// OTTypeChangeOff has tier 1+2+3 = 0 in HRIS; R1 shows its calculation hours instead (HD-14).
	OTTypeChangeOff = "Change Off"

	// NoDept labels rows whose employee has no department in HRIS.
	NoDept = "(Tanpa Dept)"

	factScale = 4   // bi_fact_metric.value is NUMERIC(20,4)
	groupMax  = 120 // bi_fact_metric.group_1/2/3 are VARCHAR(120); staging dept_name is 200
)

// Row is one staging row (one employee, one day, one OT type).
type Row struct {
	WorkDate     time.Time
	NIK          string
	StagingDept  string // dept name as of the pull that loaded the row
	OTType       string
	Tier1, Tier2 decimal.Decimal
	Tier3        decimal.Decimal
	CalcHours    decimal.Decimal
}

// Fact is one aggregated bi_fact_metric row. Group2/Group3 empty = NULL.
type Fact struct {
	Type        string
	Group1      string
	Group2      string
	Group3      string
	Grain       string
	PeriodDate  time.Time
	PeriodLabel string
	Value       decimal.Decimal
}

// PayrollPeriodEnd returns the 25th closing the payroll period (26–25) that contains d (K3).
func PayrollPeriodEnd(d time.Time) time.Time {
	end := time.Date(d.Year(), d.Month(), 25, 0, 0, 0, 0, time.UTC)
	if d.Day() >= 26 {
		end = end.AddDate(0, 1, 0)
	}
	return end
}

// PayrollPeriodStart returns the 26th opening the payroll period that contains d.
func PayrollPeriodStart(d time.Time) time.Time {
	return PayrollPeriodEnd(d).AddDate(0, -1, 1)
}

// Hours is the R1 measure of a row: tier 1+2+3 (= MPAR_LEMBUR_*), except Change Off which uses
// the calculation hours (HD-14).
func (r Row) Hours() decimal.Decimal {
	if r.isChangeOff() {
		return r.CalcHours
	}
	return r.tiers()
}

func (r Row) isChangeOff() bool {
	return strings.EqualFold(strings.TrimSpace(r.OTType), OTTypeChangeOff)
}

func (r Row) tiers() decimal.Decimal { return r.Tier1.Add(r.Tier2).Add(r.Tier3) }

// dqGap returns |calc − tier 1+2+3| (K7: both are actual hours). Change Off is an intended
// difference (HD-14) and never reported.
func (r Row) dqGap() decimal.Decimal {
	if r.isChangeOff() {
		return decimal.Zero
	}
	return r.CalcHours.Sub(r.tiers()).Abs()
}

type key struct {
	typ, g1, g2, g3, grain string
	date                   time.Time
}

// Aggregate builds R1 facts from staging rows.
//
// DAILY facts are produced only for rows with WorkDate in [d1, d2] (the re-pulled window); MONTHLY
// and Pareto facts for every payroll period present in rows that starts on or after loadedFrom (the
// first date staging holds), so a period staging only partly covers is never written as if it were
// complete. The caller passes all staging rows of each touched period (HD-04).
// Department = the employee's current department from currentDept (K15), falling back to the
// staging dept. Sums are rounded half-up to 4 decimals once, after summing. Zero facts are dropped.
func Aggregate(rows []Row, currentDept map[string]string, d1, d2, loadedFrom time.Time) []Fact {
	acc := accumulator{sums: map[key]decimal.Decimal{}, perEmp: map[time.Time]map[string]decimal.Decimal{}}
	for _, r := range rows {
		inWindow := !r.WorkDate.Before(d1) && !r.WorkDate.After(d2)
		complete := !PayrollPeriodStart(r.WorkDate).Before(loadedFrom)
		acc.addRow(r, deptOf(r, currentDept), inWindow, complete)
	}
	countDept := empCountDept(rows, currentDept)
	for pEnd, emp := range acc.perEmp {
		top, total := paretoTop10(emp)
		acc.add(key{TypePareto, ParetoTop, "", "", GrainMonthly, pEnd}, top)
		acc.add(key{TypePareto, ParetoTotal, "", "", GrainMonthly, pEnd}, total)
		for nik, h := range emp {
			if h.IsPositive() {
				acc.add(key{TypeEmpCount, countDept[nik], "", "", GrainMonthly, pEnd}, decimal.NewFromInt(1))
			}
		}
	}

	acc.countDQEmployees()

	facts := acc.facts()
	SortFacts(facts)
	return facts
}

// deptOf returns the employee's current dept (K15), falling back to the staging dept, clipped to
// the fact column width.
func deptOf(r Row, currentDept map[string]string) string {
	dept := currentDept[r.NIK]
	if dept == "" {
		dept = r.StagingDept
	}
	if dept == "" {
		dept = NoDept
	}
	return clip(dept)
}

// empCountDept returns NIK → dept for HR_OT_EMP_COUNT, the same rule as HR_HEADCOUNT
// (hratt.Aggregate) so numerator and denominator share a dept: the current dept (K15), else the
// staging dept of the latest row passed in (ties: greatest dept name, so reruns are identical).
func empCountDept(rows []Row, currentDept map[string]string) map[string]string {
	type pick struct {
		date time.Time
		dept string
	}
	latest := map[string]pick{}
	for _, r := range rows {
		cur, ok := latest[r.NIK]
		if !ok || r.WorkDate.After(cur.date) || (r.WorkDate.Equal(cur.date) && r.StagingDept > cur.dept) {
			latest[r.NIK] = pick{r.WorkDate, r.StagingDept}
		}
	}
	out := make(map[string]string, len(latest))
	for nik, p := range latest {
		out[nik] = deptOf(Row{NIK: nik, StagingDept: p.dept}, currentDept)
	}
	return out
}

type accumulator struct {
	sums   map[key]decimal.Decimal
	perEmp map[time.Time]map[string]decimal.Decimal // period end → NIK → hours (Pareto, emp count)
}

func (a *accumulator) add(k key, v decimal.Decimal) {
	if !v.IsZero() {
		a.sums[k] = a.sums[k].Add(v)
	}
}

func (a *accumulator) addRow(r Row, dept string, inWindow, periodComplete bool) {
	pEnd := PayrollPeriodEnd(r.WorkDate)
	hours, gap := r.Hours(), r.dqGap()
	if inWindow {
		a.add(key{TypeHours, dept, r.OTType, r.NIK, GrainDaily, r.WorkDate}, hours)
		a.add(key{TypeDQ, dept, r.OTType, r.NIK, GrainDaily, r.WorkDate}, gap)
	}
	if !periodComplete {
		return
	}
	a.add(key{TypeHours, dept, r.OTType, r.NIK, GrainMonthly, pEnd}, hours)
	a.add(key{TypeDQ, dept, r.OTType, r.NIK, GrainMonthly, pEnd}, gap)
	if hours.IsZero() {
		return
	}
	if a.perEmp[pEnd] == nil {
		a.perEmp[pEnd] = map[string]decimal.Decimal{}
	}
	a.perEmp[pEnd][r.NIK] = a.perEmp[pEnd][r.NIK].Add(hours)
}

// countDQEmployees adds HR_OT_DQ_EMP: per period and dept, the NIKs whose monthly gap (summed over
// OT types) is positive. Must run after all rows are added.
func (a *accumulator) countDQEmployees() {
	type nikKey struct {
		g1, g3 string
		date   time.Time
	}
	gap := map[nikKey]decimal.Decimal{}
	for k, v := range a.sums {
		if k.typ == TypeDQ && k.grain == GrainMonthly {
			nk := nikKey{k.g1, k.g3, k.date}
			gap[nk] = gap[nk].Add(v)
		}
	}
	for nk, v := range gap {
		if v.IsPositive() {
			a.add(key{TypeDQEmp, nk.g1, "", "", GrainMonthly, nk.date}, decimal.NewFromInt(1))
		}
	}
}

// paretoTop10 returns the hours of the top ⌈10%⌉ employees (by hours desc, NIK asc on ties) and the
// total hours of all employees with OT in the period.
func paretoTop10(emp map[string]decimal.Decimal) (top, total decimal.Decimal) {
	niks := make([]string, 0, len(emp))
	for nik, h := range emp {
		if h.IsPositive() {
			niks = append(niks, nik)
		}
		total = total.Add(h)
	}
	sort.Slice(niks, func(i, j int) bool {
		if c := emp[niks[i]].Cmp(emp[niks[j]]); c != 0 {
			return c > 0
		}
		return niks[i] < niks[j]
	})
	n := (len(niks) + 9) / 10
	for _, nik := range niks[:n] {
		top = top.Add(emp[nik])
	}
	return top, total
}

func clip(s string) string {
	if r := []rune(s); len(r) > groupMax {
		return string(r[:groupMax])
	}
	return s
}

func periodLabel(grain string, d time.Time) string {
	switch grain {
	case GrainMonthly:
		return d.Format("200601")
	case GrainWeekly:
		return isoWeekLabel(d)
	}
	return d.Format("2006-01-02")
}

// SortFacts orders facts deterministically (type, grain, date, groups).
func SortFacts(f []Fact) {
	sort.Slice(f, func(i, j int) bool {
		a, b := f[i], f[j]
		switch {
		case a.Type != b.Type:
			return a.Type < b.Type
		case a.Grain != b.Grain:
			return a.Grain < b.Grain
		case !a.PeriodDate.Equal(b.PeriodDate):
			return a.PeriodDate.Before(b.PeriodDate)
		case a.Group1 != b.Group1:
			return a.Group1 < b.Group1
		case a.Group2 != b.Group2:
			return a.Group2 < b.Group2
		default:
			return a.Group3 < b.Group3
		}
	})
}
