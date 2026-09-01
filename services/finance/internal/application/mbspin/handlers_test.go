// Package mbspin_test provides unit tests for MB Spin application layer handlers.
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

// MockRepository is a mock implementation of mbspin.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *mbspindomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbspindomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbspindomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter mbspindomain.ListFilter) ([]*mbspindomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*mbspindomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *mbspindomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) GetByMBCosting(ctx context.Context, code string) (*mbspindomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbspindomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByOrionItemCode(ctx context.Context, code string) (*mbspindomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbspindomain.Entity), args.Error(1)
}

// --- P8 duplicate/lineage primitives. Not exercised by these tests; present so
// MockRepository still satisfies the widened mbspin.Repository interface. ---

func (m *MockRepository) DuplicateSpin(ctx context.Context, in mbspindomain.DuplicateInput) (mbspindomain.DuplicateOutput, error) {
	args := m.Called(ctx, in)
	out, _ := args.Get(0).(mbspindomain.DuplicateOutput)
	return out, args.Error(1)
}

func (m *MockRepository) ListChildren(ctx context.Context, parentID uuid.UUID) ([]*mbspindomain.Entity, error) {
	args := m.Called(ctx, parentID)
	items, _ := args.Get(0).([]*mbspindomain.Entity)
	return items, args.Error(1)
}

func (m *MockRepository) ExistsByOrionItemCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ResolveUniqueByOrionItemCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	args := m.Called(ctx, code)
	id, _ := args.Get(0).(uuid.UUID)
	return id, args.Bool(1), args.Error(2)
}

func (m *MockRepository) ListByOrionItemCode(ctx context.Context, code string) ([]*mbspindomain.Entity, error) {
	args := m.Called(ctx, code)
	items, _ := args.Get(0).([]*mbspindomain.Entity)
	return items, args.Error(1)
}

func (m *MockRepository) HasChildren(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) IsUsedByCostProduct(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new MB Spin", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewCreateHandler(mockRepo)
		ctx := context.Background()

		headID := uuid.New()
		cmd := mbspin.CreateCommand{
			HeadID:    headID,
			MgtName:   "Spin Alpha",
			CreatedBy: "admin",
		}

		mockRepo.On("Create", ctx, mock.AnythingOfType("*mbspin.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Spin Alpha", result.MgtName())
		assert.Equal(t, headID, result.HeadID())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil head ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbspin.CreateCommand{
			HeadID:    uuid.Nil,
			MgtName:   "Spin Alpha",
			CreatedBy: "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrInvalidHeadID)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("error - empty mgt_name", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbspin.CreateCommand{
			HeadID:    uuid.New(),
			MgtName:   "",
			CreatedBy: "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrEmptyMgtName)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("error - empty created_by", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbspin.CreateCommand{
			HeadID:    uuid.New(),
			MgtName:   "Spin Alpha",
			CreatedBy: "",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrEmptyCreatedBy)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("error - repo returns error", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbspin.CreateCommand{
			HeadID:    uuid.New(),
			MgtName:   "Spin Alpha",
			CreatedBy: "admin",
		}

		mockRepo.On("Create", ctx, mock.AnythingOfType("*mbspin.Entity")).Return(mbspindomain.ErrAlreadyExists)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns entity by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		headID := uuid.New()
		expected, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := mbspin.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Spin Alpha", result.MgtName())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbspindomain.ErrNotFound)

		query := mbspin.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewGetHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, uuid.Nil).Return(nil, mbspindomain.ErrNotFound)

		query := mbspin.GetQuery{ID: uuid.Nil}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrNotFound)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		headID := uuid.New()
		entity, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		newName := "Spin Beta"
		cmd := mbspin.UpdateCommand{
			ID:        id,
			MgtName:   &newName,
			UpdatedBy: "admin",
		}

		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*mbspin.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Spin Beta", result.MgtName())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbspindomain.ErrNotFound)

		cmd := mbspin.UpdateCommand{ID: id, UpdatedBy: "admin"}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbspindomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes entity when no children and no product usage", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("ExistsByID", ctx, id).Return(true, nil)
		mockRepo.On("HasChildren", ctx, id).Return(false, nil)
		mockRepo.On("IsUsedByCostProduct", ctx, id).Return(false, nil)
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := mbspin.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("ExistsByID", ctx, id).Return(false, nil)

		cmd := mbspin.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbspindomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - blocked when spin has live children", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("ExistsByID", ctx, id).Return(true, nil)
		mockRepo.On("HasChildren", ctx, id).Return(true, nil)

		cmd := mbspin.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbspindomain.ErrHasChildren)
		mockRepo.AssertExpectations(t)
		mockRepo.AssertNotCalled(t, "IsUsedByCostProduct", ctx, id)
		mockRepo.AssertNotCalled(t, "SoftDelete", ctx, id, "admin")
	})

	t.Run("error - blocked when spin is referenced by a cost product parameter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("ExistsByID", ctx, id).Return(true, nil)
		mockRepo.On("HasChildren", ctx, id).Return(false, nil)
		mockRepo.On("IsUsedByCostProduct", ctx, id).Return(true, nil)

		cmd := mbspin.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbspindomain.ErrInUse)
		mockRepo.AssertExpectations(t)
		mockRepo.AssertNotCalled(t, "SoftDelete", ctx, id, "admin")
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewListHandler(mockRepo)
		ctx := context.Background()

		headID := uuid.New()
		entity1, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)
		entity2, err := mbspindomain.New(headID, "Spin Beta", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("mbspin.ListFilter")).Return(
			[]*mbspindomain.Entity{entity1, entity2},
			int64(2),
			nil,
		)

		query := mbspin.ListQuery{HeadID: headID, Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - empty result", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("mbspin.ListFilter")).Return(
			[]*mbspindomain.Entity{},
			int64(0),
			nil,
		)

		query := mbspin.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Equal(t, int64(0), result.TotalItems)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

// TestUpdateHandler_LDRAdjustmentAndLock_DoesNotTriggerRecalc is the regression
// test for Task E business rule 3: the three new LDR mutations (set-adjustment /
// lock / unlock) must stay isolated from the recalc cascade trigger.
// recalcTriggered only reacts to Denier/Filament/Dozing, so a command that ONLY
// carries LDRAdjustmentPct/LDRLockActual must leave UpdateResult.Recalc nil even
// on a handler that IS wired for recalc.
func TestUpdateHandler_LDRAdjustmentAndLock_DoesNotTriggerRecalc(t *testing.T) {
	mockRepo := new(MockRepository)
	ctx := context.Background()

	id := uuid.New()
	headID := uuid.New()
	entity, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	rr := &fakeRecalcRepo{}
	svc := mbspin.NewRecalcService(mockRepo, rr, nil, nil)
	handler := mbspin.NewUpdateHandlerWithRecalc(mockRepo, svc)

	// Adjustment-only: the spin starts unlocked, so this is a legal mutation on
	// its own (rule 4 — adjustment freezes only while locked — does not apply
	// here since LDRLockActual is absent/no-op).
	adj := 1.5
	cmd := mbspin.UpdateCommand{
		ID:               id,
		LDRAdjustmentPct: &adj,
		UpdatedBy:        "admin",
	}

	mockRepo.On("GetByID", ctx, id).Return(entity, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*mbspin.Entity")).Return(nil)

	res, err := handler.HandleWithRecalc(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Nil(t, res.Recalc, "LDR-only changes must never arm the recalc cascade")
	assert.Empty(t, rr.readParents, "recalc must not even read children when the trigger did not fire")
	require.NotNil(t, res.Entity.LDRAdjustmentPct())
	assert.InDelta(t, 1.5, *res.Entity.LDRAdjustmentPct(), 0.001)
	mockRepo.AssertExpectations(t)
}

// TestUpdateHandler_LDRLockActual_DoesNotTriggerRecalc covers the lock/unlock
// half of business rule 3 separately from the adjustment half above: locking
// alone (no denier/filament/dozing change) must also leave Recalc nil.
func TestUpdateHandler_LDRLockActual_DoesNotTriggerRecalc(t *testing.T) {
	mockRepo := new(MockRepository)
	ctx := context.Background()

	id := uuid.New()
	headID := uuid.New()
	entity, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	rr := &fakeRecalcRepo{}
	svc := mbspin.NewRecalcService(mockRepo, rr, nil, nil)
	handler := mbspin.NewUpdateHandlerWithRecalc(mockRepo, svc)

	lock := true
	cmd := mbspin.UpdateCommand{
		ID:            id,
		LDRLockActual: &lock,
		UpdatedBy:     "admin",
	}

	mockRepo.On("GetByID", ctx, id).Return(entity, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*mbspin.Entity")).Return(nil)

	res, err := handler.HandleWithRecalc(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Nil(t, res.Recalc, "LDR-only changes must never arm the recalc cascade")
	assert.Empty(t, rr.readParents, "recalc must not even read children when the trigger did not fire")
	assert.True(t, res.Entity.LDRIsActual())
	mockRepo.AssertExpectations(t)
}

// TestUpdateHandler_SetLDRAdjustment_RejectedWhenLocked proves the domain
// rejection (ErrLDRLockedActual) propagates as a real error out of
// HandleWithRecalc/Handle instead of being swallowed, and that the repository's
// Update is never reached once the domain mutation fails.
func TestUpdateHandler_SetLDRAdjustment_RejectedWhenLocked(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbspin.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	headID := uuid.New()
	entity, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	entity.LockLDRActual()

	adj := 2.5
	cmd := mbspin.UpdateCommand{
		ID:               id,
		LDRAdjustmentPct: &adj,
		UpdatedBy:        "admin",
	}

	mockRepo.On("GetByID", ctx, id).Return(entity, nil)

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, mbspindomain.ErrLDRLockedActual)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}
