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

func boolPtr(b bool) *bool { return &b }

func TestCreateHandler_SetsOilGroup(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)
	repo.On("ExistsHeadByCode", ctx, mock.AnythingOfType("rmgroup.Code")).Return(false, nil)
	repo.On("CreateHead", ctx, mock.MatchedBy(func(h *rmgroup.Head) bool { return h.IsOilGroup() })).Return(nil)

	h := appgroup.NewCreateHandler(repo)
	head, err := h.Handle(ctx, appgroup.CreateCommand{Code: "OIL-1", Name: "Coning Oil", CreatedBy: "u", IsOilGroup: true})
	require.NoError(t, err)
	assert.True(t, head.IsOilGroup())
	repo.AssertExpectations(t)
}

// TestUpdateHandler_OilFlagOnly_OlderPeriod_WritesAnchorWithoutSnapshot: the flag is
// global — a flag-only edit with an older period writes the anchor head and creates
// no period snapshot.
func TestUpdateHandler_OilFlagOnly_OlderPeriod_WritesAnchorWithoutSnapshot(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound)
	repo.On("UpdateHead", ctx, mock.MatchedBy(func(h *rmgroup.Head) bool { return h.IsOilGroup() })).Return(nil)

	h := appgroup.NewUpdateHandler(repo)
	_, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID: head.ID().String(), Period: "202503", IsOilGroup: boolPtr(true), UpdatedBy: "u",
	})
	require.NoError(t, err)
	assert.True(t, head.IsOilGroup())
	repo.AssertExpectations(t)
	repo.AssertNotCalled(t, "UpsertHeadPeriod", mock.Anything, mock.Anything)
}

// TestUpdateHandler_OilFlagWithPeriodPatch_OlderPeriod_AnchorFlagOnly: a mixed edit on
// an older period upserts the snapshot and writes only the flag onto the anchor.
func TestUpdateHandler_OilFlagWithPeriodPatch_OlderPeriod_AnchorFlagOnly(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	originalName := head.Name()
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound)
	repo.On("UpsertHeadPeriod", ctx, mock.Anything).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)
	repo.On("UpdateHead", ctx, head).Return(nil)

	name := "Old Period Name"
	h := appgroup.NewUpdateHandler(repo)
	_, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID: head.ID().String(), Period: "202503", Name: &name, IsOilGroup: boolPtr(true), UpdatedBy: "u",
	})
	require.NoError(t, err)
	assert.True(t, head.IsOilGroup())
	assert.Equal(t, originalName, head.Name(), "older-period name edit must not reach the anchor")
	repo.AssertExpectations(t)
}

func TestUpdateHandler_UnflagInUseOilGroup_Rejected(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	head.SetOilGroup(true)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("IsOilGroupInUse", ctx, head.ID()).Return(true, nil)

	h := appgroup.NewUpdateHandler(repo)
	_, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID: head.ID().String(), Period: "202604", IsOilGroup: boolPtr(false), UpdatedBy: "u",
	})
	require.ErrorIs(t, err, rmgroup.ErrOilGroupInUse)
	assert.True(t, head.IsOilGroup())
	repo.AssertNotCalled(t, "UpdateHead", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpsertHeadPeriod", mock.Anything, mock.Anything)
}

func TestUpdateHandler_UnflagUnusedOilGroup_Allowed(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	head.SetOilGroup(true)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("IsOilGroupInUse", ctx, head.ID()).Return(false, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202604").Return(nil, rmgroup.ErrNotFound)
	repo.On("UpdateHead", ctx, head).Return(nil)

	h := appgroup.NewUpdateHandler(repo)
	_, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID: head.ID().String(), Period: "202604", IsOilGroup: boolPtr(false), UpdatedBy: "u",
	})
	require.NoError(t, err)
	assert.False(t, head.IsOilGroup())
	repo.AssertExpectations(t)
}

func TestDeleteHandler_InUseOilGroup_Rejected(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := new(mockRepo)
	repo.On("ExistsHeadByID", ctx, id).Return(true, nil)
	repo.On("IsOilGroupInUse", ctx, id).Return(true, nil)

	h := appgroup.NewDeleteHandler(repo, nil)
	err := h.Handle(ctx, appgroup.DeleteCommand{HeadID: id.String(), DeletedBy: "u"})
	require.ErrorIs(t, err, rmgroup.ErrOilGroupInUse)
	repo.AssertNotCalled(t, "SoftDeleteHead", mock.Anything, mock.Anything, mock.Anything)
}

func TestListHandler_PassesOilGroupFilter(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)
	repo.On("ListHeads", ctx, mock.MatchedBy(func(f rmgroup.ListFilter) bool {
		return f.IsOilGroup != nil && *f.IsOilGroup
	})).Return(nil, int64(0), nil)

	h := appgroup.NewListHandler(repo)
	_, err := h.Handle(ctx, appgroup.ListQuery{Page: 1, PageSize: 10, IsOilGroup: boolPtr(true)})
	require.NoError(t, err)
	repo.AssertExpectations(t)
}
