// Package notification provides infrastructure implementations for the
// notification application layer.
package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// DBUserResolver queries the IAM PostgreSQL database to resolve recipient
// user IDs from semantic rules (permission, dept, role, user_id).
type DBUserResolver struct {
	db *sql.DB
}

// NewDBUserResolver constructs the resolver.
func NewDBUserResolver(db *sql.DB) *DBUserResolver {
	return &DBUserResolver{db: db}
}

// GetByPermission returns all active users who have the given permission
// code through either a role assignment or a direct user_permissions entry.
func (r *DBUserResolver) GetByPermission(ctx context.Context, code string) ([]uuid.UUID, error) {
	const q = `
		SELECT DISTINCT u.user_id
		FROM mst_user u
		WHERE u.deleted_at IS NULL AND u.is_active = true
		  AND (
		    EXISTS (
		      SELECT 1
		      FROM user_roles ur
		      JOIN role_permissions rp ON rp.role_id = ur.role_id
		      JOIN mst_permission p    ON p.permission_id = rp.permission_id
		      WHERE ur.user_id = u.user_id AND p.permission_code = $1
		    )
		    OR EXISTS (
		      SELECT 1
		      FROM user_permissions up
		      JOIN mst_permission p ON p.permission_id = up.permission_id
		      WHERE up.user_id = u.user_id AND p.permission_code = $1
		    )
		  )`
	return r.scan(ctx, q, code)
}

// GetByDept returns all active users whose section is in the given department.
func (r *DBUserResolver) GetByDept(ctx context.Context, deptCode string) ([]uuid.UUID, error) {
	const q = `
		SELECT DISTINCT u.user_id
		FROM mst_user u
		JOIN mst_user_detail ud ON ud.user_id = u.user_id
		JOIN mst_section s      ON s.section_id = ud.section_id AND s.deleted_at IS NULL
		JOIN mst_department d   ON d.department_id = s.department_id
		  AND d.department_code = $1 AND d.deleted_at IS NULL
		WHERE u.deleted_at IS NULL AND u.is_active = true`
	return r.scan(ctx, q, deptCode)
}

// GetByRole returns all active users who have the given role by role_name.
func (r *DBUserResolver) GetByRole(ctx context.Context, roleName string) ([]uuid.UUID, error) {
	const q = `
		SELECT DISTINCT u.user_id
		FROM mst_user u
		JOIN user_roles ur ON ur.user_id = u.user_id
		JOIN mst_role r    ON r.role_id = ur.role_id
		  AND r.role_name = $1 AND r.deleted_at IS NULL
		WHERE u.deleted_at IS NULL AND u.is_active = true`
	return r.scan(ctx, q, roleName)
}

// GetByUserID validates the user exists and is active, returning it as a slice.
func (r *DBUserResolver) GetByUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
		SELECT user_id FROM mst_user
		WHERE user_id = $1 AND deleted_at IS NULL AND is_active = true`
	return r.scan(ctx, q, userID)
}

// LookupEmail returns the email address and display name for the given user ID.
// Returns ("", "", nil) when the user does not exist or is soft-deleted.
func (r *DBUserResolver) LookupEmail(ctx context.Context, userID uuid.UUID) (email, displayName string, err error) {
	const q = `
		SELECT u.email, COALESCE(ud.full_name, u.username) AS display_name
		FROM mst_user u
		LEFT JOIN mst_user_detail ud ON ud.user_id = u.user_id
		WHERE u.user_id = $1 AND u.deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, q, userID)
	if scanErr := row.Scan(&email, &displayName); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("lookup user email: %w", scanErr)
	}
	return email, displayName, nil
}

func (r *DBUserResolver) scan(ctx context.Context, q string, arg any) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("user resolver query: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("user resolver scan: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("user resolver rows: %w", rowsErr)
	}
	return ids, nil
}
