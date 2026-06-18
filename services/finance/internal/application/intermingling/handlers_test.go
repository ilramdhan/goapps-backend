// Package intermingling_test provides unit tests for application layer handlers.
package intermingling_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/intermingling"
	interminglingdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// MockRepository is a mock implementation of interminglingdomain.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *interminglingdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*interminglingdomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interminglingdomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*interminglingdomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interminglingdomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter interminglingdomain.ListFilter) ([]*interminglingdomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*interminglingdomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *interminglingdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func newTestIntermingling(t *testing.T) *interminglingdomain.Entity {
	t.Helper()
	entity, err := interminglingdomain.New("HIM", "High Intermingling", 0.5, "test notes", "admin")
	require.NoError(t, err)
	return entity
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new intermingling", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := intermingling.CreateCommand{
			Code:      "HIM",
			Name:      "High Intermingling",
			CostPerKg: 0.5,
			Notes:     "test notes",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "HIM").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*intermingling.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "HIM", result.Code())
		assert.Equal(t, "High Intermingling", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := intermingling.CreateCommand{
			Code:      "HIM",
			Name:      "High Intermingling",
			CostPerKg: 0.5,
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "HIM").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid input (empty name)", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := intermingling.CreateCommand{
			Code:      "HIM",
			Name:      "", // empty name triggers domain validation
			CostPerKg: 0.5,
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "HIM").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrEmptyName)
		mockRepo.AssertExpectations(t)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns intermingling by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := newTestIntermingling(t)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := intermingling.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "HIM", result.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, interminglingdomain.ErrNotFound)

		query := intermingling.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewGetHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, interminglingdomain.ErrNotFound)

		query := intermingling.GetQuery{ID: zeroID}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates intermingling", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		existing := newTestIntermingling(t)
		newName := "High Intermingling Updated"

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*intermingling.Entity")).Return(nil)

		cmd := intermingling.UpdateCommand{
			ID:        id,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "High Intermingling Updated", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, interminglingdomain.ErrNotFound)

		newName := "Updated"
		cmd := intermingling.UpdateCommand{
			ID:        id,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, interminglingdomain.ErrNotFound)

		newName := "Updated"
		cmd := intermingling.UpdateCommand{
			ID:        zeroID,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes intermingling", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := intermingling.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(interminglingdomain.ErrNotFound)

		cmd := intermingling.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, interminglingdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := intermingling.NewListHandler(mockRepo)
		ctx := context.Background()

		item1 := newTestIntermingling(t)
		item2, err := interminglingdomain.New("SIM", "Standard Intermingling", 0.3, "", "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("intermingling.ListFilter")).Return(
			[]*interminglingdomain.Entity{item1, item2},
			int64(2),
			nil,
		)

		query := intermingling.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		mockRepo.AssertExpectations(t)
	})
}
