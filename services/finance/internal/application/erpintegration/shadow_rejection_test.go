package erpintegration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// TestShadowBatch_EveryWriterEntryPointRefuses is the P5-T10a acceptance
// criterion: each writer entry point returns ErrShadowNotPushable for a
// SHADOW batch and makes 0 writer calls.
func TestShadowBatch_EveryWriterEntryPointRefuses(t *testing.T) {
	ctx := context.Background()

	t.Run("push", func(t *testing.T) {
		e := newPushEnv(t)
		st := e.f.state(e.id)
		st.Mode = domain.ModeShadow
		e.f.put(st)
		_, err := e.step.Run(ctx, e.cmd(), nil)
		require.ErrorIs(t, err, domain.ErrShadowNotPushable)
		assert.Empty(t, e.writer.Calls())
		assert.Empty(t, e.calls.starts)
	})

	for _, op := range []domain.AdjOperation{domain.AdjOpValuate, domain.AdjOpApprove, domain.AdjOpRestore} {
		t.Run(string(op), func(t *testing.T) {
			e := newAdjEnv(t, domain.StatusPushed)
			cmd := e.preview(t, op)
			st := e.f.state(e.id)
			st.Mode = domain.ModeShadow
			e.f.put(st)
			_, err := e.step().Run(ctx, cmd, nil)
			require.ErrorIs(t, err, domain.ErrShadowNotPushable)
			assert.Zero(t, e.w2Calls())
			assert.Empty(t, e.calls.starts)
		})
	}

	t.Run("lock batch", func(t *testing.T) {
		e := newAdjEnv(t, domain.StatusReconciled)
		st := e.f.state(e.id)
		st.Mode = domain.ModeShadow
		e.f.put(st)
		_, err := newLockStep(e, &fakeProber{posted: 1}, true).Run(ctx,
			LockBatchCommand{BatchID: e.id, Actor: "alice", HasPermission: true}, nil)
		require.ErrorIs(t, err, domain.ErrShadowNotPushable)
		for _, c := range e.writer.Calls() {
			assert.NotEqual(t, oracle.KeyW2LockBatch, c.Key)
		}
		assert.Zero(t, e.w2Calls())
	})
}
