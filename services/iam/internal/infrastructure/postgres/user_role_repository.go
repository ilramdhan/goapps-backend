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

// UserRoleRepository implements role.UserRoleRepository interface.
type UserRoleRepository struct {
	db *DB
}

// NewUserRoleRepository creates a new UserRoleRepository.
func NewUserRoleRepository(db *DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

// AssignRoles assigns roles to a user.
func (r *UserRoleRepository) AssignRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, assignedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, roleID := range roleIDs {
			query := `
				INSERT INTO user_roles (id, user_id, role_id, assigned_at, assigned_by)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (user_id, role_id) DO NOTHING
			`
			_, err := tx.ExecContext(ctx, query, uuid.New(), userID, roleID, time.Now(), assignedBy)
			if err != nil {
				return fmt.Errorf("failed to assign role: %w", err)
			}
		}
		return nil
	})
}

// RemoveRoles removes roles from a user.
func (r *UserRoleRepository) RemoveRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error {
	if len(roleIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(roleIDs))
	args := []interface{}{userID}
	for i, id := range roleIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf("DELETE FROM user_roles WHERE user_id = $1 AND role_id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetUserRoles gets all roles for a user.
func (r *UserRoleRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]*role.Role, error) {
	query := `
		SELECT r.role_id, r.role_code, r.role_name, r.description, r.is_system, r.is_active,
			r.created_at, r.created_by, r.updated_at, r.updated_by, r.deleted_at, r.deleted_by
		FROM mst_role r
		INNER JOIN user_roles ur ON r.role_id = ur.role_id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL
		ORDER BY r.role_code
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in user roles")
		}
	}()

	var roles []*role.Role
	for rows.Next() {
		var row roleRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description, &row.IsSystem, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, row.toDomain())
	}

	return roles, nil
}

// GetUsersWithRole gets all user IDs that have a specific role.
func (r *UserRoleRepository) GetUsersWithRole(ctx context.Context, roleID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT user_id FROM user_roles WHERE role_id = $1`

	rows, err := r.db.QueryContext(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get users with role: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in users with role")
		}
	}()

	var userIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan user ID: %w", err)
		}
		userIDs = append(userIDs, id)
	}

	return userIDs, nil
}

// Verify interface compliance.
var _ role.UserRoleRepository = (*UserRoleRepository)(nil)
