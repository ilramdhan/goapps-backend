package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

func permUniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// cleanupPermission hard-deletes a permission by ID so tests leave no residue.
func cleanupPermission(t *testing.T, db *postgres.DB, permID fmt.Stringer) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM mst_permission WHERE permission_id = $1", permID.String())
}

// insertTestMenu inserts a minimal mst_menu row for testing and registers cleanup.
func insertTestMenu(t *testing.T, db *postgres.DB, menuID uuid.UUID, menuCode, menuTitle string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO mst_menu (menu_id, menu_code, menu_title, menu_url, menu_level, icon_name, service_name, sort_order, is_active, created_at, created_by)
		VALUES ($1, $2, $3, '/test', 1, 'TestIcon', 'test', 99, true, NOW(), 'test')
		ON CONFLICT (menu_id) DO NOTHING
	`, menuID, menuCode, menuTitle)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM mst_menu WHERE menu_id = $1 AND created_by = 'test'", menuID)
	})
}

func TestPermissionRepository_MenuIDRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewPermissionRepository(db)
	ctx := context.Background()

	suffix := permUniqueSuffix()
	menuID := uuid.New()
	menuCode := "TEST_MENU_" + suffix[:8]
	menuTitle := "Test Menu " + suffix[:8]

	insertTestMenu(t, db, menuID, menuCode, menuTitle)

	code := "iam.test.perm" + suffix[:8] + ".view"
	perm, err := role.NewPermission(code, "Test Perm "+suffix[:8], "desc", "iam", "test", "view", "integration-test", &menuID)
	require.NoError(t, err)

	t.Cleanup(func() { cleanupPermission(t, db, perm.ID()) })

	err = repo.Create(ctx, perm)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, perm.ID())
	require.NoError(t, err)

	require.NotNil(t, got.MenuID(), "MenuID should be set after round-trip")
	assert.Equal(t, menuID, *got.MenuID())
	assert.Equal(t, menuTitle, got.MenuTitle())
}

func TestPermissionRepository_MenuIDNilForGlobal(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewPermissionRepository(db)
	ctx := context.Background()

	suffix := permUniqueSuffix()
	code := "iam.test.global" + suffix[:8] + ".view"
	perm, err := role.NewPermission(code, "Global Perm "+suffix[:8], "desc", "iam", "test", "view", "integration-test", nil)
	require.NoError(t, err)

	t.Cleanup(func() { cleanupPermission(t, db, perm.ID()) })

	err = repo.Create(ctx, perm)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, perm.ID())
	require.NoError(t, err)

	assert.Nil(t, got.MenuID(), "MenuID should be nil for global permissions")
	assert.Empty(t, got.MenuTitle())
}

func TestPermissionRepository_ListByMenu(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewPermissionRepository(db)
	ctx := context.Background()

	suffix := permUniqueSuffix()
	menuID := uuid.New()
	menuCode := "TEST_LBM_" + suffix[:8]
	menuTitle := "Test LBM " + suffix[:8]

	insertTestMenu(t, db, menuID, menuCode, menuTitle)

	// Insert two permissions for this menu and one global (no menu).
	type codeAction struct {
		code   string
		action string
	}
	entries := []codeAction{
		{"iam.test.lbm" + suffix[:8] + ".view", "view"},
		{"iam.test.lbm" + suffix[:8] + ".create", "create"},
	}
	codes := make([]string, 0, len(entries))
	var created []*role.Permission
	for _, e := range entries {
		p, err := role.NewPermission(e.code, "LBM Perm "+e.code, "desc", "iam", "test", e.action, "integration-test", &menuID)
		require.NoError(t, err)
		err = repo.Create(ctx, p)
		require.NoError(t, err)
		created = append(created, p)
		codes = append(codes, e.code)
	}

	globalCode := "iam.test.lbmg" + suffix[:8] + ".view"
	globalPerm, err := role.NewPermission(globalCode, "Global "+globalCode, "desc", "iam", "test", "view", "integration-test", nil)
	require.NoError(t, err)
	err = repo.Create(ctx, globalPerm)
	require.NoError(t, err)

	t.Cleanup(func() {
		for _, p := range created {
			cleanupPermission(t, db, p.ID())
		}
		cleanupPermission(t, db, globalPerm.ID())
	})

	perms, err := repo.ListByMenu(ctx, menuID)
	require.NoError(t, err)

	// Should find both menu-scoped permissions but not the global one.
	var foundCodes []string
	for _, p := range perms {
		for _, c := range codes {
			if p.Code() == c {
				foundCodes = append(foundCodes, p.Code())
				assert.NotNil(t, p.MenuID())
				assert.Equal(t, menuID, *p.MenuID())
				assert.Equal(t, menuTitle, p.MenuTitle())
			}
		}
		// Confirm global perm is NOT in the result.
		assert.NotEqual(t, globalCode, p.Code())
	}

	assert.Len(t, foundCodes, len(codes), "should return all menu-scoped permissions")
}
