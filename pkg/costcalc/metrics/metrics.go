// Package metrics defines Prometheus collectors shared by the cost-calc engine
// across the finance, finance-cost-orchestrator, and finance-cost-worker
// services. Each service imports this package so the collectors register
// themselves (via promauto) into the default registry and the existing
// /metrics endpoint serves them.
//
// Metric taxonomy follows Phase C plan §8.6.
package metrics

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// chunkDurationBuckets is the shared latency bucket layout for the heavier
// engine timings (chunk processing, single product compute, bulk loaders, db
// transactions). FormulaEvalSeconds uses a finer set since formulas are
// typically sub-millisecond.
var chunkDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30}

// Counters.
var (
	// JobsTotal increments once per terminal job state transition.
	JobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "finance_cost_jobs_total",
			Help: "Total cost calc jobs by terminal status.",
		},
		[]string{"status", "calc_type", "scope", "triggered_by"},
	)

	// ChunksTotal increments once per chunk terminal state transition.
	ChunksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "finance_cost_chunks_total",
			Help: "Total chunks by terminal status.",
		},
		[]string{"status"},
	)

	// ProductsTotal increments once per JobProduct terminal state. block_reason
	// is empty for non-BLOCKED outcomes.
	ProductsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "finance_cost_products_total",
			Help: "Total job products by terminal status and block reason.",
		},
		[]string{"status", "block_reason"},
	)

	// RecomputeTotal increments when UpsertWithSupersede finds a previous row
	// (the new result supersedes an active row).
	RecomputeTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "finance_cost_recompute_total",
		Help: "Cost results that superseded a previous version.",
	})

	// AuditWritesTotal increments per aud_cost_history row written.
	AuditWritesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "finance_cost_audit_writes_total",
		Help: "Audit history rows written.",
	})
)

// Histograms.
var (
	// ChunkDurationSeconds observes wall-clock time to process a chunk,
	// labelled by wave number.
	ChunkDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "finance_cost_chunk_duration_seconds",
			Help:    "Chunk wall-clock processing time.",
			Buckets: chunkDurationBuckets,
		},
		[]string{"wave_no"},
	)

	// ProductComputeSeconds observes wall-clock time of one ComputeProduct.
	ProductComputeSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "finance_cost_product_compute_duration_seconds",
		Help:    "Single product compute time.",
		Buckets: chunkDurationBuckets,
	})

	// FormulaEvalSeconds observes per-formula evaluation latency.
	FormulaEvalSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "finance_cost_formula_eval_duration_seconds",
			Help:    "Per-formula evaluation latency.",
			Buckets: []float64{0.0001, 0.001, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"formula_code"},
	)

	// BulkLoadSeconds observes bulk loader latency, labelled by kind
	// (routes / capp / formulas / rmcosts / upstream).
	BulkLoadSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "finance_cost_bulk_load_duration_seconds",
			Help:    "Bulk loader latency by kind.",
			Buckets: chunkDurationBuckets,
		},
		[]string{"kind"},
	)

	// DBTxSeconds observes per-phase DB transaction latency
	// (supersede / insert / audit / upsert).
	DBTxSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "finance_cost_db_tx_duration_seconds",
			Help:    "Per-phase DB transaction latency.",
			Buckets: chunkDurationBuckets,
		},
		[]string{"phase"},
	)
)

// Gauges.
var (
	// WorkerActiveChunks reports the number of chunks currently being
	// processed by a worker, keyed by worker id.
	WorkerActiveChunks = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "finance_cost_worker_active_chunks",
			Help: "Chunks currently in-flight on the worker.",
		},
		[]string{"worker_id"},
	)

	// JobQueueDepth reports the depth of the finance.cost.chunk RabbitMQ
	// queue (scraped by the orchestrator).
	JobQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "finance_cost_job_queue_depth",
		Help: "Depth of the finance.cost.chunk queue.",
	})

	// DBPoolInUse reports database connections currently in use, labelled by
	// service (finance / orchestrator / worker).
	DBPoolInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "finance_cost_db_pool_in_use",
			Help: "Database connections currently in use.",
		},
		[]string{"service"},
	)

	// EvalCacheEntries reports the size of the compiled-formula cache.
	EvalCacheEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "finance_cost_evalcache_entries",
		Help: "Compiled-formula cache size.",
	})

	// EvalCacheHitRatio is the cumulative hit ratio of the compiled-formula
	// cache. Updated via RecordEvalCacheHit / RecordEvalCacheMiss.
	EvalCacheHitRatio = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "finance_cost_evalcache_hit_ratio",
		Help: "Cumulative cache hits / (hits + misses) for the compiled formula cache.",
	})
)

// Cumulative counters backing EvalCacheHitRatio. Public functions update them
// atomically; the gauge is recomputed each time.
var (
	evalCacheHits   atomic.Uint64
	evalCacheMisses atomic.Uint64
)

// RecordEvalCacheHit increments the evaluator cache hit counter and updates
// the hit-ratio gauge.
func RecordEvalCacheHit() {
	hits := evalCacheHits.Add(1)
	misses := evalCacheMisses.Load()
	updateHitRatio(hits, misses)
}

// RecordEvalCacheMiss increments the evaluator cache miss counter and updates
// the hit-ratio gauge.
func RecordEvalCacheMiss() {
	hits := evalCacheHits.Load()
	misses := evalCacheMisses.Add(1)
	updateHitRatio(hits, misses)
}

func updateHitRatio(hits, misses uint64) {
	total := hits + misses
	if total == 0 {
		EvalCacheHitRatio.Set(0)
		return
	}
	EvalCacheHitRatio.Set(float64(hits) / float64(total))
}
