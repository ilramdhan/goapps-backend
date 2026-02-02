// Package uom provides unit tests for application layer handlers.
package uom_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/uom"
	uomdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// MockRepository is a mock implementation of uom.Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *uomdomain.UOM) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*uomdomain.UOM, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*uomdomain.UOM), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code uomdomain.Code) (*uomdomain.UOM, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*uomdomain.UOM), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, entity *uomdomain.UOM) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) List(ctx context.Context, filter uomdomain.ListFilter) ([]*uomdomain.UOM, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*uomdomain.UOM), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) ListAll(ctx context.Context, filter uomdomain.ExportFilter) ([]*uomdomain.UOM, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*uomdomain.UOM), args.Error(1)
}

func (m *MockRepository) ExistsByCode(ctx context.Context, code uomdomain.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new UOM", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := uom.CreateCommand{
			UOMCode:     "KG",
			UOMName:     "Kilogram",
			UOMCategory: "WEIGHT",
			Description: "Weight in kilograms",
			CreatedBy:   "admin",
		}

		// Setup expectations
		mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("uom.Code")).Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*uom.UOM")).Return(nil)

		// Execute
		result, err := handler.Handle(ctx, cmd)

		// Assert
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "KG", result.Code().String())
		assert.Equal(t, "Kilogram", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := uom.CreateCommand{
			UOMCode:     "KG",
			UOMName:     "Kilogram",
			UOMCategory: "WEIGHT",
			CreatedBy:   "admin",
		}

		mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("uom.Code")).Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, uomdomain.ErrAlreadyExists)
	})

	t.Run("error - invalid code format", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := uom.CreateCommand{
			UOMCode:     "invalid",
			UOMName:     "Test",
			UOMCategory: "WEIGHT",
			CreatedBy:   "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns UOM by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		code, _ := uomdomain.NewCode("KG")
		category, _ := uomdomain.NewCategory("WEIGHT")
		expected, _ := uomdomain.NewUOM(code, "Kilogram", category, "Weight", "admin")

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := uom.GetQuery{UOMID: id.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "KG", result.Code().String())
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := uom.GetQuery{UOMID: "invalid-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, uomdomain.ErrNotFound)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, uomdomain.ErrNotFound)

		query := uom.GetQuery{UOMID: id.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, uomdomain.ErrNotFound)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes UOM", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()

		mockRepo.On("ExistsByID", ctx, id).Return(true, nil)
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := uom.DeleteCommand{UOMID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()

		mockRepo.On("ExistsByID", ctx, id).Return(false, nil)

		cmd := uom.DeleteCommand{UOMID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, uomdomain.ErrNotFound)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := uom.NewListHandler(mockRepo)
		ctx := context.Background()

		code1, _ := uomdomain.NewCode("KG")
		cat1, _ := uomdomain.NewCategory("WEIGHT")
		uom1, _ := uomdomain.NewUOM(code1, "Kilogram", cat1, "", "admin")

		code2, _ := uomdomain.NewCode("LTR")
		cat2, _ := uomdomain.NewCategory("VOLUME")
		uom2, _ := uomdomain.NewUOM(code2, "Liter", cat2, "", "admin")

		mockRepo.On("List", ctx, mock.AnythingOfType("uom.ListFilter")).Return(
			[]*uomdomain.UOM{uom1, uom2},
			int64(2),
			nil,
		)

		query := uom.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.UOMs, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
	})
}
