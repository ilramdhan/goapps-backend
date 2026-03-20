// Package menu_test provides unit tests for the menu application service.
package menu_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appmenu "github.com/mutugading/goapps-backend/services/iam/internal/application/menu"
	domainmenu "github.com/mutugading/goapps-backend/services/iam/internal/domain/menu"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockMenuRepository is a mock implementation of menu.Repository.
type MockMenuRepository struct {
	mock.Mock
}

func (m *MockMenuRepository) Create(ctx context.Context, menu *domainmenu.Menu) error {
	args := m.Called(ctx, menu)
	return args.Error(0)
}

func (m *MockMenuRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainmenu.Menu, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainmenu.Menu), args.Error(1)
}

func (m *MockMenuRepository) GetByCode(ctx context.Context, code string) (*domainmenu.Menu, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainmenu.Menu), args.Error(1)
}

func (m *MockMenuRepository) Update(ctx context.Context, menu *domainmenu.Menu) error {
	args := m.Called(ctx, menu)
	return args.Error(0)
}

func (m *MockMenuRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockMenuRepository) DeleteWithChildren(ctx context.Context, id uuid.UUID, deletedBy string) (int, error) {
	args := m.Called(ctx, id, deletedBy)
	return args.Int(0), args.Error(1)
}

func (m *MockMenuRepository) List(ctx context.Context, params domainmenu.ListParams) ([]*domainmenu.Menu, int64, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*domainmenu.Menu), args.Get(1).(int64), args.Error(2)
}

func (m *MockMenuRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockMenuRepository) HasChildren(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockMenuRepository) BatchCreate(ctx context.Context, menus []*domainmenu.Menu) (int, error) {
	args := m.Called(ctx, menus)
	return args.Int(0), args.Error(1)
}

func (m *MockMenuRepository) GetTree(ctx context.Context, serviceName string, includeInactive, includeHidden bool) ([]*domainmenu.WithChildren, error) {
	args := m.Called(ctx, serviceName, includeInactive, includeHidden)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainmenu.WithChildren), args.Error(1)
}

func (m *MockMenuRepository) GetTreeForUser(ctx context.Context, userID uuid.UUID, serviceName string) ([]*domainmenu.WithChildren, error) {
	args := m.Called(ctx, userID, serviceName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domainmenu.WithChildren), args.Error(1)
}

func (m *MockMenuRepository) AssignPermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	args := m.Called(ctx, menuID, permissionIDs, assignedBy)
	return args.Error(0)
}

func (m *MockMenuRepository) RemovePermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID) error {
	args := m.Called(ctx, menuID, permissionIDs)
	return args.Error(0)
}

func (m *MockMenuRepository) GetPermissions(ctx context.Context, menuID uuid.UUID) ([]*role.Permission, error) {
	args := m.Called(ctx, menuID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*role.Permission), args.Error(1)
}

func (m *MockMenuRepository) Reorder(ctx context.Context, parentID *uuid.UUID, menuIDs []uuid.UUID) error {
	args := m.Called(ctx, parentID, menuIDs)
	return args.Error(0)
}

// =============================================================================
// Test Helpers
// =============================================================================

func validCreateInput() appmenu.CreateMenuInput {
	return appmenu.CreateMenuInput{
		Code:        "TEST_MENU",
		Title:       "Test Menu",
		URL:         "/test",
		IconName:    "home",
		ServiceName: "iam",
		Level:       1,
		SortOrder:   1,
		IsVisible:   true,
		CreatedBy:   "admin",
	}
}

func validChildCreateInput(parentID uuid.UUID) appmenu.CreateMenuInput {
	return appmenu.CreateMenuInput{
		ParentID:    &parentID,
		Code:        "TEST_CHILD",
		Title:       "Test Child Menu",
		URL:         "/test/child",
		IconName:    "",
		ServiceName: "iam",
		Level:       2,
		SortOrder:   1,
		IsVisible:   true,
		CreatedBy:   "admin",
	}
}

// =============================================================================
// Tests — CreateMenu
// =============================================================================

func TestCreateMenu_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	input := validCreateInput()
	repo.On("ExistsByCode", ctx, input.Code).Return(false, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*menu.Menu")).Return(nil)

	result, err := svc.CreateMenu(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, input.Code, result.Code())
	assert.Equal(t, input.Title, result.Title())
	repo.AssertExpectations(t)
}

func TestCreateMenu_DuplicateCode(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	input := validCreateInput()
	repo.On("ExistsByCode", ctx, input.Code).Return(true, nil)

	result, err := svc.CreateMenu(ctx, input)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, appmenu.ErrConflict)
	repo.AssertNotCalled(t, "Create")
}

func TestCreateMenu_WithParent_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	parentID := uuid.New()
	parentMenu, _ := domainmenu.NewMenu(nil, "PARENT", "Parent", "/parent", "home", "iam", 1, 1, true, "admin")

	input := validChildCreateInput(parentID)
	repo.On("ExistsByCode", ctx, input.Code).Return(false, nil)
	repo.On("GetByID", ctx, parentID).Return(parentMenu, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*menu.Menu")).Return(nil)

	result, err := svc.CreateMenu(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, input.Code, result.Code())
	repo.AssertExpectations(t)
}

func TestCreateMenu_InvalidParent(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	parentID := uuid.New()
	input := validChildCreateInput(parentID)
	repo.On("ExistsByCode", ctx, input.Code).Return(false, nil)
	repo.On("GetByID", ctx, parentID).Return(nil, shared.ErrNotFound)

	result, err := svc.CreateMenu(ctx, input)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parent menu")
}

func TestCreateMenu_InvalidEntity(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	// Empty code triggers domain validation error.
	input := validCreateInput()
	input.Code = ""
	repo.On("ExistsByCode", ctx, "").Return(false, nil)

	result, err := svc.CreateMenu(ctx, input)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create menu entity")
}

// =============================================================================
// Tests — UpdateMenu
// =============================================================================

func TestUpdateMenu_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	existingMenu, _ := domainmenu.NewMenu(nil, "TEST", "Original", "/test", "home", "iam", 1, 1, true, "admin")
	menuID := existingMenu.ID()

	newTitle := "Updated Title"
	input := appmenu.UpdateMenuInput{
		ID:        menuID,
		Title:     &newTitle,
		UpdatedBy: "admin",
	}

	repo.On("GetByID", ctx, menuID).Return(existingMenu, nil)
	repo.On("Update", ctx, mock.AnythingOfType("*menu.Menu")).Return(nil)

	result, err := svc.UpdateMenu(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Updated Title", result.Title())
	repo.AssertExpectations(t)
}

func TestUpdateMenu_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuID := uuid.New()
	newTitle := "Updated Title"
	input := appmenu.UpdateMenuInput{
		ID:        menuID,
		Title:     &newTitle,
		UpdatedBy: "admin",
	}

	repo.On("GetByID", ctx, menuID).Return(nil, shared.ErrNotFound)

	result, err := svc.UpdateMenu(ctx, input)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

// =============================================================================
// Tests — DeleteMenu
// =============================================================================

func TestDeleteMenu_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuID := uuid.New()
	repo.On("HasChildren", ctx, menuID).Return(false, nil)
	repo.On("Delete", ctx, menuID, "admin").Return(nil)

	err := svc.DeleteMenu(ctx, menuID, "admin")
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestDeleteMenu_HasChildren(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuID := uuid.New()
	repo.On("HasChildren", ctx, menuID).Return(true, nil)

	err := svc.DeleteMenu(ctx, menuID, "admin")
	assert.ErrorIs(t, err, domainmenu.ErrHasChildren)
	repo.AssertNotCalled(t, "Delete")
}

func TestDeleteMenuWithChildren_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuID := uuid.New()
	repo.On("DeleteWithChildren", ctx, menuID, "admin").Return(3, nil)

	count, err := svc.DeleteMenuWithChildren(ctx, menuID, "admin")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	repo.AssertExpectations(t)
}

// =============================================================================
// Tests — GetMenu
// =============================================================================

func TestGetMenuByID_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	existingMenu, _ := domainmenu.NewMenu(nil, "TEST", "Test", "/test", "home", "iam", 1, 1, true, "admin")
	menuID := existingMenu.ID()

	repo.On("GetByID", ctx, menuID).Return(existingMenu, nil)

	result, err := svc.GetMenuByID(ctx, menuID)
	require.NoError(t, err)
	assert.Equal(t, menuID, result.ID())
	repo.AssertExpectations(t)
}

func TestGetMenuByCode_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	existingMenu, _ := domainmenu.NewMenu(nil, "TEST", "Test", "/test", "home", "iam", 1, 1, true, "admin")

	repo.On("GetByCode", ctx, "TEST").Return(existingMenu, nil)

	result, err := svc.GetMenuByCode(ctx, "TEST")
	require.NoError(t, err)
	assert.Equal(t, "TEST", result.Code())
	repo.AssertExpectations(t)
}

// =============================================================================
// Tests — AssignPermissions
// =============================================================================

func TestAssignPermissions_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	existingMenu, _ := domainmenu.NewMenu(nil, "TEST", "Test", "/test", "home", "iam", 1, 1, true, "admin")
	menuID := existingMenu.ID()
	permIDs := []uuid.UUID{uuid.New(), uuid.New()}

	repo.On("GetByID", ctx, menuID).Return(existingMenu, nil)
	repo.On("AssignPermissions", ctx, menuID, permIDs, "admin").Return(nil)

	err := svc.AssignPermissions(ctx, menuID, permIDs, "admin")
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestAssignPermissions_InvalidMenu(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuID := uuid.New()
	permIDs := []uuid.UUID{uuid.New()}

	repo.On("GetByID", ctx, menuID).Return(nil, shared.ErrNotFound)

	err := svc.AssignPermissions(ctx, menuID, permIDs, "admin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid menu")
	repo.AssertNotCalled(t, "AssignPermissions")
}

// =============================================================================
// Tests — ListMenus and Tree
// =============================================================================

func TestListMenus_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	params := domainmenu.ListParams{Page: 1, PageSize: 10}
	repo.On("List", ctx, params).Return([]*domainmenu.Menu{}, int64(0), nil)

	menus, total, err := svc.ListMenus(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, menus)
	repo.AssertExpectations(t)
}

func TestGetMenuTree_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	repo.On("GetTree", ctx, "iam", false, false).Return([]*domainmenu.WithChildren{}, nil)

	tree, err := svc.GetMenuTree(ctx, "iam", false, false)
	require.NoError(t, err)
	assert.NotNil(t, tree)
	repo.AssertExpectations(t)
}

func TestGetMenuTreeForUser_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	userID := uuid.New()
	repo.On("GetTreeForUser", ctx, userID, "iam").Return([]*domainmenu.WithChildren{}, nil)

	tree, err := svc.GetMenuTreeForUser(ctx, userID, "iam")
	require.NoError(t, err)
	assert.NotNil(t, tree)
	repo.AssertExpectations(t)
}

// =============================================================================
// Tests — Reorder
// =============================================================================

func TestReorderMenus_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	menuIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	repo.On("Reorder", ctx, (*uuid.UUID)(nil), menuIDs).Return(nil)

	err := svc.ReorderMenus(ctx, nil, menuIDs)
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestReorderMenus_RepoError(t *testing.T) {
	ctx := context.Background()
	repo := new(MockMenuRepository)
	svc := appmenu.NewService(repo)

	repoErr := errors.New("db error")
	menuIDs := []uuid.UUID{uuid.New()}
	repo.On("Reorder", ctx, (*uuid.UUID)(nil), menuIDs).Return(repoErr)

	err := svc.ReorderMenus(ctx, nil, menuIDs)
	assert.ErrorIs(t, err, repoErr)
}
