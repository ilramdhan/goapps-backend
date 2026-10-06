package auditadapter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

type recordingSink struct {
	got []auditdomain.NewInput
	err error
}

func (r *recordingSink) Emit(_ context.Context, in auditdomain.NewInput) error {
	r.got = append(r.got, in)
	return r.err
}

func TestCostCalcEmitter_MapsApproveEvent(t *testing.T) {
	sink := &recordingSink{}
	a := &CostCalcEmitter{sink: sink}
	require.NoError(t, a.Emit(context.Background(), costcalcapp.AuditEvent{
		EventType: "COST_RESULT_APPROVED", EntityKind: "COST_RESULT", EntityID: "42",
		Actor: "alice", Message: "cost 42 approved",
	}))
	require.Len(t, sink.got, 1)
	in := sink.got[0]
	assert.Equal(t, "COST_RESULT", in.EntityType)
	assert.Equal(t, int64(42), in.EntityID)
	assert.Equal(t, auditdomain.OpStatusChange, in.Operation)
	assert.Equal(t, "alice", in.UserID)
	assert.Empty(t, in.BeforeData)
	require.NoError(t, in.Validate(), "operation must pass the chk_cal_operation whitelist")

	var after map[string]any
	require.NoError(t, json.Unmarshal([]byte(in.AfterData), &after))
	assert.Equal(t, "COST_RESULT_APPROVED", after["event_type"])
	assert.Equal(t, "cost 42 approved", after["message"])
	_, hasPayload := after["payload"]
	assert.False(t, hasPayload)
}

func TestCostCalcEmitter_AllKnownEventsPassWhitelist(t *testing.T) {
	for ev := range costCalcOperations {
		sink := &recordingSink{}
		a := &CostCalcEmitter{sink: sink}
		require.NoError(t, a.Emit(context.Background(), costcalcapp.AuditEvent{
			EventType: ev, EntityKind: "COST_CALC_JOB", EntityID: "1",
		}))
		require.NoError(t, sink.got[0].Validate(), ev)
		assert.Equal(t, "system", sink.got[0].UserID, "empty actor falls back to system")
	}
}

func TestCostCalcEmitter_UnknownEventFallsBackToStatusChange(t *testing.T) {
	sink := &recordingSink{}
	a := &CostCalcEmitter{sink: sink}
	require.NoError(t, a.Emit(context.Background(), costcalcapp.AuditEvent{
		EventType: "COST_SOMETHING_NEW", EntityKind: "X", EntityID: "7", Payload: []byte(`{"k":1}`),
	}))
	assert.Equal(t, auditdomain.OpStatusChange, sink.got[0].Operation)
	assert.JSONEq(t, `{"event_type":"COST_SOMETHING_NEW","payload":{"k":1}}`, sink.got[0].AfterData)
}

func TestCostCalcEmitter_InvalidPayloadDropped(t *testing.T) {
	sink := &recordingSink{}
	a := &CostCalcEmitter{sink: sink}
	require.NoError(t, a.Emit(context.Background(), costcalcapp.AuditEvent{
		EventType: "COST_RESULT_VERIFIED", EntityKind: "COST_RESULT", EntityID: "3", Payload: []byte("not json"),
	}))
	assert.JSONEq(t, `{"event_type":"COST_RESULT_VERIFIED"}`, sink.got[0].AfterData)
}

func TestCostCalcEmitter_NonNumericEntityID(t *testing.T) {
	sink := &recordingSink{}
	a := &CostCalcEmitter{sink: sink}
	require.Error(t, a.Emit(context.Background(), costcalcapp.AuditEvent{EventType: "X", EntityID: "abc"}))
	assert.Empty(t, sink.got)
}

func TestCostCalcEmitter_PropagatesSinkError(t *testing.T) {
	boom := errors.New("boom")
	a := &CostCalcEmitter{sink: &recordingSink{err: boom}}
	err := a.Emit(context.Background(), costcalcapp.AuditEvent{EventType: "COST_RESULT_APPROVED", EntityID: "1"})
	require.ErrorIs(t, err, boom)
}
