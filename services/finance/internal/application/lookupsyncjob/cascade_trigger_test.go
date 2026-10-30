package lookupsyncjob

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
)

// summaryWalker is fakeWalker plus a real-looking Summary (ProductsWithChanges +
// Fingerprint), and records the scopes it was asked to walk.
type summaryWalker struct {
	fakeWalker
	scopes []lookupsync.Scope
}

func (w *summaryWalker) Walk(ctx context.Context, scope lookupsync.Scope, visit func(lookupsync.Item) error) (lookupsync.Summary, error) {
	w.scopes = append(w.scopes, scope)
	if visit == nil {
		visit = func(lookupsync.Item) error { return nil }
	}
	if _, err := w.fakeWalker.Walk(ctx, scope, visit); err != nil {
		return lookupsync.Summary{}, err
	}
	n := 0
	for _, it := range w.items {
		if it.SkipReason == "" && len(it.Changes) > 0 {
			n++
		}
	}
	return lookupsync.Summary{ProductsWithChanges: n, Fingerprint: "fp-cascade"}, nil
}

func TestCascadeTrigger_SpinSyncEnqueuesMBSpinScope(t *testing.T) {
	pub := &fakePublisher{}
	w := &summaryWalker{fakeWalker: fakeWalker{items: []lookupsync.Item{{ProductSysID: 7, Changes: change()}}}}
	spinID := uuid.New()

	err := NewCascadeTrigger(w, NewEnqueuer(newFakeJobRepo(), pub, w)).TriggerSpinSync(context.Background(), spinID, "editor")
	require.NoError(t, err)

	require.NotEmpty(t, w.scopes)
	ref := w.scopes[0].MasterRef
	require.NotNil(t, ref)
	assert.Equal(t, lookupsync.MasterMBSpin, ref.LookupMasterCode)
	assert.Equal(t, spinID.String(), ref.MasterKey)
	require.Len(t, pub.calls, 1)
	assert.Equal(t, int64(7), pub.calls[0].product)
	assert.Equal(t, "fp-cascade", pub.calls[0].fp)
}

func TestCascadeTrigger_NothingToApplyIsNotAnError(t *testing.T) {
	pub := &fakePublisher{}
	w := &summaryWalker{fakeWalker: fakeWalker{items: []lookupsync.Item{{ProductSysID: 1}}}}

	require.NoError(t, NewCascadeTrigger(w, NewEnqueuer(newFakeJobRepo(), pub, w)).TriggerSpinSync(context.Background(), uuid.New(), "editor"))
	assert.Empty(t, pub.calls)
}

func TestCascadeTrigger_Errors(t *testing.T) {
	ctx := context.Background()
	withChange := &summaryWalker{fakeWalker: fakeWalker{items: []lookupsync.Item{{ProductSysID: 1, Changes: change()}}}}

	err := NewCascadeTrigger(withChange, NewEnqueuer(newFakeJobRepo(), nil, withChange)).TriggerSpinSync(ctx, uuid.New(), "editor")
	require.ErrorIs(t, err, ErrPublisherUnavailable)

	failing := &summaryWalker{fakeWalker: fakeWalker{err: errors.New("db")}}
	require.Error(t, NewCascadeTrigger(failing, NewEnqueuer(newFakeJobRepo(), &fakePublisher{}, failing)).TriggerSpinSync(ctx, uuid.New(), "editor"))

	// nil enqueuer is a no-op, never a panic.
	require.NoError(t, NewCascadeTrigger(withChange, nil).TriggerSpinSync(ctx, uuid.New(), "editor"))
}
