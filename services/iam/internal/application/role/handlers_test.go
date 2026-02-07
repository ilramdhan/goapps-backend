// Package role_test provides unit tests for application layer role handlers.
package role_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	approle "github.com/mutugading/goapps-backend/services/iam/internal/application/role"
	domainrole "github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockRoleRepository is a mock implementation of role.Repository.
type MockRoleRepository struct {
	mock.Mock
}

func (m *MockRoleRepository) Create(ctx context.Context, r *domainrole.Role) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *MockRoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainrole.Role, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainrole.Role), args.Error(1)
}

func (m *MockRoleRepository) GetByCode(ctx context.Context, code string) (*domainrole.Role, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainrole.Role), args.Error(1)
}

func (m *MockRoleRepository) Update(ctx context.Context, r *domainrole.Role) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *MockRoleRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRoleRepository) List(ctx context.Context, params domainrole.ListParams) ([]*domainrole.Role, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*domainrole.Role), args.Get(1).(int64), args.Error(2)
}

func (m *MockRoleRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRoleRepository) BatchCreate(ctx context.Context, roles []*domainrole.Role) (int, error) {
	args := m.Called(ctx, roles)
	return args.Int(0), args.Error(1)
}

func (m *MockRoleRepository) AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	args := m.Called(ctx, roleID, permissionIDs, assignedBy)
	return args.Error(0)
}

func (m *MockRoleRepository) RemovePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	args := m.Called(ctx, roleID, permissionIDs)
	return args.Error(0)
}

func (m *MockRoleRepository) GetPermissions(ctx context.Context, roleID uuid.UUID) ([]*domainrole.Permission, error) {
	args := m.Called(ctx, roleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainrole.Permission), args.Error(1)
}

func (m *MockRoleRepository) GetRolesByPermission(ctx context.Context, permissionID uuid.UUID) ([]*domainrole.Role, error) {
	args := m.Called(ctx, permissionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainrole.Role), args.Error(1)
}

// =============================================================================
// CreateHandler
// =============================================================================

func TestCreateHandler(t *testing.T) {
	t.Run("success - creates role", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := approle.CreateCommand{
			Code:        "ADMIN",
			Name:        "Administrator",
			Description: "Full access role",
			CreatedBy:   "system",
		}

		mockRepo.On("ExistsByCode", ctx, "ADMIN").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*role.Role")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ADMIN", result.Code())
		assert.Equal(t, "Administrator", result.Name())
		assert.Equal(t, "Full access role", result.Description())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate code", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := approle.CreateCommand{
			Code:      "ADMIN",
			Name:      "Administrator",
			CreatedBy: "system",
		}

		mockRepo.On("ExistsByCode", ctx, "ADMIN").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrAlreadyExists)
	})

	t.Run("error - invalid code format", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := approle.CreateCommand{
			Code:      "invalid",
			Name:      "Test",
			CreatedBy: "system",
		}

		mockRepo.On("ExistsByCode", ctx, "invalid").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, domainrole.ErrInvalidRoleCodeFormat)
	})
}

// =============================================================================
// GetHandler
// =============================================================================

func TestGetHandler(t *testing.T) {
	t.Run("success - returns role by ID", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		expected := domainrole.ReconstructRole(id, "ADMIN", "Administrator", "desc", false, true, audit)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := approle.GetQuery{RoleID: id.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ADMIN", result.Code())
		assert.Equal(t, id, result.ID())
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		query := approle.GetQuery{RoleID: id.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := approle.GetQuery{RoleID: "not-a-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

// =============================================================================
// DeleteHandler
// =============================================================================

func TestDeleteHandler(t *testing.T) {
	t.Run("success - soft deletes role", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		existing := domainrole.ReconstructRole(id, "CUSTOM", "Custom Role", "desc", false, true, audit)

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Delete", ctx, id, "admin").Return(nil)

		cmd := approle.DeleteCommand{RoleID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		cmd := approle.DeleteCommand{RoleID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		cmd := approle.DeleteCommand{RoleID: "bad-uuid", DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - system role cannot be deleted", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		systemRole := domainrole.ReconstructRole(id, "SUPER_ADMIN", "Super Admin", "desc", true, true, audit)

		mockRepo.On("GetByID", ctx, id).Return(systemRole, nil)

		cmd := approle.DeleteCommand{RoleID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, domainrole.ErrSystemRoleDelete)
	})
}

// =============================================================================
// ListHandler
// =============================================================================

func TestListHandler(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewListHandler(mockRepo)
		ctx := context.Background()

		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		role1 := domainrole.ReconstructRole(uuid.New(), "ADMIN", "Administrator", "desc", false, true, audit)
		role2 := domainrole.ReconstructRole(uuid.New(), "EDITOR", "Editor", "desc", false, true, audit)

		mockRepo.On("List", ctx, mock.AnythingOfType("role.ListParams")).Return(
			[]*domainrole.Role{role1, role2},
			int64(2),
			nil,
		)

		query := approle.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Roles, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("default pagination when values are zero", func(t *testing.T) {
		mockRepo := new(MockRoleRepository)
		handler := approle.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.MatchedBy(func(p domainrole.ListParams) bool {
			return p.Page == 1 && p.PageSize == 10
		})).Return(
			[]*domainrole.Role{},
			int64(0),
			nil,
		)

		query := approle.ListQuery{Page: 0, PageSize: 0}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Roles)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		mockRepo.AssertExpectations(t)
	})
}
