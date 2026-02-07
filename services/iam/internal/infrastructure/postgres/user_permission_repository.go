// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
)

// UserPermissionRepository implements role.UserPermissionRepository interface.
type UserPermissionRepository struct {
	db *DB
}

// NewUserPermissionRepository creates a new UserPermissionRepository.
func NewUserPermissionRepository(db *DB) *UserPermissionRepository {
	return &UserPermissionRepository{db: db}
}

// AssignPermissions assigns direct permissions to a user.
func (r *UserPermissionRepository) AssignPermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, permID := range permissionIDs {
			query := `
				INSERT INTO user_permissions (id, user_id, permission_id, assigned_at, assigned_by)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (user_id, permission_id) DO NOTHING
			`
			_, err := tx.ExecContext(ctx, query, uuid.New(), userID, permID, time.Now(), assignedBy)
			if err != nil {
				return fmt.Errorf("failed to assign permission: %w", err)
			}
		}
		return nil
	})
}

// RemovePermissions removes direct permissions from a user.
func (r *UserPermissionRepository) RemovePermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(permissionIDs))
	args := []interface{}{userID}
	for i, id := range permissionIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf("DELETE FROM user_permissions WHERE user_id = $1 AND permission_id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetUserDirectPermissions gets direct permissions assigned to a user.
func (r *UserPermissionRepository) GetUserDirectPermissions(ctx context.Context, userID uuid.UUID) ([]*role.Permission, error) {
	query := `
		SELECT p.permission_id, p.permission_code, p.permission_name, p.description,
			p.service_name, p.module_name, p.action_type, p.is_active,
			p.created_at, p.created_by, p.updated_at, p.updated_by
		FROM mst_permission p
		INNER JOIN user_permissions up ON p.permission_id = up.permission_id
		WHERE up.user_id = $1 AND p.is_active = true
		ORDER BY p.service_name, p.module_name, p.action_type
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user direct permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in user direct permissions")
		}
	}()

	var permissions []*role.Permission
	for rows.Next() {
		var row permissionRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description,
			&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, row.toDomain())
	}

	return permissions, nil
}

// GetEffectivePermissions gets all permissions for a user (from roles + direct assignments).
func (r *UserPermissionRepository) GetEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]*role.Permission, error) {
	query := `
		SELECT DISTINCT p.permission_id, p.permission_code, p.permission_name, p.description,
			p.service_name, p.module_name, p.action_type, p.is_active,
			p.created_at, p.created_by, p.updated_at, p.updated_by
		FROM mst_permission p
		WHERE p.is_active = true AND (
			p.permission_id IN (
				SELECT up.permission_id FROM user_permissions up WHERE up.user_id = $1
			)
			OR p.permission_id IN (
				SELECT rp.permission_id FROM role_permissions rp
				INNER JOIN user_roles ur ON rp.role_id = ur.role_id
				WHERE ur.user_id = $1
			)
		)
		ORDER BY p.service_name, p.module_name, p.action_type
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effective permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in effective permissions")
		}
	}()

	var permissions []*role.Permission
	for rows.Next() {
		var row permissionRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description,
			&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, row.toDomain())
	}

	return permissions, nil
}

// Verify interface compliance.
var _ role.UserPermissionRepository = (*UserPermissionRepository)(nil)
