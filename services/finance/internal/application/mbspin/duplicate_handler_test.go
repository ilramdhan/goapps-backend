// Package mbspin_test provides unit tests for the MB Spin duplicate handler.
package mbspin_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	mbspindomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// newTestSpin builds a minimal valid mbspin.Entity for duplicate-handler tests,
// optionally hydrating it with a parent lineage.
func newTestSpin(t *testing.T, headID uuid.UUID, parentSpinID *uuid.UUID) *mbspindomain.Entity {
	t.Helper()
	entity, err := mbspindomain.New(
		headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin",
	)
	require.NoError(t, err)
	entity.HydrateLineage(mbspindomain.Lineage{ParentSpinID: parentSpinID})
	return entity
}

func TestDuplicateHandler_Handle(t *testing.T) {
	t.Run("error - source already has a parent, rejected before any write", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDuplicateHandler(mockRepo, nil)
		ctx := context.Background()

		headID := uuid.New()
		sourceID := uuid.New()
		grandparentID := uuid.New()
		source := newTestSpin(t, headID, &grandparentID)

		mockRepo.On("GetByID", ctx, sourceID).Return(source, nil)

		result, err := handler.Handle(ctx, mbspin.DuplicateCommand{
			SourceSpinID: sourceID,
			ActorUserID:  "admin",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrAlreadyDuplicated)
		mockRepo.AssertNotCalled(t, "DuplicateSpin", mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - root spin (no parent) may be duplicated", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDuplicateHandler(mockRepo, nil)
		ctx := context.Background()

		headID := uuid.New()
		sourceID := uuid.New()
		newID := uuid.New()
		source := newTestSpin(t, headID, nil)

		mockRepo.On("GetByID", ctx, sourceID).Return(source, nil)
		mockRepo.On("DuplicateSpin", ctx, mock.MatchedBy(func(in mbspindomain.DuplicateInput) bool {
			return in.SourceSpinID == sourceID && in.ActorUserID == "admin"
		})).Return(
			mbspindomain.DuplicateOutput{
				NewSpinID:    newID,
				ParentSpinID: sourceID,
				HeadID:       headID,
				MgtName:      "Spin Alpha (copy)",
			}, nil,
		)
		clone := newTestSpin(t, headID, &sourceID)
		mockRepo.On("GetByID", ctx, newID).Return(clone, nil)

		result, err := handler.Handle(ctx, mbspin.DuplicateCommand{
			SourceSpinID: sourceID,
			ActorUserID:  "admin",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, newID, result.Output.NewSpinID)
		mockRepo.AssertExpectations(t)
	})
}
