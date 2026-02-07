package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

func roleUniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func TestRoleRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	suffix := roleUniqueSuffix()
	code := "TEST_" + suffix
	name := "Test Role " + suffix

	rl, err := role.NewRole(code, name, "integration test role", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupRole(t, db, rl.ID()) })

	err = repo.Create(ctx, rl)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, rl.ID())
	require.NoError(t, err)

	assert.Equal(t, rl.ID(), got.ID())
	assert.Equal(t, code, got.Code())
	assert.Equal(t, name, got.Name())
	assert.Equal(t, "integration test role", got.Description())
	assert.False(t, got.IsSystem())
	assert.True(t, got.IsActive())
}

func TestRoleRepository_GetByCode(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	suffix := roleUniqueSuffix()
	code := "TEST_" + suffix
	name := "Test Role " + suffix

	rl, err := role.NewRole(code, name, "desc", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupRole(t, db, rl.ID()) })

	err = repo.Create(ctx, rl)
	require.NoError(t, err)

	got, err := repo.GetByCode(ctx, code)
	require.NoError(t, err)

	assert.Equal(t, rl.ID(), got.ID())
	assert.Equal(t, code, got.Code())
	assert.Equal(t, name, got.Name())
}

func TestRoleRepository_ExistsByCode(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	suffix := roleUniqueSuffix()
	code := "TEST_" + suffix

	// Should not exist before creation.
	exists, err := repo.ExistsByCode(ctx, code)
	require.NoError(t, err)
	assert.False(t, exists)

	rl, err := role.NewRole(code, "Test Role "+suffix, "desc", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupRole(t, db, rl.ID()) })

	err = repo.Create(ctx, rl)
	require.NoError(t, err)

	// Should exist after creation.
	exists, err = repo.ExistsByCode(ctx, code)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRoleRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	suffix := roleUniqueSuffix()
	const count = 3
	var createdRoles []*role.Role

	for i := 0; i < count; i++ {
		code := fmt.Sprintf("TESTLIST_%s_%d", suffix, i)
		name := fmt.Sprintf("Test List Role %s %d", suffix, i)

		rl, err := role.NewRole(code, name, "desc", "integration-test")
		require.NoError(t, err)
		createdRoles = append(createdRoles, rl)

		err = repo.Create(ctx, rl)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		for _, rl := range createdRoles {
			cleanupRole(t, db, rl.ID())
		}
	})

	// List with search filter to only pick up our test roles.
	params := role.ListParams{
		Page:     1,
		PageSize: 10,
		Search:   "TESTLIST_" + suffix,
	}

	roles, total, err := repo.List(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, int64(count), total)
	assert.Len(t, roles, count)
}

func TestRoleRepository_SoftDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	suffix := roleUniqueSuffix()
	code := "TEST_" + suffix
	name := "Test Role " + suffix

	rl, err := role.NewRole(code, name, "desc", "integration-test")
	require.NoError(t, err)

	// Always hard-delete in cleanup regardless of soft-delete state.
	t.Cleanup(func() { cleanupRole(t, db, rl.ID()) })

	err = repo.Create(ctx, rl)
	require.NoError(t, err)

	err = repo.Delete(ctx, rl.ID(), "integration-test")
	require.NoError(t, err)

	// GetByID should return ErrNotFound after soft delete.
	_, err = repo.GetByID(ctx, rl.ID())
	assert.ErrorIs(t, err, shared.ErrNotFound)

	// ExistsByCode should return false after soft delete.
	exists, err := repo.ExistsByCode(ctx, code)
	require.NoError(t, err)
	assert.False(t, exists)
}
