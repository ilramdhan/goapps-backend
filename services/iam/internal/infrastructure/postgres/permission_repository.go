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

// PermissionRepository implements role.PermissionRepository interface.
type PermissionRepository struct {
	db *DB
}

// NewPermissionRepository creates a new PermissionRepository.
func NewPermissionRepository(db *DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

// Create creates a new permission.
func (r *PermissionRepository) Create(ctx context.Context, perm *role.Permission) error {
	query := `
		INSERT INTO mst_permission (
			permission_id, permission_code, permission_name, description,
			service_name, module_name, action_type, is_active,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, query,
		perm.ID(), perm.Code(), perm.Name(), perm.Description(),
		perm.ServiceName(), perm.ModuleName(), perm.ActionType(), perm.IsActive(),
		perm.Audit().CreatedAt, perm.Audit().CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to insert permission: %w", err)
	}

	return nil
}

// GetByID retrieves a permission by ID.
func (r *PermissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*role.Permission, error) {
	query := `
		SELECT permission_id, permission_code, permission_name, description,
			service_name, module_name, action_type, is_active,
			created_at, created_by, updated_at, updated_by
		FROM mst_permission
		WHERE permission_id = $1
	`

	var row permissionRow
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&row.ID, &row.Code, &row.Name, &row.Description,
		&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
		&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get permission: %w", err)
	}

	return row.toDomain(), nil
}

// GetByCode retrieves a permission by code.
func (r *PermissionRepository) GetByCode(ctx context.Context, code string) (*role.Permission, error) {
	query := `
		SELECT permission_id, permission_code, permission_name, description,
			service_name, module_name, action_type, is_active,
			created_at, created_by, updated_at, updated_by
		FROM mst_permission
		WHERE permission_code = $1
	`

	var row permissionRow
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&row.ID, &row.Code, &row.Name, &row.Description,
		&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
		&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get permission: %w", err)
	}

	return row.toDomain(), nil
}

// Update updates a permission.
func (r *PermissionRepository) Update(ctx context.Context, perm *role.Permission) error {
	query := `
		UPDATE mst_permission SET
			permission_name = $2, description = $3, is_active = $4,
			updated_at = $5, updated_by = $6
		WHERE permission_id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		perm.ID(), perm.Name(), perm.Description(), perm.IsActive(),
		perm.Audit().UpdatedAt, perm.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
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

// Delete soft-deletes a permission by setting is_active to false.
func (r *PermissionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_permission SET
			is_active = false, updated_at = $2, updated_by = $3
		WHERE permission_id = $1 AND is_active = true
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete permission: %w", err)
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

// List lists permissions with pagination.
func (r *PermissionRepository) List(ctx context.Context, params role.PermissionListParams) ([]*role.Permission, int64, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(permission_code ILIKE $%d OR permission_name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}

	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *params.IsActive)
		argPos++
	}

	if params.ServiceName != "" {
		conditions = append(conditions, fmt.Sprintf("service_name = $%d", argPos))
		args = append(args, params.ServiceName)
		argPos++
	}

	if params.ModuleName != "" {
		conditions = append(conditions, fmt.Sprintf("module_name = $%d", argPos))
		args = append(args, params.ModuleName)
		argPos++
	}

	if params.ActionType != "" {
		conditions = append(conditions, fmt.Sprintf("action_type = $%d", argPos))
		args = append(args, params.ActionType)
		argPos++
	}

	whereClause := "TRUE"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_permission WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count permissions: %w", err)
	}

	// Build query
	sortBy := "permission_code"
	if params.SortBy != "" {
		sortBy = params.SortBy
	}
	sortOrder := sortASC
	if strings.EqualFold(params.SortOrder, sortDESC) {
		sortOrder = sortDESC
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT permission_id, permission_code, permission_name, description,
			service_name, module_name, action_type, is_active,
			created_at, created_by, updated_at, updated_by
		FROM mst_permission
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in permission list")
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
			return nil, 0, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, row.toDomain())
	}

	return permissions, total, nil
}

// ExistsByCode checks if a permission code exists.
func (r *PermissionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_permission WHERE permission_code = $1)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// BatchCreate creates multiple permissions.
func (r *PermissionRepository) BatchCreate(ctx context.Context, permissions []*role.Permission) (int, error) {
	count := 0
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, perm := range permissions {
			query := `
				INSERT INTO mst_permission (
					permission_id, permission_code, permission_name, description,
					service_name, module_name, action_type, is_active,
					created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`
			_, err := tx.ExecContext(ctx, query,
				perm.ID(), perm.Code(), perm.Name(), perm.Description(),
				perm.ServiceName(), perm.ModuleName(), perm.ActionType(), perm.IsActive(),
				perm.Audit().CreatedAt, perm.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert permission %s: %w", perm.Code(), err)
			}
			count++
		}
		return nil
	})
	return count, err
}

// GetByService retrieves permissions grouped by service and module.
func (r *PermissionRepository) GetByService(ctx context.Context, serviceName string, includeInactive bool) ([]*role.ServicePermissions, error) {
	activeFilter := "AND is_active = true"
	if includeInactive {
		activeFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT permission_id, permission_code, permission_name, description,
			service_name, module_name, action_type, is_active,
			created_at, created_by, updated_at, updated_by
		FROM mst_permission
		WHERE service_name = $1 %s
		ORDER BY module_name, action_type
	`, activeFilter)

	rows, err := r.db.QueryContext(ctx, query, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions by service: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in permission modules")
		}
	}()

	moduleMap := make(map[string][]*role.Permission)
	var moduleOrder []string
	for rows.Next() {
		var row permissionRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description,
			&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		if _, exists := moduleMap[row.ModuleName]; !exists {
			moduleOrder = append(moduleOrder, row.ModuleName)
		}
		moduleMap[row.ModuleName] = append(moduleMap[row.ModuleName], row.toDomain())
	}

	modules := make([]*role.ModulePermissions, 0, len(moduleOrder))
	for _, moduleName := range moduleOrder {
		modules = append(modules, &role.ModulePermissions{
			ModuleName:  moduleName,
			Permissions: moduleMap[moduleName],
		})
	}

	return []*role.ServicePermissions{
		{
			ServiceName: serviceName,
			Modules:     modules,
		},
	}, nil
}
