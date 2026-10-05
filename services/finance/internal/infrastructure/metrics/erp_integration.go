// Package metrics holds the finance-service Prometheus collectors for the ERP
// cost integration (plan-06 P5-T8; design Part 3 §15). Collectors register in
// the default registry via promauto, so the existing /metrics endpoint
// (httpdelivery gateway) serves them.
//
// Two complementary sources feed them:
//
//   - InstrumentedCallLog decorates the Oracle call log (§S-R11) and counts
//     every write call in-process (calls by key/status/ora_code, duration,
//     UNKNOWN outcomes) with a structured log line per call.
//   - ErpStatsSampler periodically reads PG truth (batch counts per status,
//     call-log rows per key/status, stale STARTED calls, stale OPEN previews)
//     into gauges. It survives restarts and works from a process whose own
//     counters are not scraped (the worker has no metrics port), so alerts
//     are built on it.
//
// Observability only: nothing here talks to Oracle, and no bind value, name,
// price or credential is ever used as a label or logged.
package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// labelNone is used for an empty ora_code label.
const labelNone = "none"

// oracleCallBuckets covers a fast insert up to a long package call (the W2
// call timeout is minutes, not seconds).
var oracleCallBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}

// In-process call collectors (fed by InstrumentedCallLog).
var (
	// OracleCallsTotal counts terminal Oracle write calls.
	OracleCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "erp_oracle_calls_total",
		Help: "ERP Oracle write calls by allowlist key, terminal status and ORA code.",
	}, []string{"key", "status", "ora_code"})

	// OracleCallDuration observes STARTED -> terminal duration per key.
	OracleCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "erp_oracle_call_duration_seconds",
		Help:    "Duration of ERP Oracle write calls (STARTED to terminal) by allowlist key.",
		Buckets: oracleCallBuckets,
	}, []string{"key"})

	// UnknownOutcomeTotal counts calls whose outcome could not be determined.
	UnknownOutcomeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "erp_unknown_outcome_total",
		Help: "ERP Oracle write calls that ended UNKNOWN (outcome must be reconciled, never re-pushed).",
	}, []string{"key"})
)

// PG-derived gauges (fed by ErpStatsSampler).
var (
	// BatchStatus is the number of ERP batches per status and mode.
	BatchStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "erp_batch_status",
		Help: "Number of ERP integration batches per status and mode (sampled from PG).",
	}, []string{"status", "mode"})

	// OracleCallLogRows is the number of call-log rows per key and status.
	OracleCallLogRows = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "erp_oracle_call_log_rows",
		Help: "Rows in cst_erp_oracle_call_log per allowlist key and status (sampled from PG).",
	}, []string{"key", "status"})

	// OracleCallStartedOldestAge is the age of the oldest STARTED call.
	OracleCallStartedOldestAge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "erp_oracle_call_started_oldest_age_seconds",
		Help: "Age in seconds of the oldest STARTED Oracle call-log row (0 when none).",
	})

	// PreviewStaleOpen is the number of OPEN previews past their TTL.
	PreviewStaleOpen = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "erp_valuation_preview_stale_open",
		Help: "Valuation previews still OPEN after cevp_expires_at, per operation (sampled from PG).",
	}, []string{"operation"})

	// PreviewOpenOldestAge is the age of the oldest OPEN preview.
	PreviewOpenOldestAge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "erp_valuation_preview_open_oldest_age_seconds",
		Help: "Age in seconds of the oldest OPEN valuation preview (0 when none).",
	})

	// SampleErrorsTotal counts failed PG samples.
	SampleErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "erp_metrics_sample_errors_total",
		Help: "Failed ERP observability samples from PG.",
	})
)

// Known label domains, so a value that disappears from PG is reset to 0
// instead of keeping its last sample.
var (
	callKeys = []string{
		"W1_INSERT_BATCH", "W1_INSERT_COST",
		"W2_VALUATE_ADJ", "W2_APPROVE_ADJ", "W2_RESTORE_ADJ", "W2_LOCK_BATCH",
	}
	callStatuses = []domain.OracleCallStatus{
		domain.OracleCallStarted, domain.OracleCallSuccess, domain.OracleCallFailed, domain.OracleCallUnknown,
	}
	batchModes         = []domain.BatchMode{domain.ModeLive, domain.ModeShadow}
	previewOperations  = []string{"VALUATE", "APPROVE", "RESTORE"}
	defaultSampleEvery = time.Minute
)

// ---------------------------------------------------------------------------
// InstrumentedCallLog
// ---------------------------------------------------------------------------

type callStart struct {
	key     string
	batchID int64
	at      time.Time
}

// InstrumentedCallLog wraps a domain.OracleCallLog: it delegates every call
// unchanged and, on a successful Finish, records the call metrics and a
// structured log line. A failure of the inner log is returned as-is and is
// not counted (the call log row is the source of truth).
type InstrumentedCallLog struct {
	inner domain.OracleCallLog
	now   func() time.Time

	mu      sync.Mutex
	started map[string]callStart
}

var _ domain.OracleCallLog = (*InstrumentedCallLog)(nil)

// NewInstrumentedCallLog decorates inner. A nil inner returns nil so the
// caller's "call log not configured" refusal still fires.
func NewInstrumentedCallLog(inner domain.OracleCallLog) domain.OracleCallLog {
	if inner == nil {
		return nil
	}
	return &InstrumentedCallLog{inner: inner, now: time.Now, started: map[string]callStart{}}
}

// HasAttempt delegates.
func (l *InstrumentedCallLog) HasAttempt(ctx context.Context, batchID int64, statementKey string) (bool, error) {
	return l.inner.HasAttempt(ctx, batchID, statementKey)
}

// Start delegates and remembers the start time of the call.
func (l *InstrumentedCallLog) Start(ctx context.Context, c domain.OracleCallStart) error {
	if err := l.inner.Start(ctx, c); err != nil {
		return err
	}
	l.mu.Lock()
	l.started[c.CallID] = callStart{key: c.StatementKey, batchID: c.BatchID, at: l.now()}
	l.mu.Unlock()
	log.Info().Str("call_id", c.CallID).Int64("batch_id", c.BatchID).Str("job_id", c.JobID).
		Str("statement_key", c.StatementKey).Int("attempt", c.Attempt).Msg("erp oracle call started")
	return nil
}

// Finish delegates, then records metrics for the terminal status.
func (l *InstrumentedCallLog) Finish(ctx context.Context, callID string, f domain.OracleCallFinish) error {
	if err := l.inner.Finish(ctx, callID, f); err != nil {
		return err
	}
	l.mu.Lock()
	st, ok := l.started[callID]
	delete(l.started, callID)
	l.mu.Unlock()

	key := st.key
	if !ok || key == "" {
		key = "unknown"
	}
	ora := strings.TrimSpace(f.OraCode)
	if ora == "" {
		ora = labelNone
	}
	OracleCallsTotal.WithLabelValues(key, string(f.Status), ora).Inc()
	if ok {
		OracleCallDuration.WithLabelValues(key).Observe(l.now().Sub(st.at).Seconds())
	}

	ev := log.Info()
	switch f.Status {
	case domain.OracleCallUnknown:
		UnknownOutcomeTotal.WithLabelValues(key).Inc()
		ev = log.Error()
	case domain.OracleCallFailed:
		ev = log.Warn()
	case domain.OracleCallStarted, domain.OracleCallSuccess:
	}
	ev = ev.Str("call_id", callID).Int64("batch_id", st.batchID).Str("statement_key", key).
		Str("status", string(f.Status)).Str("ora_code", strings.TrimSpace(f.OraCode))
	if f.Rows != nil {
		ev = ev.Int64("rows", *f.Rows)
	}
	ev.Msg("erp oracle call finished")
	return nil
}

// ---------------------------------------------------------------------------
// ErpStatsSampler
// ---------------------------------------------------------------------------

// BatchStatusCount is one (status, mode) batch count.
type BatchStatusCount struct {
	Status string
	Mode   string
	Count  int64
}

// CallLogCount is one (key, status) call-log row count.
type CallLogCount struct {
	Key    string
	Status string
	Count  int64
}

// ErpStats is one PG sample.
type ErpStats struct {
	Batches []BatchStatusCount
	Calls   []CallLogCount
	// OldestStartedAge is the age of the oldest STARTED call (0 when none).
	OldestStartedAge time.Duration
	// StaleOpenPreviews counts OPEN previews past their TTL per operation.
	StaleOpenPreviews map[string]int64
	// OldestOpenPreviewAge is the age of the oldest OPEN preview (0 when none).
	OldestOpenPreviewAge time.Duration
}

// ErpStatsSource reads one sample from PG (read-only aggregates).
type ErpStatsSource interface {
	ErpStats(ctx context.Context) (ErpStats, error)
}

// ErpStatsSampler copies ErpStatsSource samples into the gauges.
type ErpStatsSampler struct {
	src      ErpStatsSource
	interval time.Duration
}

// NewErpStatsSampler builds a sampler; interval <= 0 means one minute.
func NewErpStatsSampler(src ErpStatsSource, interval time.Duration) *ErpStatsSampler {
	if interval <= 0 {
		interval = defaultSampleEvery
	}
	return &ErpStatsSampler{src: src, interval: interval}
}

// SampleOnce reads one sample and sets every gauge. Known label values that
// are absent from the sample are reset to 0. On error the gauges keep their
// previous values and the error counter is incremented.
func (s *ErpStatsSampler) SampleOnce(ctx context.Context) error {
	st, err := s.src.ErpStats(ctx)
	if err != nil {
		SampleErrorsTotal.Inc()
		return err
	}
	for _, status := range domain.AllBatchStatuses() {
		for _, mode := range batchModes {
			BatchStatus.WithLabelValues(string(status), string(mode)).Set(0)
		}
	}
	for _, b := range st.Batches {
		BatchStatus.WithLabelValues(b.Status, b.Mode).Set(float64(b.Count))
	}
	for _, k := range callKeys {
		for _, cs := range callStatuses {
			OracleCallLogRows.WithLabelValues(k, string(cs)).Set(0)
		}
	}
	for _, c := range st.Calls {
		OracleCallLogRows.WithLabelValues(c.Key, c.Status).Set(float64(c.Count))
	}
	OracleCallStartedOldestAge.Set(st.OldestStartedAge.Seconds())
	for _, op := range previewOperations {
		PreviewStaleOpen.WithLabelValues(op).Set(float64(st.StaleOpenPreviews[op]))
	}
	for op, n := range st.StaleOpenPreviews {
		PreviewStaleOpen.WithLabelValues(op).Set(float64(n))
	}
	PreviewOpenOldestAge.Set(st.OldestOpenPreviewAge.Seconds())
	return nil
}

// Run samples immediately and then every interval until ctx is done. A
// failed sample is logged and retried at the next tick.
func (s *ErpStatsSampler) Run(ctx context.Context) {
	sample := func() {
		if err := s.SampleOnce(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("erp metrics: PG sample failed")
		}
	}
	sample()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sample()
		}
	}
}
