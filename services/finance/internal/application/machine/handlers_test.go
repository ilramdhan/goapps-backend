// Package machine_test provides unit tests for application layer handlers.
package machine_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/machine"
	machinedomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// MockRepository is a mock implementation of machinedomain.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *machinedomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*machinedomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*machinedomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*machinedomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*machinedomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter machinedomain.ListFilter) ([]*machinedomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*machinedomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *machinedomain.Entity) error {
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

func newTestMachine(t *testing.T) *machinedomain.Entity {
	t.Helper()
	entity, err := machinedomain.New(
		"MC001", "Machine One", "DTY", "Hall A",
		96, 2, 600.0, nil, 0.95, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"test notes", "admin",
	)
	require.NoError(t, err)
	return entity
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new machine", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := machine.CreateCommand{
			Code:         "MC001",
			Name:         "Machine One",
			MCType:       "DTY",
			Location:     "Hall A",
			NoOfPosition: 96,
			NoOfEnd:      2,
			MCSpeed:      600.0,
			MCEfficiency: 0.95,
			Notes:        "test notes",
			CreatedBy:    "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "MC001").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*machine.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MC001", result.Code())
		assert.Equal(t, "Machine One", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := machine.CreateCommand{
			Code:      "MC001",
			Name:      "Machine One",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "MC001").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid input (empty name)", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := machine.CreateCommand{
			Code:      "MC001",
			Name:      "", // empty name triggers domain validation
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "MC001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrEmptyName)
		mockRepo.AssertExpectations(t)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns machine by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := newTestMachine(t)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := machine.GetQuery{MachineID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MC001", result.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, machinedomain.ErrNotFound)

		query := machine.GetQuery{MachineID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewGetHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, machinedomain.ErrNotFound)

		query := machine.GetQuery{MachineID: zeroID}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates machine", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		existing := newTestMachine(t)
		newName := "Machine One Updated"

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*machine.Entity")).Return(nil)

		cmd := machine.UpdateCommand{
			MachineID: id,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Machine One Updated", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, machinedomain.ErrNotFound)

		newName := "Updated"
		cmd := machine.UpdateCommand{
			MachineID: id,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, machinedomain.ErrNotFound)

		newName := "Updated"
		cmd := machine.UpdateCommand{
			MachineID: zeroID,
			Name:      &newName,
			UpdatedBy: "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes machine", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := machine.DeleteCommand{MachineID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(machinedomain.ErrNotFound)

		cmd := machine.DeleteCommand{MachineID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, machinedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := machine.NewListHandler(mockRepo)
		ctx := context.Background()

		m1 := newTestMachine(t)
		m2, err := machinedomain.New("MC002", "Machine Two", "POY", "Hall B", 48, 1, 400.0, nil, 0.90, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("machine.ListFilter")).Return(
			[]*machinedomain.Entity{m1, m2},
			int64(2),
			nil,
		)

		query := machine.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Machines, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		mockRepo.AssertExpectations(t)
	})
}
