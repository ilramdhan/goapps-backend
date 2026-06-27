// Package mbhead_test provides unit tests for MB Head application layer handlers.
package mbhead_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// MockRepository is a mock implementation of mbhead.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *mbheaddomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbheaddomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbheaddomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByMBCosting(ctx context.Context, mbCosting string) (*mbheaddomain.Entity, error) {
	args := m.Called(ctx, mbCosting)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbheaddomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter mbheaddomain.ListFilter) ([]*mbheaddomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*mbheaddomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *mbheaddomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error) {
	args := m.Called(ctx, mbCosting)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new MB Head", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbhead.CreateCommand{
			MBCosting: "MB001",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate mb_costing", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbhead.CreateCommand{
			MBCosting: "MB001",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - empty mb_costing", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbhead.CreateCommand{
			MBCosting: "",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByMBCosting", ctx, "").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrEmptyMBCosting)
	})

	t.Run("error - empty created_by", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := mbhead.CreateCommand{
			MBCosting: "MB002",
			CreatedBy: "",
		}

		mockRepo.On("ExistsByMBCosting", ctx, "MB002").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrEmptyCreatedBy)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns entity by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected, err := mbheaddomain.New("MB001", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := mbhead.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbheaddomain.ErrNotFound)

		query := mbhead.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, uuid.Nil).Return(nil, mbheaddomain.ErrNotFound)

		query := mbhead.GetQuery{ID: uuid.Nil}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity, err := mbheaddomain.New("MB001", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		newCosting := "MB001-UPD"
		cmd := mbhead.UpdateCommand{
			ID:        id,
			MBCosting: &newCosting,
			UpdatedBy: "admin",
		}

		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001-UPD", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbheaddomain.ErrNotFound)

		cmd := mbhead.UpdateCommand{ID: id, UpdatedBy: "admin"}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := mbhead.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(mbheaddomain.ErrNotFound)

		cmd := mbhead.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		entity1, err := mbheaddomain.New("MB001", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)
		entity2, err := mbheaddomain.New("MB002", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("mbhead.ListFilter")).Return(
			[]*mbheaddomain.Entity{entity1, entity2},
			int64(2),
			nil,
		)

		query := mbhead.ListQuery{Page: 1, PageSize: 10}
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
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("mbhead.ListFilter")).Return(
			[]*mbheaddomain.Entity{},
			int64(0),
			nil,
		)

		query := mbhead.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Equal(t, int64(0), result.TotalItems)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}
