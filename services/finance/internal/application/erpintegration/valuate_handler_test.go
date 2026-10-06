package erpintegration

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// --- fakes -----------------------------------------------------------------

// adjExecFake is an AdjExecStore-capable runner over memBatches.
type adjExecFake struct {
	*memBatches
	std     []domain.StdRow
	locked  bool
	busy    bool
	notLast bool
}

func (f *adjExecFake) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	if f.busy {
		return domain.ErrConcurrentRun
	}
	return fn(ctx, &adjMemStore{memStore: memStore{m: f.memBatches, id: batchID}, f: f})
}

type adjMemStore struct {
	memStore
	f *adjExecFake
}

func (s *adjMemStore) IsPeriodLocked(context.Context, string, string) (bool, error) {
	return s.f.locked, nil
}
func (s *adjMemStore) ListStdRows(context.Context) ([]domain.StdRow, error) {
	return append([]domain.StdRow(nil), s.f.std...), nil
}

func (s *adjMemStore) ActiveForUpdate(_ context.Context, period string) (*domain.Batch, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	for id, st := range s.m.batches {
		if id == s.id || st.Period != period || st.Mode != domain.ModeLive {
			continue
		}
		switch st.Status {
		case domain.StatusValuated, domain.StatusReconciled, domain.StatusLocked:
			return domain.ReconstituteBatch(st)
		}
	}
	return nil, domain.ErrBatchNotFound
}

func (s *adjMemStore) SaveOther(ctx context.Context, b *domain.Batch, expected domain.BatchStatus) error {
	if b.ID() == s.id {
		return errors.New("save other: locked batch")
	}
	return s.m.Save(ctx, b, expected, domain.Invalidation{})
}

func (s *adjMemStore) IsLatestBatch(context.Context, string) (bool, error) { return !s.f.notLast, nil }

// adjPreviewStore is an in-memory PreviewGetter + PreviewConsumer.
type adjPreviewStore struct {
	mu       sync.Mutex
	previews map[string]domain.ValuationPreview
	consumed int
}

func (p *adjPreviewStore) Get(_ context.Context, id string) (domain.ValuationPreview, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.previews[id]
	if !ok {
		return v, domain.ErrPreviewNotFound
	}
	return v, nil
}

func (p *adjPreviewStore) Consume(_ context.Context, id, actor string, at time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.previews[id]
	if !ok || v.Status != domain.PreviewOpen {
		return domain.ErrPreviewConsumed
	}
	v.Status, v.ConsumedAt, v.ConsumedBy = domain.PreviewConsumed, &at, actor
	p.previews[id] = v
	p.consumed++
	return nil
}

// adjOracle wraps the fake writer and applies the observable effect of a
// successful call to the rows the read-only reader returns.
type adjOracle struct {
	*oracle.FakeWriter
	reader *fakeSnapshotReader
}

func (o *adjOracle) ValuateAdj(ctx context.Context, batchID int64) (domain.Summary, error) {
	s, err := o.FakeWriter.ValuateAdj(ctx, batchID)
	if err == nil {
		o.stamp(batchID, func(r *domain.AdjSnapshotRow) { r.Flex[adjFlexBatchIdx] = strPtr(strconv.FormatInt(batchID, 10)) })
	}
	return s, err
}

func (o *adjOracle) ApproveAdj(ctx context.Context, batchID int64, uid string) (domain.Summary, error) {
	s, err := o.FakeWriter.ApproveAdj(ctx, batchID, uid)
	if err == nil {
		o.stamp(batchID, func(r *domain.AdjSnapshotRow) {
			if stampedBy(*r, batchID) {
				r.HeadApprStatus = approved()
			}
		})
	}
	return s, err
}

func (o *adjOracle) RestoreAdj(ctx context.Context, batchID int64) (domain.Summary, error) {
	s, err := o.FakeWriter.RestoreAdj(ctx, batchID)
	if err == nil {
		o.stamp(batchID, func(r *domain.AdjSnapshotRow) { r.Flex[adjFlexBatchIdx] = nil })
	}
	return s, err
}

func (o *adjOracle) stamp(_ int64, fn func(r *domain.AdjSnapshotRow)) {
	for i := range o.reader.rows {
		fn(&o.reader.rows[i])
	}
}

func strPtr(s string) *string { return &s }

// --- env -------------------------------------------------------------------

type adjEnv struct {
	f        *adjExecFake
	reader   *fakeSnapshotReader
	previews *adjPreviewStore
	writer   *oracle.FakeWriter
	ora      *adjOracle
	calls    *memCallLog
	audit    *recordingAudit
	cfg      AdjExecuteConfig
	mode     domain.WriterMode
	sleeps   []time.Duration
	direct   int // W2 calls made directly by the test setup
	id       int64
	now      time.Time
}

func newAdjEnv(t *testing.T, status domain.BatchStatus) *adjEnv {
	t.Helper()
	e := &adjEnv{
		f:        &adjExecFake{memBatches: newMemBatches(), locked: true, std: []domain.StdRow{stdOK("POY001", "1.2")}},
		reader:   &fakeSnapshotReader{},
		previews: &adjPreviewStore{previews: map[string]domain.ValuationPreview{}},
		writer:   oracle.NewFakeWriter(),
		calls:    newMemCallLog(),
		audit:    &recordingAudit{},
		cfg:      AdjExecuteConfig{ValuationEnabled: true, AdjApproveEnabled: true, CallTimeout: time.Minute, BusyRetries: 3},
		mode:     domain.WriterModeFake,
		now:      previewT0.Add(time.Minute),
	}
	e.ora = &adjOracle{FakeWriter: e.writer, reader: e.reader}
	e.id = e.f.put(domain.BatchState{Period: tPeriod, Status: status, CreatedAt: testNow, UpdatedAt: testNow})
	_, err := e.writer.InsertBatch(context.Background(), domain.PushBatch{BatchID: e.id, Period: tPeriod}, nil)
	require.NoError(t, err)
	e.reader.rows = []domain.AdjSnapshotRow{
		adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1"),
		adjRow(11, 111, domain.TxnMbInvAdj, "POY001", "2000", "1.5", "3"),
	}
	return e
}

func (e *adjEnv) step() *AdjExecuteStep {
	return NewAdjExecuteStep(e.f, WriterGate{Writer: e.ora, Mode: e.mode}, e.calls, e.audit, e.previews, e.previews, e.reader, e.cfg).
		WithClock(func() time.Time { return e.now }).
		WithSleep(func(_ context.Context, d time.Duration) error { e.sleeps = append(e.sleeps, d); return nil })
}

// preview stores an OPEN preview of op built from the current rows.
func (e *adjEnv) preview(t *testing.T, op domain.AdjOperation) AdjExecuteCommand {
	t.Helper()
	spec, err := adjSpecFor(op)
	require.NoError(t, err)
	eligible, excl := domain.SplitAdjSnapshot(e.reader.rows)
	set, err := spec.selectSet(e.reader.rows, eligible, e.f.std, e.id)
	require.NoError(t, err)
	id := "prev-" + string(op) + "-" + strconv.Itoa(len(e.previews.previews))
	p := domain.ValuationPreview{
		ID: id, BatchID: e.id, Operation: op, Status: domain.PreviewOpen, Excluded: excl,
		SetHash: domain.AdjSetHash(set.rows), ConfirmText: domain.PreviewConfirmText(tPeriod, e.id),
		CreatedBy: "alice", CreatedAt: previewT0, ExpiresAt: previewT0.Add(10 * time.Minute),
	}
	e.previews.previews[id] = p
	return AdjExecuteCommand{
		BatchID: e.id, PreviewID: id, Operation: op, ConfirmSetHash: p.SetHash, ConfirmText: p.ConfirmText,
		Actor: "alice", JobID: "", HasPermission: true,
	}
}

func (e *adjEnv) w2Calls() int {
	n := 0
	for _, c := range e.writer.Calls() {
		if c.Key != oracle.KeyW1InsertBatch {
			n++
		}
	}
	return n
}

// assertPairs is AC-08: one STARTED + one terminal row per step writer
// call (direct test-setup calls on the fake writer are not counted).
func (e *adjEnv) assertPairs(t *testing.T) {
	t.Helper()
	assert.Len(t, e.calls.starts, e.w2Calls()-e.direct)
	for _, s := range e.calls.starts {
		fin, ok := e.calls.finishes[s.CallID]
		require.True(t, ok, "terminal row for %s", s.CallID)
		assert.NotEqual(t, domain.OracleCallStarted, fin.Status)
	}
}

// --- VALUATE ---------------------------------------------------------------

func TestAdjExecute_ValuateHappyPathSupersedes(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	prior := e.f.put(domain.BatchState{Period: tPeriod, Status: domain.StatusReconciled, CreatedAt: testNow, UpdatedAt: testNow})
	cmd := e.preview(t, domain.AdjOpValuate)

	sum, err := e.step().Run(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, e.w2Calls(), "one period-wide VALUATE_ADJ")
	assert.Equal(t, oracle.KeyW2ValuateAdj, e.writer.Calls()[1].Key)
	e.assertPairs(t)
	assert.Equal(t, domain.CallKeyW2ValuateAdj, e.calls.starts[0].StatementKey)
	assert.Equal(t, 1, e.calls.starts[0].Attempt)

	assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	require.NotNil(t, e.f.state(e.id).Valuated)
	assert.Equal(t, domain.StatusSuperseded, e.f.state(prior).Status)
	assert.Equal(t, prior, sum.SupersededBatch)
	assert.Equal(t, domain.PreviewConsumed, e.previews.previews[cmd.PreviewID].Status)
	assert.Contains(t, e.f.summary(e.id), "adj_valuate")
	assert.Equal(t, 2, sum.Heads)
	require.Len(t, e.audit.inputs, 1)
	assert.Equal(t, auditdomain.OpErpValuate, e.audit.inputs[0].Operation)
}

// TestAdjExecute_RerunIsAlreadyDone is AC-07: a re-run after success (fresh
// preview, batch reset to PUSHED) finalizes as ALREADY_DONE.
func TestAdjExecute_RerunIsAlreadyDone(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpValuate), nil)
	require.NoError(t, err)

	// PG lost the success (e.g. crash after the call): the batch is PUSHED,
	// Oracle already VALUATED. The writer reports ALREADY_DONE.
	st := e.f.state(e.id)
	st.Status, st.Valuated = domain.StatusPushed, nil
	e.f.put(st)
	stamped := e.reader.rows
	e.reader.rows = []domain.AdjSnapshotRow{
		adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1"),
		adjRow(11, 111, domain.TxnMbInvAdj, "POY001", "2000", "1.5", "3"),
	}
	cmd := e.preview(t, domain.AdjOpValuate)
	e.reader.rows = stamped

	sum, err := e.step().Run(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.True(t, sum.AlreadyDone)
	assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
	assert.Equal(t, 1, e.w2Calls(), "ALREADY_DONE detected by the re-probe: no second call")

	// The writer-side G9: the effect is not visible to the probe but Oracle
	// answers ALREADY_DONE.
	e2 := newAdjEnv(t, domain.StatusPushed)
	_, err = e2.writer.ValuateAdj(context.Background(), e2.id)
	require.NoError(t, err)
	e2.direct++
	sum, err = e2.step().Run(context.Background(), e2.preview(t, domain.AdjOpValuate), nil)
	require.NoError(t, err)
	assert.True(t, sum.AlreadyDone)
	assert.Equal(t, domain.StatusValuated, e2.f.state(e2.id).Status)
	e2.assertPairs(t)
}

func TestAdjExecute_PreviewSingleUse(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	cmd := e.preview(t, domain.AdjOpValuate)
	_, err := e.step().Run(context.Background(), cmd, nil)
	require.NoError(t, err)
	st := e.f.state(e.id)
	st.Status = domain.StatusPushed
	e.f.put(st)
	_, err = e.step().Run(context.Background(), cmd, nil)
	require.ErrorIs(t, err, domain.ErrPreviewConsumed)
	assert.Equal(t, 1, e.w2Calls())
}

// TestAdjExecute_SafetyRefusals is §S-T6: every refusal makes 0 writer
// calls, 0 call-log rows and leaves the batch status untouched.
func TestAdjExecute_SafetyRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, e *adjEnv) AdjExecuteCommand
		want  error
	}{
		{"no preview", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.PreviewID = "missing"
			return c
		}, domain.ErrPreviewRequired},
		{"empty preview id", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.PreviewID = ""
			return c
		}, domain.ErrPreviewRequired},
		{"expired preview", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			e.now = previewT0.Add(time.Hour)
			return c
		}, domain.ErrPreviewExpired},
		{"hash mismatch", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.ConfirmSetHash = "deadbeef"
			return c
		}, domain.ErrConfirmMismatch},
		{"wrong confirm text", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.ConfirmText = tPeriod + "/999"
			return c
		}, domain.ErrConfirmMismatch},
		{"preview already used", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			p := e.previews.previews[c.PreviewID]
			p.Status = domain.PreviewConsumed
			e.previews.previews[c.PreviewID] = p
			return c
		}, domain.ErrPreviewConsumed},
		{"preview stale", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			p := e.previews.previews[c.PreviewID]
			p.Status = domain.PreviewStale
			e.previews.previews[c.PreviewID] = p
			return c
		}, domain.ErrPreviewStale},
		{"preview of another operation", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.Operation = domain.AdjOpRestore
			return c
		}, domain.ErrPreviewRequired},
		{"flag off", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			e.cfg.ValuationEnabled = false
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrFeatureDisabled},
		{"writer disabled", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			e.mode = domain.WriterModeDisabled
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrWriterNotConfigured},
		{"no permission", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.HasPermission = false
			return c
		}, ErrValuatePermissionDenied},
		{"period not locked", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			e.f.locked = false
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrPeriodNotLocked},
		{"advisory lock held", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			e.f.busy = true
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrConcurrentRun},
		{"posted head", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			e.reader.rows[0].HeadPostStatus = posted()
			return c
		}, domain.ErrPeriodPosted},
		{"approved head", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			e.reader.rows[1].HeadApprStatus = approved()
			return c
		}, ErrAdjIneligible},
		{"V-07 fails", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			e.f.std = []domain.StdRow{stdOK("POY001", "25")}
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrValidationFailed},
		{"head changed (G6)", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			e.reader.rows[0].QtyBu = nd("1001")
			return c
		}, domain.ErrPreviewStale},
		{"shadow batch", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			st := e.f.state(e.id)
			st.Mode = domain.ModeShadow
			e.f.put(st)
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrShadowNotPushable},
		{"batch not pushed", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			st := e.f.state(e.id)
			st.Status = domain.StatusValidated
			e.f.put(st)
			return e.preview(t, domain.AdjOpValuate)
		}, ErrStepNotAllowed},
		{"needs repush", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			st := e.f.state(e.id)
			st.NeedsRepush = true
			e.f.put(st)
			return e.preview(t, domain.AdjOpValuate)
		}, domain.ErrNeedsRepush},
		{"no actor", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.Actor = " "
			return c
		}, domain.ErrActorRequired},
		{"unknown operation", func(t *testing.T, e *adjEnv) AdjExecuteCommand {
			c := e.preview(t, domain.AdjOpValuate)
			c.Operation = "DROP"
			return c
		}, ErrAdjOperationUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdjEnv(t, domain.StatusPushed)
			cmd := tc.setup(t, e)
			before := e.f.state(e.id).Status
			_, err := e.step().Run(context.Background(), cmd, nil)
			require.ErrorIs(t, err, tc.want)
			assert.Zero(t, e.w2Calls(), "0 writer calls")
			assert.Empty(t, e.calls.starts, "0 call-log rows")
			assert.Equal(t, before, e.f.state(e.id).Status)
			assert.Empty(t, e.audit.inputs)
		})
	}
}

func TestAdjExecute_BusyRetriesThenSucceeds(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	e.writer.InjectBusy(oracle.KeyW2ValuateAdj, 2)
	sum, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpValuate), nil)
	require.NoError(t, err)
	assert.Equal(t, 3, e.w2Calls())
	assert.Equal(t, 3, sum.Attempts)
	assert.Equal(t, []time.Duration{2 * time.Second, 4 * time.Second}, e.sleeps)
	e.assertPairs(t)
	for i, s := range e.calls.starts {
		assert.Equal(t, i+1, s.Attempt)
	}
	assert.Equal(t, domain.StatusValuated, e.f.state(e.id).Status)
}

func TestAdjExecute_BusyExhaustedKeepsPushed(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	e.writer.InjectBusy(oracle.KeyW2ValuateAdj, 10)
	sum, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpValuate), nil)
	require.ErrorIs(t, err, ErrAdjCallFailed)
	require.ErrorIs(t, err, domain.ErrOracleBusy)
	assert.Equal(t, 4, e.w2Calls(), "1 + 3 retries")
	assert.Equal(t, []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}, e.sleeps)
	e.assertPairs(t)
	assert.Equal(t, "ORA-00054", sum.OraCode)
	st := e.f.state(e.id)
	assert.Equal(t, domain.StatusPushed, st.Status, "BUSY: the batch stays PUSHED")
	assert.NotEmpty(t, st.Error)
	assert.Len(t, e.audit.inputs, 1)
}

func TestAdjExecute_AppErrorKeepsPushed(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	e.writer.FreezePeriod(tPeriod)
	sum, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpValuate), nil)
	require.ErrorIs(t, err, ErrAdjCallFailed)
	assert.Equal(t, 1, e.w2Calls(), "an application error is not retried")
	assert.Equal(t, "ORA-20901", sum.OraCode)
	assert.Equal(t, domain.StatusPushed, e.f.state(e.id).Status)
	assert.Equal(t, domain.OracleCallFailed, e.calls.finishes[e.calls.starts[0].CallID].Status)
}

func TestAdjExecute_UnknownOutcomeProbe(t *testing.T) {
	cases := []struct {
		name    string
		stamp   []int // row indexes stamped by the "committed" call
		want    error
		status  domain.BatchStatus
		outcome string
	}{
		{"all stamped -> OK", []int{0, 1}, nil, domain.StatusValuated, UnknownResolvedOK},
		{"none stamped -> FAILED", nil, ErrAdjCallFailed, domain.StatusPushed, UnknownResolvedFailed},
		{"partial -> UNKNOWN_PARTIAL", []int{0}, ErrAdjUnknownPartial, domain.StatusPushed, UnknownPartial},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdjEnv(t, domain.StatusPushed)
			cmd := e.preview(t, domain.AdjOpValuate)
			e.writer.Inject(oracle.KeyW2ValuateAdj, domain.ErrOutcomeUnknown)
			step := e.step()
			// The probe re-read sees the rows the call may have stamped.
			reads := 0
			e.reader.rows = append([]domain.AdjSnapshotRow(nil), e.reader.rows...)
			probe := &stampingReader{inner: e.reader, after: 1, apply: func(rows []domain.AdjSnapshotRow) {
				for _, i := range tc.stamp {
					rows[i].Flex[adjFlexBatchIdx] = strPtr(strconv.FormatInt(e.id, 10))
				}
			}, reads: &reads}
			step.reader = probe
			sum, err := step.Run(context.Background(), cmd, nil)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Equal(t, 1, e.w2Calls(), "an UNKNOWN outcome is never retried")
			assert.Equal(t, domain.OracleCallUnknown, e.calls.finishes[e.calls.starts[0].CallID].Status)
			require.NotNil(t, sum.Resolution)
			assert.Equal(t, tc.outcome, sum.Resolution.Outcome)
			assert.Equal(t, tc.status, e.f.state(e.id).Status)
			assert.Equal(t, 2, reads, "G6 re-probe + UNKNOWN probe")
		})
	}
}

// stampingReader applies apply to the rows from read number after+1 on.
type stampingReader struct {
	inner *fakeSnapshotReader
	after int
	apply func(rows []domain.AdjSnapshotRow)
	reads *int
}

func (r *stampingReader) SnapshotAdjRows(ctx context.Context, period string) ([]domain.AdjSnapshotRow, error) {
	*r.reads++
	rows, err := r.inner.SnapshotAdjRows(ctx, period)
	out := append([]domain.AdjSnapshotRow(nil), rows...)
	if *r.reads > r.after {
		r.apply(out)
	}
	return out, err
}

func TestAdjExecute_TimeoutIsUnknown(t *testing.T) {
	fin := classifyW2(domain.Summary{}, domain.ErrOracleTimeout)
	assert.Equal(t, domain.OracleCallUnknown, fin.Status)
	fin = classifyW2(domain.Summary{Text: "ALREADY_DONE batch=1"}, nil)
	assert.Equal(t, domain.OracleCallSuccess, fin.Status)
	assert.Contains(t, string(fin.Summary), "ALREADY_DONE")
	assert.True(t, isBusy(&domain.OracleAppError{Code: domain.OraResourceBusyWait}))
}

func TestAdjExecute_CancelledBeforeCall(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	cmd := e.preview(t, domain.AdjOpValuate)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.step().Run(ctx, cmd, nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, e.w2Calls())
	assert.Empty(t, e.calls.starts)
}

// --- APPROVE / RESTORE -----------------------------------------------------

// valuated moves env e through a successful VALUATE and sets the batch to
// status (RECONCILED by the recon step in production).
func valuated(t *testing.T, e *adjEnv, status domain.BatchStatus) {
	t.Helper()
	_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpValuate), nil)
	require.NoError(t, err)
	st := e.f.state(e.id)
	st.Status = status
	e.f.put(st)
}

func TestAdjExecute_Approve(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	valuated(t, e, domain.StatusReconciled)
	cmd := e.preview(t, domain.AdjOpApprove)
	_, err := e.step().Run(context.Background(), cmd, nil)
	require.NoError(t, err)
	st := e.f.state(e.id)
	assert.Equal(t, domain.StatusReconciled, st.Status, "D-D2: status stays RECONCILED")
	require.NotNil(t, st.AdjApproved)
	calls := e.writer.Calls()
	assert.Equal(t, oracle.KeyW2ApproveAdj, calls[len(calls)-1].Key)
	assert.Equal(t, "alice", calls[len(calls)-1].ApprUID)
	assert.Equal(t, auditdomain.OpErpAdjApprove, e.audit.inputs[len(e.audit.inputs)-1].Operation)
	e.assertPairs(t)
}

func TestAdjExecute_ApproveRefusals(t *testing.T) {
	t.Run("flag off", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusReconciled)
		e.cfg.AdjApproveEnabled = false
		_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpApprove), nil)
		require.ErrorIs(t, err, domain.ErrFeatureDisabled)
		assert.Equal(t, 1, e.w2Calls())
	})
	t.Run("not reconciled", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusValuated)
		_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpApprove), nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
		assert.Equal(t, 1, e.w2Calls())
	})
	t.Run("no permission", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusReconciled)
		c := e.preview(t, domain.AdjOpApprove)
		c.HasPermission = false
		_, err := e.step().Run(context.Background(), c, nil)
		require.ErrorIs(t, err, ErrAdjPermissionDenied)
		assert.Equal(t, 1, e.w2Calls())
	})
}

func TestAdjExecute_Restore(t *testing.T) {
	e := newAdjEnv(t, domain.StatusPushed)
	valuated(t, e, domain.StatusValuated)
	sum, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpRestore), nil)
	require.NoError(t, err)
	st := e.f.state(e.id)
	assert.Equal(t, domain.StatusFailed, st.Status)
	assert.Equal(t, restoreReason, st.Error)
	require.NotNil(t, sum.RestoreVerified)
	assert.True(t, *sum.RestoreVerified, "post-restore compare: no row carries the batch stamp")
	assert.Equal(t, auditdomain.OpErpRestore, e.audit.inputs[len(e.audit.inputs)-1].Operation)
	e.assertPairs(t)
}

func TestAdjExecute_RestoreRefusals(t *testing.T) {
	t.Run("not latest batch", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusValuated)
		e.f.notLast = true
		_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpRestore), nil)
		require.ErrorIs(t, err, ErrNotLatestBatch)
		assert.Equal(t, 1, e.w2Calls())
	})
	t.Run("pushed batch", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		_, err := e.step().Run(context.Background(), e.preview(t, domain.AdjOpRestore), nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
		assert.Zero(t, e.w2Calls())
	})
	t.Run("approved head", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusValuated)
		c := e.preview(t, domain.AdjOpRestore)
		e.reader.rows[0].HeadApprStatus = approved()
		_, err := e.step().Run(context.Background(), c, nil)
		require.ErrorIs(t, err, ErrAdjIneligible)
		assert.Equal(t, 1, e.w2Calls())
	})
}

// --- LOCK_BATCH ------------------------------------------------------------

func newLockStep(e *adjEnv, prober domain.ErpAdjHeadProber, enabled bool) *LockBatchStep {
	return NewLockBatchStep(e.f, WriterGate{Writer: e.ora, Mode: e.mode}, e.calls, e.audit, prober, enabled, time.Minute, 3).
		WithClock(func() time.Time { return e.now }).
		WithSleep(func(context.Context, time.Duration) error { return nil })
}

func TestLockBatch_HappyPathAndRefusals(t *testing.T) {
	ready := func(t *testing.T) *adjEnv {
		e := newAdjEnv(t, domain.StatusPushed)
		valuated(t, e, domain.StatusReconciled)
		_, err := e.writer.ApproveAdj(context.Background(), e.id, "alice")
		require.NoError(t, err)
		e.direct++
		return e
	}
	cmd := func(e *adjEnv) LockBatchCommand {
		return LockBatchCommand{BatchID: e.id, Actor: "alice", HasPermission: true}
	}

	e := ready(t)
	sum, err := newLockStep(e, &fakeProber{posted: 2}, true).Run(context.Background(), cmd(e), nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusLocked, e.f.state(e.id).Status)
	require.NotNil(t, e.f.state(e.id).Locked)
	assert.Equal(t, int64(2), sum.PostedHeads)
	assert.Equal(t, auditdomain.OpErpBatchLock, e.audit.inputs[len(e.audit.inputs)-1].Operation)
	e.assertPairs(t)

	refusals := []struct {
		name   string
		prober *fakeProber
		on     bool
		mut    func(e *adjEnv, c *LockBatchCommand)
		want   error
	}{
		{"not posted", &fakeProber{}, true, nil, ErrAdjNotPosted},
		{"flag off", &fakeProber{posted: 1}, false, nil, domain.ErrFeatureDisabled},
		{"no permission", &fakeProber{posted: 1}, true, func(_ *adjEnv, c *LockBatchCommand) { c.HasPermission = false }, ErrAdjPermissionDenied},
		{"period not locked", &fakeProber{posted: 1}, true, func(e *adjEnv, _ *LockBatchCommand) { e.f.locked = false }, domain.ErrPeriodNotLocked},
		{"not reconciled", &fakeProber{posted: 1}, true, func(e *adjEnv, _ *LockBatchCommand) {
			st := e.f.state(e.id)
			st.Status = domain.StatusValuated
			e.f.put(st)
		}, ErrStepNotAllowed},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			e := ready(t)
			c := cmd(e)
			if tc.mut != nil {
				tc.mut(e, &c)
			}
			_, err := newLockStep(e, tc.prober, tc.on).Run(context.Background(), c, nil)
			require.ErrorIs(t, err, tc.want)
			for _, call := range e.writer.Calls() {
				assert.NotEqual(t, oracle.KeyW2LockBatch, call.Key)
			}
		})
	}

	t.Run("ORA-20904 maps to not posted", func(t *testing.T) {
		e := ready(t)
		e.writer.InjectOraCode(oracle.KeyW2LockBatch, domain.OraCode20904)
		_, err := newLockStep(e, &fakeProber{posted: 1}, true).Run(context.Background(), cmd(e), nil)
		require.ErrorIs(t, err, ErrAdjNotPosted)
		assert.Equal(t, domain.StatusReconciled, e.f.state(e.id).Status)
		e.assertPairs(t)
	})
}

func TestResolveUnknown(t *testing.T) {
	stamped := adjRow(1, 1, domain.TxnInvAdj, "A", "1", "1", "1")
	stamped.Flex[adjFlexBatchIdx] = strPtr("7")
	plain := adjRow(1, 2, domain.TxnInvAdj, "A", "1", "1", "1")
	done := func(r domain.AdjSnapshotRow) bool { return stampedBy(r, 7) }
	rows := []domain.AdjSnapshotRow{stamped, plain}
	assert.Equal(t, UnknownResolvedOK, ResolveUnknown(rows, []int64{1}, done).Outcome)
	assert.Equal(t, UnknownResolvedFailed, ResolveUnknown(rows, []int64{2}, done).Outcome)
	assert.Equal(t, UnknownPartial, ResolveUnknown(rows, []int64{1, 2}, done).Outcome)
	assert.Equal(t, UnknownPartial, ResolveUnknown(rows, []int64{1, 3}, done).Outcome, "a missing item is not done")
	assert.Equal(t, UnknownResolvedFailed, ResolveUnknown(rows, nil, done).Outcome)
}
