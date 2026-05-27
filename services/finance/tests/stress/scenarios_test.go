//go:build stress

// S8e.2 — calc engine stress scenarios S1-S9 (per Phase C PRD §7.4).
//
// All scenarios are stubs that compile under `-tags=stress` but skip unless
// STRESS_TEST=true is set in the environment. The first real run produces the
// numbers committed back into baseline.json; subsequent runs assert no p95
// regression > 20% versus the baseline.
//
// To execute end-to-end on a populated stress corpus:
//
//	make seed-stress PRODUCTS=12000 PARAMS=150 FORMULAS=30
//	STRESS_TEST=true go test -tags=stress -timeout 30m ./tests/stress/...
//	make stress-clean
package stress

import (
	"testing"
	"time"
)

// recordPlaceholder is a helper used by every scenario stub. Real
// implementations will replace this with: (1) trigger a calc job via the local
// repo / worker harness, (2) poll job status until terminal, (3) collect per-
// chunk durations + outcome counts from cost_product_cost.
func recordPlaceholder(t *testing.T, scenario string, productCount int, notes string) {
	t.Helper()
	t.Logf("scenario %s: placeholder run with %d products — replace with real job trigger",
		scenario, productCount)

	// Emit a synthetic metrics row so the report pipeline + baseline plumbing
	// can be exercised end-to-end before the real engine is wired.
	chunkDurations := []float64{1, 1, 1}
	m := ScenarioMetrics{
		Scenario:     scenario,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		ProductCount: productCount,
		ChunkCount:   len(chunkDurations),
		P50ChunkMS:   percentile(chunkDurations, 50),
		P95ChunkMS:   percentile(chunkDurations, 95),
		P99ChunkMS:   percentile(chunkDurations, 99),
		TotalMS:      0,
		SuccessCount: productCount,
		Notes:        notes,
	}
	writeReport(t, m)
}

// S1 — single product, cold cache. Forces a full DAG walk by superseding any
// upstream cost_product_cost rows for the chosen product's transitive deps.
func TestStressS1_SingleProductCold(t *testing.T) {
	requireStress(t)
	const key = "s1_single_cold"
	recordPlaceholder(t, key, 1, "single FG, upstream forced cold")
	// TODO(s8e.2): pick STRESS_FG_xxxxx product, supersede upstream costs,
	// trigger calc, time chunks. Then:
	assertNoP95Regression(t, key, 0)
}

// S2 — single product, hot cache. Upstream computed → only the leaf chunk
// runs. Measures compute + persist latency for the simplest non-trivial case.
func TestStressS2_SingleProductHot(t *testing.T) {
	requireStress(t)
	const key = "s2_single_hot"
	recordPlaceholder(t, key, 1, "single FG, upstream already computed")
	assertNoP95Regression(t, key, 0)
}

// S3 — 500-product filtered batch. Validates small-batch perf, including
// scheduler overhead and worker pickup time.
func TestStressS3_FilteredBatch500(t *testing.T) {
	requireStress(t)
	const key = "s3_filtered_500"
	recordPlaceholder(t, key, 500, "filter: STRESS_FG_00001..00500")
	assertNoP95Regression(t, key, 0)
}

// S4 — canonical 12k full batch. This is the headline number. Target wall is
// ~5min with 50 workers, ~30min single-worker proportional.
func TestStressS4_Full12k(t *testing.T) {
	requireStress(t)
	const key = "s4_full_12k"
	recordPlaceholder(t, key, 12000, "all FG + intermediate products")
	assertNoP95Regression(t, key, 0)
}

// S5 — 2 concurrent batches. Validates that workers don't serialize on locks
// and that the scheduler honours per-product chunk uniqueness.
func TestStressS5_ConcurrentBatches(t *testing.T) {
	requireStress(t)
	const key = "s5_concurrent_2"
	recordPlaceholder(t, key, 24000, "two parallel 12k jobs, period A + B")
	assertNoP95Regression(t, key, 0)
}

// S6 — failure isolation. Inject 100 products with a synthetic CAPP gap. Engine
// must BLOCK those 100 and SUCCEED the remaining ~11_900 without cascading.
func TestStressS6_FailureIsolation(t *testing.T) {
	requireStress(t)
	const key = "s6_failure_isolation"
	recordPlaceholder(t, key, 12000, "100 BLOCKED via CAPP gap, 11_900 SUCCESS")
	assertNoP95Regression(t, key, 0)
}

// S7 — recompute storm. Re-run calc on the full 12k corpus with existing cost
// rows present. Validates that recompute path correctly supersedes prior rows
// and stays within wall budget.
func TestStressS7_RecomputeStorm(t *testing.T) {
	requireStress(t)
	const key = "s7_recompute_storm"
	recordPlaceholder(t, key, 12000, "all upstream already computed")
	assertNoP95Regression(t, key, 0)
}

// S8 — mid-batch cancel. Trigger 12k, sleep 30s, request cancel. All in-flight
// chunks must finish, queued chunks must drop, and final job state must be
// CANCELLED with partial cost rows persisted.
func TestStressS8_MidBatchCancel(t *testing.T) {
	requireStress(t)
	const key = "s8_mid_batch_cancel"
	recordPlaceholder(t, key, 12000, "cancel after 30s wall")
	assertNoP95Regression(t, key, 0)
}

// S9 — worker scale-up (HPA validation). Run a 12k batch while observing
// per-chunk wall before / after HPA reaction. Single-pod test mode records
// per-chunk timings only; real HPA validation belongs in the cluster.
func TestStressS9_WorkerScaleUp(t *testing.T) {
	requireStress(t)
	const key = "s9_worker_scale_up"
	recordPlaceholder(t, key, 12000, "single-pod here; observe per-chunk wall")
	assertNoP95Regression(t, key, 0)
}
