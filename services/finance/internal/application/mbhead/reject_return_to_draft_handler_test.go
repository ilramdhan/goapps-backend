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

// headInState builds a minimal head fixture in the given entry status carrying the
// given stored state reason, so the reject / return-to-draft flows can be exercised.
func headInState(entryStatus, stateReason string) *mbheaddomain.Entity {
	return mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		entryStatus, false, 1, nil,
		stateReason, "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "34", "S",
		nil,
	)
}

// TestRejectHandler_Handle_FromSubmitted_TransitionsToRejected confirms the happy path
// forwards fromState="SUBMITTED" and toState="REJECTED" to the repository.
func TestRejectHandler_Handle_FromSubmitted_TransitionsToRejected(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusSubmitted, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusSubmitted, mbheaddomain.StatusRejected,
		int32(1), "composition does not add up", "approver-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.RejectCommand{
		MbhID:       entity.ID(),
		Reason:      "composition does not add up",
		ActorUserID: "approver-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusRejected, got.EntryStatus())
	assert.Equal(t, "composition does not add up", got.StateReason())

	mockRepo.AssertExpectations(t)
}

// TestRejectHandler_Handle_EmptyReason_ReturnsErrorWithoutTransition locks the
// mandatory-reason rule: the domain rejects it and nothing is ever persisted.
func TestRejectHandler_Handle_EmptyReason_ReturnsErrorWithoutTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusSubmitted, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RejectCommand{
		MbhID:       entity.ID(),
		Reason:      "",
		ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrReasonRequired)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

// TestRejectHandler_Handle_InvalidState_ReturnsErrorWithoutTransition guards the state
// machine: only SUBMITTED may be rejected.
func TestRejectHandler_Handle_InvalidState_ReturnsErrorWithoutTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.RejectCommand{
		MbhID:       entity.ID(),
		Reason:      "nope",
		ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrInvalidTransition)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestRejectHandler_Handle_GetByIDError_PropagatesAsIs confirms repository lookup
// failures surface unwrapped.
func TestRejectHandler_Handle_GetByIDError_PropagatesAsIs(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewRejectHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	sentinel := errors.New("boom")
	mockRepo.On("GetByID", ctx, id).Return(nil, sentinel)

	got, err := handler.Handle(ctx, mbhead.RejectCommand{
		MbhID: id, Reason: "r", ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, sentinel, err)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestReturnToDraftHandler_Handle_EmptyReason_PreservesOldRejectReason is the K-29
// locking test. An empty Reason is legal, and the stateReason forwarded to Transition
// must remain the ORIGINAL reject reason — ⛔ it must not be blanked.
func TestReturnToDraftHandler_Handle_EmptyReason_PreservesOldRejectReason(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewReturnToDraftHandler(mockRepo)
	ctx := context.Background()

	const oldRejectReason = "composition does not add up"
	entity := headInState(mbheaddomain.StatusRejected, oldRejectReason)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	var capturedReason string
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusRejected, mbheaddomain.StatusDraft,
		int32(1), mock.AnythingOfType("string"), "author-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Run(func(args mock.Arguments) {
		capturedReason = args.String(5)
	}).Return(nil)

	got, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{
		MbhID:       entity.ID(),
		Reason:      "",
		ActorUserID: "author-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus())
	assert.Equal(t, oldRejectReason, capturedReason,
		"K-29: empty reason must preserve the previous REJECT reason, not blank it")
	assert.Equal(t, oldRejectReason, got.StateReason())

	mockRepo.AssertExpectations(t)
}

// TestReturnToDraftHandler_Handle_NonEmptyReason_OverwritesStoredReason covers the
// other half of the K-29 contract: a supplied reason wins.
func TestReturnToDraftHandler_Handle_NonEmptyReason_OverwritesStoredReason(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewReturnToDraftHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusRejected, "old reject reason")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusRejected, mbheaddomain.StatusDraft,
		int32(1), "please rework the shade", "author-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{
		MbhID:       entity.ID(),
		Reason:      "please rework the shade",
		ActorUserID: "author-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "please rework the shade", got.StateReason())

	mockRepo.AssertExpectations(t)
}

// TestReturnToDraftHandler_Handle_NonRejectedState_ReturnsErrorWithoutTransition
// guards the state machine: only REJECTED may go back to DRAFT.
func TestReturnToDraftHandler_Handle_NonRejectedState_ReturnsErrorWithoutTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewReturnToDraftHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusSubmitted, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{
		MbhID:       entity.ID(),
		Reason:      "",
		ActorUserID: "author-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrInvalidTransition)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestReturnToDraftHandler_Handle_GetByIDError_PropagatesAsIs confirms repository
// lookup failures surface unwrapped.
func TestReturnToDraftHandler_Handle_GetByIDError_PropagatesAsIs(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewReturnToDraftHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	sentinel := errors.New("boom")
	mockRepo.On("GetByID", ctx, id).Return(nil, sentinel)

	got, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{
		MbhID: id, ActorUserID: "author-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, sentinel, err)

	mockRepo.AssertNotCalled(t, "Transition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
