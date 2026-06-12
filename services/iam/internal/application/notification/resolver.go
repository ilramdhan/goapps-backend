package notification

import (
	"context"

	"github.com/google/uuid"
)

// UserResolver resolves a set of recipient user IDs from a semantic rule.
// IAM's RequestHandler uses this to fan-out notifications without requiring
// callers to know user IDs.
type UserResolver interface {
	// GetByPermission returns all active users who have the given permission
	// code (via role assignment or direct user_permissions assignment).
	GetByPermission(ctx context.Context, permissionCode string) ([]uuid.UUID, error)

	// GetByDept returns all active users whose section belongs to the
	// department identified by deptCode.
	GetByDept(ctx context.Context, deptCode string) ([]uuid.UUID, error)

	// GetByRole returns all active users who have the given role (by role_name).
	GetByRole(ctx context.Context, roleName string) ([]uuid.UUID, error)

	// GetByUserID validates and returns the user ID in a single-element slice
	// if the user exists and is active; returns empty slice otherwise.
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}
