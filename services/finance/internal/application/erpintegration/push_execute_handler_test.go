package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// --- fakes -----------------------------------------------------------------

// pushFake wraps memBatches with a PushStore-capable runner.
type pushFake struct {
	*memBatches
	std    map[int64][]domain.StdRow
	locked bool
	ax     map[int64]domain.AxComponents
	busy   bool // RunLocked refuses (G11 held)
}

func newPushFake() *pushFake {
	return &pushFake{memBatches: newMemBatches(), std: map[int64][]domain.StdRow{}, locked: true, ax: map[int64]domain.AxComponents{}}
}

func (f *pushFake) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	if f.busy {
		return domain.ErrConcurrentRun
	}
	return fn(ctx, &pushMemStore{memStore: memStore{m: f.memBatches, id: batchID}, f: f})
}

type pushMemStore struct {
	memStore
	f *pushFake
}

func (s *pushMemStore) IsPeriodLocked(context.Context, string, string) (bool, error) {
	return s.f.locked, nil
}

func (s *pushMemStore) ListStdRows(context.Context) ([]domain.StdRow, error) {
	return append([]domain.StdRow(nil), s.f.std[s.id]...), nil
}

func (s *pushMemStore) LoadAxComponents(_ context.Context, _ string, ids []int64) (map[int64]domain.AxComponents, error) {
	out := map[int64]domain.AxComponents{}
	for _, id := range ids {
		if a, ok := s.f.ax[id]; ok {
			out[id] = a
		}
	}
	return out, nil
}

// memCallLog is an in-memory OracleCallLog.
type memCallLog struct {
	starts   []domain.OracleCallStart
	finishes map[string]domain.OracleCallFinish
	prior    bool
	startErr error
}

func newMemCallLog() *memCallLog { return &memCallLog{finishes: map[string]domain.OracleCallFinish{}} }

func (l *memCallLog) HasAttempt(_ context.Context, batchID int64, key string) (bool, error) {
	if l.prior {
		return true, nil
	}
	for _, s := range l.starts {
		if s.BatchID == batchID && s.StatementKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (l *memCallLog) Start(_ context.Context, c domain.OracleCallStart) error {
	if l.startErr != nil {
		return l.startErr
	}
	l.starts = append(l.starts, c)
	return nil
}

func (l *memCallLog) Finish(_ context.Context, id string, f domain.OracleCallFinish) error {
	l.finishes[id] = f
	return nil
}

// --- fixtures --------------------------------------------------------------

// validatedBatch stores a VALIDATED LIVE batch whose totals, derive digest
// and validate summary match its std rows.
func validatedBatch(t *testing.T, f *pushFake, mode domain.BatchMode) int64 {
	t.Helper()
	rows := []domain.StdRow{
		vRow(vKeyAX, domain.SourceAX, "1.60000", 11),
		vRow(vKeyB, domain.SourceDerived, "1.40000", 11),
		vRow(vKeyMB, domain.SourceMB, "2.00000", 22),
	}
	bad := vRow(domain.ErpKey{ItemCode: "POY900", GradeCode: "AX", ShadeCode: "NL"}, domain.SourceAX, "9", 99)
	bad.Status = domain.DeriveNoRule
	rows = append(rows, bad)
	d, err := domain.ComputeStdDigest(rows)
	require.NoError(t, err)
	sum, err := json.Marshal(map[string]any{
		StepDerive:   map[string]any{"run_at": vDeriveAt, "rows_md5": d.RowsMD5, "digest_version": domain.StdDigestVersion},
		StepValidate: map[string]any{"validated": true, "rule_hash": vRuleHash1, "derive_run_at": vDeriveAt},
	})
	require.NoError(t, err)
	loaded := testNow
	tot := d.Totals
	id := f.put(domain.BatchState{
		Period: tPeriod, Mode: mode, Status: domain.StatusValidated, DemandLoadedAt: &loaded,
		RuleHash: vRuleHash1, Totals: &tot, Summary: sum, CreatedAt: testNow, UpdatedAt: testNow,
	})
	f.std[id] = rows
	f.ax[11] = domain.AxComponents{CostID: 11, Version: 1}
	f.ax[22] = domain.AxComponents{CostID: 22, Version: 1}
	return id
}

type pushEnv struct {
	f      *pushFake
	writer *oracle.FakeWriter
	calls  *memCallLog
	audit  *recordingAudit
	step   *PushStep
	id     int64
}

func newPushEnv(t *testing.T) *pushEnv {
	t.Helper()
	e := &pushEnv{f: newPushFake(), writer: oracle.NewFakeWriter(), calls: newMemCallLog(), audit: &recordingAudit{}}
	e.id = validatedBatch(t, e.f, domain.ModeLive)
	e.step = e.build(true, domain.WriterModeFake)
	return e
}

func (e *pushEnv) build(enabled bool, mode domain.WriterMode) *PushStep {
	return NewPushStep(e.f, WriterGate{Writer: e.writer, Mode: mode}, e.calls, e.audit, enabled, time.Minute).
		WithClock(func() time.Time { return testNow })
}

func (e *pushEnv) cmd() PushCommand {
	n := int64(3)
	return PushCommand{BatchID: e.id, Actor: "approver", HasPermission: true, ConfirmRowCount: &n, ConfirmSumStd: "5.00000"}
}

// --- tests -----------------------------------------------------------------

func TestPushStep_HappyPath(t *testing.T) {
	e := newPushEnv(t)
	sum, err := e.step.Run(context.Background(), e.cmd(), nil)
	require.NoError(t, err)

	calls := e.writer.Calls()
	require.Len(t, calls, 1, "exactly one W1 call")
	assert.Equal(t, oracle.KeyW1InsertBatch, calls[0].Key)
	assert.Equal(t, 3, calls[0].Rows, "only OK rows are pushed")
	assert.Equal(t, 3, e.writer.RowCount(e.id))

	require.Len(t, e.calls.starts, 1)
	st := e.calls.starts[0]
	assert.Equal(t, domain.CallKeyW1InsertBatch, st.StatementKey)
	assert.Equal(t, e.id, st.BatchID)
	assert.NotContains(t, string(st.Params), "password")
	fin := e.calls.finishes[st.CallID]
	assert.Equal(t, domain.OracleCallSuccess, fin.Status)
	require.NotNil(t, fin.Rows)
	assert.Equal(t, int64(3), *fin.Rows)

	bs := e.f.state(e.id)
	assert.Equal(t, domain.StatusPushed, bs.Status)
	require.NotNil(t, bs.Pushed)
	assert.Equal(t, "approver", bs.Pushed.By)
	assert.Contains(t, e.f.summary(e.id), StepPush)
	assert.Equal(t, string(domain.OracleCallSuccess), sum.CallStatus)
	assert.Equal(t, int64(3), sum.CostRows)

	require.Len(t, e.audit.inputs, 1)
	assert.Equal(t, auditdomain.OpErpPush, e.audit.inputs[0].Operation)
	assert.Equal(t, e.id, e.audit.inputs[0].EntityID)
}

// TestPushStep_RefusalsMakeNoWriterCall is AC-06: every refused case
// returns a FailedPrecondition-class error with 0 writer calls and 0 call
// log rows, and leaves the batch VALIDATED.
func TestPushStep_RefusalsMakeNoWriterCall(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *pushEnv) PushCommand
		want  error
	}{
		{"flag off", func(e *pushEnv) PushCommand { e.step = e.build(false, domain.WriterModeFake); return e.cmd() }, domain.ErrFeatureDisabled},
		{"writer disabled", func(e *pushEnv) PushCommand {
			e.step = e.build(true, domain.WriterModeDisabled)
			return e.cmd()
		}, domain.ErrWriterNotConfigured},
		{"writer mode invalid", func(e *pushEnv) PushCommand { e.step = e.build(true, "bogus"); return e.cmd() }, domain.ErrWriterNotConfigured},
		{"no permission", func(e *pushEnv) PushCommand { c := e.cmd(); c.HasPermission = false; return c }, ErrPushPermissionDenied},
		{"no lock (G11 held)", func(e *pushEnv) PushCommand { e.f.busy = true; return e.cmd() }, domain.ErrConcurrentRun},
		{"period not locked", func(e *pushEnv) PushCommand { e.f.locked = false; return e.cmd() }, domain.ErrPeriodNotLocked},
		{"not validated", func(e *pushEnv) PushCommand {
			st := e.f.state(e.id)
			st.Status = domain.StatusDerived
			e.f.put(st)
			return e.cmd()
		}, ErrStepNotAllowed},
		{"shadow", func(e *pushEnv) PushCommand {
			st := e.f.state(e.id)
			st.Mode = domain.ModeShadow
			e.f.put(st)
			return e.cmd()
		}, domain.ErrShadowNotPushable},
		{"needs repush", func(e *pushEnv) PushCommand {
			st := e.f.state(e.id)
			st.NeedsRepush = true
			e.f.put(st)
			return e.cmd()
		}, domain.ErrNeedsRepush},
		{"confirm row count mismatch", func(e *pushEnv) PushCommand { c := e.cmd(); n := int64(4); c.ConfirmRowCount = &n; return c }, domain.ErrConfirmMismatch},
		{"confirm sum mismatch", func(e *pushEnv) PushCommand { c := e.cmd(); c.ConfirmSumStd = "5.00001"; return c }, domain.ErrConfirmMismatch},
		{"confirm missing", func(e *pushEnv) PushCommand { c := e.cmd(); c.ConfirmRowCount = nil; return c }, domain.ErrConfirmMismatch},
		{"rows changed since derive", func(e *pushEnv) PushCommand {
			e.f.std[e.id][0].StdCost = nd("1.70000")
			return e.cmd()
		}, domain.ErrPreviewStale},
		{"stale source cost (R-9)", func(e *pushEnv) PushCommand { delete(e.f.ax, 22); return e.cmd() }, ErrStaleSourceCost},
		{"source cost re-versioned (R-9)", func(e *pushEnv) PushCommand {
			e.f.ax[11] = domain.AxComponents{CostID: 11, Version: 2}
			return e.cmd()
		}, ErrStaleSourceCost},
		{"earlier W1 attempt (no retry)", func(e *pushEnv) PushCommand { e.calls.prior = true; return e.cmd() }, ErrPushAlreadyAttempted},
		{"validate stale for derive", func(e *pushEnv) PushCommand {
			st := e.f.state(e.id)
			var m map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(st.Summary, &m))
			m[StepValidate] = json.RawMessage(`{"validated":true,"rule_hash":"` + vRuleHash1 + `"}`)
			st.Summary, _ = json.Marshal(m)
			e.f.put(st)
			return e.cmd()
		}, domain.ErrValidationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPushEnv(t)
			cmd := tc.setup(e)
			before := e.f.state(e.id).Status
			_, err := e.step.Run(context.Background(), cmd, nil)
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, e.writer.Calls(), "0 writer calls")
			assert.Empty(t, e.calls.starts, "0 call-log rows")
			assert.Equal(t, before, e.f.state(e.id).Status, "batch status unchanged")
			assert.Empty(t, e.audit.inputs)
		})
	}
}

func TestPushStep_AlreadyPushedIsRefused(t *testing.T) {
	e := newPushEnv(t)
	_, err := e.step.Run(context.Background(), e.cmd(), nil)
	require.NoError(t, err)
	_, err = e.step.Run(context.Background(), e.cmd(), nil)
	require.ErrorIs(t, err, ErrStepNotAllowed)
	assert.Len(t, e.writer.Calls(), 1, "no second W1 call")
	assert.Len(t, e.calls.starts, 1)
}

func TestPushStep_OracleFailures(t *testing.T) {
	cases := []struct {
		name    string
		inject  func(w *oracle.FakeWriter)
		status  domain.OracleCallStatus
		oraCode string
	}{
		{"app error", func(w *oracle.FakeWriter) { w.InjectOraCode(oracle.KeyW1InsertBatch, domain.OraPeriodFrozen) }, domain.OracleCallFailed, "ORA-20901"},
		{"outcome unknown", func(w *oracle.FakeWriter) { w.Inject(oracle.KeyW1InsertBatch, domain.ErrOutcomeUnknown) }, domain.OracleCallUnknown, ""},
		{"busy", func(w *oracle.FakeWriter) { w.InjectBusy(oracle.KeyW1InsertBatch, 1) }, domain.OracleCallFailed, "ORA-00054"},
		{"timeout", func(w *oracle.FakeWriter) { w.Inject(oracle.KeyW1InsertBatch, domain.ErrOracleTimeout) }, domain.OracleCallFailed, ""},
		{"unclassified", func(w *oracle.FakeWriter) { w.Inject(oracle.KeyW1InsertBatch, errors.New("driver: bad connection")) }, domain.OracleCallUnknown, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPushEnv(t)
			tc.inject(e.writer)
			sum, err := e.step.Run(context.Background(), e.cmd(), nil)
			require.Error(t, err)
			require.Len(t, e.writer.Calls(), 1, "one attempt, no retry")
			require.Len(t, e.calls.starts, 1)
			fin := e.calls.finishes[e.calls.starts[0].CallID]
			assert.Equal(t, tc.status, fin.Status)
			assert.Equal(t, tc.oraCode, fin.OraCode)
			assert.Equal(t, string(tc.status), sum.CallStatus)
			bs := e.f.state(e.id)
			assert.Equal(t, domain.StatusFailed, bs.Status)
			assert.Contains(t, bs.Error, string(tc.status))
			require.Len(t, e.audit.inputs, 1)

			// No retry with the same batch id.
			_, err = e.step.Run(context.Background(), e.cmd(), nil)
			require.Error(t, err)
			assert.Len(t, e.writer.Calls(), 1)
		})
	}
}

func TestPushStep_CallLogStartFailureMakesNoWriterCall(t *testing.T) {
	e := newPushEnv(t)
	e.calls.startErr = errors.New("pg down")
	_, err := e.step.Run(context.Background(), e.cmd(), nil)
	require.Error(t, err)
	assert.Empty(t, e.writer.Calls(), "§S-R11: no call without its STARTED row")
	assert.Equal(t, domain.StatusValidated, e.f.state(e.id).Status)
}

func TestPushStep_NilCallLogFailsClosed(t *testing.T) {
	e := newPushEnv(t)
	s := NewPushStep(e.f, WriterGate{Writer: e.writer, Mode: domain.WriterModeFake}, nil, nil, true, 0)
	_, err := s.Run(context.Background(), e.cmd(), nil)
	require.ErrorIs(t, err, ErrCallLogNotConfigured)
	assert.Empty(t, e.writer.Calls())
}

func TestPushStep_DisabledWriterFactoryDefault(t *testing.T) {
	e := newPushEnv(t)
	s := NewPushStep(e.f, WriterGate{Writer: oracle.NewDisabledWriter(), Mode: domain.WriterModeDisabled}, e.calls, nil, true, 0)
	_, err := s.Run(context.Background(), e.cmd(), nil)
	require.ErrorIs(t, err, domain.ErrWriterNotConfigured)
	assert.Empty(t, e.calls.starts)
}

func TestJobExecutor_DispatchesPush(t *testing.T) {
	e := newPushEnv(t)
	jobs := newMemJobs()
	n := int64(3)
	raw, err := json.Marshal(JobParams{BatchID: e.id, ConfirmRowCount: &n, ConfirmSumStd: "5.00000", PushPermitted: true})
	require.NoError(t, err)
	exec := newTestJob(t, jobs, StepPush, raw)
	ex := NewJobExecutor(jobs, e.f, nil, nil).WithPush(e.step)
	require.NoError(t, ex.Execute(context.Background(), exec.ID()))
	assert.Len(t, e.writer.Calls(), 1)
	assert.Equal(t, domain.StatusPushed, e.f.state(e.id).Status)
	require.Len(t, e.calls.starts, 1)
	assert.NotEmpty(t, e.calls.starts[0].JobID)
}

func TestJobExecutor_PushWithoutPermitIsRefused(t *testing.T) {
	e := newPushEnv(t)
	jobs := newMemJobs()
	raw, err := json.Marshal(JobParams{BatchID: e.id})
	require.NoError(t, err)
	exec := newTestJob(t, jobs, StepPush, raw)
	ex := NewJobExecutor(jobs, e.f, nil, nil).WithPush(e.step)
	require.ErrorIs(t, ex.Execute(context.Background(), exec.ID()), ErrPushPermissionDenied)
	assert.Empty(t, e.writer.Calls())
}

func newTestJob(t *testing.T, jobs *memJobs, step string, params json.RawMessage) *job.Execution {
	t.Helper()
	exec, err := job.NewExecution(job.TypeErpIntegration, step, tPeriod, "approver", 5, params)
	require.NoError(t, err)
	require.NoError(t, jobs.Create(context.Background(), exec))
	return exec
}
