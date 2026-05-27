package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestCollectorsRegistered verifies that every finance_cost_* collector
// defined in metrics.go is registered with the default Prometheus registry.
// The check enumerates the registry rather than introspecting the collectors
// directly so we exercise the same path /metrics uses.
func TestCollectorsRegistered(t *testing.T) {
	// Force a non-zero observation on each labelled metric so it shows up in
	// the gather output (CounterVec / GaugeVec / HistogramVec only register
	// a series once a label combination is observed; the registry still lists
	// the family even with no observations, but with one we also confirm the
	// label cardinality is correct).
	JobsTotal.WithLabelValues("SUCCESS", "ACTUAL", "SINGLE_PRODUCT", "manual").Inc()
	ChunksTotal.WithLabelValues("SUCCESS").Inc()
	ProductsTotal.WithLabelValues("SUCCESS", "").Inc()
	RecomputeTotal.Inc()
	AuditWritesTotal.Inc()

	ChunkDurationSeconds.WithLabelValues("0").Observe(0.05)
	ProductComputeSeconds.Observe(0.05)
	FormulaEvalSeconds.WithLabelValues("F1").Observe(0.001)
	BulkLoadSeconds.WithLabelValues("routes").Observe(0.05)
	DBTxSeconds.WithLabelValues("upsert").Observe(0.05)

	WorkerActiveChunks.WithLabelValues("test-worker").Set(0)
	JobQueueDepth.Set(0)
	DBPoolInUse.WithLabelValues("finance").Set(0)
	EvalCacheEntries.Set(0)
	EvalCacheHitRatio.Set(0)
	RecordEvalCacheHit()
	RecordEvalCacheMiss()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := map[string]bool{}
	for _, f := range families {
		got[f.GetName()] = true
	}

	want := []string{
		"finance_cost_jobs_total",
		"finance_cost_chunks_total",
		"finance_cost_products_total",
		"finance_cost_recompute_total",
		"finance_cost_audit_writes_total",
		"finance_cost_chunk_duration_seconds",
		"finance_cost_product_compute_duration_seconds",
		"finance_cost_formula_eval_duration_seconds",
		"finance_cost_bulk_load_duration_seconds",
		"finance_cost_db_tx_duration_seconds",
		"finance_cost_worker_active_chunks",
		"finance_cost_job_queue_depth",
		"finance_cost_db_pool_in_use",
		"finance_cost_evalcache_entries",
		"finance_cost_evalcache_hit_ratio",
	}
	missing := []string{}
	for _, name := range want {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing collectors: %s", strings.Join(missing, ", "))
	}
	if len(want) < 15 {
		t.Fatalf("expected at least 15 finance_cost_* series, taxonomy: %d", len(want))
	}
}
