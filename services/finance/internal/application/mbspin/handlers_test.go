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
		expected, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
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
		entity, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
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
	t.Run("success - soft deletes entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
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
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(mbspindomain.ErrNotFound)

		cmd := mbspin.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbspindomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbspin.NewListHandler(mockRepo)
		ctx := context.Background()

		headID := uuid.New()
		entity1, err := mbspindomain.New(headID, "Spin Alpha", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)
		entity2, err := mbspindomain.New(headID, "Spin Beta", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
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
