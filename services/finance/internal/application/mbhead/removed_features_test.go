package mbhead_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
)

// ---------------------------------------------------------------------------
// USER DECISION 2026-08-26 — Revoke and Un-approve were REMOVED from the MB Recipe
// workflow, which is now DRAFT (editable) → SUBMITTED (not editable) → APPROVED
// (locked). From SUBMITTED the only actions are Approve and Reject; a locked recipe
// offers only Request Unlock.
//
// ⛔ The RPCs and these handlers were NOT deleted — removing an RPC from the proto
// contract is a breaking change. The SURFACE was switched off instead, and this file
// pins that: both handlers must refuse, and must refuse WITHOUT touching the
// repository, so a stray caller cannot leave an audit trail for an action that no
// longer exists.
//
// The mock repository is deliberately given NO expectations. testify's mock panics on
// any unexpected call, so the absence of `.On(...)` here IS the assertion that no I/O
// happens — a handler that started reading the head again would fail loudly.
// ---------------------------------------------------------------------------

func TestRevokeHandler_IsRemoved_RefusesWithoutTouchingRepository(t *testing.T) {
	repo := new(MockRepository)
	h := mbhead.NewRevokeHandler(repo)

	entity, err := h.Handle(context.Background(), mbhead.RevokeCommand{
		MbhID:       uuid.New(),
		Reason:      "no longer needed",
		ActorUserID: "tester",
	})

	assert.Nil(t, entity)
	assert.ErrorIs(t, err, mbhead.ErrFeatureRemovedRevoke)
	// The message must explain the replacement, not merely refuse.
	assert.Contains(t, err.Error(), "active flag")
	repo.AssertExpectations(t)
}

func TestUnApproveHandler_IsRemoved_RefusesWithoutTouchingRepository(t *testing.T) {
	repo := new(MockRepository)
	h := mbhead.NewUnApproveHandler(repo)

	entity, err := h.Handle(context.Background(), mbhead.UnApproveCommand{
		MbhID:       uuid.New(),
		Reason:      "wrong recipe",
		ActorUserID: "tester",
	})

	assert.Nil(t, entity)
	assert.ErrorIs(t, err, mbhead.ErrFeatureRemovedUnApprove)
	// The message must point at the surviving way to reopen a locked recipe.
	assert.Contains(t, err.Error(), "Request Unlock")
	repo.AssertExpectations(t)
}

// TestRemovedHandlers_RefuseEvenWithAnEmptyReason — the old handlers rejected an empty
// reason with ErrReasonRequired before doing anything else. That contract is gone with
// the feature: the reason is not examined at all, so an empty one yields the SAME
// removal error, never ErrReasonRequired.
func TestRemovedHandlers_RefuseEvenWithAnEmptyReason(t *testing.T) {
	repo := new(MockRepository)

	_, revokeErr := mbhead.NewRevokeHandler(repo).Handle(
		context.Background(), mbhead.RevokeCommand{MbhID: uuid.New()})
	assert.ErrorIs(t, revokeErr, mbhead.ErrFeatureRemovedRevoke)

	_, unApproveErr := mbhead.NewUnApproveHandler(repo).Handle(
		context.Background(), mbhead.UnApproveCommand{MbhID: uuid.New()})
	assert.ErrorIs(t, unApproveErr, mbhead.ErrFeatureRemovedUnApprove)

	repo.AssertExpectations(t)
}
