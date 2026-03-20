// Package role_test provides unit tests for Role and Permission domain entities.
package role_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// NewRole
// =============================================================================

func TestNewRole(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		roleName    string
		description string
		createdBy   string
		wantErr     error
	}{
		{
			name:        "valid creation",
			code:        "ADMIN",
			roleName:    "Administrator",
			description: "Full access",
			createdBy:   "system",
		},
		{
			name:     "empty code",
			code:     "",
			roleName: "Admin",
			wantErr:  shared.ErrEmptyCode,
		},
		{
			name:     "invalid code - lowercase",
			code:     "admin",
			roleName: "Admin",
			wantErr:  role.ErrInvalidRoleCodeFormat,
		},
		{
			name:     "invalid code - special chars",
			code:     "ADMIN@ROLE",
			roleName: "Admin",
			wantErr:  role.ErrInvalidRoleCodeFormat,
		},
		{
			name:     "invalid code - starts with number",
			code:     "1ADMIN",
			roleName: "Admin",
			wantErr:  role.ErrInvalidRoleCodeFormat,
		},
		{
			name:     "empty name",
			code:     "ADMIN",
			roleName: "",
			wantErr:  shared.ErrEmptyName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := role.NewRole(tc.code, tc.roleName, tc.description, tc.createdBy)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, r)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, r)
			assert.NotEqual(t, uuid.Nil, r.ID())
			assert.Equal(t, tc.code, r.Code())
			assert.Equal(t, tc.roleName, r.Name())
			assert.Equal(t, tc.description, r.Description())
			assert.False(t, r.IsSystem())
			assert.True(t, r.IsActive())
			assert.False(t, r.IsDeleted())
		})
	}
}

// =============================================================================
// Role.Update
// =============================================================================

func TestRole_Update(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		r, err := role.NewRole("ADMIN", "Admin", "desc", "system")
		require.NoError(t, err)

		newName := "Administrator"
		err = r.Update(&newName, nil, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "Administrator", r.Name())
	})

	t.Run("update description", func(t *testing.T) {
		r, err := role.NewRole("ADMIN", "Admin", "old desc", "system")
		require.NoError(t, err)

		newDesc := "new desc"
		err = r.Update(nil, &newDesc, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "new desc", r.Description())
	})

	t.Run("update isActive", func(t *testing.T) {
		r, err := role.NewRole("ADMIN", "Admin", "desc", "system")
		require.NoError(t, err)

		inactive := false
		err = r.Update(nil, nil, &inactive, "editor")

		require.NoError(t, err)
		assert.False(t, r.IsActive())
	})

	t.Run("error - empty name", func(t *testing.T) {
		r, err := role.NewRole("ADMIN", "Admin", "desc", "system")
		require.NoError(t, err)

		emptyName := ""
		err = r.Update(&emptyName, nil, nil, "editor")

		assert.ErrorIs(t, err, shared.ErrEmptyName)
	})

	t.Run("error - deleted role", func(t *testing.T) {
		r, err := role.NewRole("CUSTOM", "Custom", "desc", "system")
		require.NoError(t, err)

		err = r.SoftDelete("admin")
		require.NoError(t, err)

		newName := "Updated"
		err = r.Update(&newName, nil, nil, "editor")

		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// Role.SoftDelete
// =============================================================================

func TestRole_SoftDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r, err := role.NewRole("CUSTOM", "Custom Role", "desc", "system")
		require.NoError(t, err)

		err = r.SoftDelete("admin")

		require.NoError(t, err)
		assert.True(t, r.IsDeleted())
		assert.False(t, r.IsActive())
	})

	t.Run("error - system role cannot be deleted", func(t *testing.T) {
		// Reconstruct a system role since NewRole always creates non-system roles.
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		r := role.ReconstructRole(uuid.New(), "SUPER_ADMIN", "Super Admin", "desc", true, true, audit)

		err := r.SoftDelete("admin")

		assert.ErrorIs(t, err, role.ErrSystemRoleDelete)
	})

	t.Run("error - already deleted", func(t *testing.T) {
		r, err := role.NewRole("CUSTOM", "Custom Role", "desc", "system")
		require.NoError(t, err)

		err = r.SoftDelete("admin")
		require.NoError(t, err)

		err = r.SoftDelete("admin")
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// NewPermission
// =============================================================================

func TestNewPermission(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		permName    string
		description string
		serviceName string
		moduleName  string
		actionType  string
		createdBy   string
		wantErr     error
	}{
		{
			name:        "valid creation",
			code:        "finance.accounting.journal.view",
			permName:    "View Journal",
			description: "Can view journal entries",
			serviceName: "finance",
			moduleName:  "accounting",
			actionType:  "view",
			createdBy:   "system",
		},
		{
			name:     "empty code",
			code:     "",
			permName: "View Journal",
			wantErr:  shared.ErrEmptyCode,
		},
		{
			name:       "invalid code format - uppercase",
			code:       "FINANCE.ACCOUNTING.JOURNAL.VIEW",
			permName:   "View Journal",
			actionType: "view",
			wantErr:    role.ErrInvalidPermissionCodeFormat,
		},
		{
			name:       "invalid code format - missing segments",
			code:       "finance.view",
			permName:   "View",
			actionType: "view",
			wantErr:    role.ErrInvalidPermissionCodeFormat,
		},
		{
			name:       "empty name",
			code:       "finance.accounting.journal.view",
			permName:   "",
			actionType: "view",
			wantErr:    shared.ErrEmptyName,
		},
		{
			name:       "invalid action type",
			code:       "finance.accounting.journal.read",
			permName:   "Read Journal",
			actionType: "read",
			wantErr:    role.ErrInvalidActionType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := role.NewPermission(tc.code, tc.permName, tc.description, tc.serviceName, tc.moduleName, tc.actionType, tc.createdBy)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, p)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, p)
			assert.NotEqual(t, uuid.Nil, p.ID())
			assert.Equal(t, tc.code, p.Code())
			assert.Equal(t, tc.permName, p.Name())
			assert.Equal(t, tc.description, p.Description())
			assert.Equal(t, tc.serviceName, p.ServiceName())
			assert.Equal(t, tc.moduleName, p.ModuleName())
			assert.Equal(t, tc.actionType, p.ActionType())
			assert.True(t, p.IsActive())
		})
	}
}

// =============================================================================
// Permission.Update
// =============================================================================

func TestPermission_Update(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		p, err := role.NewPermission("finance.accounting.journal.view", "View", "desc", "finance", "accounting", "view", "system")
		require.NoError(t, err)

		newName := "View Journals"
		err = p.Update(&newName, nil, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "View Journals", p.Name())
	})

	t.Run("update description", func(t *testing.T) {
		p, err := role.NewPermission("finance.accounting.journal.view", "View", "old", "finance", "accounting", "view", "system")
		require.NoError(t, err)

		newDesc := "new desc"
		err = p.Update(nil, &newDesc, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "new desc", p.Description())
	})

	t.Run("update isActive", func(t *testing.T) {
		p, err := role.NewPermission("finance.accounting.journal.view", "View", "desc", "finance", "accounting", "view", "system")
		require.NoError(t, err)

		inactive := false
		err = p.Update(nil, nil, &inactive, "editor")

		require.NoError(t, err)
		assert.False(t, p.IsActive())
	})

	t.Run("error - empty name", func(t *testing.T) {
		p, err := role.NewPermission("finance.accounting.journal.view", "View", "desc", "finance", "accounting", "view", "system")
		require.NoError(t, err)

		emptyName := ""
		err = p.Update(&emptyName, nil, nil, "editor")

		assert.ErrorIs(t, err, shared.ErrEmptyName)
	})
}

// =============================================================================
// ReconstructRole
// =============================================================================

func TestReconstructRole(t *testing.T) {
	id := uuid.New()
	audit := shared.AuditInfo{
		CreatedAt: time.Now(),
		CreatedBy: "system",
	}

	r := role.ReconstructRole(id, "ADMIN", "Administrator", "Full access", true, true, audit)

	assert.Equal(t, id, r.ID())
	assert.Equal(t, "ADMIN", r.Code())
	assert.Equal(t, "Administrator", r.Name())
	assert.Equal(t, "Full access", r.Description())
	assert.True(t, r.IsSystem())
	assert.True(t, r.IsActive())
	assert.Equal(t, audit.CreatedBy, r.Audit().CreatedBy)
}

// =============================================================================
// ReconstructPermission
// =============================================================================

func TestReconstructPermission(t *testing.T) {
	id := uuid.New()
	audit := shared.AuditInfo{
		CreatedAt: time.Now(),
		CreatedBy: "system",
	}

	p := role.ReconstructPermission(id, "finance.accounting.journal.view", "View Journal", "desc", "finance", "accounting", "view", true, audit)

	assert.Equal(t, id, p.ID())
	assert.Equal(t, "finance.accounting.journal.view", p.Code())
	assert.Equal(t, "View Journal", p.Name())
	assert.Equal(t, "desc", p.Description())
	assert.Equal(t, "finance", p.ServiceName())
	assert.Equal(t, "accounting", p.ModuleName())
	assert.Equal(t, "view", p.ActionType())
	assert.True(t, p.IsActive())
	assert.Equal(t, audit.CreatedBy, p.Audit().CreatedBy)
}
