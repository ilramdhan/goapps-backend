// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// RoleRepository implements role.Repository interface.
type RoleRepository struct {
	db *DB
}

// NewRoleRepository creates a new RoleRepository.
func NewRoleRepository(db *DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// Create creates a new role.
func (r *RoleRepository) Create(ctx context.Context, rl *role.Role) error {
	query := `
		INSERT INTO mst_role (
			role_id, role_code, role_name, description, is_system, is_active,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		rl.ID(), rl.Code(), rl.Name(), rl.Description(), rl.IsSystem(), rl.IsActive(),
		rl.Audit().CreatedAt, rl.Audit().CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to insert role: %w", err)
	}

	return nil
}

// GetByID retrieves a role by ID.
func (r *RoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*role.Role, error) {
	query := `
		SELECT role_id, role_code, role_name, description, is_system, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_role
		WHERE role_id = $1 AND deleted_at IS NULL
	`

	var row roleRow
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&row.ID, &row.Code, &row.Name, &row.Description, &row.IsSystem, &row.IsActive,
		&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return row.toDomain(), nil
}

// GetByCode retrieves a role by code.
func (r *RoleRepository) GetByCode(ctx context.Context, code string) (*role.Role, error) {
	query := `
		SELECT role_id, role_code, role_name, description, is_system, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_role
		WHERE role_code = $1 AND deleted_at IS NULL
	`

	var row roleRow
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&row.ID, &row.Code, &row.Name, &row.Description, &row.IsSystem, &row.IsActive,
		&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return row.toDomain(), nil
}

// Update updates a role.
func (r *RoleRepository) Update(ctx context.Context, rl *role.Role) error {
	query := `
		UPDATE mst_role SET
			role_name = $2, description = $3, is_active = $4,
			updated_at = $5, updated_by = $6
		WHERE role_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		rl.ID(), rl.Name(), rl.Description(), rl.IsActive(),
		rl.Audit().UpdatedAt, rl.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// Delete soft-deletes a role.
func (r *RoleRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_role SET
			is_active = false, deleted_at = $2, deleted_by = $3
		WHERE role_id = $1 AND deleted_at IS NULL AND is_system = false
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// List lists roles with pagination.
func (r *RoleRepository) List(ctx context.Context, params role.ListParams) ([]*role.Role, int64, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(role_code ILIKE $%d OR role_name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}

	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *params.IsActive)
		argPos++
	}

	if params.IsSystem != nil {
		conditions = append(conditions, fmt.Sprintf("is_system = $%d", argPos))
		args = append(args, *params.IsSystem)
		argPos++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_role WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count roles: %w", err)
	}

	// Build query
	sortBy := "role_code"
	if params.SortBy != "" {
		sortBy = params.SortBy
	}
	sortOrder := sortASC
	if params.SortOrder != "" && (params.SortOrder == sortDESC || params.SortOrder == "desc") {
		sortOrder = sortDESC
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT role_id, role_code, role_name, description, is_system, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_role
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list roles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in role list")
		}
	}()

	var roles []*role.Role
	for rows.Next() {
		var row roleRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description, &row.IsSystem, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, row.toDomain())
	}

	return roles, total, nil
}

// ExistsByCode checks if a role code exists.
func (r *RoleRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_role WHERE role_code = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// BatchCreate creates multiple roles.
func (r *RoleRepository) BatchCreate(ctx context.Context, roles []*role.Role) (int, error) {
	count := 0
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, rl := range roles {
			query := `
				INSERT INTO mst_role (
					role_id, role_code, role_name, description, is_system, is_active,
					created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`
			_, err := tx.ExecContext(ctx, query,
				rl.ID(), rl.Code(), rl.Name(), rl.Description(), rl.IsSystem(), rl.IsActive(),
				rl.Audit().CreatedAt, rl.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert role %s: %w", rl.Code(), err)
			}
			count++
		}
		return nil
	})
	return count, err
}

// AssignPermissions assigns permissions to a role.
func (r *RoleRepository) AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, permID := range permissionIDs {
			query := `
				INSERT INTO role_permissions (id, role_id, permission_id, assigned_at, assigned_by)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (role_id, permission_id) DO NOTHING
			`
			_, err := tx.ExecContext(ctx, query, uuid.New(), roleID, permID, time.Now(), assignedBy)
			if err != nil {
				return fmt.Errorf("failed to assign permission: %w", err)
			}
		}
		return nil
	})
}

// RemovePermissions removes permissions from a role.
func (r *RoleRepository) RemovePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(permissionIDs))
	args := []interface{}{roleID}
	for i, id := range permissionIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf("DELETE FROM role_permissions WHERE role_id = $1 AND permission_id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetPermissions gets all permissions for a role.
func (r *RoleRepository) GetPermissions(ctx context.Context, roleID uuid.UUID) ([]*role.Permission, error) {
	query := `
		SELECT p.permission_id, p.permission_code, p.permission_name, p.description,
			p.service_name, p.module_name, p.action_type, p.is_active,
			p.created_at, p.created_by, p.updated_at, p.updated_by
		FROM mst_permission p
		INNER JOIN role_permissions rp ON p.permission_id = rp.permission_id
		WHERE rp.role_id = $1 AND p.is_active = true
		ORDER BY p.service_name, p.module_name, p.action_type
	`

	rows, err := r.db.QueryContext(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in role permissions")
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
			return nil, err
		}
		permissions = append(permissions, row.toDomain())
	}

	return permissions, nil
}

// GetRolesByPermission gets all roles that have a specific permission.
func (r *RoleRepository) GetRolesByPermission(ctx context.Context, permissionID uuid.UUID) ([]*role.Role, error) {
	query := `
		SELECT r.role_id, r.role_code, r.role_name, r.description, r.is_system, r.is_active,
			r.created_at, r.created_by, r.updated_at, r.updated_by, r.deleted_at, r.deleted_by
		FROM mst_role r
		INNER JOIN role_permissions rp ON r.role_id = rp.role_id
		WHERE rp.permission_id = $1 AND r.deleted_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, permissionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get roles by permission: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in roles by permission")
		}
	}()

	var roles []*role.Role
	for rows.Next() {
		var row roleRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description, &row.IsSystem, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
		); err != nil {
			return nil, err
		}
		roles = append(roles, row.toDomain())
	}

	return roles, nil
}

// Helper struct for scanning
type roleRow struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Description sql.NullString
	IsSystem    bool
	IsActive    bool
	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   *time.Time
	UpdatedBy   *string
	DeletedAt   *time.Time
	DeletedBy   *string
}

func (r *roleRow) toDomain() *role.Role {
	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
		DeletedAt: r.DeletedAt,
		DeletedBy: r.DeletedBy,
	}

	return role.ReconstructRole(
		r.ID, r.Code, r.Name, r.Description.String, r.IsSystem, r.IsActive, audit,
	)
}

type permissionRow struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Description sql.NullString
	ServiceName string
	ModuleName  string
	ActionType  string
	IsActive    bool
	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   *time.Time
	UpdatedBy   *string
}

func (r *permissionRow) toDomain() *role.Permission {
	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
	}

	return role.ReconstructPermission(
		r.ID, r.Code, r.Name, r.Description.String,
		r.ServiceName, r.ModuleName, r.ActionType, r.IsActive, audit,
	)
}
