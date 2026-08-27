package mbhead_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// lockedHead builds a head fixture in the given entry status, hydrated with the P10 lock
// columns the unlock flows read.
//
// pendingSince non-nil marks a PENDING unlock request (mbh_unlock_requested_at); nil means
// nothing is pending. preUnlock is the state the head was parked from — the value the
// repository restores from mst_mb_workflow_log. An EMPTY preUnlock is the
// ErrUnlockOriginUnknown condition (K-52): the trail is missing, so there is no target.
func lockedHead(entryStatus string, isLocked bool, pendingSince *time.Time, preUnlock string) *mbheaddomain.Entity {
	e := headInState(entryStatus, "")
	e.HydrateExtras(mbheaddomain.PersistedExtras{
		IsLocked:          isLocked,
		UnlockRequestedAt: pendingSince,
		PreUnlockStatus:   preUnlock,
	})
	return e
}

// ---------------------------------------------------------------------------
// RequestUnlockHandler
// ---------------------------------------------------------------------------

// TestRequestUnlockHandler_Handle_FromValidated_ParksInUnlockRequested is the happy path:
// VALIDATED -> UNLOCK_REQUESTED, forwarding the TRIMMED reason so it lands in both
// mbh_unlock_reason and mbhl_reason.
func TestRequestUnlockHandler_Handle_FromValidated_ParksInUnlockRequested(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRequestUnlockHandler(mockRepo)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
		int32(1), "shade is wrong", "drafter-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "  shade is wrong  ", ActorUserID: "drafter-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusUnlockRequested, got.EntryStatus())
	require.NotNil(t, got.UnlockReason())
	assert.Equal(t, "shade is wrong", *got.UnlockReason(),
		"the TRIMMED reason must be what is persisted")
	assert.Equal(t, mbheaddomain.StatusValidated, got.PreUnlockStatus(),
		"the origin state must be remembered for a later RejectUnlock")

	mockRepo.AssertExpectations(t)
}

// TestRequestUnlockHandler_Handle_UnlockedLegacyRow_IsAccepted pins K-53 option (a): the
// request must NOT require mbh_is_locked to already be true. Production holds 4190 VALIDATED
// rows with mbh_is_locked = NULL (reads as "not locked"); refusing them would make the
// feature unusable on exactly the data it exists for.
func TestRequestUnlockHandler_Handle_UnlockedLegacyRow_IsAccepted(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRequestUnlockHandler(mockRepo)
	ctx := context.Background()

	// isLocked = false — the Go view of a legacy NULL mbh_is_locked.
	entity := lockedHead(mbheaddomain.StatusValidated, false, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
		int32(1), "legacy row needs a fix", "drafter-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "legacy row needs a fix", ActorUserID: "drafter-1",
	})
	require.NoError(t, err, "K-53: an unlocked (NULL) head must still be requestable")
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusUnlockRequested, got.EntryStatus())

	mockRepo.AssertExpectations(t)
}

// TestRequestUnlockHandler_Handle_BlankReason_ReturnsErrorWithoutTransition locks the
// mandatory-reason rule, including the whitespace-only case.
func TestRequestUnlockHandler_Handle_BlankReason_ReturnsErrorWithoutTransition(t *testing.T) {
	for name, reason := range map[string]string{"empty": "", "whitespace only": "   \t "} {
		t.Run(name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			handler := mbhead.NewRequestUnlockHandler(mockRepo)
			ctx := context.Background()

			entity := lockedHead(mbheaddomain.StatusApproved, true, nil, "")
			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

			got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
				MbhID: entity.ID(), Reason: reason, ActorUserID: "drafter-1",
			})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.ErrorIs(t, err, mbheaddomain.ErrReasonRequired)

			mockRepo.AssertNotCalled(t, "Transition",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// TestRequestUnlockHandler_Handle_FromDraft_ReturnsInvalidTransition guards the state
// machine: an already-editable recipe has nothing to unlock.
func TestRequestUnlockHandler_Handle_FromDraft_ReturnsInvalidTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRequestUnlockHandler(mockRepo)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusDraft, false, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "nope", ActorUserID: "drafter-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrInvalidTransition)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestRequestUnlockHandler_Handle_GetByIDError_PropagatesAsIs confirms lookup failures
// surface unwrapped.
func TestRequestUnlockHandler_Handle_GetByIDError_PropagatesAsIs(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRequestUnlockHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	sentinel := errors.New("boom")
	mockRepo.On("GetByID", ctx, id).Return(nil, sentinel)

	got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: id, Reason: "r", ActorUserID: "drafter-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, sentinel, err)
}

// ---------------------------------------------------------------------------
// GrantUnlockHandler
// ---------------------------------------------------------------------------

// TestGrantUnlockHandler_Handle_PendingRequest_UnlocksToDraft is the happy path:
// UNLOCK_REQUESTED -> DRAFT, unlocked, pending markers cleared.
func TestGrantUnlockHandler_Handle_PendingRequest_UnlocksToDraft(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewGrantUnlockHandler(mockRepo)
	ctx := context.Background()

	pending := time.Now()
	entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusDraft,
		int32(1), "", "approver-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus())
	assert.False(t, got.IsLocked(), "a granted unlock must leave the head editable")
	assert.Nil(t, got.UnlockRequestedAt(), "the pending marker must be cleared")
	assert.True(t, got.IsEditable())

	mockRepo.AssertExpectations(t)
}

// TestGrantUnlockHandler_Handle_NoPendingRequest_ReturnsErrUnlockNotRequested covers both
// shapes of "nothing pending": the head is not parked at all, and the head is parked but
// carries no request timestamp. ⛔ Never a silent success.
func TestGrantUnlockHandler_Handle_NoPendingRequest_ReturnsErrUnlockNotRequested(t *testing.T) {
	cases := map[string]*mbheaddomain.Entity{
		"not parked — still VALIDATED": lockedHead(mbheaddomain.StatusValidated, true, nil, ""),
		"parked but no request timestamp": lockedHead(
			mbheaddomain.StatusUnlockRequested, true, nil, mbheaddomain.StatusApproved),
	}
	for name, entity := range cases {
		t.Run(name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			handler := mbhead.NewGrantUnlockHandler(mockRepo)
			ctx := context.Background()

			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

			got, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
				MbhID: entity.ID(), ActorUserID: "approver-1",
			})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.ErrorIs(t, err, mbheaddomain.ErrUnlockNotRequested)

			mockRepo.AssertNotCalled(t, "Transition",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// TestGrantUnlockHandler_Handle_PreservesUnlockReason pins principle U-2: granting clears
// the PENDING markers but ⛔ never the reason the unlock was asked for.
func TestGrantUnlockHandler_Handle_PreservesUnlockReason(t *testing.T) {
	mockRepo := new(MockRepository)
	requestH := mbhead.NewRequestUnlockHandler(mockRepo)
	grantH := mbhead.NewGrantUnlockHandler(mockRepo)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusApproved, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		int32(1), mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := requestH.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "wrong dozing", ActorUserID: "drafter-1",
	})
	require.NoError(t, err)

	got, err := grantH.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got.UnlockReason())
	assert.Equal(t, "wrong dozing", *got.UnlockReason(),
		"U-2: the unlock reason must stay readable after the grant")
}

// ---------------------------------------------------------------------------
// RejectUnlockHandler
// ---------------------------------------------------------------------------

// TestRejectUnlockHandler_Handle_ReturnsToOriginAndStaysLocked is the happy path for both
// possible origins. The head goes back exactly where it came from and stays LOCKED.
func TestRejectUnlockHandler_Handle_ReturnsToOriginAndStaysLocked(t *testing.T) {
	for _, origin := range []string{mbheaddomain.StatusApproved, mbheaddomain.StatusValidated} {
		t.Run(origin, func(t *testing.T) {
			mockRepo := new(MockRepository)
			handler := mbhead.NewRejectUnlockHandler(mockRepo)
			ctx := context.Background()

			pending := time.Now()
			entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, origin)
			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
			mockRepo.On("Transition", ctx, entity.ID(),
				mbheaddomain.StatusUnlockRequested, origin,
				int32(1), "recipe is in production", "approver-1",
				(*mbheaddomain.ParamSnapshot)(nil),
			).Return(nil)

			got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
				MbhID: entity.ID(), Reason: "recipe is in production", ActorUserID: "approver-1",
			})
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, origin, got.EntryStatus(), "must return to the ORIGIN state")
			assert.True(t, got.IsLocked(), "refusing an unlock cannot leave the recipe open")
			assert.Nil(t, got.UnlockRequestedAt(), "the pending marker must be cleared")
			assert.Empty(t, got.PreUnlockStatus(), "the origin is consumed")

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestRejectUnlockHandler_Handle_NoOriginTrail_ReturnsErrUnlockOriginUnknown pins K-52
// option (a). With no UNLOCK_REQUESTED row in mst_mb_workflow_log to restore the origin
// from, the handler must REFUSE — ⛔ never guess between APPROVED and VALIDATED, and
// nothing may be persisted.
func TestRejectUnlockHandler_Handle_NoOriginTrail_ReturnsErrUnlockOriginUnknown(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectUnlockHandler(mockRepo)
	ctx := context.Background()

	pending := time.Now()
	// preUnlock = "" — the origin trail is missing.
	entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
		MbhID: entity.ID(), Reason: "no", ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrUnlockOriginUnknown)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestRejectUnlockHandler_Handle_BlankReason_ReturnsErrorWithoutTransition locks the
// mandatory-reason rule for the refusal path.
func TestRejectUnlockHandler_Handle_BlankReason_ReturnsErrorWithoutTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectUnlockHandler(mockRepo)
	ctx := context.Background()

	pending := time.Now()
	entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, mbheaddomain.StatusApproved)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
		MbhID: entity.ID(), Reason: "  ", ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrReasonRequired)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestRejectUnlockHandler_Handle_NoPendingRequest_ReturnsErrUnlockNotRequested — refusing
// something that was never asked for is an error, not a no-op.
func TestRejectUnlockHandler_Handle_NoPendingRequest_ReturnsErrUnlockNotRequested(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectUnlockHandler(mockRepo)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
		MbhID: entity.ID(), Reason: "no", ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrUnlockNotRequested)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ---------------------------------------------------------------------------
// Cross-cutting: the lock actually blocks edits
// ---------------------------------------------------------------------------

// TestUpdate_OnLockedHead_ReturnsErrHeadLocked confirms the whole point of the feature: a
// locked head refuses edits, and a GRANTED unlock makes it editable again.
func TestUpdate_OnLockedHead_ReturnsErrHeadLocked(t *testing.T) {
	locked := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	err := locked.Update(mbheaddomain.UpdateInput{}, "editor-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, mbheaddomain.ErrHeadLocked)
	assert.False(t, locked.IsEditable())

	// After a granted unlock the same head accepts the edit.
	pending := time.Now()
	granted := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, mbheaddomain.StatusValidated)
	require.NoError(t, granted.GrantUnlock("approver-1"))
	assert.True(t, granted.IsEditable())
	assert.NoError(t, granted.Update(mbheaddomain.UpdateInput{}, "editor-1"))
}

// ---------------------------------------------------------------------------
// D7 — a save on a reopened recipe leaves it in DRAFT, and no other path can save
// an APPROVED/VALIDATED head at all
// ---------------------------------------------------------------------------

// TestD7_SaveAfterGrantedUnlock_StaysInDraft pins the D7 contract EXACTLY as the user
// stated it: once an unlock is granted, the recipe is back in DRAFT and every subsequent
// save keeps it there, so the author must walk submit → approve again.
//
// 🔴 D7 needs ⛔ NO new code: GrantUnlock already parks the head in DRAFT
// (domain/mbhead/lock.go, GrantUnlock sets entryStatus = StatusDraft) and Entity.Update
// never touches entryStatus at all, so a save cannot promote the head back. This test
// exists to make that a PINNED guarantee rather than an accident — adding a status write
// to Update would now fail here.
func TestD7_SaveAfterGrantedUnlock_StaysInDraft(t *testing.T) {
	pending := time.Now()
	granted := lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, mbheaddomain.StatusValidated)
	require.NoError(t, granted.GrantUnlock("approver-1"))
	require.Equal(t, mbheaddomain.StatusDraft, granted.EntryStatus(),
		"a granted unlock parks the head in DRAFT — this IS the D7 downgrade")
	require.False(t, granted.IsLocked())

	name := "reworked while the window is open"
	for i := 0; i < 3; i++ {
		require.NoError(t, granted.Update(mbheaddomain.UpdateInput{MgtName: &name}, "editor-1"))
		assert.Equal(t, mbheaddomain.StatusDraft, granted.EntryStatus(),
			"⛔ a save must NEVER promote the recipe out of DRAFT")
		assert.False(t, granted.IsLocked(), "⛔ a save must not re-lock the recipe either")
	}
	require.NotNil(t, granted.MgtName())
	assert.Equal(t, name, *granted.MgtName())
}

// TestD7_NoSavePathBypassesTheLock is the other half of D7: there must be NO way to save
// an APPROVED or VALIDATED head without going through an unlock first. If such a path
// existed, that path would have to perform the downgrade to DRAFT itself.
//
// Entity.Update is the ONE mutation door — every application save path funnels through it
// (application/mbhead/update_handler.go:81 and import_handler.go:278 are the only two
// callers of repo.Update, and both call entity.Update first, so both are covered by this
// single guard). It refuses whenever IsEditable() is false, which covers a locked head,
// a head parked in UNLOCK_REQUESTED, and a soft-deleted head.
func TestD7_NoSavePathBypassesTheLock(t *testing.T) {
	name := "sneaky edit"
	for _, status := range []string{mbheaddomain.StatusApproved, mbheaddomain.StatusValidated} {
		t.Run(status+"/locked", func(t *testing.T) {
			e := lockedHead(status, true, nil, "")
			err := e.Update(mbheaddomain.UpdateInput{MgtName: &name}, "editor-1")
			require.ErrorIs(t, err, mbheaddomain.ErrHeadLocked)
			assert.Nil(t, e.MgtName(), "⛔ the edit must not have landed")
			assert.Equal(t, status, e.EntryStatus(), "the refusal must not move the state either")
		})
	}

	// Parked awaiting a decision: asking for an unlock is not the same as getting one.
	parked := lockedHead(mbheaddomain.StatusUnlockRequested, true, nil, mbheaddomain.StatusValidated)
	require.ErrorIs(t, parked.Update(mbheaddomain.UpdateInput{MgtName: &name}, "editor-1"),
		mbheaddomain.ErrHeadLocked)
	assert.Nil(t, parked.MgtName())
}

// TestD7_LegacyNullLockRow_IsStillSavableWithoutUnlock documents the ONE remaining D7
// hole. It is ⚠ GERBANG KEPUTUSAN USER — deliberately NOT closed here, because closing it
// either way changes decided behavior on 4190 production rows.
//
// THE HOLE. mbh_is_locked is NULL on every legacy head (000485), and NULL reads as "not
// locked" — GetByID hydrates it through COALESCE(mbh_is_locked, FALSE)
// (mb_head_repository.go:357). IsEditable (domain/mbhead/entity.go:439) then answers TRUE
// for such a head even while it sits in VALIDATED, so UpdateMBHead saves it ⛔ WITHOUT an
// unlock and ⛔ WITHOUT the D7 downgrade to DRAFT. Only heads locked by P10 going forward
// (which stamp mbh_is_locked = TRUE on entering APPROVED/VALIDATED) are actually refused.
//
// WHY NOT SIMPLY FIXED HERE. The two ways to close it both overturn a prior decision:
//
//	(a) make IsEditable status-based (refuse APPROVED/VALIDATED whatever mbh_is_locked
//	    says) — this contradicts K-53 head-on, which accepted NULL-as-unlocked precisely
//	    so the 4190 legacy rows stay usable, and would make every one of them abruptly
//	    read-only until someone requests an unlock;
//	(b) auto-downgrade to DRAFT inside the save transaction (the shape the D7 brief
//	    describes) — this silently strips a legacy VALIDATED recipe of its costing
//	    standing on its very first edit, with no human assent, which is the same class of
//	    harm the auto-relock fix was written to prevent.
//
// This test therefore PINS today's behavior so neither can happen by accident. When the
// user decides, this test is the one to rewrite — ⛔ do not "fix" it silently.
func TestD7_LegacyNullLockRow_IsStillSavableWithoutUnlock(t *testing.T) {
	name := "legacy edit, no unlock requested"
	// isLocked=false is exactly what a NULL mbh_is_locked hydrates to.
	legacy := lockedHead(mbheaddomain.StatusValidated, false, nil, "")

	require.True(t, legacy.IsEditable(),
		"⚠ known hole: a legacy NULL-lock VALIDATED head is still editable")
	require.NoError(t, legacy.Update(mbheaddomain.UpdateInput{MgtName: &name}, "editor-1"),
		"⚠ known hole: the save is accepted with no unlock")
	assert.Equal(t, mbheaddomain.StatusValidated, legacy.EntryStatus(),
		"⚠ known hole: and it does NOT fall back to DRAFT — this is the open D7 question")
}
