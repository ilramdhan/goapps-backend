package hrot

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestPayrollPeriod(t *testing.T) {
	tests := []struct{ d, start, end string }{
		{"2026-10-25", "2026-09-26", "2026-10-25"},
		{"2026-10-26", "2026-10-26", "2026-11-25"},
		{"2026-12-26", "2026-12-26", "2027-01-25"},
		{"2027-01-01", "2026-12-26", "2027-01-25"},
		{"2026-02-28", "2026-02-26", "2026-03-25"},
		{"2026-03-01", "2026-02-26", "2026-03-25"},
	}
	for _, tc := range tests {
		t.Run(tc.d, func(t *testing.T) {
			assert.Equal(t, tc.start, PayrollPeriodStart(day(tc.d)).Format("2006-01-02"))
			assert.Equal(t, tc.end, PayrollPeriodEnd(day(tc.d)).Format("2006-01-02"))
		})
	}
}

func TestRow_HoursAndGap(t *testing.T) {
	tests := []struct {
		name, typ, t1, t2, t3, calc, hours, gap string
	}{
		{"tiers sum", "Normal", "1", "1.5", "0.25", "2.75", "2.75", "0"},
		{"calc above tiers", "Normal", "1", "0", "0", "1.5", "1", "0.5"},
		{"calc below tiers", "Off", "2", "2", "0", "3.5", "4", "0.5"},
		{"change off uses calc, no DQ (HD-14)", OTTypeChangeOff, "0", "0", "0", "7.333333", "7.333333", "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Row{OTType: tc.typ, Tier1: dec(tc.t1), Tier2: dec(tc.t2), Tier3: dec(tc.t3), CalcHours: dec(tc.calc)}
			assert.True(t, r.Hours().Equal(dec(tc.hours)), "hours %s", r.Hours())
			assert.True(t, r.dqGap().Equal(dec(tc.gap)), "gap %s", r.dqGap())
		})
	}
}

func TestParetoTop10(t *testing.T) {
	mk := func(vals ...string) map[string]decimal.Decimal {
		m := map[string]decimal.Decimal{}
		for i, v := range vals {
			m[fmt.Sprintf("N%02d", i)] = dec(v)
		}
		return m
	}
	tests := []struct {
		name       string
		emp        map[string]decimal.Decimal
		top, total string
	}{
		{"one employee", mk("3"), "3", "3"},
		{"ten → top 1", mk("1", "2", "3", "4", "5", "6", "7", "8", "9", "10"), "10", "55"},
		{"eleven → top 2 (ceil)", mk("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"), "21", "66"},
		{"zero hours not counted", mk("0", "0", "5"), "5", "5"},
		{"empty", mk(), "0", "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			top, total := paretoTop10(tc.emp)
			assert.True(t, top.Equal(dec(tc.top)), "top %s", top)
			assert.True(t, total.Equal(dec(tc.total)), "total %s", total)
		})
	}
}

func TestAggregate_Table(t *testing.T) {
	rows := []Row{
		// 25 → periode 202610; 26 → periode 202611 (K3).
		{WorkDate: day("2026-10-25"), NIK: "A", StagingDept: "OLD", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-26"), NIK: "A", StagingDept: "OLD", OTType: "Normal", Tier1: dec("2"), CalcHours: dec("2")},
		// outside the DAILY window but inside the period → MONTHLY only.
		{WorkDate: day("2026-09-26"), NIK: "A", StagingDept: "OLD", OTType: "Normal", Tier1: dec("4"), CalcHours: dec("4")},
	}
	facts := Aggregate(rows, map[string]string{"A": "NEW"}, day("2026-10-01"), day("2026-10-31"), day("2026-09-26"))

	got := map[string]string{}
	for _, f := range facts {
		if f.Type != TypeHours {
			continue
		}
		assert.Equal(t, "NEW", f.Group1, "current dept wins (K15)")
		got[f.Grain+" "+f.PeriodLabel] = f.Value.String()
	}
	assert.Equal(t, map[string]string{
		"DAILY 2026-10-25": "1",
		"DAILY 2026-10-26": "2",
		"MONTHLY 202610":   "5",
		"MONTHLY 202611":   "2",
	}, got)
}

func TestAggregate_PartialPeriodSkipsMonthly(t *testing.T) {
	// Staging starts Monday 2026-08-24, inside period 202608 (07-26..08-25): that period is partial.
	rows := []Row{
		{WorkDate: day("2026-08-24"), NIK: "A", StagingDept: "D", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-08-26"), NIK: "A", StagingDept: "D", OTType: "Normal", Tier1: dec("2"), CalcHours: dec("2")},
	}
	got := map[string]string{}
	for _, f := range Aggregate(rows, nil, day("2026-07-26"), day("2026-09-25"), day("2026-08-24")) {
		got[f.Type+" "+f.Grain+" "+f.PeriodLabel+" "+f.Group1] = f.Value.String()
	}
	assert.Equal(t, map[string]string{
		"HR_OT_HOURS DAILY 2026-08-24 D":        "1",
		"HR_OT_HOURS DAILY 2026-08-26 D":        "2",
		"HR_OT_HOURS MONTHLY 202609 D":          "2",
		"HR_OT_PARETO MONTHLY 202609 TOP_10PCT": "2",
		"HR_OT_PARETO MONTHLY 202609 TOTAL":     "2",
		"HR_OT_EMP_COUNT MONTHLY 202609 D":      "1",
	}, got)
}

func TestAggregate_EmpCount(t *testing.T) {
	// Period 202611 (10-26..11-25): A has OT on two days and two types (counted once), B only zero
	// hours (not counted), C has no current dept and is counted under the dept of its latest row in
	// the load (also for the earlier period, like HR_HEADCOUNT), E has a current dept and staging
	// rows in two depts (counted once, under the current dept).
	rows := []Row{
		{WorkDate: day("2026-10-27"), NIK: "A", StagingDept: "X", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-28"), NIK: "A", StagingDept: "X", OTType: "Off", Tier2: dec("2"), CalcHours: dec("2")},
		{WorkDate: day("2026-10-27"), NIK: "B", StagingDept: "X", OTType: "Normal", CalcHours: dec("0")},
		{WorkDate: day("2026-10-27"), NIK: "C", StagingDept: "OLD", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-11-02"), NIK: "C", StagingDept: "NEW", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-27"), NIK: "E", StagingDept: "P", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-28"), NIK: "E", StagingDept: "Q", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-12-01"), NIK: "C", StagingDept: "LATEST", OTType: "Normal", CalcHours: dec("0")},
		// Period 202610 starts before staging (loadedFrom): no count.
		{WorkDate: day("2026-10-20"), NIK: "D", StagingDept: "X", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
	}
	got := map[string]string{}
	for _, f := range Aggregate(rows, map[string]string{"A": "CUR", "B": "CUR", "E": "CUR"}, day("2026-10-20"), day("2026-12-25"), day("2026-10-20")) {
		if f.Type == TypeEmpCount {
			assert.Equal(t, GrainMonthly, f.Grain)
			assert.Empty(t, f.Group2)
			assert.Empty(t, f.Group3)
			got[f.PeriodLabel+" "+f.Group1] = f.Value.String()
		}
	}
	assert.Equal(t, map[string]string{"202611 CUR": "2", "202611 LATEST": "1"}, got)
}

func TestAggregate_ChangeOffSpacing(t *testing.T) {
	r := Row{OTType: " change off ", CalcHours: dec("3")}
	assert.True(t, r.Hours().Equal(dec("3")))
	assert.True(t, r.dqGap().IsZero())
}

func TestAggregate_RoundsAfterSumming(t *testing.T) {
	rows := []Row{
		{WorkDate: day("2026-10-01"), NIK: "A", StagingDept: "D", OTType: "Normal", Tier1: dec("0.33333"), CalcHours: dec("0.33333")},
		{WorkDate: day("2026-10-02"), NIK: "A", StagingDept: "D", OTType: "Normal", Tier1: dec("0.33333"), CalcHours: dec("0.33333")},
	}
	for _, f := range Aggregate(rows, nil, day("2026-10-01"), day("2026-10-02"), day("2026-09-26")) {
		if f.Type == TypeHours && f.Grain == GrainMonthly {
			assert.Equal(t, "0.6667", f.Value.String()) // 0.66666 → half-up, not 0.3333+0.3333
		}
	}
}

func TestAggregate_LongDeptClipped(t *testing.T) {
	long := strings.Repeat("Ä", 150)
	rows := []Row{{WorkDate: day("2026-10-01"), NIK: "A", StagingDept: long, OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")}}
	for _, f := range Aggregate(rows, nil, day("2026-10-01"), day("2026-10-01"), day("2026-09-26")) {
		if f.Type == TypeHours {
			assert.Len(t, []rune(f.Group1), groupMax)
		}
	}
}

// TestAggregate_Golden runs the small fixture (period 26–25, current dept) through Aggregate and
// compares with testdata/r1_fixture.golden. Regenerate with -update and review the diff by hand.
func TestAggregate_Golden(t *testing.T) {
	rows := readFixture(t, "testdata/r1_fixture.csv")
	// N03 moved from GUDANG LAMA to GUDANG; N07 has no dept anywhere.
	dept := map[string]string{"N01": "PRODUKSI", "N02": "PRODUKSI", "N03": "GUDANG", "N04": "QC", "N05": "QC"}
	facts := Aggregate(rows, dept, day("2026-10-01"), day("2026-10-31"), day("2026-09-26"))

	var b strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s|%s\n", f.Type, f.Grain, f.PeriodLabel, f.Group1, f.Group2, f.Group3, f.Value)
	}
	const golden = "testdata/r1_fixture.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(b.String()), 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), b.String())
}

func readFixture(t *testing.T, path string) []Row {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.Comment = '#'
	recs, err := r.ReadAll()
	require.NoError(t, err)
	out := make([]Row, 0, len(recs))
	for _, c := range recs {
		out = append(out, Row{
			WorkDate: day(c[0]), NIK: c[1], StagingDept: c[2], OTType: c[3],
			Tier1: dec(c[4]), Tier2: dec(c[5]), Tier3: dec(c[6]), CalcHours: dec(c[7]),
		})
	}
	return out
}

func TestAggregate_DQEmployees(t *testing.T) {
	// Period 202611: A has gaps in two types (counted once), B has a gap of 0 (not counted),
	// C is Change Off (no gap, HD-14), D has a gap in another dept.
	rows := []Row{
		{WorkDate: day("2026-10-27"), NIK: "A", StagingDept: "X", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1.5")},
		{WorkDate: day("2026-10-28"), NIK: "A", StagingDept: "X", OTType: "Off", Tier2: dec("2"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-27"), NIK: "B", StagingDept: "X", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("1")},
		{WorkDate: day("2026-10-27"), NIK: "C", StagingDept: "X", OTType: "Change Off", CalcHours: dec("4")},
		{WorkDate: day("2026-10-27"), NIK: "D", StagingDept: "Y", OTType: "Normal", Tier1: dec("1"), CalcHours: dec("3")},
	}
	got := map[string]string{}
	var gapTotal = decimal.Zero
	for _, f := range Aggregate(rows, nil, day("2026-10-20"), day("2026-11-25"), day("2026-10-20")) {
		switch {
		case f.Type == TypeDQEmp:
			assert.Equal(t, GrainMonthly, f.Grain)
			assert.Empty(t, f.Group3)
			got[f.PeriodLabel+" "+f.Group1] = f.Value.String()
		case f.Type == TypeDQ && f.Grain == GrainMonthly:
			gapTotal = gapTotal.Add(f.Value)
		}
	}
	assert.Equal(t, map[string]string{"202611 X": "1", "202611 Y": "1"}, got)
	assert.Equal(t, "3.5", gapTotal.String())
}
