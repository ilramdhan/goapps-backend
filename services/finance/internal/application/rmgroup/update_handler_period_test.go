package rmgroup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// TestUpdateHandler_LatestPeriod_WritesThroughAnchorRow locks the "latest
// period write-through" rule (design §2.2/§5.1): when cmd.Period equals
// LatestSyncPeriod, the handler must both upsert the period snapshot AND
// apply the same patch to the anchor Head row via UpdateHead.
func TestUpdateHandler_LatestPeriod_WritesThroughAnchorRow(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)

	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202604").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("UpsertHeadPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.HeadPeriodSnapshot) bool {
		return snap.Period == "202604" && snap.Name == "Latest Name"
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)
	repo.On("UpdateHead", ctx, head).Return(nil)

	newName := "Latest Name"
	h := appgroup.NewUpdateHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID:    head.ID().String(),
		Period:    "202604",
		Name:      &newName,
		UpdatedBy: "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "Latest Name", out.Name)
	// Anchor row must reflect the same patch — this is the write-through contract.
	assert.Equal(t, "Latest Name", head.Name())
	repo.AssertExpectations(t)
	repo.AssertCalled(t, "UpdateHead", ctx, head)
}

// TestUpdateHandler_OlderPeriod_AnchorRowUntouched locks the complementary
// half of the write-through rule: when cmd.Period is NOT the latest sync
// period, only the period snapshot is upserted — UpdateHead must never be
// called, and the anchor Head's in-memory fields must stay unchanged.
func TestUpdateHandler_OlderPeriod_AnchorRowUntouched(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	originalName := head.Name()
	repo := new(mockRepo)

	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("UpsertHeadPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.HeadPeriodSnapshot) bool {
		return snap.Period == "202503" && snap.Name == "Old Period Name"
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)

	oldName := "Old Period Name"
	h := appgroup.NewUpdateHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID:    head.ID().String(),
		Period:    "202503",
		Name:      &oldName,
		UpdatedBy: "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "Old Period Name", out.Name)
	// Anchor row must be untouched by an older-period edit.
	assert.Equal(t, originalName, head.Name())
	repo.AssertExpectations(t)
	repo.AssertNotCalled(t, "UpdateHead", mock.Anything, mock.Anything)
}

// TestUpdateHandler_GetOrCreate_UsesExistingSnapshotWhenPresent locks the
// "get" half of get-or-create: when a snapshot already exists for
// (headID, period), the handler patches that snapshot rather than building a
// fresh one from the anchor head's current values.
func TestUpdateHandler_GetOrCreate_UsesExistingSnapshotWhenPresent(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)

	existing := rmgroup.NewHeadPeriodSnapshotFromHead("202503", head)
	existing.Description = "existing description"

	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(&existing, nil)
	repo.On("UpsertHeadPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.HeadPeriodSnapshot) bool {
		return snap.ID == existing.ID && snap.Description == "existing description"
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202604", nil)

	newName := "Patched"
	h := appgroup.NewUpdateHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID:    head.ID().String(),
		Period:    "202503",
		Name:      &newName,
		UpdatedBy: "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "Patched", out.Name)
	assert.Equal(t, "existing description", out.Description)
	repo.AssertExpectations(t)
}
