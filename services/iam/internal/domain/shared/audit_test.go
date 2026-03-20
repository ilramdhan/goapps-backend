// Package shared_test provides unit tests for the shared domain types.
package shared_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// NewAuditInfo
// =============================================================================

func TestNewAuditInfo(t *testing.T) {
	t.Run("sets CreatedAt and CreatedBy", func(t *testing.T) {
		before := time.Now()
		audit := shared.NewAuditInfo("admin")
		after := time.Now()

		assert.Equal(t, "admin", audit.CreatedBy)
		assert.False(t, audit.CreatedAt.IsZero())
		assert.True(t, !audit.CreatedAt.Before(before), "CreatedAt should be >= before")
		assert.True(t, !audit.CreatedAt.After(after), "CreatedAt should be <= after")
	})

	t.Run("UpdatedAt and UpdatedBy are nil", func(t *testing.T) {
		audit := shared.NewAuditInfo("system")

		assert.Nil(t, audit.UpdatedAt)
		assert.Nil(t, audit.UpdatedBy)
	})

	t.Run("DeletedAt and DeletedBy are nil", func(t *testing.T) {
		audit := shared.NewAuditInfo("system")

		assert.Nil(t, audit.DeletedAt)
		assert.Nil(t, audit.DeletedBy)
	})

	t.Run("empty createdBy is allowed", func(t *testing.T) {
		audit := shared.NewAuditInfo("")

		assert.Equal(t, "", audit.CreatedBy)
		assert.False(t, audit.CreatedAt.IsZero())
	})
}

// =============================================================================
// AuditInfo.Update
// =============================================================================

func TestAuditInfo_Update(t *testing.T) {
	t.Run("sets UpdatedAt and UpdatedBy", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")

		before := time.Now()
		audit.Update("editor")
		after := time.Now()

		require.NotNil(t, audit.UpdatedAt)
		require.NotNil(t, audit.UpdatedBy)
		assert.Equal(t, "editor", *audit.UpdatedBy)
		assert.True(t, !audit.UpdatedAt.Before(before), "UpdatedAt should be >= before")
		assert.True(t, !audit.UpdatedAt.After(after), "UpdatedAt should be <= after")
	})

	t.Run("does not modify CreatedAt or CreatedBy", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")
		originalCreatedAt := audit.CreatedAt

		audit.Update("editor")

		assert.Equal(t, "creator", audit.CreatedBy)
		assert.Equal(t, originalCreatedAt, audit.CreatedAt)
	})

	t.Run("does not modify DeletedAt or DeletedBy", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")

		audit.Update("editor")

		assert.Nil(t, audit.DeletedAt)
		assert.Nil(t, audit.DeletedBy)
	})

	t.Run("overwrites previous update", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")

		audit.Update("first-editor")
		audit.Update("second-editor")

		require.NotNil(t, audit.UpdatedBy)
		assert.Equal(t, "second-editor", *audit.UpdatedBy)
	})
}

// =============================================================================
// AuditInfo.SoftDelete
// =============================================================================

func TestAuditInfo_SoftDelete(t *testing.T) {
	t.Run("sets DeletedAt and DeletedBy", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")

		before := time.Now()
		audit.SoftDelete("deleter")
		after := time.Now()

		require.NotNil(t, audit.DeletedAt)
		require.NotNil(t, audit.DeletedBy)
		assert.Equal(t, "deleter", *audit.DeletedBy)
		assert.True(t, !audit.DeletedAt.Before(before), "DeletedAt should be >= before")
		assert.True(t, !audit.DeletedAt.After(after), "DeletedAt should be <= after")
	})

	t.Run("does not modify CreatedAt or CreatedBy", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")
		originalCreatedAt := audit.CreatedAt

		audit.SoftDelete("deleter")

		assert.Equal(t, "creator", audit.CreatedBy)
		assert.Equal(t, originalCreatedAt, audit.CreatedAt)
	})
}

// =============================================================================
// AuditInfo.IsDeleted
// =============================================================================

func TestAuditInfo_IsDeleted(t *testing.T) {
	t.Run("returns false for new audit info", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")

		assert.False(t, audit.IsDeleted())
	})

	t.Run("returns false after update", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")
		audit.Update("editor")

		assert.False(t, audit.IsDeleted())
	})

	t.Run("returns true after soft delete", func(t *testing.T) {
		audit := shared.NewAuditInfo("creator")
		audit.SoftDelete("deleter")

		assert.True(t, audit.IsDeleted())
	})

	t.Run("returns true when DeletedAt is set manually", func(t *testing.T) {
		now := time.Now()
		audit := shared.AuditInfo{
			CreatedAt: now,
			CreatedBy: "system",
			DeletedAt: &now,
		}

		assert.True(t, audit.IsDeleted())
	})

	t.Run("returns false when DeletedAt is nil on reconstructed struct", func(t *testing.T) {
		audit := shared.AuditInfo{
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}

		assert.False(t, audit.IsDeleted())
	})
}

// =============================================================================
// AuditInfo struct literal reconstruction
// =============================================================================

func TestAuditInfo_Reconstruct(t *testing.T) {
	t.Run("full reconstruction with all fields", func(t *testing.T) {
		now := time.Now()
		updatedAt := now.Add(time.Hour)
		deletedAt := now.Add(2 * time.Hour)
		updatedBy := "editor"
		deletedBy := "deleter"

		audit := shared.AuditInfo{
			CreatedAt: now,
			CreatedBy: "creator",
			UpdatedAt: &updatedAt,
			UpdatedBy: &updatedBy,
			DeletedAt: &deletedAt,
			DeletedBy: &deletedBy,
		}

		assert.Equal(t, now, audit.CreatedAt)
		assert.Equal(t, "creator", audit.CreatedBy)
		require.NotNil(t, audit.UpdatedAt)
		assert.Equal(t, updatedAt, *audit.UpdatedAt)
		require.NotNil(t, audit.UpdatedBy)
		assert.Equal(t, "editor", *audit.UpdatedBy)
		require.NotNil(t, audit.DeletedAt)
		assert.Equal(t, deletedAt, *audit.DeletedAt)
		require.NotNil(t, audit.DeletedBy)
		assert.Equal(t, "deleter", *audit.DeletedBy)
		assert.True(t, audit.IsDeleted())
	})

	t.Run("partial reconstruction without optional fields", func(t *testing.T) {
		now := time.Now()

		audit := shared.AuditInfo{
			CreatedAt: now,
			CreatedBy: "system",
		}

		assert.Equal(t, now, audit.CreatedAt)
		assert.Equal(t, "system", audit.CreatedBy)
		assert.Nil(t, audit.UpdatedAt)
		assert.Nil(t, audit.UpdatedBy)
		assert.Nil(t, audit.DeletedAt)
		assert.Nil(t, audit.DeletedBy)
		assert.False(t, audit.IsDeleted())
	})
}
