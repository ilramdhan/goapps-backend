package notification_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("IAM_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDBUserResolver_GetByUserID(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}

	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)
	ctx := context.Background()

	tests := []struct {
		name        string
		userID      uuid.UUID
		wantEmpty   bool
	}{
		{
			name:      "unknown UUID returns empty",
			userID:    uuid.New(),
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := r.GetByUserID(ctx, tc.userID)
			require.NoError(t, err)
			if tc.wantEmpty {
				assert.Empty(t, ids)
			} else {
				assert.NotEmpty(t, ids)
			}
		})
	}
}

func TestDBUserResolver_GetByPermission(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}

	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)
	ctx := context.Background()

	tests := []struct {
		name          string
		permissionCode string
		wantEmpty     bool
	}{
		{
			name:           "unknown permission code returns empty",
			permissionCode: "nonexistent.permission.code",
			wantEmpty:      true,
		},
		{
			// SUPER_ADMIN role is seeded with all permissions; admin user has SUPER_ADMIN.
			// iam.user.account.view is always seeded via defaultPermissions().
			name:           "seeded permission with assigned users returns non-empty",
			permissionCode: "iam.user.account.view",
			wantEmpty:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := r.GetByPermission(ctx, tc.permissionCode)
			require.NoError(t, err)
			if tc.wantEmpty {
				assert.Empty(t, ids)
			} else {
				assert.NotEmpty(t, ids)
			}
		})
	}
}

func TestDBUserResolver_GetByDept(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}

	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		deptCode  string
		wantEmpty bool
	}{
		{
			name:      "unknown department code returns empty",
			deptCode:  "NONEXISTENT_DEPT_CODE",
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := r.GetByDept(ctx, tc.deptCode)
			require.NoError(t, err)
			if tc.wantEmpty {
				assert.Empty(t, ids)
			} else {
				assert.NotEmpty(t, ids)
			}
		})
	}
}

func TestDBUserResolver_GetByRole(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}

	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		roleName  string
		wantEmpty bool
	}{
		{
			name:      "unknown role name returns empty",
			roleName:  "NonExistentRoleName",
			wantEmpty: true,
		},
		{
			// Seeds assign the admin user the SUPER_ADMIN role (name: "Super Administrator").
			name:      "seeded Super Administrator role with assigned users returns non-empty",
			roleName:  "Super Administrator",
			wantEmpty: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := r.GetByRole(ctx, tc.roleName)
			require.NoError(t, err)
			if tc.wantEmpty {
				assert.Empty(t, ids)
			} else {
				assert.NotEmpty(t, ids)
			}
		})
	}
}
