// Package permission_test provides unit tests for application layer permission handlers.
package permission_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appperm "github.com/mutugading/goapps-backend/services/iam/internal/application/permission"
	domainrole "github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockPermissionRepository is a mock implementation of role.PermissionRepository.
type MockPermissionRepository struct {
	mock.Mock
}

func (m *MockPermissionRepository) Create(ctx context.Context, permission *domainrole.Permission) error {
	args := m.Called(ctx, permission)
	return args.Error(0)
}

func (m *MockPermissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainrole.Permission, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainrole.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetByCode(ctx context.Context, code string) (*domainrole.Permission, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainrole.Permission), args.Error(1)
}

func (m *MockPermissionRepository) Update(ctx context.Context, permission *domainrole.Permission) error {
	args := m.Called(ctx, permission)
	return args.Error(0)
}

func (m *MockPermissionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockPermissionRepository) List(ctx context.Context, params domainrole.PermissionListParams) ([]*domainrole.Permission, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*domainrole.Permission), args.Get(1).(int64), args.Error(2)
}

func (m *MockPermissionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockPermissionRepository) BatchCreate(ctx context.Context, permissions []*domainrole.Permission) (int, error) {
	args := m.Called(ctx, permissions)
	return args.Int(0), args.Error(1)
}

func (m *MockPermissionRepository) GetByService(ctx context.Context, serviceName string, includeInactive bool) ([]*domainrole.ServicePermissions, error) {
	args := m.Called(ctx, serviceName, includeInactive)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainrole.ServicePermissions), args.Error(1)
}

func (m *MockPermissionRepository) ListByMenu(ctx context.Context, menuID uuid.UUID) ([]*domainrole.Permission, error) {
	args := m.Called(ctx, menuID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainrole.Permission), args.Error(1)
}

// =============================================================================
// CreateHandler
// =============================================================================

func TestCreateHandler(t *testing.T) {
	t.Run("success - creates permission", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appperm.CreateCommand{
			Code:        "iam.user.user.view",
			Name:        "View Users",
			Description: "Allows viewing user list",
			ServiceName: "iam",
			ModuleName:  "user",
			ActionType:  "view",
			CreatedBy:   "system",
		}

		mockRepo.On("ExistsByCode", ctx, "iam.user.user.view").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*role.Permission")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "iam.user.user.view", result.Code())
		assert.Equal(t, "View Users", result.Name())
		assert.Equal(t, "Allows viewing user list", result.Description())
		assert.Equal(t, "iam", result.ServiceName())
		assert.Equal(t, "user", result.ModuleName())
		assert.Equal(t, "view", result.ActionType())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appperm.CreateCommand{
			Code:        "iam.user.user.view",
			Name:        "View Users",
			ServiceName: "iam",
			ModuleName:  "user",
			ActionType:  "view",
			CreatedBy:   "system",
		}

		mockRepo.On("ExistsByCode", ctx, "iam.user.user.view").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrAlreadyExists)
	})

	t.Run("error - invalid code format", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appperm.CreateCommand{
			Code:        "invalid",
			Name:        "Test",
			ServiceName: "iam",
			ModuleName:  "user",
			ActionType:  "view",
			CreatedBy:   "system",
		}

		mockRepo.On("ExistsByCode", ctx, "invalid").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, domainrole.ErrInvalidPermissionCodeFormat)
	})
}

// =============================================================================
// GetHandler
// =============================================================================

func TestGetHandler(t *testing.T) {
	t.Run("success - returns permission by ID", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		expected := domainrole.ReconstructPermission(id, "iam.user.user.view", "View Users", "desc", "iam", "user", "view", true, audit, nil, "")

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := appperm.GetQuery{PermissionID: id.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "iam.user.user.view", result.Code())
		assert.Equal(t, id, result.ID())
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		query := appperm.GetQuery{PermissionID: id.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := appperm.GetQuery{PermissionID: "not-a-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

// =============================================================================
// UpdateHandler
// =============================================================================

func TestUpdateHandler(t *testing.T) {
	t.Run("success - updates permission name", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		existing := domainrole.ReconstructPermission(id, "iam.user.user.view", "View Users", "desc", "iam", "user", "view", true, audit, nil, "")

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*role.Permission")).Return(nil)

		newName := "View All Users"
		cmd := appperm.UpdateCommand{
			PermissionID: id.String(),
			Name:         &newName,
			UpdatedBy:    "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "View All Users", result.Name())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		newName := "Updated Name"
		cmd := appperm.UpdateCommand{
			PermissionID: id.String(),
			Name:         &newName,
			UpdatedBy:    "admin",
		}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

// =============================================================================
// DeleteHandler
// =============================================================================

func TestDeleteHandler(t *testing.T) {
	t.Run("success - soft deletes permission", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		existing := domainrole.ReconstructPermission(id, "iam.user.user.view", "View Users", "desc", "iam", "user", "view", true, audit, nil, "")

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Delete", ctx, id, "admin").Return(nil)

		cmd := appperm.DeleteCommand{PermissionID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		cmd := appperm.DeleteCommand{PermissionID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		cmd := appperm.DeleteCommand{PermissionID: "bad-uuid", DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

// =============================================================================
// ListHandler
// =============================================================================

func TestListHandler(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewListHandler(mockRepo)
		ctx := context.Background()

		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		perm1 := domainrole.ReconstructPermission(uuid.New(), "iam.user.user.view", "View Users", "desc", "iam", "user", "view", true, audit, nil, "")
		perm2 := domainrole.ReconstructPermission(uuid.New(), "iam.user.user.create", "Create Users", "desc", "iam", "user", "create", true, audit, nil, "")

		mockRepo.On("List", ctx, mock.AnythingOfType("role.PermissionListParams")).Return(
			[]*domainrole.Permission{perm1, perm2},
			int64(2),
			nil,
		)

		query := appperm.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Permissions, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("default pagination when values are zero", func(t *testing.T) {
		mockRepo := new(MockPermissionRepository)
		handler := appperm.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.MatchedBy(func(p domainrole.PermissionListParams) bool {
			return p.Page == 1 && p.PageSize == 10
		})).Return(
			[]*domainrole.Permission{},
			int64(0),
			nil,
		)

		query := appperm.ListQuery{Page: 0, PageSize: 0}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Permissions)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		mockRepo.AssertExpectations(t)
	})
}
