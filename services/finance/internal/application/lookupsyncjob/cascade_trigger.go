package lookupsyncjob

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
)

// CascadeTrigger queues a lookup sync for the products referencing one MB master record
// after an MB Recipe edit cascaded into it (implements mbhead.LookupSyncTrigger). It
// mirrors the gRPC ApplyLookupSync flow — plan the scope for its fingerprint, then
// Enqueue — without the client-side drift check: the plan is computed here, server-side.
type CascadeTrigger struct {
	walker   ScopeWalker
	enqueuer *Enqueuer
}

// NewCascadeTrigger constructs the trigger. A nil enqueuer makes every call a no-op.
func NewCascadeTrigger(walker ScopeWalker, enqueuer *Enqueuer) *CascadeTrigger {
	return &CascadeTrigger{walker: walker, enqueuer: enqueuer}
}

// TriggerSpinSync queues a sync for products referencing the spin (MB_SPIN key = mbs_id).
func (t *CascadeTrigger) TriggerSpinSync(ctx context.Context, spinID uuid.UUID, actor string) error {
	return t.trigger(ctx, lookupsync.MasterMBSpin, spinID.String(), actor)
}

// trigger plans scope once for the fingerprint and enqueues it. A scope with nothing to
// apply is the common case (most spins are referenced by no yarn product) and is not an
// error.
func (t *CascadeTrigger) trigger(ctx context.Context, masterCode, key, actor string) error {
	if t == nil || t.enqueuer == nil || t.walker == nil {
		return nil
	}
	scope := lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: masterCode, MasterKey: key}}
	sum, err := t.walker.Walk(ctx, scope, nil)
	if err != nil {
		return err
	}
	if sum.ProductsWithChanges == 0 {
		return nil
	}
	exec, err := t.enqueuer.Enqueue(ctx, scope, sum.Fingerprint, actor)
	if errors.Is(err, ErrNothingToApply) {
		return nil
	}
	if err != nil {
		return err
	}
	log.Info().Str("master", masterCode).Str("key", key).Str("job_id", exec.ID().String()).
		Int("products", sum.ProductsWithChanges).Msg("MB recipe cascade: lookup sync queued")
	return nil
}
