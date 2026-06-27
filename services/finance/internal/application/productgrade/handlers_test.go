// Package productgrade_test provides unit tests for application layer handlers.
package productgrade_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/productgrade"
	productgradedomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// MockRepository is a mock implementation of productgradedomain.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *productgradedomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*productgradedomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productgradedomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*productgradedomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productgradedomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter productgradedomain.ListFilter) ([]*productgradedomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*productgradedomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *productgradedomain.Entity) error {
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

func newTestProductGrade(t *testing.T) *productgradedomain.Entity {
	t.Helper()
	entity, err := productgradedomain.New("A", "Grade A", "Top quality", 2.0, 1.5, 0.8, "", "", 0, 0, nil, nil, "notes", "admin")
	require.NoError(t, err)
	return entity
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new product grade", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := productgrade.CreateCommand{
			Code:           "A",
			Name:           "Grade A",
			Description:    "Top quality",
			BCPerc:         2.0,
			NonStdPerc:     1.5,
			BCRecoveryRate: 0.8,
			Notes:          "notes",
			CreatedBy:      "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "A").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*productgrade.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "A", result.Code())
		assert.Equal(t, "Grade A", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := productgrade.CreateCommand{
			Code:      "A",
			Name:      "Grade A",
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "A").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid input (empty name)", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := productgrade.CreateCommand{
			Code:      "A",
			Name:      "", // empty name triggers domain validation
			CreatedBy: "admin",
		}

		mockRepo.On("ExistsByCode", ctx, "A").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrEmptyName)
		mockRepo.AssertExpectations(t)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns product grade by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := newTestProductGrade(t)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := productgrade.GetQuery{ProductGradeID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "A", result.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, productgradedomain.ErrNotFound)

		query := productgrade.GetQuery{ProductGradeID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewGetHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, productgradedomain.ErrNotFound)

		query := productgrade.GetQuery{ProductGradeID: zeroID}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates product grade", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		existing := newTestProductGrade(t)
		newName := "Grade A Updated"

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*productgrade.Entity")).Return(nil)

		cmd := productgrade.UpdateCommand{
			ProductGradeID: id,
			Name:           &newName,
			UpdatedBy:      "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Grade A Updated", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, productgradedomain.ErrNotFound)

		newName := "Updated"
		cmd := productgrade.UpdateCommand{
			ProductGradeID: id,
			Name:           &newName,
			UpdatedBy:      "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - zero UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		zeroID := uuid.UUID{}
		mockRepo.On("GetByID", ctx, zeroID).Return(nil, productgradedomain.ErrNotFound)

		newName := "Updated"
		cmd := productgrade.UpdateCommand{
			ProductGradeID: zeroID,
			Name:           &newName,
			UpdatedBy:      "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes product grade", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := productgrade.DeleteCommand{ProductGradeID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(productgradedomain.ErrNotFound)

		cmd := productgrade.DeleteCommand{ProductGradeID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, productgradedomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := productgrade.NewListHandler(mockRepo)
		ctx := context.Background()

		grade1 := newTestProductGrade(t)
		grade2, err := productgradedomain.New("B", "Grade B", "Standard quality", 3.0, 2.0, 0.7, "", "", 0, 0, nil, nil, "", "admin")
		require.NoError(t, err)

		mockRepo.On("List", ctx, mock.AnythingOfType("productgrade.ListFilter")).Return(
			[]*productgradedomain.Entity{grade1, grade2},
			int64(2),
			nil,
		)

		query := productgrade.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Grades, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		mockRepo.AssertExpectations(t)
	})
}
