// Package metrics defines Prometheus metrics for the PPC service, all under the
// ppc_ namespace. Scraped via /metrics on the HTTP gateway (port 8082).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DBPoolInUse tracks the number of in-use database connections, scraped by a
	// background ticker in main.go for pool observability.
	DBPoolInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ppc_db_pool_in_use",
			Help: "Number of PPC database connections currently in use.",
		},
		[]string{"service"},
	)

	// ETLRunsTotal counts ETL worker runs by source and status.
	ETLRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_etl_runs_total",
			Help: "Total number of PPC ETL worker runs.",
		},
		[]string{"source", "status"},
	)

	// ETLErrorsTotal counts ETL errors by source.
	ETLErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_etl_error_total",
			Help: "Total number of PPC ETL errors.",
		},
		[]string{"source"},
	)

	// ETLDurationSeconds observes ETL run duration by source.
	ETLDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ppc_etl_duration_seconds",
			Help:    "PPC ETL worker run duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"source"},
	)

	// ETLRowsTotal counts ETL rows processed by source and outcome
	// (upserted/unmatched).
	ETLRowsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_etl_rows_total",
			Help: "Total number of PPC ETL rows processed.",
		},
		[]string{"source", "outcome"},
	)

	// EfficiencySnapshotsWritten counts efficiency snapshot rows written by scope.
	EfficiencySnapshotsWritten = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_efficiency_snapshots_written_total",
			Help: "Total number of PPC efficiency snapshot rows written.",
		},
		[]string{"scope"},
	)

	// WorkerRunsTotal counts background worker runs by worker name and status.
	WorkerRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_worker_runs_total",
			Help: "Total number of PPC background worker runs.",
		},
		[]string{"worker", "status"},
	)

	// MachineSyncRunsTotal counts machine-sync runs by status.
	MachineSyncRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_machine_sync_runs_total",
			Help: "Total number of PPC machine-sync runs.",
		},
		[]string{"status"},
	)

	// MachineSyncUpsertsTotal counts machine rows upserted by outcome
	// (inserted/updated/skipped).
	MachineSyncUpsertsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_machine_sync_upserts_total",
			Help: "Total number of PPC machine rows upserted during sync.",
		},
		[]string{"outcome"},
	)

	// WOAutoApproveRunsTotal counts auto-approve worker runs by status.
	WOAutoApproveRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_wo_autoapprove_runs_total",
			Help: "Total number of PPC WO auto-approve worker runs.",
		},
		[]string{"status"},
	)

	// WOAutoApproveActionsTotal counts WO approval actions taken by the
	// auto-approve worker by kind (pc/pm/approved).
	WOAutoApproveActionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ppc_wo_autoapprove_actions_total",
			Help: "Total number of PPC WO auto-approve actions taken.",
		},
		[]string{"kind"},
	)
)

// Status label constants for worker/ETL metrics.
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// Status returns the success/failure label for a boolean outcome.
func Status(success bool) string {
	if success {
		return StatusSuccess
	}
	return StatusFailure
}
