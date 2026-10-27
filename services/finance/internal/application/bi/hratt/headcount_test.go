package hratt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hrot"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func find(facts []hrot.Fact, grain, date, dept, status string) (int64, bool) {
	for _, f := range facts {
		if f.Grain == grain && f.PeriodDate.Equal(d(date)) && f.Group1 == dept && f.Group2 == status {
			return f.Value.IntPart(), true
		}
	}
	return 0, false
}

func TestAggregate_PayrollPeriodBoundary(t *testing.T) {
	// 25 Sep belongs to period 202509 (end 09-25); 26 Sep to 202510 (end 10-25).
	rows := []Row{
		{d("2026-09-25"), "A", "Weaving", "Tetap"},
		{d("2026-09-26"), "A", "Weaving", "Tetap"},
		{d("2026-09-26"), "B", "Weaving", "Kontrak"},
		{d("2026-10-01"), "A", "Weaving", "Tetap"},
		{d("2026-10-01"), "B", "Weaving", "Kontrak"},
	}
	facts := Aggregate(rows, nil, d("2026-09-25"), d("2026-10-01"), d("2026-08-26"))

	n, ok := find(facts, "MONTHLY", "2026-09-25", "Weaving", "Tetap")
	require.True(t, ok)
	assert.EqualValues(t, 1, n)
	_, ok = find(facts, "MONTHLY", "2026-09-25", "Weaving", "Kontrak")
	assert.False(t, ok, "B has no row in period 202509")

	// A appears on 3 days in the period but counts once (distinct NIK, not person-days).
	n, _ = find(facts, "MONTHLY", "2026-10-25", "Weaving", "Tetap")
	assert.EqualValues(t, 1, n)
	n, _ = find(facts, "MONTHLY", "2026-10-25", "Weaving", "Kontrak")
	assert.EqualValues(t, 1, n)

	n, _ = find(facts, "DAILY", "2026-10-01", "Weaving", "Tetap")
	assert.EqualValues(t, 1, n)
}

func TestAggregate_DeptStatusLatestAndWindow(t *testing.T) {
	rows := []Row{
		{d("2026-10-01"), "A", "Weaving", "Kontrak"},
		{d("2026-10-02"), "A", "Dyeing", "Tetap"}, // moved dept + status
		{d("2026-10-02"), "B", "Dyeing", "Tetap"},
	}
	facts := Aggregate(rows, nil, d("2026-10-02"), d("2026-10-02"), d("2026-09-26"))

	// DAILY only for the window; 10-01 is outside.
	_, ok := find(facts, "DAILY", "2026-10-01", "Dyeing", "Kontrak")
	assert.False(t, ok)
	n, _ := find(facts, "DAILY", "2026-10-02", "Dyeing", "Tetap")
	assert.EqualValues(t, 2, n)
	// MONTHLY: A is restated to current dept (K15) with the latest status.
	n, _ = find(facts, "MONTHLY", "2026-10-25", "Dyeing", "Tetap")
	assert.EqualValues(t, 2, n)
	_, ok = find(facts, "MONTHLY", "2026-10-25", "Weaving", "Kontrak")
	assert.False(t, ok)
}

func TestAggregate_PartialPeriodSkippedAndNoDept(t *testing.T) {
	rows := []Row{
		{d("2026-09-30"), "A", "Weaving", "Tetap"}, // period 202510 starts 09-26 < loadedFrom
		{d("2026-10-05"), "B", "", "Tetap"},
	}
	facts := Aggregate(rows, nil, d("2026-09-30"), d("2026-10-05"), d("2026-09-30"))
	for _, f := range facts {
		assert.NotEqual(t, hrot.GrainMonthly, f.Grain, "period only partly staged")
	}
	n, ok := find(facts, "DAILY", "2026-10-05", hrot.NoDept, "Tetap")
	require.True(t, ok)
	assert.EqualValues(t, 1, n)
}

func TestAggregate_CurrentDeptOverridesAndStatusChange(t *testing.T) {
	rows := []Row{
		{d("2026-10-01"), "A", "Weaving", "Kontrak"},
		{d("2026-10-10"), "A", "Weaving", "Tetap"}, // status changes mid-period: counted once, latest status
		{d("2026-10-10"), "A", "Weaving", "Tetap"}, // duplicate NIK/day: one person
	}
	facts := Aggregate(rows, map[string]string{"A": "Dyeing"}, d("2026-10-01"), d("2026-10-10"), d("2026-09-26"))

	n, _ := find(facts, "MONTHLY", "2026-10-25", "Dyeing", "Tetap")
	assert.EqualValues(t, 1, n)
	_, ok := find(facts, "MONTHLY", "2026-10-25", "Dyeing", "Kontrak")
	assert.False(t, ok)
	n, _ = find(facts, "DAILY", "2026-10-10", "Dyeing", "Tetap")
	assert.EqualValues(t, 1, n)
}

func TestAggregate_ClipsLongNames(t *testing.T) {
	long := string(make([]rune, 0))
	for i := 0; i < 150; i++ {
		long += "x"
	}
	facts := Aggregate([]Row{{d("2026-10-05"), "A", long, long}}, nil, d("2026-10-05"), d("2026-10-05"), d("2026-09-26"))
	require.NotEmpty(t, facts)
	assert.Len(t, []rune(facts[0].Group1), 120)
	assert.Len(t, []rune(facts[0].Group2), 120)
}
