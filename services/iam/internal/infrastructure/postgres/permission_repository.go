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
	sortColumnMap := map[string]string{
		"code":       "permission_code",
		"name":       "permission_name",
		"service":    "service_name",
		"module":     "module_name",
		"action":     "action_type",
		"status":     "is_active",
		"created_at": "created_at",
	}

	sortBy := "permission_code"
	if params.SortBy != "" {
		if mapped, ok := sortColumnMap[params.SortBy]; ok {
			sortBy = mapped
		} else {
			sortBy = params.SortBy
		}
	}
	sortOrder := sortASC
	if strings.EqualFold(params.SortOrder, sortDESC) {
		sortOrder = sortDESC
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT p.permission_id, p.permission_code, p.permission_name, p.description,
			p.service_name, p.module_name, p.action_type, p.is_active,
			p.created_at, p.created_by, p.updated_at, p.updated_by,
			COALESCE((SELECT COUNT(*) FROM role_permissions rp WHERE rp.permission_id = p.permission_id), 0) AS role_count
		FROM mst_permission p
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
		var roleCount int32
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description,
			&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
			&roleCount,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan permission: %w", err)
		}
		p := row.toDomain()
		p.SetRoleCount(roleCount)
		permissions = append(permissions, p)
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

	var query string
	var args []interface{}

	if serviceName != "" {
		query = fmt.Sprintf(`
			SELECT permission_id, permission_code, permission_name, description,
				service_name, module_name, action_type, is_active,
				created_at, created_by, updated_at, updated_by
			FROM mst_permission
			WHERE service_name = $1 %s
			ORDER BY service_name, module_name, action_type
		`, activeFilter)
		args = append(args, serviceName)
	} else {
		// Return ALL services when serviceName is empty
		query = fmt.Sprintf(`
			SELECT permission_id, permission_code, permission_name, description,
				service_name, module_name, action_type, is_active,
				created_at, created_by, updated_at, updated_by
			FROM mst_permission
			WHERE TRUE %s
			ORDER BY service_name, module_name, action_type
		`, activeFilter)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions by service: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in permission modules")
		}
	}()

	// Group by service_name -> module_name -> permissions
	type serviceData struct {
		moduleMap   map[string][]*role.Permission
		moduleOrder []string
	}
	serviceMap := make(map[string]*serviceData)
	var serviceOrder []string

	for rows.Next() {
		var row permissionRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description,
			&row.ServiceName, &row.ModuleName, &row.ActionType, &row.IsActive,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}

		sd, exists := serviceMap[row.ServiceName]
		if !exists {
			sd = &serviceData{
				moduleMap:   make(map[string][]*role.Permission),
				moduleOrder: nil,
			}
			serviceMap[row.ServiceName] = sd
			serviceOrder = append(serviceOrder, row.ServiceName)
		}

		if _, modExists := sd.moduleMap[row.ModuleName]; !modExists {
			sd.moduleOrder = append(sd.moduleOrder, row.ModuleName)
		}
		sd.moduleMap[row.ModuleName] = append(sd.moduleMap[row.ModuleName], row.toDomain())
	}

	// Build result
	result := make([]*role.ServicePermissions, 0, len(serviceOrder))
	for _, svcName := range serviceOrder {
		sd := serviceMap[svcName]
		modules := make([]*role.ModulePermissions, 0, len(sd.moduleOrder))
		for _, modName := range sd.moduleOrder {
			modules = append(modules, &role.ModulePermissions{
				ModuleName:  modName,
				Permissions: sd.moduleMap[modName],
			})
		}
		result = append(result, &role.ServicePermissions{
			ServiceName: svcName,
			Modules:     modules,
		})
	}

	return result, nil
}
