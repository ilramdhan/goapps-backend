package rmgroup_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

func newDetailForHead(t *testing.T, headID uuid.UUID) *rmgroup.Detail {
	t.Helper()
	code, err := rmgroup.NewItemCode("ITEM-001")
	require.NoError(t, err)
	d, err := rmgroup.NewDetail(headID, code, "user:test")
	require.NoError(t, err)
	return d
}

// TestUpdateItemHandler_LatestPeriod_WritesThroughAnchorRow mirrors
// TestUpdateHandler_LatestPeriod_WritesThroughAnchorRow for the detail/item
// side of the write-through rule (design §2.2/§5.2).
func TestUpdateItemHandler_LatestPeriod_WritesThroughAnchorRow(t *testing.T) {
	ctx := context.Background()
	headID := uuid.New()
	detail := newDetailForHead(t, headID)
	repo := new(mockRepo)

	sortOrder := int32(3)
	repo.On("GetDetailByID", ctx, detail.ID()).Return(detail, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202604").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("UpsertDetailPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.DetailPeriodSnapshot) bool {
		return snap.Period == "202604" && snap.SortOrder == sortOrder
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)
	repo.On("UpdateDetail", ctx, detail).Return(nil)

	h := appgroup.NewUpdateItemHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateItemCommand{
		HeadID:        headID.String(),
		GroupDetailID: detail.ID().String(),
		Period:        "202604",
		SortOrder:     &sortOrder,
		UpdatedBy:     "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, sortOrder, out.SortOrder)
	// Anchor row must reflect the same patch — the write-through contract.
	assert.Equal(t, sortOrder, detail.SortOrder())
	repo.AssertExpectations(t)
	repo.AssertCalled(t, "UpdateDetail", ctx, detail)
}

// TestUpdateItemHandler_OlderPeriod_AnchorRowUntouched mirrors
// TestUpdateHandler_OlderPeriod_AnchorRowUntouched for the item side.
func TestUpdateItemHandler_OlderPeriod_AnchorRowUntouched(t *testing.T) {
	ctx := context.Background()
	headID := uuid.New()
	detail := newDetailForHead(t, headID)
	originalSortOrder := detail.SortOrder()
	repo := new(mockRepo)

	sortOrder := int32(9)
	repo.On("GetDetailByID", ctx, detail.ID()).Return(detail, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202503").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("UpsertDetailPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.DetailPeriodSnapshot) bool {
		return snap.Period == "202503" && snap.SortOrder == sortOrder
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)

	h := appgroup.NewUpdateItemHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateItemCommand{
		HeadID:        headID.String(),
		GroupDetailID: detail.ID().String(),
		Period:        "202503",
		SortOrder:     &sortOrder,
		UpdatedBy:     "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, sortOrder, out.SortOrder)
	// Anchor row must be untouched by an older-period edit.
	assert.Equal(t, originalSortOrder, detail.SortOrder())
	repo.AssertExpectations(t)
	repo.AssertNotCalled(t, "UpdateDetail", mock.Anything, mock.Anything)
}

// TestUpdateItemHandler_GetOrCreate_UsesExistingSnapshotWhenPresent locks the
// "get" half of get-or-create for the item/detail side.
func TestUpdateItemHandler_GetOrCreate_UsesExistingSnapshotWhenPresent(t *testing.T) {
	ctx := context.Background()
	headID := uuid.New()
	detail := newDetailForHead(t, headID)
	repo := new(mockRepo)

	existing := rmgroup.NewDetailPeriodSnapshotFromDetail("202503", detail)
	freight := 7.0
	existing.ValuationInputs.FreightRate = &freight

	sortOrder := int32(2)
	repo.On("GetDetailByID", ctx, detail.ID()).Return(detail, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202503").Return(&existing, nil)
	repo.On("UpsertDetailPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.DetailPeriodSnapshot) bool {
		return snap.ID == existing.ID && snap.ValuationInputs.FreightRate != nil && *snap.ValuationInputs.FreightRate == 7.0
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)

	h := appgroup.NewUpdateItemHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateItemCommand{
		HeadID:        headID.String(),
		GroupDetailID: detail.ID().String(),
		Period:        "202503",
		SortOrder:     &sortOrder,
		UpdatedBy:     "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, sortOrder, out.SortOrder)
	require.NotNil(t, out.ValuationInputs.FreightRate)
	assert.InEpsilon(t, 7.0, *out.ValuationInputs.FreightRate, 1e-9)
	repo.AssertExpectations(t)
}
