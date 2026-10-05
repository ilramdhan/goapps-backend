package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// --- fakes -----------------------------------------------------------------

// reconFake is a ReconStore-capable runner over memBatches.
type reconFake struct {
	*memBatches
	std     []domain.StdRow
	applied []domain.ReconRow
}

func (f *reconFake) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	return fn(ctx, &reconMemStore{memStore: memStore{m: f.memBatches, id: batchID}, f: f})
}

type reconMemStore struct {
	memStore
	f *reconFake
}

func (s *reconMemStore) ListStdRows(context.Context) ([]domain.StdRow, error) {
	return append([]domain.StdRow(nil), s.f.std...), nil
}

func (s *reconMemStore) ApplyRecon(_ context.Context, rows []domain.ReconRow) (int64, error) {
	s.f.applied = append([]domain.ReconRow(nil), rows...)
	return int64(len(rows)), nil
}

// fakeReconReader serves canned read-back data.
type fakeReconReader struct {
	batch    domain.OracleBatchReadBack
	adj      []domain.AdjReadBackCombo
	err      error
	reads    int
	adjReads int
}

func (r *fakeReconReader) ReadBackBatch(context.Context, int64) (domain.OracleBatchReadBack, error) {
	r.reads++
	return r.batch, r.err
}

func (r *fakeReconReader) ReadBackAdj(context.Context, string, int64) ([]domain.AdjReadBackCombo, error) {
	r.adjReads++
	return r.adj, r.err
}

// memResolver is an in-memory OracleCallResolver.
type memResolver struct {
	recs     []domain.OracleCallRecord
	resolved map[string]domain.OracleCallStatus
}

func (m *memResolver) ListUnresolved(context.Context, int64) ([]domain.OracleCallRecord, error) {
	var out []domain.OracleCallRecord
	for _, r := range m.recs {
		if _, done := m.resolved[r.CallID]; !done {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memResolver) Resolve(_ context.Context, id string, to domain.OracleCallStatus, _ json.RawMessage) error {
	if _, done := m.resolved[id]; done {
		return domain.ErrCallAlreadyResolved
	}
	m.resolved[id] = to
	return nil
}

// --- env -------------------------------------------------------------------

const reconBatchID = 1

var reconRuleHash = sha256Hex("recon-rules")

type reconEnv struct {
	f      *reconFake
	reader *fakeReconReader
	res    *memResolver
	writer *oracle.FakeWriter // never handed to the step; asserts 0 writes
	id     int64
}

func reconStd(code, std string) domain.StdRow {
	r := stdOK(code, std)
	r.Source = domain.SourceAX
	return r
}

// newReconEnv builds a VALUATED batch with two OK std rows whose Oracle
// header, cost rows and ADJ read-back all match (a clean recon).
func newReconEnv(t *testing.T, status domain.BatchStatus) *reconEnv {
	t.Helper()
	std := []domain.StdRow{reconStd("POY001", "1.20000"), reconStd("POY002", "2.50000")}
	digest, err := domain.ComputeStdDigest(std)
	require.NoError(t, err)
	totals := digest.Totals
	e := &reconEnv{
		f:      &reconFake{memBatches: newMemBatches(), std: std},
		reader: &fakeReconReader{},
		res:    &memResolver{resolved: map[string]domain.OracleCallStatus{}},
		writer: oracle.NewFakeWriter(),
	}
	e.id = e.f.put(domain.BatchState{
		ID: reconBatchID, Period: tPeriod, Status: status, RuleHash: reconRuleHash, Totals: &totals,
		CreatedAt: testNow, UpdatedAt: testNow,
	})
	e.reader.batch = domain.OracleBatchReadBack{
		Found: true, BatchID: e.id, Period: tPeriod, Seq: 1, Status: domain.GsbValuated, RuleHash: reconRuleHash,
		Header: totals, CostRows: append([]domain.StdRow(nil), std...),
	}
	e.reader.adj = []domain.AdjReadBackCombo{
		adjCombo("POY001", "1.2", 2, 2, 1),
		adjCombo("POY002", "2.500004", 1, 1, 1),
	}
	return e
}

func adjCombo(code, rate string, items, stamped, variants int64) domain.AdjReadBackCombo {
	return domain.AdjReadBackCombo{
		Key: domain.ErpKey{ItemCode: code, GradeCode: "A", ShadeCode: "S1"}, Items: items,
		RateVariants: variants, MaxRate: nd(rate), QtyKg: nd("1"), Value: nd("1.2"),
		Flex13: "1", Stamped: stamped,
	}
}

func (e *reconEnv) step() *ReconStep {
	return NewReconStep(e.f, e.reader, e.res).WithClock(func() time.Time { return testNow })
}

func (e *reconEnv) run(t *testing.T) (ReconSummary, error) {
	t.Helper()
	sum, err := e.step().Run(context.Background(), e.id, "costing", nil)
	assert.Empty(t, e.writer.Calls(), "recon never calls the writer")
	return sum, err
}

func (e *reconEnv) statusOf(key string) domain.ReconStatus {
	for _, r := range e.f.applied {
		if r.Key.ItemCode == key {
			return r.Status
		}
	}
	return ""
}

// --- tests -----------------------------------------------------------------

func TestRecon_AllMatchReconciles(t *testing.T) {
	e := newReconEnv(t, domain.StatusValuated)
	sum, err := e.run(t)
	require.NoError(t, err)
	assert.True(t, sum.Reconciled, "reasons: %v", sum.Reasons)
	assert.True(t, sum.TotalsEqual)
	assert.True(t, sum.MD5Equal)
	assert.Equal(t, 2, sum.Counts.Match)
	assert.Equal(t, domain.StatusReconciled, e.f.state(e.id).Status)
	require.NotNil(t, e.f.state(e.id).ReconciledAt)
	assert.Equal(t, domain.ReconMatch, e.statusOf("POY002"), "C-12: ROUND(rate,5) = std")
	assert.Contains(t, e.f.summary(e.id), StepRecon)
}

func TestRecon_EachStatusFixture(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(e *reconEnv)
		key    string
		want   domain.ReconStatus
		reason string
	}{
		{"DIFF rate", func(e *reconEnv) { e.reader.adj[0].MaxRate = nd("1.3") }, "POY001", domain.ReconDiff, "DIFF"},
		{"DIFF two rates", func(e *reconEnv) { e.reader.adj[0].RateVariants = 2 }, "POY001", domain.ReconDiff, "DIFF"},
		{"DIFF not stamped", func(e *reconEnv) { e.reader.adj[0].Stamped = 1 }, "POY001", domain.ReconDiff, "DIFF"},
		{"DIFF other batch flex", func(e *reconEnv) { e.reader.adj[0].Flex13 = "9" }, "POY001", domain.ReconDiff, "DIFF"},
		{"NOT_IN_ADJ", func(e *reconEnv) { e.reader.adj = e.reader.adj[:1] }, "POY002", domain.ReconNotInAdj, "NOT_IN_ADJ"},
		{"NOT_COVERED", func(e *reconEnv) {
			e.reader.adj = append(e.reader.adj, adjCombo("POY999", "1", 3, 0, 1))
		}, "POY001", domain.ReconMatch, "NOT_COVERED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newReconEnv(t, domain.StatusValuated)
			tc.mut(e)
			sum, err := e.run(t)
			require.NoError(t, err)
			assert.False(t, sum.Reconciled)
			assert.Equal(t, tc.want, e.statusOf(tc.key))
			assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status, "stays VALUATED")
			assert.Contains(t, e.f.state(e.id).Error, tc.reason)
		})
	}
	t.Run("NOT_COVERED counted", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		e.reader.adj = append(e.reader.adj, adjCombo("POY999", "1", 3, 0, 1))
		sum, err := e.run(t)
		require.NoError(t, err)
		assert.Equal(t, int64(3), sum.Counts.NotCoveredItems)
		assert.Equal(t, []string{"POY999/A/S1"}, sum.NotCovered)
	})
}

func TestRecon_TotalsAndMD5Mismatch(t *testing.T) {
	t.Run("md5", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		e.reader.batch.CostRows[1].ConvCost = nd("0.1")
		sum, err := e.run(t)
		require.NoError(t, err)
		assert.False(t, sum.MD5Equal)
		assert.False(t, sum.Reconciled)
	})
	t.Run("header totals", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		bad, err := domain.NewControlTotals(3, dec("1"), dec("0"), dec("0"))
		require.NoError(t, err)
		e.reader.batch.Header = bad
		sum, err := e.run(t)
		require.NoError(t, err)
		assert.False(t, sum.TotalsEqual)
		assert.False(t, sum.Reconciled)
	})
	t.Run("not in oracle", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		e.reader.batch = domain.OracleBatchReadBack{BatchID: e.id}
		sum, err := e.run(t)
		require.NoError(t, err)
		assert.False(t, sum.Reconciled)
		assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	})
}

func TestRecon_ReReconDiffMovesBackToValuated(t *testing.T) {
	e := newReconEnv(t, domain.StatusReconciled)
	e.reader.adj[1].MaxRate = nd("2.6")
	sum, err := e.run(t)
	require.NoError(t, err)
	assert.False(t, sum.Reconciled)
	assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	assert.Nil(t, e.f.state(e.id).ReconciledAt)
}

func TestRecon_Refusals(t *testing.T) {
	t.Run("no reader", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		_, err := NewReconStep(e.f, nil, e.res).Run(context.Background(), e.id, "a", nil)
		require.ErrorIs(t, err, ErrReconReaderNotConfigured)
	})
	t.Run("no resolver", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		_, err := NewReconStep(e.f, e.reader, nil).Run(context.Background(), e.id, "a", nil)
		require.ErrorIs(t, err, ErrCallLogNotConfigured)
	})
	for _, st := range []domain.BatchStatus{domain.StatusValidated, domain.StatusLocked, domain.StatusDraft} {
		t.Run(string(st), func(t *testing.T) {
			e := newReconEnv(t, st)
			_, err := e.run(t)
			require.ErrorIs(t, err, ErrStepNotAllowed)
			assert.Zero(t, e.reader.reads, "no Oracle read before the status check")
		})
	}
	t.Run("shadow", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusFailed)
		st := e.f.state(e.id)
		st.Mode = domain.ModeShadow
		e.f.put(st)
		_, err := e.run(t)
		require.ErrorIs(t, err, domain.ErrShadowNotPushable)
	})
	t.Run("reader error", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		e.reader.err = errors.New("ora down")
		_, err := e.run(t)
		require.Error(t, err)
		assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	})
}

func TestRecon_ResolvesUnknownCalls(t *testing.T) {
	type rc = domain.OracleCallRecord
	cases := []struct {
		name   string
		status domain.BatchStatus
		mut    func(e *reconEnv)
		key    string
		want   domain.OracleCallStatus // "" = stays unresolved
	}{
		{"W1 committed", domain.StatusFailed, nil, domain.CallKeyW1InsertBatch, domain.OracleCallSuccess},
		{"W1 not committed", domain.StatusFailed, func(e *reconEnv) { e.reader.batch = domain.OracleBatchReadBack{BatchID: e.id} },
			domain.CallKeyW1InsertBatch, domain.OracleCallFailed},
		{"W1 partial rows ambiguous", domain.StatusFailed, func(e *reconEnv) { e.reader.batch.CostRows = e.reader.batch.CostRows[:1] },
			domain.CallKeyW1InsertBatch, ""},
		{"VALUATE committed", domain.StatusPushed, nil, domain.CallKeyW2ValuateAdj, domain.OracleCallSuccess},
		{"VALUATE not committed", domain.StatusPushed, func(e *reconEnv) {
			e.reader.batch.Status = domain.GsbPushed
			for i := range e.reader.adj {
				e.reader.adj[i].Stamped = 0
			}
		}, domain.CallKeyW2ValuateAdj, domain.OracleCallFailed},
		{"VALUATE status vs stamps contradict", domain.StatusPushed, func(e *reconEnv) {
			e.reader.batch.Status = domain.GsbPushed // stamps still present
		}, domain.CallKeyW2ValuateAdj, ""},
		{"LOCK committed", domain.StatusReconciled, func(e *reconEnv) { e.reader.batch.Status = domain.GsbLocked },
			domain.CallKeyW2LockBatch, domain.OracleCallSuccess},
		{"W2 no header", domain.StatusPushed, func(e *reconEnv) { e.reader.batch = domain.OracleBatchReadBack{BatchID: e.id} },
			domain.CallKeyW2ValuateAdj, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newReconEnv(t, tc.status)
			if tc.mut != nil {
				tc.mut(e)
			}
			e.res.recs = []rc{{CallID: "c1", StatementKey: tc.key, Status: domain.OracleCallUnknown}}
			sum, err := e.run(t)
			require.NoError(t, err)
			got, resolved := e.res.resolved["c1"]
			if tc.want == "" {
				assert.False(t, resolved)
				assert.Equal(t, 1, sum.Unresolved)
			} else {
				assert.Equal(t, tc.want, got)
				assert.Zero(t, sum.Unresolved)
			}
			if tc.status == domain.StatusPushed || tc.status == domain.StatusFailed {
				assert.Equal(t, reconModeResolveOnly, sum.Mode)
				assert.Equal(t, tc.status, e.f.state(e.id).Status, "resolve-only never moves the batch")
				assert.Empty(t, e.f.applied)
			}
		})
	}
	t.Run("unresolved blocks RECONCILED", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		e.reader.batch.Status = domain.GsbPushed // VALUATE stamps present, header disagrees
		e.res.recs = []rc{{CallID: "c1", StatementKey: domain.CallKeyW2ValuateAdj, Status: domain.OracleCallStarted}}
		sum, err := e.run(t)
		require.NoError(t, err)
		assert.False(t, sum.Reconciled)
		assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	})
}

func TestJobExecutor_DispatchesRecon(t *testing.T) {
	e := newReconEnv(t, domain.StatusValuated)
	jobs := newMemJobs()
	raw, err := json.Marshal(JobParams{BatchID: e.id})
	require.NoError(t, err)
	exec := newTestJob(t, jobs, StepRecon, raw)
	ex := NewJobExecutor(jobs, e.f, nil, nil).WithRecon(e.step())
	require.NoError(t, ex.Execute(context.Background(), exec.ID()))
	assert.Equal(t, domain.StatusReconciled, e.f.state(e.id).Status)

	t.Run("not wired", func(t *testing.T) {
		e := newReconEnv(t, domain.StatusValuated)
		exec := newTestJob(t, jobs, StepRecon, raw)
		ex := NewJobExecutor(jobs, e.f, nil, nil)
		require.ErrorIs(t, ex.Execute(context.Background(), exec.ID()), ErrReconReaderNotConfigured)
	})
}

func TestStepTrigger_Recon(t *testing.T) {
	m := newMemBatches()
	jobs := newMemJobs()
	pub := &fakePublisher{}
	trig := NewStepTriggerHandler(jobs, m, pub, 0)
	ok := m.put(domain.BatchState{Period: tPeriod, Status: domain.StatusValuated, CreatedAt: testNow, UpdatedAt: testNow})
	_, err := trig.Handle(context.Background(), StepTriggerCommand{BatchID: ok, Step: StepRecon, Actor: "a"})
	require.NoError(t, err)
	assert.Len(t, pub.calls, 1)

	bad := m.put(domain.BatchState{Period: "202606", Status: domain.StatusValidated, CreatedAt: testNow, UpdatedAt: testNow})
	_, err = trig.Handle(context.Background(), StepTriggerCommand{BatchID: bad, Step: StepRecon, Actor: "a"})
	require.ErrorIs(t, err, ErrStepNotAllowed)
}
