package erpintegration

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// P4-T5: derive performance + memory check. Target for 10k rows: < 2 s and
// < 200 MB (plan-05 P4-T5), keeping the DERIVE step inside the job timeout.
const (
	perfRows        = 10_000
	perfMaxDuration = 2 * time.Second
	perfMaxBytes    = 200 << 20
)

// perfRuleSet builds a realistic rule set: 40 FG types x 3 prod types x the
// 6 non-AX grade groups (720 rules), both priced bases, and ~30 grades.
func perfRuleSet(tb testing.TB) *erprule.RuleSet {
	tb.Helper()
	groups := []erprule.GradeGroup{
		erprule.GradeGroupNS, erprule.GradeGroupAE, erprule.GradeGroupBC,
		erprule.GradeGroupBB, erprule.GradeGroupJLT, erprule.GradeGroupPOYA,
	}
	bases := []erprule.Basis{erprule.BasisCost, erprule.BasisSPPTY, erprule.BasisSPITY, erprule.BasisSPBSD}
	pts := []erprule.ProdType{erprule.ProdTypePOY, erprule.ProdTypePTY, erprule.ProdTypeITY}
	var rules []erprule.Rule
	for f := 0; f < 40; f++ {
		for pi, pt := range pts {
			for gi, g := range groups {
				rules = append(rules, erprule.Rule{
					Key:     erprule.RuleKey{FgType: erprule.FgType(fmt.Sprintf("Type %d", f)), ProdType: pt, GradeGroup: g},
					Basis:   bases[(f+pi+gi)%len(bases)],
					ValLoss: decimal.New(int64(f*7+gi*13+pi), -3),
				})
			}
		}
	}
	prices := []erprule.Price{
		{Basis: erprule.BasisSPPTY, Price: d("1.300000")},
		{Basis: erprule.BasisSPITY, Price: d("1.500000")},
		{Basis: erprule.BasisSPBSD, Price: d("1.100000")},
	}
	grades := []erprule.GradeAssignment{{GradeCode: GradeAX, Group: erprule.GradeGroupAX}}
	for i := 0; i < 30; i++ {
		grades = append(grades, erprule.GradeAssignment{GradeCode: fmt.Sprintf("G%02d", i), Group: groups[i%len(groups)]})
	}
	rs, err := erprule.NewRuleSet(rules, prices, grades)
	require.NoError(tb, err)
	return rs
}

// perfInputs builds n inputs with a production-like mix: ~5% MB, the rest
// yarn across POY/PTY/ITY; ~1 in 8 AX grade, the rest derived grades; ~2%
// NO_AX and ~1% unknown grade so failure paths are exercised too.
func perfInputs(n int) []DeriveInput {
	prefixes := []string{"POY", "PTY", "ITY"}
	out := make([]DeriveInput, 0, n)
	for i := 0; i < n; i++ {
		var in DeriveInput
		ax := &AxComponents{
			CostID: int64(i + 1), Version: 1, ProductSysID: int64(i/8 + 1),
			CostPerUnit: decimal.New(int64(1_000_000+i*37), -6),
			TotalRMCost: decimal.New(int64(700_000+i*29), -6),
			FgType:      fmt.Sprintf("Type %d", i%40),
			ChpItemCode: "REG", ItemType: prefixes[i%3],
		}
		switch {
		case i%20 == 0:
			in = DeriveInput{Key: ErpKey{ItemCode: fmt.Sprintf("CMB%06d", i), GradeCode: GradeMBA, ShadeCode: "NL"}, Ax: ax}
		default:
			grade := fmt.Sprintf("G%02d", i%30)
			if i%8 == 1 {
				grade = GradeAX
			}
			if i%100 == 7 {
				grade = "ZZ"
			}
			in = DeriveInput{Key: ErpKey{ItemCode: fmt.Sprintf("%s%06d", prefixes[i%3], i/8), GradeCode: grade, ShadeCode: fmt.Sprintf("S%03d", i%250)}, Ax: ax}
			if i%50 == 3 {
				in.Ax = nil
			}
		}
		in.ItemName = fmt.Sprintf("Item %d", i)
		in.ShadeName = fmt.Sprintf("Shade %d", i%250)
		out = append(out, in)
	}
	return out
}

func benchmarkDerive(b *testing.B, n int) {
	rs := perfRuleSet(b)
	inputs := perfInputs(n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, _ := Derive(inputs, rs)
		if len(rows) != n {
			b.Fatalf("rows = %d, want %d", len(rows), n)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(n), "ns/row") //nolint:forbidigo // benchmark metric only, not money
}

func BenchmarkDerive_10k(b *testing.B)  { benchmarkDerive(b, 10_000) }
func BenchmarkDerive_50k(b *testing.B)  { benchmarkDerive(b, 50_000) }
func BenchmarkDerive_100k(b *testing.B) { benchmarkDerive(b, 100_000) }

// TestDerive_Perf10kWithinBudget asserts the P4-T5 budget: deriving 10k
// rows takes < 2 s and allocates < 200 MB in total (an upper bound on the
// peak heap growth, since the GC may reclaim some of it during the run).
func TestDerive_Perf10kWithinBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("perf check skipped in -short")
	}
	rs := perfRuleSet(t)
	inputs := perfInputs(perfRows)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	rows, issues := Derive(inputs, rs)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	require.Len(t, rows, perfRows)
	status := map[DeriveStatus]int{}
	for i := range rows {
		status[rows[i].Status]++
	}
	require.Positive(t, status[DeriveOK])
	require.Positive(t, status[DeriveNoAX])

	allocated := after.TotalAlloc - before.TotalAlloc
	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("derive %d rows: %s, total alloc %.1f MiB, %d mallocs, heap growth %.1f MiB, issues %d, status %v",
		perfRows, elapsed, mib(allocated), after.Mallocs-before.Mallocs, mib(uint64(max(heapGrowth, 0))), len(issues), status)

	require.Less(t, elapsed, perfMaxDuration, "derive of %d rows exceeded the time budget", perfRows)
	require.Less(t, allocated, uint64(perfMaxBytes), "derive of %d rows exceeded the memory budget", perfRows)
}

func mib(b uint64) float64 { return float64(b) / (1 << 20) } //nolint:forbidigo // report only, not money
