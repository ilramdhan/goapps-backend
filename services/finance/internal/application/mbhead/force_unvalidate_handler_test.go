package mbhead_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestForceUnvalidateHandler_Handle_FromValidated_Succeeds confirms the happy path:
// GetByID -> Entity.ForceUnvalidate -> ForceUnvalidateTransition, mirroring
// TestRejectHandler_Handle_FromSubmitted_TransitionsToRejected's shape
// (reject_return_to_draft_handler_test.go), adapted for ForceUnvalidateTransition's
// distinct repository signature.
func TestForceUnvalidateHandler_Handle_FromValidated_Succeeds(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewForceUnvalidateHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusValidated, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("ForceUnvalidateTransition", ctx, entity.ID(), 1, "bulk regenerate", "super-admin-1").
		Return(nil)

	got, err := handler.Handle(ctx, mbhead.ForceUnvalidateCommand{
		MbhID:       entity.ID(),
		Reason:      "bulk regenerate",
		ActorUserID: "super-admin-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus())
	assert.Equal(t, "bulk regenerate", got.StateReason())

	mockRepo.AssertExpectations(t)
}

// TestForceUnvalidateHandler_Handle_InvalidState_ReturnsErrorWithoutTransition guards
// the state machine: only VALIDATED may be force-unvalidated.
func TestForceUnvalidateHandler_Handle_InvalidState_ReturnsErrorWithoutTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewForceUnvalidateHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	got, err := handler.Handle(ctx, mbhead.ForceUnvalidateCommand{
		MbhID:       entity.ID(),
		Reason:      "bulk regenerate",
		ActorUserID: "super-admin-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbheaddomain.ErrInvalidTransition)

	mockRepo.AssertNotCalled(t, "ForceUnvalidateTransition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestForceUnvalidateHandler_Handle_GetByIDError_PropagatesAsIs confirms repository
// lookup failures surface unwrapped.
func TestForceUnvalidateHandler_Handle_GetByIDError_PropagatesAsIs(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewForceUnvalidateHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	sentinel := errors.New("boom")
	mockRepo.On("GetByID", ctx, id).Return(nil, sentinel)

	got, err := handler.Handle(ctx, mbhead.ForceUnvalidateCommand{
		MbhID: id, Reason: "r", ActorUserID: "super-admin-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, sentinel, err)

	mockRepo.AssertNotCalled(t, "ForceUnvalidateTransition",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestForceUnvalidateHandler_Handle_RepoTransitionError_PropagatesAsIs confirms that
// once the domain transition succeeds in-memory, a persistence failure from
// ForceUnvalidateTransition still surfaces to the caller unwrapped.
func TestForceUnvalidateHandler_Handle_RepoTransitionError_PropagatesAsIs(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewForceUnvalidateHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusValidated, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	sentinel := errors.New("db unavailable")
	mockRepo.On("ForceUnvalidateTransition", ctx, entity.ID(), 1, "", "super-admin-1").
		Return(sentinel)

	got, err := handler.Handle(ctx, mbhead.ForceUnvalidateCommand{
		MbhID:       entity.ID(),
		ActorUserID: "super-admin-1",
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, sentinel, err)

	mockRepo.AssertExpectations(t)
}
