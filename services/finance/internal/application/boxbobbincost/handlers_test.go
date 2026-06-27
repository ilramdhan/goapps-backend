// Package boxbobbincost_test provides unit tests for Box Bobbin Cost application layer handlers.
package boxbobbincost_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/boxbobbincost"
	bbcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// MockRepository is a mock implementation of boxbobbincost.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *bbcdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*bbcdomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bbcdomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*bbcdomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bbcdomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter bbcdomain.ListFilter) ([]*bbcdomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*bbcdomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *bbcdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ListRates(ctx context.Context, parentID uuid.UUID) ([]*bbcdomain.RateEntry, error) {
	args := m.Called(ctx, parentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*bbcdomain.RateEntry), args.Error(1)
}

func (m *MockRepository) CreateRate(ctx context.Context, rate *bbcdomain.RateEntry) error {
	args := m.Called(ctx, rate)
	return args.Error(0)
}

func (m *MockRepository) DeleteRate(ctx context.Context, rateID uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, rateID, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := boxbobbincost.CreateCommand{
			Code:      "BBC001",
			Name:      "Box Bobbin A",
			BBCType:   "BOX",
			NoOfBob:   10,
			Notes:     "Test notes",
			CreatedBy: "admin",
		}

		mockRepo.On("Create", ctx, mock.AnythingOfType("*boxbobbincost.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "BBC001", result.Code())
		assert.Equal(t, "Box Bobbin A", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code (repo returns ErrAlreadyExists)", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := boxbobbincost.CreateCommand{
			Code:      "BBC001",
			Name:      "Box Bobbin A",
			CreatedBy: "admin",
		}

		mockRepo.On("Create", ctx, mock.AnythingOfType("*boxbobbincost.Entity")).Return(bbcdomain.ErrAlreadyExists)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - empty code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := boxbobbincost.CreateCommand{
			Code:      "",
			Name:      "Box Bobbin A",
			CreatedBy: "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrEmptyCode)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("error - empty name", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := boxbobbincost.CreateCommand{
			Code:      "BBC002",
			Name:      "",
			CreatedBy: "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrEmptyName)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("error - empty created_by", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := boxbobbincost.CreateCommand{
			Code:      "BBC003",
			Name:      "Box Bobbin C",
			CreatedBy: "",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrEmptyCreatedBy)
		mockRepo.AssertNotCalled(t, "Create")
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns entity by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected, err := bbcdomain.New("BBC001", "Box Bobbin A", "BOX", 10, "", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := boxbobbincost.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "BBC001", result.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, bbcdomain.ErrNotFound)

		query := boxbobbincost.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewGetHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, uuid.Nil).Return(nil, bbcdomain.ErrNotFound)

		query := boxbobbincost.GetQuery{ID: uuid.Nil}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrNotFound)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity, err := bbcdomain.New("BBC001", "Box Bobbin A", "BOX", 10, "", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		newName := "Box Bobbin Updated"
		cmd := boxbobbincost.UpdateCommand{
			ID:        id,
			Name:      &newName,
			UpdatedBy: "admin",
		}

		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*boxbobbincost.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Box Bobbin Updated", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, bbcdomain.ErrNotFound)

		cmd := boxbobbincost.UpdateCommand{ID: id, UpdatedBy: "admin"}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("Delete", ctx, id, "admin").Return(nil)

		cmd := boxbobbincost.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("Delete", ctx, id, "admin").Return(bbcdomain.ErrNotFound)

		cmd := boxbobbincost.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, bbcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewListHandler(mockRepo)
		ctx := context.Background()

		entity1, err := bbcdomain.New("BBC001", "Box Bobbin A", "BOX", 10, "", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)
		entity2, err := bbcdomain.New("BBC002", "Box Bobbin B", "BOB", 5, "", nil, nil, nil, nil, nil, nil, nil, nil, "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("boxbobbincost.ListFilter")).Return(
			[]*bbcdomain.Entity{entity1, entity2},
			int64(2),
			nil,
		)

		query := boxbobbincost.ListQuery{Page: 1, PageSize: 10}
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
		handler := boxbobbincost.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("boxbobbincost.ListFilter")).Return(
			[]*bbcdomain.Entity{},
			int64(0),
			nil,
		)

		query := boxbobbincost.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Equal(t, int64(0), result.TotalItems)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

func TestCreateRateHandler_Handle(t *testing.T) {
	t.Run("success - creates new rate entry", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateRateHandler(mockRepo)
		ctx := context.Background()

		parentID := uuid.New()
		cmd := boxbobbincost.CreateRateCommand{
			ParentID:   parentID,
			Period:     "202501",
			BobRateMkt: 100.0,
			BoxRateMkt: 200.0,
			CreatedBy:  "admin",
		}

		mockRepo.On("CreateRate", ctx, mock.AnythingOfType("*boxbobbincost.RateEntry")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, parentID, result.ParentID())
		assert.Equal(t, "202501", result.Period())
		assert.InDelta(t, 100.0, result.BobRateMkt(), 0.001)
		assert.InDelta(t, 200.0, result.BoxRateMkt(), 0.001)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate period", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewCreateRateHandler(mockRepo)
		ctx := context.Background()

		parentID := uuid.New()
		cmd := boxbobbincost.CreateRateCommand{
			ParentID:   parentID,
			Period:     "202501",
			BobRateMkt: 100.0,
			BoxRateMkt: 200.0,
			CreatedBy:  "admin",
		}

		mockRepo.On("CreateRate", ctx, mock.AnythingOfType("*boxbobbincost.RateEntry")).Return(bbcdomain.ErrDuplicatePeriod)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, bbcdomain.ErrDuplicatePeriod)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteRateHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes rate entry", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewDeleteRateHandler(mockRepo)
		ctx := context.Background()

		rateID := uuid.New()
		mockRepo.On("DeleteRate", ctx, rateID, "admin").Return(nil)

		cmd := boxbobbincost.DeleteRateCommand{RateID: rateID, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - rate not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := boxbobbincost.NewDeleteRateHandler(mockRepo)
		ctx := context.Background()

		rateID := uuid.New()
		mockRepo.On("DeleteRate", ctx, rateID, "admin").Return(bbcdomain.ErrNotFound)

		cmd := boxbobbincost.DeleteRateCommand{RateID: rateID, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, bbcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}
