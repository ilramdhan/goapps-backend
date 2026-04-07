// Package parameter provides unit tests for application layer handlers.
package parameter_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	paramapp "github.com/mutugading/goapps-backend/services/finance/internal/application/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// MockRepository is a mock implementation of parameter.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *parameter.Parameter) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*parameter.Parameter, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*parameter.Parameter), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code parameter.Code) (*parameter.Parameter, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*parameter.Parameter), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter parameter.ListFilter) ([]*parameter.Parameter, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*parameter.Parameter), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *parameter.Parameter) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByCode(ctx context.Context, code parameter.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ListAll(ctx context.Context, filter parameter.ExportFilter) ([]*parameter.Parameter, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*parameter.Parameter), args.Error(1)
}

func (m *MockRepository) ResolveUOMCode(ctx context.Context, uomCode string) (*uuid.UUID, error) {
	args := m.Called(ctx, uomCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*uuid.UUID), args.Error(1)
}

// =============================================================================
// CreateHandler Tests
// =============================================================================

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new parameter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.CreateCommand{
			ParamCode:      "SPEED",
			ParamName:      "Speed",
			ParamShortName: "Spd",
			DataType:       "NUMBER",
			ParamCategory:  "INPUT",
			DefaultValue:   "100",
			MinValue:       "0",
			MaxValue:       "9999",
			CreatedBy:      "admin",
		}

		mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("parameter.Code")).Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*parameter.Parameter")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "SPEED", result.Code().String())
		assert.Equal(t, "Speed", result.Name())
		assert.Equal(t, "Spd", result.ShortName())
		assert.Equal(t, "NUMBER", result.DataType().String())
		assert.Equal(t, "INPUT", result.ParamCategory().String())
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - with UOM reference", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		uomID := uuid.New()
		cmd := paramapp.CreateCommand{
			ParamCode:     "DENIER",
			ParamName:     "Denier",
			DataType:      "NUMBER",
			ParamCategory: "INPUT",
			UOMID:         uomID.String(),
			CreatedBy:     "admin",
		}

		mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("parameter.Code")).Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*parameter.Parameter")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.UOMID())
		assert.Equal(t, uomID, *result.UOMID())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.CreateCommand{
			ParamCode:     "SPEED",
			ParamName:     "Speed",
			DataType:      "NUMBER",
			ParamCategory: "INPUT",
			CreatedBy:     "admin",
		}

		mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("parameter.Code")).Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrAlreadyExists)
	})

	t.Run("error - invalid code format", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.CreateCommand{
			ParamCode:     "invalid",
			ParamName:     "Test",
			DataType:      "NUMBER",
			ParamCategory: "INPUT",
			CreatedBy:     "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
	})

	t.Run("error - invalid data type", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.CreateCommand{
			ParamCode:     "SPEED",
			ParamName:     "Speed",
			DataType:      "INVALID",
			ParamCategory: "INPUT",
			CreatedBy:     "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrInvalidDataType)
	})

	t.Run("error - invalid category", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.CreateCommand{
			ParamCode:     "SPEED",
			ParamName:     "Speed",
			DataType:      "NUMBER",
			ParamCategory: "UNKNOWN",
			CreatedBy:     "admin",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrInvalidParamCategory)
	})
}

// =============================================================================
// GetHandler Tests
// =============================================================================

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns parameter by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		expected, _ := parameter.NewParameter(code, "Speed", "Spd", dt, cat, nil, nil, nil, nil, "admin")

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := paramapp.GetQuery{ParamID: id.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "SPEED", result.Code().String())
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := paramapp.GetQuery{ParamID: "invalid-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, parameter.ErrNotFound)

		query := paramapp.GetQuery{ParamID: id.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})
}

// =============================================================================
// DeleteHandler Tests
// =============================================================================

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes parameter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()

		mockRepo.On("ExistsByID", ctx, id).Return(true, nil)
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := paramapp.DeleteCommand{ParamID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()

		mockRepo.On("ExistsByID", ctx, id).Return(false, nil)

		cmd := paramapp.DeleteCommand{ParamID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		cmd := paramapp.DeleteCommand{ParamID: "invalid", DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})
}

// =============================================================================
// ListHandler Tests
// =============================================================================

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewListHandler(mockRepo)
		ctx := context.Background()

		code1, _ := parameter.NewCode("SPEED")
		dt1, _ := parameter.NewDataType("NUMBER")
		cat1, _ := parameter.NewParamCategory("INPUT")
		p1, _ := parameter.NewParameter(code1, "Speed", "Spd", dt1, cat1, nil, nil, nil, nil, "admin")

		code2, _ := parameter.NewCode("DENIER")
		dt2, _ := parameter.NewDataType("NUMBER")
		cat2, _ := parameter.NewParamCategory("INPUT")
		p2, _ := parameter.NewParameter(code2, "Denier", "Den", dt2, cat2, nil, nil, nil, nil, "admin")

		mockRepo.On("List", ctx, mock.AnythingOfType("parameter.ListFilter")).Return(
			[]*parameter.Parameter{p1, p2},
			int64(2),
			nil,
		)

		query := paramapp.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Parameters, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
	})

	t.Run("success - with data type filter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("parameter.ListFilter")).Return(
			[]*parameter.Parameter{},
			int64(0),
			nil,
		)

		dt := "NUMBER"
		query := paramapp.ListQuery{Page: 1, PageSize: 10, DataType: &dt}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Parameters, 0)
	})

	t.Run("success - with category filter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("parameter.ListFilter")).Return(
			[]*parameter.Parameter{},
			int64(0),
			nil,
		)

		cat := "RATE"
		query := paramapp.ListQuery{Page: 1, PageSize: 10, ParamCategory: &cat}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Parameters, 0)
	})

	t.Run("error - invalid data type filter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewListHandler(mockRepo)
		ctx := context.Background()

		dt := "INVALID"
		query := paramapp.ListQuery{Page: 1, PageSize: 10, DataType: &dt}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrInvalidDataType)
	})
}

// =============================================================================
// UpdateHandler Tests
// =============================================================================

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - update name", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		existing, _ := parameter.NewParameter(code, "Speed", "Spd", dt, cat, nil, nil, nil, nil, "admin")

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*parameter.Parameter")).Return(nil)

		newName := "Speed Updated"
		cmd := paramapp.UpdateCommand{
			ParamID:   id.String(),
			ParamName: &newName,
			UpdatedBy: "editor",
		}

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.Equal(t, "Speed Updated", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, parameter.ErrNotFound)

		newName := "Test"
		cmd := paramapp.UpdateCommand{
			ParamID:   id.String(),
			ParamName: &newName,
			UpdatedBy: "editor",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := paramapp.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		newName := "Test"
		cmd := paramapp.UpdateCommand{
			ParamID:   "invalid",
			ParamName: &newName,
			UpdatedBy: "editor",
		}

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNotFound)
	})
}
