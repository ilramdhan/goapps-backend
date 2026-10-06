package metrics

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// fakeCallLog is an in-memory domain.OracleCallLog.
type fakeCallLog struct {
	startErr, finishErr error
	starts              []domain.OracleCallStart
	finishes            []domain.OracleCallFinish
}

func (f *fakeCallLog) HasAttempt(context.Context, int64, string) (bool, error) {
	return len(f.starts) > 0, nil
}

func (f *fakeCallLog) Start(_ context.Context, c domain.OracleCallStart) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.starts = append(f.starts, c)
	return nil
}

func (f *fakeCallLog) Finish(_ context.Context, _ string, fin domain.OracleCallFinish) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finishes = append(f.finishes, fin)
	return nil
}

func TestMetrics_Registered(t *testing.T) {
	// Touch every vector so it is gathered.
	OracleCallsTotal.WithLabelValues("W1_INSERT_BATCH", "SUCCESS", labelNone)
	OracleCallDuration.WithLabelValues("W1_INSERT_BATCH")
	UnknownOutcomeTotal.WithLabelValues("W1_INSERT_BATCH")
	BatchStatus.WithLabelValues("DRAFT", "LIVE")
	OracleCallLogRows.WithLabelValues("W1_INSERT_BATCH", "STARTED")
	PreviewStaleOpen.WithLabelValues("VALUATE")

	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	for _, n := range []string{
		"erp_oracle_calls_total", "erp_oracle_call_duration_seconds", "erp_unknown_outcome_total",
		"erp_batch_status", "erp_oracle_call_log_rows", "erp_oracle_call_started_oldest_age_seconds",
		"erp_valuation_preview_stale_open", "erp_valuation_preview_open_oldest_age_seconds",
		"erp_metrics_sample_errors_total",
	} {
		assert.True(t, names[n], "metric %s registered", n)
	}
}

func TestInstrumentedCallLog_NilInner(t *testing.T) {
	assert.Nil(t, NewInstrumentedCallLog(nil))
}

func TestInstrumentedCallLog_CountsTerminalCalls(t *testing.T) {
	ctx := context.Background()
	inner := &fakeCallLog{}
	l, ok := NewInstrumentedCallLog(inner).(*InstrumentedCallLog)
	require.True(t, ok)
	t0 := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	clock := t0
	l.now = func() time.Time { return clock }

	const key = "W2_VALUATE_ADJ"
	succ0 := value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "SUCCESS", "ora_code": labelNone})
	fail0 := value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "FAILED", "ora_code": "ORA-20901"})
	unk0 := value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "UNKNOWN", "ora_code": labelNone})
	unkOut0 := value(t, "erp_unknown_outcome_total", map[string]string{"key": key})

	has, err := l.HasAttempt(ctx, 7, key)
	require.NoError(t, err)
	assert.False(t, has)

	rows := int64(3)
	require.NoError(t, l.Start(ctx, domain.OracleCallStart{CallID: "c1", BatchID: 7, StatementKey: key}))
	clock = t0.Add(2 * time.Second)
	require.NoError(t, l.Finish(ctx, "c1", domain.OracleCallFinish{Status: domain.OracleCallSuccess, Rows: &rows}))

	require.NoError(t, l.Start(ctx, domain.OracleCallStart{CallID: "c2", BatchID: 7, StatementKey: key}))
	require.NoError(t, l.Finish(ctx, "c2", domain.OracleCallFinish{Status: domain.OracleCallFailed, OraCode: " ORA-20901 "}))

	require.NoError(t, l.Start(ctx, domain.OracleCallStart{CallID: "c3", BatchID: 7, StatementKey: key}))
	require.NoError(t, l.Finish(ctx, "c3", domain.OracleCallFinish{Status: domain.OracleCallUnknown}))

	assert.InDelta(t, succ0+1, value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "SUCCESS", "ora_code": labelNone}), 0)
	assert.InDelta(t, fail0+1, value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "FAILED", "ora_code": "ORA-20901"}), 0)
	assert.InDelta(t, unk0+1, value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "UNKNOWN", "ora_code": labelNone}), 0)
	assert.InDelta(t, unkOut0+1, value(t, "erp_unknown_outcome_total", map[string]string{"key": key}), 0)
	assert.Len(t, inner.starts, 3, "every call is delegated")
	assert.Len(t, inner.finishes, 3)
	assert.Empty(t, l.started, "start bookkeeping is released on finish")

	// The histogram saw the 2s call.
	sum, count := histSum(t, "erp_oracle_call_duration_seconds", map[string]string{"key": key})
	assert.GreaterOrEqual(t, count, uint64(3))
	assert.GreaterOrEqual(t, sum, 2.0, "the 2s call was observed")
}

func TestInstrumentedCallLog_InnerErrorsNotCounted(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("pg down")
	const key = "W2_LOCK_BATCH"
	before := value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "SUCCESS", "ora_code": labelNone})

	l := NewInstrumentedCallLog(&fakeCallLog{startErr: boom})
	require.ErrorIs(t, l.Start(ctx, domain.OracleCallStart{CallID: "x", StatementKey: key}), boom)

	l2 := NewInstrumentedCallLog(&fakeCallLog{finishErr: boom})
	require.NoError(t, l2.Start(ctx, domain.OracleCallStart{CallID: "y", StatementKey: key}))
	require.ErrorIs(t, l2.Finish(ctx, "y", domain.OracleCallFinish{Status: domain.OracleCallSuccess}), boom)

	assert.InDelta(t, before, value(t, "erp_oracle_calls_total", map[string]string{"key": key, "status": "SUCCESS", "ora_code": labelNone}), 0)
}

func TestInstrumentedCallLog_FinishWithoutStart(t *testing.T) {
	ctx := context.Background()
	before := value(t, "erp_oracle_calls_total", map[string]string{"key": "unknown", "status": "SUCCESS", "ora_code": labelNone})
	l := NewInstrumentedCallLog(&fakeCallLog{})
	require.NoError(t, l.Finish(ctx, "orphan", domain.OracleCallFinish{Status: domain.OracleCallSuccess}))
	assert.InDelta(t, before+1, value(t, "erp_oracle_calls_total", map[string]string{"key": "unknown", "status": "SUCCESS", "ora_code": labelNone}), 0)
}

type fakeStats struct {
	st  ErpStats
	err error
	n   atomic.Int64
}

func (f *fakeStats) ErpStats(context.Context) (ErpStats, error) {
	f.n.Add(1)
	return f.st, f.err
}

func TestErpStatsSampler_SetsAndResetsGauges(t *testing.T) {
	ctx := context.Background()
	src := &fakeStats{st: ErpStats{
		Batches: []BatchStatusCount{{Status: "VALIDATED", Mode: "LIVE", Count: 2}, {Status: "DERIVED", Mode: "SHADOW", Count: 1}},
		Calls: []CallLogCount{
			{Key: "W1_INSERT_BATCH", Status: "SUCCESS", Count: 4},
			{Key: "W2_VALUATE_ADJ", Status: "UNKNOWN", Count: 1},
		},
		OldestStartedAge:     90 * time.Second,
		StaleOpenPreviews:    map[string]int64{"VALUATE": 1},
		OldestOpenPreviewAge: 45 * time.Minute,
	}}
	s := NewErpStatsSampler(src, 0)
	assert.Equal(t, time.Minute, s.interval)
	require.NoError(t, s.SampleOnce(ctx))

	assert.InDelta(t, 2, value(t, "erp_batch_status", map[string]string{"status": "VALIDATED", "mode": "LIVE"}), 0)
	assert.InDelta(t, 1, value(t, "erp_batch_status", map[string]string{"status": "DERIVED", "mode": "SHADOW"}), 0)
	assert.InDelta(t, 0, value(t, "erp_batch_status", map[string]string{"status": "PUSHED", "mode": "LIVE"}), 0)
	assert.InDelta(t, 4, value(t, "erp_oracle_call_log_rows", map[string]string{"key": "W1_INSERT_BATCH", "status": "SUCCESS"}), 0)
	assert.InDelta(t, 1, value(t, "erp_oracle_call_log_rows", map[string]string{"key": "W2_VALUATE_ADJ", "status": "UNKNOWN"}), 0)
	assert.InDelta(t, 90, value(t, "erp_oracle_call_started_oldest_age_seconds", nil), 0)
	assert.InDelta(t, 1, value(t, "erp_valuation_preview_stale_open", map[string]string{"operation": "VALUATE"}), 0)
	assert.InDelta(t, 0, value(t, "erp_valuation_preview_stale_open", map[string]string{"operation": "APPROVE"}), 0)
	assert.InDelta(t, 2700, value(t, "erp_valuation_preview_open_oldest_age_seconds", nil), 0)

	// Next sample: everything cleared -> known labels reset to 0.
	src.st = ErpStats{}
	require.NoError(t, s.SampleOnce(ctx))
	assert.InDelta(t, 0, value(t, "erp_batch_status", map[string]string{"status": "VALIDATED", "mode": "LIVE"}), 0)
	assert.InDelta(t, 0, value(t, "erp_oracle_call_log_rows", map[string]string{"key": "W2_VALUATE_ADJ", "status": "UNKNOWN"}), 0)
	assert.InDelta(t, 0, value(t, "erp_valuation_preview_stale_open", map[string]string{"operation": "VALUATE"}), 0)
	assert.InDelta(t, 0, value(t, "erp_oracle_call_started_oldest_age_seconds", nil), 0)
}

func TestErpStatsSampler_ErrorKeepsGauges(t *testing.T) {
	ctx := context.Background()
	src := &fakeStats{st: ErpStats{Batches: []BatchStatusCount{{Status: "PUSHED", Mode: "LIVE", Count: 3}}}}
	s := NewErpStatsSampler(src, time.Hour)
	require.NoError(t, s.SampleOnce(ctx))
	errs0 := value(t, "erp_metrics_sample_errors_total", nil)

	src.err = errors.New("pg down")
	require.Error(t, s.SampleOnce(ctx))
	assert.InDelta(t, errs0+1, value(t, "erp_metrics_sample_errors_total", nil), 0)
	assert.InDelta(t, 3, value(t, "erp_batch_status", map[string]string{"status": "PUSHED", "mode": "LIVE"}), 0)
}

func TestErpStatsSampler_RunStopsOnCancel(t *testing.T) {
	src := &fakeStats{}
	s := NewErpStatsSampler(src, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	require.Eventually(t, func() bool { return src.n.Load() >= 2 }, 2*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}

// find returns the gathered metric with exactly the given labels.
func find(t *testing.T, name string, labels map[string]string) (*float64, *histVal) {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if len(m.GetLabel()) != len(labels) {
				continue
			}
			match := true
			for _, lp := range m.GetLabel() {
				if labels[lp.GetName()] != lp.GetValue() {
					match = false
				}
			}
			if !match {
				continue
			}
			switch {
			case m.GetCounter() != nil:
				v := m.GetCounter().GetValue()
				return &v, nil
			case m.GetGauge() != nil:
				v := m.GetGauge().GetValue()
				return &v, nil
			case m.GetHistogram() != nil:
				return nil, &histVal{sum: m.GetHistogram().GetSampleSum(), count: m.GetHistogram().GetSampleCount()}
			}
		}
	}
	return nil, nil
}

type histVal struct {
	sum   float64
	count uint64
}

// value returns a counter/gauge value (0 when the series does not exist).
func value(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	v, _ := find(t, name, labels)
	if v == nil {
		return 0
	}
	return *v
}

func histSum(t *testing.T, name string, labels map[string]string) (float64, uint64) {
	t.Helper()
	_, h := find(t, name, labels)
	require.NotNil(t, h, "histogram %s present", name)
	return h.sum, h.count
}
