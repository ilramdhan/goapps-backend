package rmcost

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// mockGroupRepo is a minimal testify/mock implementation of rmgroup.Repository,
// used only to exercise CalculateHandlerV2.loadHeadAndDetails' period-aware
// read path. Methods not touched by that path are left unimplemented (not
// registered with .On(...)) — calling them fails the test via testify's
// "mock: I don't know what to return" panic, which is intentional: it flags
// an unexpected call rather than silently returning a zero value.
type mockGroupRepo struct {
	mock.Mock
}

func (m *mockGroupRepo) CreateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockGroupRepo) GetHeadByID(ctx context.Context, id uuid.UUID) (*rmgroup.Head, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockGroupRepo) GetHeadByCode(ctx context.Context, code rmgroup.Code) (*rmgroup.Head, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockGroupRepo) ListHeads(ctx context.Context, filter rmgroup.ListFilter) ([]*rmgroup.Head, int64, error) {
	args := m.Called(ctx, filter)
	var heads []*rmgroup.Head
	if v := args.Get(0); v != nil {
		heads = v.([]*rmgroup.Head)
	}
	return heads, args.Get(1).(int64), args.Error(2)
}

func (m *mockGroupRepo) ListAllHeads(ctx context.Context, activeFilter *bool) ([]*rmgroup.Head, error) {
	args := m.Called(ctx, activeFilter)
	var heads []*rmgroup.Head
	if v := args.Get(0); v != nil {
		heads = v.([]*rmgroup.Head)
	}
	return heads, args.Error(1)
}

func (m *mockGroupRepo) UpdateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockGroupRepo) SoftDeleteHead(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

func (m *mockGroupRepo) ExistsHeadByCode(ctx context.Context, code rmgroup.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) ExistsHeadByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) IsOilGroupInUse(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) AddDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockGroupRepo) UpdateDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockGroupRepo) GetDetailByID(ctx context.Context, id uuid.UUID) (*rmgroup.Detail, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockGroupRepo) GetActiveDetailByItemCodeGrade(ctx context.Context, itemCode rmgroup.ItemCode, gradeCode string) (*rmgroup.Detail, error) {
	args := m.Called(ctx, itemCode, gradeCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockGroupRepo) ListDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockGroupRepo) ListActiveDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockGroupRepo) SoftDeleteDetail(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

func (m *mockGroupRepo) GetHeadPeriodSnapshot(ctx context.Context, headID uuid.UUID, period string) (*rmgroup.HeadPeriodSnapshot, error) {
	args := m.Called(ctx, headID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.HeadPeriodSnapshot), args.Error(1)
}

func (m *mockGroupRepo) UpsertHeadPeriod(ctx context.Context, snap *rmgroup.HeadPeriodSnapshot) error {
	return m.Called(ctx, snap).Error(0)
}

func (m *mockGroupRepo) GetDetailPeriodSnapshot(ctx context.Context, detailID uuid.UUID, period string) (*rmgroup.DetailPeriodSnapshot, error) {
	args := m.Called(ctx, detailID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.DetailPeriodSnapshot), args.Error(1)
}

func (m *mockGroupRepo) UpsertDetailPeriod(ctx context.Context, snap *rmgroup.DetailPeriodSnapshot) error {
	return m.Called(ctx, snap).Error(0)
}

func (m *mockGroupRepo) GetLatestHeadPeriodSnapshotBefore(ctx context.Context, headID uuid.UUID, period string) (*rmgroup.HeadPeriodSnapshot, error) {
	args := m.Called(ctx, headID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.HeadPeriodSnapshot), args.Error(1)
}

func (m *mockGroupRepo) GetLatestDetailPeriodSnapshotsBefore(ctx context.Context, headID uuid.UUID, period string) (map[uuid.UUID]*rmgroup.DetailPeriodSnapshot, error) {
	args := m.Called(ctx, headID, period)
	var out map[uuid.UUID]*rmgroup.DetailPeriodSnapshot
	if v := args.Get(0); v != nil {
		out = v.(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot)
	}
	return out, args.Error(1)
}

func (m *mockGroupRepo) InsertHeadPeriodIfAbsent(ctx context.Context, snap *rmgroup.HeadPeriodSnapshot) (bool, error) {
	args := m.Called(ctx, snap)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) InsertDetailPeriodIfAbsent(ctx context.Context, snap *rmgroup.DetailPeriodSnapshot) (bool, error) {
	args := m.Called(ctx, snap)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) LatestSyncPeriod(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func newCalcTestHead(t *testing.T) *rmgroup.Head {
	t.Helper()
	code, err := rmgroup.NewCode("GRP-CALC")
	require.NoError(t, err)
	head, err := rmgroup.NewHead(code, "Calc Group", "", 1.0, 10.0, "user:test")
	require.NoError(t, err)
	return head
}

func newCalcTestDetail(t *testing.T, headID uuid.UUID) *rmgroup.Detail {
	t.Helper()
	code, err := rmgroup.NewItemCode("ITEM-CALC")
	require.NoError(t, err)
	d, err := rmgroup.NewDetail(headID, code, "user:test")
	require.NoError(t, err)
	return d
}

// TestCalculateHandlerV2_LoadHeadAndDetails_UsesSnapshotWhenPresent locks the
// "snapshot exists for period" branch of the calc-engine's period-aware read
// (design §2.2's "Consequence for the calc engine"): when a period-scoped
// head/detail snapshot exists, the values fed into the calc engine come from
// the snapshot, not the anchor row.
func TestCalculateHandlerV2_LoadHeadAndDetails_UsesSnapshotWhenPresent(t *testing.T) {
	ctx := context.Background()
	head := newCalcTestHead(t)
	detail := newCalcTestDetail(t, head.ID())

	headSnap := rmgroup.NewHeadPeriodSnapshotFromHead("202503", head)
	headSnap.CostPercentage = 99.0

	detailSnap := rmgroup.NewDetailPeriodSnapshotFromDetail("202503", detail)
	snapFreight := 42.0
	detailSnap.ValuationInputs.FreightRate = &snapFreight

	repo := new(mockGroupRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(&headSnap, nil)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{detail}, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202503").Return(&detailSnap, nil)

	h := NewCalculateHandlerV2(repo, nil, nil, nil, nil)
	gotHead, gotDetails, _, err := h.loadHeadAndDetails(ctx, head.ID(), "202503")
	require.NoError(t, err)

	assert.InEpsilon(t, 99.0, gotHead.CostPercentage(), 1e-9)
	require.Len(t, gotDetails, 1)
	require.NotNil(t, gotDetails[0].ValuationInputs().FreightRate)
	assert.InEpsilon(t, 42.0, *gotDetails[0].ValuationInputs().FreightRate, 1e-9)
	// Anchor entities themselves must remain unmutated by the overlay.
	assert.NotEqual(t, 99.0, head.CostPercentage())
	repo.AssertExpectations(t)
}

// TestCalculateHandlerV2_LoadHeadAndDetails_FallsBackToAnchorRow locks the
// "falls back to anchor row" branch: when no period snapshot exists yet for
// (headID/detailID, period), the calc engine reads the anchor row's current
// values unchanged (get-or-create baseline, same fallback rule as
// rmgroup.GetHandler.overlayHeadPeriod).
func TestCalculateHandlerV2_LoadHeadAndDetails_FallsBackToAnchorRow(t *testing.T) {
	ctx := context.Background()
	head := newCalcTestHead(t)
	detail := newCalcTestDetail(t, head.ID())

	repo := new(mockGroupRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202601").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202601").Return(nil, rmgroup.ErrNotFound)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{detail}, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202601").
		Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestDetailPeriodSnapshotsBefore", ctx, mock.Anything, "202601").
		Return(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot{}, nil)

	h := NewCalculateHandlerV2(repo, nil, nil, nil, nil)
	gotHead, gotDetails, _, err := h.loadHeadAndDetails(ctx, head.ID(), "202601")
	require.NoError(t, err)

	assert.Equal(t, head.CostPercentage(), gotHead.CostPercentage())
	assert.Equal(t, head.Name(), gotHead.Name())
	require.Len(t, gotDetails, 1)
	assert.Equal(t, detail.ValuationInputs(), gotDetails[0].ValuationInputs())
	repo.AssertExpectations(t)
}

// TestCalculateHandlerV2_FreezeOnCalc_CarriesFromEarlierPeriod locks
// freeze-on-calc: calculating a period with no exact row uses the latest
// earlier period's values and persists them as that period's own row
// (is_backfilled = true, carried_from_period = source period) via
// insert-if-absent, so later edits to the earlier period do not drift it.
func TestCalculateHandlerV2_FreezeOnCalc_CarriesFromEarlierPeriod(t *testing.T) {
	ctx := context.Background()
	head := newCalcTestHead(t)
	detail := newCalcTestDetail(t, head.ID())

	prevHead := rmgroup.NewHeadPeriodSnapshotFromHead("202603", head)
	prevHead.CostPercentage = 55.0
	prevDetail := rmgroup.NewDetailPeriodSnapshotFromDetail("202603", detail)
	prevFreight := 3.25
	prevDetail.ValuationInputs.FreightRate = &prevFreight

	repo := new(mockGroupRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202605").Return(&prevHead, nil)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{detail}, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202605").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestDetailPeriodSnapshotsBefore", ctx, head.ID(), "202605").
		Return(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot{detail.ID(): &prevDetail}, nil)
	repo.On("InsertHeadPeriodIfAbsent", ctx, mock.MatchedBy(func(s *rmgroup.HeadPeriodSnapshot) bool {
		return s.Period == "202605" && s.IsBackfilled &&
			s.CarriedFromPeriod != nil && *s.CarriedFromPeriod == "202603" &&
			s.CostPercentage == 55.0 && s.CreatedBy == "calc-user"
	})).Return(true, nil).Once()
	repo.On("InsertDetailPeriodIfAbsent", ctx, mock.MatchedBy(func(s *rmgroup.DetailPeriodSnapshot) bool {
		return s.Period == "202605" && s.IsBackfilled &&
			s.CarriedFromPeriod != nil && *s.CarriedFromPeriod == "202603" &&
			s.ValuationInputs.FreightRate != nil && *s.ValuationInputs.FreightRate == 3.25
	})).Return(true, nil).Once()

	h := NewCalculateHandlerV2(repo, nil, nil, nil, nil)
	gotHead, gotDetails, freeze, err := h.loadHeadAndDetails(ctx, head.ID(), "202605")
	require.NoError(t, err)
	assert.InEpsilon(t, 55.0, gotHead.CostPercentage(), 1e-9)
	require.Len(t, gotDetails, 1)
	require.NotNil(t, gotDetails[0].ValuationInputs().FreightRate)
	assert.InEpsilon(t, 3.25, *gotDetails[0].ValuationInputs().FreightRate, 1e-9)

	require.NoError(t, h.freezeInheritedSnapshots(ctx, freeze, "calc-user"))
	repo.AssertExpectations(t)
}

// TestCalculateHandlerV2_FreezeOnCalc_AnchorHasNilCarriedFrom locks the
// anchor branch: the frozen row is still backfilled, but carried_from_period
// is NULL because no earlier period existed.
func TestCalculateHandlerV2_FreezeOnCalc_AnchorHasNilCarriedFrom(t *testing.T) {
	ctx := context.Background()
	head := newCalcTestHead(t)
	detail := newCalcTestDetail(t, head.ID())

	repo := new(mockGroupRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202601").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202601").Return(nil, rmgroup.ErrNotFound)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{detail}, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202601").Return(nil, rmgroup.ErrNotFound)
	repo.On("GetLatestDetailPeriodSnapshotsBefore", ctx, head.ID(), "202601").
		Return(map[uuid.UUID]*rmgroup.DetailPeriodSnapshot{}, nil)
	repo.On("InsertHeadPeriodIfAbsent", ctx, mock.MatchedBy(func(s *rmgroup.HeadPeriodSnapshot) bool {
		return s.Period == "202601" && s.IsBackfilled && s.CarriedFromPeriod == nil
	})).Return(true, nil).Once()
	repo.On("InsertDetailPeriodIfAbsent", ctx, mock.MatchedBy(func(s *rmgroup.DetailPeriodSnapshot) bool {
		return s.Period == "202601" && s.IsBackfilled && s.CarriedFromPeriod == nil
	})).Return(false, nil).Once() // already present (concurrent edit) — not an error

	h := NewCalculateHandlerV2(repo, nil, nil, nil, nil)
	_, _, freeze, err := h.loadHeadAndDetails(ctx, head.ID(), "202601")
	require.NoError(t, err)
	require.NoError(t, h.freezeInheritedSnapshots(ctx, freeze, "calc-user"))
	repo.AssertExpectations(t)
}

// TestCalculateHandlerV2_FreezeOnCalc_ExactRowsNotTouched locks that
// freeze-on-calc never writes when exact period rows already exist.
func TestCalculateHandlerV2_FreezeOnCalc_ExactRowsNotTouched(t *testing.T) {
	ctx := context.Background()
	head := newCalcTestHead(t)
	detail := newCalcTestDetail(t, head.ID())
	headSnap := rmgroup.NewHeadPeriodSnapshotFromHead("202503", head)
	detailSnap := rmgroup.NewDetailPeriodSnapshotFromDetail("202503", detail)

	repo := new(mockGroupRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(&headSnap, nil)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{detail}, nil)
	repo.On("GetDetailPeriodSnapshot", ctx, detail.ID(), "202503").Return(&detailSnap, nil)

	h := NewCalculateHandlerV2(repo, nil, nil, nil, nil)
	_, _, freeze, err := h.loadHeadAndDetails(ctx, head.ID(), "202503")
	require.NoError(t, err)
	require.NoError(t, h.freezeInheritedSnapshots(ctx, freeze, "calc-user"))
	repo.AssertNotCalled(t, "InsertHeadPeriodIfAbsent", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "InsertDetailPeriodIfAbsent", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
