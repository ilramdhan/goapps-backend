package erpintegration

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

func TestPushPreview_ReturnsTotalsWithoutWriter(t *testing.T) {
	f := newPushFake()
	id := validatedBatch(t, f, domain.ModeLive)
	h := NewPushPreviewHandler(f, false, domain.WriterModeDisabled)
	p, err := h.Handle(context.Background(), PushPreviewQuery{BatchID: id, HasPermission: true})
	require.NoError(t, err)
	assert.Equal(t, id, p.BatchID)
	assert.Equal(t, tPeriod, p.Period)
	assert.Equal(t, int64(3), p.RowCount, "only OK rows")
	assert.Equal(t, "5.00000", p.SumStd)
	assert.Equal(t, "1.50000", p.SumConv)
	assert.Equal(t, "0.30000", p.SumPvl)
	assert.Len(t, p.RowsMD5, 32)
	assert.Equal(t, vRuleHash1, p.RuleHash)
	assert.Equal(t, domain.StdDigestVersion, p.DigestVersion)
	assert.False(t, p.PushEnabled)
	assert.Equal(t, "disabled", p.WriterMode)
	assert.Equal(t, domain.StatusValidated, f.state(id).Status, "preview is read-only")
	assert.Zero(t, f.saves, "preview never saves")
}

func TestPushPreview_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *pushFake, id int64)
		perm  bool
		want  error
	}{
		{"no permission", func(*pushFake, int64) {}, false, ErrPushPermissionDenied},
		{"period not locked", func(f *pushFake, _ int64) { f.locked = false }, true, domain.ErrPeriodNotLocked},
		{"no lock", func(f *pushFake, _ int64) { f.busy = true }, true, domain.ErrConcurrentRun},
		{"shadow", func(f *pushFake, id int64) { st := f.state(id); st.Mode = domain.ModeShadow; f.put(st) }, true, domain.ErrShadowNotPushable},
		{"not validated", func(f *pushFake, id int64) { st := f.state(id); st.Status = domain.StatusDerived; f.put(st) }, true, ErrStepNotAllowed},
		{"stale rows", func(f *pushFake, id int64) { f.std[id] = f.std[id][:2] }, true, domain.ErrPreviewStale},
		{"digest missing", func(f *pushFake, id int64) {
			st := f.state(id)
			var m map[string]json.RawMessage
			_ = json.Unmarshal(st.Summary, &m)
			delete(m, StepDerive)
			m[StepValidate] = json.RawMessage(`{"validated":true,"rule_hash":"` + vRuleHash1 + `"}`)
			st.Summary, _ = json.Marshal(m)
			f.put(st)
		}, true, domain.ErrPreviewStale},
		{"R-9 stale cost", func(f *pushFake, _ int64) { delete(f.ax, 11) }, true, ErrStaleSourceCost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushFake()
			id := validatedBatch(t, f, domain.ModeLive)
			tc.setup(f, id)
			_, err := NewPushPreviewHandler(f, true, domain.WriterModeFake).
				Handle(context.Background(), PushPreviewQuery{BatchID: id, HasPermission: tc.perm})
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestGates(t *testing.T) {
	assert.ErrorIs(t, checkFlag(false, "x"), domain.ErrFeatureDisabled)
	assert.NoError(t, checkFlag(true, "x"))
	w := oracle.NewFakeWriter()
	assert.ErrorIs(t, checkWriter(WriterGate{}), domain.ErrWriterNotConfigured)
	assert.ErrorIs(t, checkWriter(WriterGate{Writer: w, Mode: domain.WriterModeDisabled}), domain.ErrWriterNotConfigured)
	assert.ErrorIs(t, checkWriter(WriterGate{Writer: w, Mode: ""}), domain.ErrWriterNotConfigured)
	assert.ErrorIs(t, checkWriter(WriterGate{Mode: domain.WriterModeFake}), domain.ErrWriterNotConfigured)
	assert.NoError(t, checkWriter(WriterGate{Writer: w, Mode: domain.WriterModeFake}))
	assert.ErrorIs(t, checkPermission(false, ErrPushPermissionDenied), ErrPushPermissionDenied)
	assert.NoError(t, checkPermission(true, ErrPushPermissionDenied))

	f := newPushFake()
	st := &pushMemStore{f: f}
	assert.NoError(t, checkPeriodLocked(context.Background(), st, tPeriod))
	f.locked = false
	assert.ErrorIs(t, checkPeriodLocked(context.Background(), st, tPeriod), domain.ErrPeriodNotLocked)

	_, err := seqInt32(0)
	assert.Error(t, err)
	_, err = seqInt32(math.MaxInt32 + 1)
	assert.Error(t, err)
	v, err := seqInt32(7)
	require.NoError(t, err)
	assert.Equal(t, int32(7), v)
}

func TestClassifyWrite(t *testing.T) {
	ok := classifyWrite(domain.WriteResult{BatchRows: 1, CostRows: 5}, nil)
	assert.Equal(t, domain.OracleCallSuccess, ok.Status)
	require.NotNil(t, ok.Rows)
	assert.Equal(t, int64(5), *ok.Rows)
	assert.Equal(t, domain.OracleCallFailed, classifyWrite(domain.WriteResult{}, domain.ErrWriterDisabled).Status)
	assert.Equal(t, domain.OracleCallUnknown, classifyWrite(domain.WriteResult{}, context.DeadlineExceeded).Status,
		"a bare deadline may have committed")
}

func TestStepTrigger_Push(t *testing.T) {
	ctx := context.Background()
	f := newPushFake()
	id := validatedBatch(t, f, domain.ModeLive)
	jobs := newMemJobs()
	pub := &fakePublisher{}
	gate := WriterGate{Writer: oracle.NewFakeWriter(), Mode: domain.WriterModeFake}
	cmd := PushTriggerCommand{BatchID: id, Actor: "approver", HasPermission: true, ConfirmRowCount: 3, ConfirmSumStd: "5.00000"}

	_, err := NewStepTriggerHandler(jobs, f, pub, time.Hour).Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepPush, Actor: "a"})
	require.ErrorIs(t, err, ErrStepNotAllowed, "generic trigger refuses push")

	_, err = NewStepTriggerHandler(jobs, f, pub, time.Hour).TriggerPush(ctx, cmd)
	require.ErrorIs(t, err, domain.ErrFeatureDisabled, "gates unset: fail closed")

	_, err = NewStepTriggerHandler(jobs, f, pub, time.Hour).WithPushGates(true, WriterGate{Writer: oracle.NewDisabledWriter(), Mode: domain.WriterModeDisabled}).TriggerPush(ctx, cmd)
	require.ErrorIs(t, err, domain.ErrWriterNotConfigured)

	trig := NewStepTriggerHandler(jobs, f, pub, time.Hour).WithPushGates(true, gate)
	noPerm := cmd
	noPerm.HasPermission = false
	_, err = trig.TriggerPush(ctx, noPerm)
	require.ErrorIs(t, err, ErrPushPermissionDenied)
	assert.Empty(t, pub.calls)

	exec, err := trig.TriggerPush(ctx, cmd)
	require.NoError(t, err)
	assert.Equal(t, StepPush, exec.Subtype())
	p, err := ParseJobParams(exec.Params())
	require.NoError(t, err)
	assert.Equal(t, id, p.BatchID)
	require.NotNil(t, p.ConfirmRowCount)
	assert.Equal(t, int64(3), *p.ConfirmRowCount)
	assert.Equal(t, "5.00000", p.ConfirmSumStd)
	assert.True(t, p.PushPermitted)
	assert.Len(t, pub.calls, 1)

	st := f.state(id)
	st.Mode = domain.ModeShadow
	f.put(st)
	_, err = trig.TriggerPush(ctx, cmd)
	require.ErrorIs(t, err, domain.ErrShadowNotPushable)
}
