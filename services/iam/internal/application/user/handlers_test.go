// Package user_test provides unit tests for application layer user handlers.
package user_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appuser "github.com/mutugading/goapps-backend/services/iam/internal/application/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// =============================================================================
// Mocks
// =============================================================================

// MockUserRepo is a mock implementation of user.Repository.
type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) Create(ctx context.Context, u *user.User, detail *user.Detail) error {
	args := m.Called(ctx, u, detail)
	return args.Error(0)
}

func (m *MockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) Update(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

func (m *MockUserRepo) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockUserRepo) GetDetailByUserID(ctx context.Context, userID uuid.UUID) (*user.Detail, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Detail), args.Error(1)
}

func (m *MockUserRepo) UpdateDetail(ctx context.Context, detail *user.Detail) error {
	args := m.Called(ctx, detail)
	return args.Error(0)
}

func (m *MockUserRepo) List(ctx context.Context, params user.ListParams) ([]*user.User, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}

func (m *MockUserRepo) ListWithDetails(ctx context.Context, params user.ListParams) ([]*user.WithDetail, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*user.WithDetail), args.Get(1).(int64), args.Error(2)
}

func (m *MockUserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	args := m.Called(ctx, username)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepo) ExistsByEmployeeCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepo) BatchCreate(ctx context.Context, users []*user.User, details []*user.Detail) (int, error) {
	args := m.Called(ctx, users, details)
	return args.Int(0), args.Error(1)
}

func (m *MockUserRepo) GetRolesAndPermissions(ctx context.Context, userID uuid.UUID) ([]user.RoleRef, []user.PermissionRef, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]user.RoleRef), args.Get(1).([]user.PermissionRef), args.Error(2)
}

// MockUserRoleRepo is a mock implementation of role.UserRoleRepository.
type MockUserRoleRepo struct {
	mock.Mock
}

func (m *MockUserRoleRepo) AssignRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, assignedBy string) error {
	args := m.Called(ctx, userID, roleIDs, assignedBy)
	return args.Error(0)
}

func (m *MockUserRoleRepo) RemoveRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error {
	args := m.Called(ctx, userID, roleIDs)
	return args.Error(0)
}

func (m *MockUserRoleRepo) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]*role.Role, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*role.Role), args.Error(1)
}

func (m *MockUserRoleRepo) GetUsersWithRole(ctx context.Context, roleID uuid.UUID) ([]uuid.UUID, error) {
	args := m.Called(ctx, roleID)
	return args.Get(0).([]uuid.UUID), args.Error(1)
}

// MockUserPermissionRepo is a mock implementation of role.UserPermissionRepository.
type MockUserPermissionRepo struct {
	mock.Mock
}

func (m *MockUserPermissionRepo) AssignPermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	args := m.Called(ctx, userID, permissionIDs, assignedBy)
	return args.Error(0)
}

func (m *MockUserPermissionRepo) RemovePermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID) error {
	args := m.Called(ctx, userID, permissionIDs)
	return args.Error(0)
}

func (m *MockUserPermissionRepo) GetUserDirectPermissions(ctx context.Context, userID uuid.UUID) ([]*role.Permission, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*role.Permission), args.Error(1)
}

func (m *MockUserPermissionRepo) GetEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]*role.Permission, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*role.Permission), args.Error(1)
}

// =============================================================================
// Helpers
// =============================================================================

// newDummyUser creates a reconstructed user for mock returns.
func newDummyUser(id uuid.UUID) *user.User {
	return user.ReconstructUser(
		id,
		"testuser",
		"testuser@example.com",
		"$2a$10$hashedpassword",
		true,  // isActive
		false, // isLocked
		0,     // failedLoginAttempts
		nil,   // lockedUntil
		false, // twoFactorEnabled
		"",    // twoFactorSecret
		nil,   // lastLoginAt
		"",    // lastLoginIP
		nil,   // passwordChangedAt
		shared.NewAuditInfo("admin"),
	)
}

// =============================================================================
// Tests
// =============================================================================

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new user", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appuser.CreateCommand{
			Username:     "johndoe",
			Email:        "johndoe@example.com",
			PasswordHash: "$2a$10$hashedpassword",
			EmployeeCode: "EMP001",
			FullName:     "John Doe",
			FirstName:    "John",
			LastName:     "Doe",
			CreatedBy:    "admin",
		}

		mockRepo.On("ExistsByUsername", ctx, "johndoe").Return(false, nil)
		mockRepo.On("ExistsByEmail", ctx, "johndoe@example.com").Return(false, nil)
		mockRepo.On("ExistsByEmployeeCode", ctx, "EMP001").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*user.User"), mock.AnythingOfType("*user.Detail")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "johndoe", result.Username())
		assert.Equal(t, "johndoe@example.com", result.Email())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate username", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appuser.CreateCommand{
			Username:     "johndoe",
			Email:        "johndoe@example.com",
			PasswordHash: "$2a$10$hashedpassword",
			EmployeeCode: "EMP001",
			FullName:     "John Doe",
			FirstName:    "John",
			LastName:     "Doe",
			CreatedBy:    "admin",
		}

		mockRepo.On("ExistsByUsername", ctx, "johndoe").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid username format", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewCreateHandler(mockRepo)
		ctx := context.Background()

		cmd := appuser.CreateCommand{
			Username:     "1invalid", // starts with number
			Email:        "test@example.com",
			PasswordHash: "$2a$10$hashedpassword",
			EmployeeCode: "EMP001",
			FullName:     "Test User",
			FirstName:    "Test",
			LastName:     "User",
			CreatedBy:    "admin",
		}

		mockRepo.On("ExistsByUsername", ctx, "1invalid").Return(false, nil)
		mockRepo.On("ExistsByEmail", ctx, "test@example.com").Return(false, nil)
		mockRepo.On("ExistsByEmployeeCode", ctx, "EMP001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrInvalidUsername)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns user by ID", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := newDummyUser(id)

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := appuser.GetQuery{UserID: id.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, id, result.ID())
		assert.Equal(t, "testuser", result.Username())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		query := appuser.GetQuery{UserID: id.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := appuser.GetQuery{UserID: "not-a-valid-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes user", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		existing := newDummyUser(id)

		mockRepo.On("GetByID", ctx, id).Return(existing, nil)
		mockRepo.On("Delete", ctx, id, "admin").Return(nil)

		cmd := appuser.DeleteCommand{UserID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, shared.ErrNotFound)

		cmd := appuser.DeleteCommand{UserID: id.String(), DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		cmd := appuser.DeleteCommand{UserID: "bad-uuid", DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list with defaults", func(t *testing.T) {
		mockRepo := new(MockUserRepo)
		handler := appuser.NewListHandler(mockRepo)
		ctx := context.Background()

		id1 := uuid.New()
		id2 := uuid.New()
		u1 := newDummyUser(id1)
		u2 := newDummyUser(id2)

		usersWithDetails := []*user.WithDetail{
			{User: u1},
			{User: u2},
		}

		mockRepo.On("ListWithDetails", ctx, mock.AnythingOfType("user.ListParams")).Return(
			usersWithDetails,
			int64(2),
			nil,
		)

		// Use zero values to trigger pagination defaults (page=1, pageSize=10).
		query := appuser.ListQuery{Page: 0, PageSize: 0}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Users, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

func TestAssignRolesHandler_Handle(t *testing.T) {
	t.Run("success - assigns roles to user", func(t *testing.T) {
		mockUserRepo := new(MockUserRepo)
		mockUserRoleRepo := new(MockUserRoleRepo)
		handler := appuser.NewAssignRolesHandler(mockUserRepo, mockUserRoleRepo)
		ctx := context.Background()

		userID := uuid.New()
		roleID1 := uuid.New()
		roleID2 := uuid.New()

		existing := newDummyUser(userID)

		mockUserRepo.On("GetByID", ctx, userID).Return(existing, nil)
		mockUserRoleRepo.On("AssignRoles", ctx, userID, []uuid.UUID{roleID1, roleID2}, "admin").Return(nil)

		cmd := appuser.AssignRolesCommand{
			UserID:     userID.String(),
			RoleIDs:    []string{roleID1.String(), roleID2.String()},
			AssignedBy: "admin",
		}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockUserRepo.AssertExpectations(t)
		mockUserRoleRepo.AssertExpectations(t)
	})

	t.Run("error - user not found", func(t *testing.T) {
		mockUserRepo := new(MockUserRepo)
		mockUserRoleRepo := new(MockUserRoleRepo)
		handler := appuser.NewAssignRolesHandler(mockUserRepo, mockUserRoleRepo)
		ctx := context.Background()

		userID := uuid.New()
		roleID := uuid.New()

		mockUserRepo.On("GetByID", ctx, userID).Return(nil, shared.ErrNotFound)

		cmd := appuser.AssignRolesCommand{
			UserID:     userID.String(),
			RoleIDs:    []string{roleID.String()},
			AssignedBy: "admin",
		}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid role UUID", func(t *testing.T) {
		mockUserRepo := new(MockUserRepo)
		mockUserRoleRepo := new(MockUserRoleRepo)
		handler := appuser.NewAssignRolesHandler(mockUserRepo, mockUserRoleRepo)
		ctx := context.Background()

		userID := uuid.New()
		existing := newDummyUser(userID)

		mockUserRepo.On("GetByID", ctx, userID).Return(existing, nil)

		cmd := appuser.AssignRolesCommand{
			UserID:     userID.String(),
			RoleIDs:    []string{"not-a-valid-uuid"},
			AssignedBy: "admin",
		}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}
