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

// TestResolveHeadSnapshot_Branches locks the three carry-forward branches of
// the shared head resolver: exact row -> "", earlier row -> its YYYYMM,
// nothing -> "ANCHOR".
func TestResolveHeadSnapshot_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("exact period row", func(t *testing.T) {
		head := newHead(t)
		exact := rmgroup.NewHeadPeriodSnapshotFromHead("202605", head)
		exact.CostPercentage = 11
		repo := new(mockRepo)
		repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202605").Return(&exact, nil)

		snap, from, err := appgroup.ResolveHeadSnapshot(ctx, repo, head, "202605")
		require.NoError(t, err)
		assert.Empty(t, from)
		assert.Same(t, &exact, snap)
		repo.AssertExpectations(t)
		repo.AssertNotCalled(t, "GetLatestHeadPeriodSnapshotBefore", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("latest earlier period row", func(t *testing.T) {
		head := newHead(t)
		prev := rmgroup.NewHeadPeriodSnapshotFromHead("202603", head)
		prev.CostPercentage = 22
		freight := 7.5
		prev.MarketingInputs.FreightRate = &freight
		repo := new(mockRepo)
		repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
		repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202605").Return(&prev, nil)

		snap, from, err := appgroup.ResolveHeadSnapshot(ctx, repo, head, "202605")
		require.NoError(t, err)
		assert.Equal(t, "202603", from)
		assert.Equal(t, "202605", snap.Period)
		assert.NotEqual(t, prev.ID, snap.ID, "carried copy must get a fresh ID")
		assert.InEpsilon(t, 22.0, snap.CostPercentage, 1e-9)
		require.NotNil(t, snap.MarketingInputs.FreightRate)
		assert.NotSame(t, prev.MarketingInputs.FreightRate, snap.MarketingInputs.FreightRate, "pointer fields must be deep-copied")
		assert.Equal(t, "202603", prev.Period, "source snapshot must not be mutated")
		repo.AssertExpectations(t)
	})

	t.Run("anchor fallback", func(t *testing.T) {
		head := newHead(t)
		repo := new(mockRepo)
		repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
		repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202605").Return(nil, rmgroup.ErrNotFound)

		snap, from, err := appgroup.ResolveHeadSnapshot(ctx, repo, head, "202605")
		require.NoError(t, err)
		assert.Equal(t, appgroup.InheritedFromAnchor, from)
		assert.Equal(t, "202605", snap.Period)
		assert.Equal(t, head.CostPercentage(), snap.CostPercentage)
		repo.AssertExpectations(t)
	})
}

// TestResolveDetailSnapshots_Branches locks the detail resolver: one detail
// with an exact row, one inheriting from an earlier period, one falling back
// to the anchor — with the earlier-period lookup batched once per head.
func TestResolveDetailSnapshots_Branches(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	dExact := newDetailForHead(t, head.ID())
	dPrev := newDetailForHead(t, head.ID())
	dAnchor := newDetailForHead(t, head.ID())

	exact := rmgroup.NewDetailPeriodSnapshotFromDetail("202605", dExact)
	prev := rmgroup.NewDetailPeriodSnapshotFromDetail("202602", dPrev)
	pct := 0.4
	prev.MarketPercentage = &pct

	repo := new(mockRepo)
	repo.On("GetDetailPeriodSnapshot", ctx, dExact.ID(), "202605").Return(&exact, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, dPrev.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetDetailPeriodSnapshot", ctx, dAnchor.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestDetailPeriodSnapshotsBefore", ctx, head.ID(), "202605").
		Return(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot{dPrev.ID(): &prev}, nil).Once()

	got, err := appgroup.ResolveDetailSnapshots(ctx, repo, head.ID(), []*rmgroup.Detail{dExact, dPrev, dAnchor}, "202605")
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Empty(t, got[0].InheritedFrom)
	assert.Same(t, &exact, got[0].Snapshot)

	assert.Equal(t, "202602", got[1].InheritedFrom)
	assert.Equal(t, "202605", got[1].Snapshot.Period)
	require.NotNil(t, got[1].Snapshot.MarketPercentage)
	assert.InEpsilon(t, 0.4, *got[1].Snapshot.MarketPercentage, 1e-9)

	assert.Equal(t, appgroup.InheritedFromAnchor, got[2].InheritedFrom)
	assert.Equal(t, dAnchor.ID(), got[2].Snapshot.GroupDetailID)
	repo.AssertExpectations(t)
}

// TestResolveDetailSnapshots_AllExact_SkipsCarryForwardQuery ensures the
// batched earlier-period query is only issued when a detail needs it.
func TestResolveDetailSnapshots_AllExact_SkipsCarryForwardQuery(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	d := newDetailForHead(t, head.ID())
	exact := rmgroup.NewDetailPeriodSnapshotFromDetail("202605", d)
	repo := new(mockRepo)
	repo.On("GetDetailPeriodSnapshot", ctx, d.ID(), "202605").Return(&exact, nil)

	_, err := appgroup.ResolveDetailSnapshots(ctx, repo, head.ID(), []*rmgroup.Detail{d}, "202605")
	require.NoError(t, err)
	repo.AssertNotCalled(t, "GetLatestDetailPeriodSnapshotsBefore", mock.Anything, mock.Anything, mock.Anything)
}

// TestFrozenCopy_Provenance locks freeze-on-calc provenance: nothing to
// freeze for an exact row, carried_from_period = source period for an
// earlier-period inheritance, NULL for an anchor inheritance.
func TestFrozenCopy_Provenance(t *testing.T) {
	head := newHead(t)
	snap := rmgroup.NewHeadPeriodSnapshotFromHead("202605", head)

	assert.Nil(t, appgroup.FrozenHeadCopy(&snap, "", "calc"))

	fromPrev := appgroup.FrozenHeadCopy(&snap, "202603", "calc")
	require.NotNil(t, fromPrev)
	assert.True(t, fromPrev.IsBackfilled)
	require.NotNil(t, fromPrev.CarriedFromPeriod)
	assert.Equal(t, "202603", *fromPrev.CarriedFromPeriod)
	assert.Equal(t, "calc", fromPrev.CreatedBy)
	assert.False(t, snap.IsBackfilled, "source must not be mutated")

	fromAnchor := appgroup.FrozenHeadCopy(&snap, appgroup.InheritedFromAnchor, "calc")
	require.NotNil(t, fromAnchor)
	assert.True(t, fromAnchor.IsBackfilled)
	assert.Nil(t, fromAnchor.CarriedFromPeriod)

	d := newDetailForHead(t, head.ID())
	dsnap := rmgroup.NewDetailPeriodSnapshotFromDetail("202605", d)
	assert.Nil(t, appgroup.FrozenDetailCopy(&dsnap, "", "calc"))
	df := appgroup.FrozenDetailCopy(&dsnap, "202601", "calc")
	require.NotNil(t, df)
	assert.True(t, df.IsBackfilled)
	require.NotNil(t, df.CarriedFromPeriod)
	assert.Equal(t, "202601", *df.CarriedFromPeriod)
}

// TestUpdateHandler_NewPeriod_SeedsFromPreviousPeriod locks carry-forward on
// write: editing a period with no exact row seeds the new snapshot from the
// latest earlier period's values (not the anchor), then applies the patch.
func TestUpdateHandler_NewPeriod_SeedsFromPreviousPeriod(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	prev := rmgroup.NewHeadPeriodSnapshotFromHead("202603", head)
	prev.CostPercentage = 33
	prev.Colorant = "prev-colorant"

	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202604").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202604").Return(&prev, nil)
	repo.On("UpsertHeadPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.HeadPeriodSnapshot) bool {
		return snap.Period == "202604" &&
			snap.ID != prev.ID &&
			snap.Name == "Patched" &&
			snap.CostPercentage == 33 &&
			snap.Colorant == "prev-colorant" &&
			!snap.IsBackfilled && snap.CarriedFromPeriod == nil
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202612", nil)

	name := "Patched"
	out, err := appgroup.NewUpdateHandler(repo).Handle(ctx, appgroup.UpdateCommand{
		HeadID:    head.ID().String(),
		Period:    "202604",
		Name:      &name,
		UpdatedBy: "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "Patched", out.Name)
	assert.Equal(t, "202603", prev.Period, "previous snapshot must be untouched")
	repo.AssertExpectations(t)
}

// TestUpdateItemHandler_NewPeriod_SeedsFromPreviousPeriod is the detail
// counterpart: the new period's detail row starts from the earlier period's
// values, then the patch is applied.
func TestUpdateItemHandler_NewPeriod_SeedsFromPreviousPeriod(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	d := newDetailForHead(t, head.ID())
	prev := rmgroup.NewDetailPeriodSnapshotFromDetail("202603", d)
	pct := 0.25
	prev.MarketPercentage = &pct

	repo := new(mockRepo)
	repo.On("GetDetailByID", ctx, d.ID()).Return(d, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, d.ID(), "202604").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestDetailPeriodSnapshotsBefore", ctx, head.ID(), "202604").
		Return(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot{d.ID(): &prev}, nil)
	repo.On("UpsertDetailPeriod", ctx, mock.MatchedBy(func(snap *rmgroup.DetailPeriodSnapshot) bool {
		return snap.Period == "202604" &&
			snap.ID != prev.ID &&
			snap.MarketPercentage != nil && *snap.MarketPercentage == 0.25 &&
			snap.SortOrder == 9
	})).Return(nil)
	repo.On("LatestSyncPeriod", ctx).Return("202612", nil)

	sort := int32(9)
	_, err := appgroup.NewUpdateItemHandler(repo).Handle(ctx, appgroup.UpdateItemCommand{
		HeadID:        head.ID().String(),
		GroupDetailID: d.ID().String(),
		Period:        "202604",
		SortOrder:     &sort,
		UpdatedBy:     "user:edit",
	})
	require.NoError(t, err)
	repo.AssertExpectations(t)
}
