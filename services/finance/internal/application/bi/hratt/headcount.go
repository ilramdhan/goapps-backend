// Package hratt aggregates HRIS attendance staging rows (bi_hr_stg_att_daily) into bi_fact_metric
// facts. Phase 1 (H016) only produces HR_HEADCOUNT (HD-12); R3/R4 facts follow in phase 2.
// Pure functions: no I/O, no float.
package hratt

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hrot"
)

// TypeHeadcount is the fact type written here (PRD §6): group_1 dept, group_2 employee status.
const TypeHeadcount = "HR_HEADCOUNT"

const groupMax = 120 // bi_fact_metric.group_1/2 are VARCHAR(120)

// Row is one attendance staging row (one employee, one day), reduced to what headcount needs.
type Row struct {
	WorkDate time.Time
	NIK      string
	Dept     string // dept name as of the pull that loaded the row
	Status   string // employee status (HM_EMP_STATUS name)
}

// Aggregate builds HR_HEADCOUNT facts from staging rows.
//
// DAILY = distinct NIK with a row that day, per dept × status, only for dates in [d1, d2].
// MONTHLY = distinct NIK with at least one row in the payroll period (26–25, K3), per dept × status,
// only for periods starting on or after loadedFrom (the first date staging holds), so a period
// staging covers partly is never written as complete. Dept = the employee's latest dept in rows
// (K15): bi_hr_employee (currentDept, like R1) first, else the latest staging row; status = latest status
// within the day/period. The caller passes all rows of each touched
// period (HD-04).
func Aggregate(rows []Row, currentDept map[string]string, d1, d2, loadedFrom time.Time) []hrot.Fact {
	latest := latestDept(rows)
	for nik, dept := range currentDept { // bi_hr_employee wins, same source as R1 (K15)
		if dept != "" {
			latest[nik] = dept
		}
	}

	type empKey struct {
		nik  string
		date time.Time
	}
	daily := map[empKey]Row{}
	monthly := map[empKey]Row{}
	for _, r := range rows {
		if !r.WorkDate.Before(d1) && !r.WorkDate.After(d2) {
			daily[empKey{r.NIK, r.WorkDate}] = r
		}
		if PeriodComplete(r.WorkDate, loadedFrom) {
			k := empKey{r.NIK, hrot.PayrollPeriodEnd(r.WorkDate)}
			if cur, ok := monthly[k]; !ok || !r.WorkDate.Before(cur.WorkDate) {
				monthly[k] = r
			}
		}
	}

	type factKey struct {
		dept, status, grain string
		date                time.Time
	}
	counts := map[factKey]int64{}
	for k, r := range daily {
		counts[factKey{deptOf(r, latest), r.Status, hrot.GrainDaily, k.date}]++
	}
	for k, r := range monthly {
		counts[factKey{deptOf(r, latest), r.Status, hrot.GrainMonthly, k.date}]++
	}

	facts := make([]hrot.Fact, 0, len(counts))
	for k, n := range counts {
		facts = append(facts, hrot.Fact{
			Type: TypeHeadcount, Group1: k.dept, Group2: clip(k.status), Grain: k.grain,
			PeriodDate: k.date, PeriodLabel: label(k.grain, k.date), Value: decimal.NewFromInt(n),
		})
	}
	hrot.SortFacts(facts)
	return facts
}

// PeriodComplete reports whether the payroll period containing d starts on or after loadedFrom.
func PeriodComplete(d, loadedFrom time.Time) bool {
	return !hrot.PayrollPeriodStart(d).Before(loadedFrom)
}

// latestDept returns NIK → dept of the row with the latest work_date (K15).
func latestDept(rows []Row) map[string]string {
	type pick struct {
		date time.Time
		dept string
	}
	m := map[string]pick{}
	for _, r := range rows {
		if cur, ok := m[r.NIK]; !ok || !r.WorkDate.Before(cur.date) {
			m[r.NIK] = pick{r.WorkDate, r.Dept}
		}
	}
	out := make(map[string]string, len(m))
	for nik, p := range m {
		out[nik] = p.dept
	}
	return out
}

func deptOf(r Row, latest map[string]string) string {
	d := latest[r.NIK]
	if d == "" {
		d = r.Dept
	}
	if d == "" {
		d = hrot.NoDept
	}
	return clip(d)
}

func clip(s string) string {
	if r := []rune(s); len(r) > groupMax {
		return string(r[:groupMax])
	}
	return s
}

func label(grain string, d time.Time) string {
	if grain == hrot.GrainMonthly {
		return d.Format("200601")
	}
	return d.Format("2006-01-02")
}

// SortRows orders rows by date then NIK (test helper for deterministic fixtures).
func SortRows(rows []Row) {
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].WorkDate.Equal(rows[j].WorkDate) {
			return rows[i].WorkDate.Before(rows[j].WorkDate)
		}
		return rows[i].NIK < rows[j].NIK
	})
}
