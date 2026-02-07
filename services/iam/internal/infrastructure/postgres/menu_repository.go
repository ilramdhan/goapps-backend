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

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/menu"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// MenuRepository implements menu.Repository interface.
type MenuRepository struct {
	db *DB
}

// NewMenuRepository creates a new MenuRepository.
func NewMenuRepository(db *DB) *MenuRepository {
	return &MenuRepository{db: db}
}

// Create creates a new menu.
func (r *MenuRepository) Create(ctx context.Context, m *menu.Menu) error {
	query := `
		INSERT INTO mst_menu (
			menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
			service_name, menu_level, sort_order, is_visible, is_active,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		m.ID(), m.ParentID(), m.Code(), m.Title(), m.URL(), m.IconName(),
		m.ServiceName(), m.Level(), m.SortOrder(), m.IsVisible(), m.IsActive(),
		m.Audit().CreatedAt, m.Audit().CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to create menu: %w", err)
	}
	return nil
}

// GetByID retrieves a menu by ID.
func (r *MenuRepository) GetByID(ctx context.Context, id uuid.UUID) (*menu.Menu, error) {
	query := `
		SELECT menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
			service_name, menu_level, sort_order, is_visible, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_menu WHERE menu_id = $1 AND deleted_at IS NULL
	`
	return r.scanMenu(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a menu by code.
func (r *MenuRepository) GetByCode(ctx context.Context, code string) (*menu.Menu, error) {
	query := `
		SELECT menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
			service_name, menu_level, sort_order, is_visible, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_menu WHERE menu_code = $1 AND deleted_at IS NULL
	`
	return r.scanMenu(r.db.QueryRowContext(ctx, query, code))
}

// Update updates a menu.
func (r *MenuRepository) Update(ctx context.Context, m *menu.Menu) error {
	query := `
		UPDATE mst_menu SET
			menu_title = $2, menu_url = $3, icon_name = $4, sort_order = $5,
			is_visible = $6, is_active = $7, updated_at = $8, updated_by = $9
		WHERE menu_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query,
		m.ID(), m.Title(), m.URL(), m.IconName(), m.SortOrder(),
		m.IsVisible(), m.IsActive(), m.Audit().UpdatedAt, m.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update menu: %w", err)
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

// Delete soft-deletes a menu.
func (r *MenuRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_menu SET deleted_at = $2, deleted_by = $3 WHERE menu_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete menu: %w", err)
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

// DeleteWithChildren deletes a menu and all its children.
func (r *MenuRepository) DeleteWithChildren(ctx context.Context, id uuid.UUID, deletedBy string) (int, error) {
	now := time.Now()
	// Delete descendants first
	query := `
		WITH RECURSIVE descendants AS (
			SELECT menu_id FROM mst_menu WHERE parent_id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT m.menu_id FROM mst_menu m
			INNER JOIN descendants d ON m.parent_id = d.menu_id
			WHERE m.deleted_at IS NULL
		)
		UPDATE mst_menu SET deleted_at = $2, deleted_by = $3
		WHERE menu_id IN (SELECT menu_id FROM descendants)
	`
	result1, err := r.db.ExecContext(ctx, query, id, now, deletedBy)
	if err != nil {
		return 0, fmt.Errorf("failed to delete menu descendants: %w", err)
	}
	count1, err := result1.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for descendants: %w", err)
	}

	// Delete the parent
	query2 := `UPDATE mst_menu SET deleted_at = $2, deleted_by = $3 WHERE menu_id = $1 AND deleted_at IS NULL`
	result2, err := r.db.ExecContext(ctx, query2, id, now, deletedBy)
	if err != nil {
		return int(count1), fmt.Errorf("failed to delete parent menu: %w", err)
	}
	count2, err := result2.RowsAffected()
	if err != nil {
		return int(count1), fmt.Errorf("failed to get rows affected for parent: %w", err)
	}

	return int(count1 + count2), nil
}

// List lists menus with pagination.
func (r *MenuRepository) List(ctx context.Context, params menu.ListParams) ([]*menu.Menu, int64, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(menu_code ILIKE $%d OR menu_title ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+params.Search+"%")
		argIndex++
	}
	if params.ServiceName != "" {
		conditions = append(conditions, fmt.Sprintf("service_name = $%d", argIndex))
		args = append(args, params.ServiceName)
		argIndex++
	}
	if params.Level != nil {
		conditions = append(conditions, fmt.Sprintf("menu_level = $%d", argIndex))
		args = append(args, *params.Level)
		argIndex++
	}
	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argIndex))
		args = append(args, *params.IsActive)
		argIndex++
	}
	if params.IsVisible != nil {
		conditions = append(conditions, fmt.Sprintf("is_visible = $%d", argIndex))
		args = append(args, *params.IsVisible)
		argIndex++
	}
	if params.ParentID != nil {
		conditions = append(conditions, fmt.Sprintf("parent_id = $%d", argIndex))
		args = append(args, *params.ParentID)
		argIndex++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_menu WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count menus: %w", err)
	}

	// Get data
	orderBy := "sort_order ASC, created_at DESC"
	if params.SortBy != "" {
		sortOrder := sortASC
		if strings.EqualFold(params.SortOrder, sortDESC) {
			sortOrder = sortDESC
		}
		orderBy = params.SortBy + " " + sortOrder
	}

	offset := (params.Page - 1) * params.PageSize
	dataQuery := fmt.Sprintf(`
		SELECT menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
			service_name, menu_level, sort_order, is_visible, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_menu WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d
	`, whereClause, orderBy, argIndex, argIndex+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list menus: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in List menus")
		}
	}()

	var menus []*menu.Menu
	for rows.Next() {
		m, err := r.scanMenuFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		menus = append(menus, m)
	}

	return menus, total, nil
}

// ExistsByCode checks if a menu with the given code exists.
func (r *MenuRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_menu WHERE menu_code = $1 AND deleted_at IS NULL)`
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check code existence: %w", err)
	}
	return exists, nil
}

// HasChildren checks if a menu has children.
func (r *MenuRepository) HasChildren(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_menu WHERE parent_id = $1 AND deleted_at IS NULL)`
	var hasChildren bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&hasChildren); err != nil {
		return false, fmt.Errorf("failed to check children: %w", err)
	}
	return hasChildren, nil
}

// BatchCreate creates multiple menus.
func (r *MenuRepository) BatchCreate(ctx context.Context, menus []*menu.Menu) (int, error) {
	count := 0
	for _, m := range menus {
		if err := r.Create(ctx, m); err == nil {
			count++
		}
	}
	return count, nil
}

// GetTree gets the menu tree.
func (r *MenuRepository) GetTree(ctx context.Context, serviceName string, includeInactive, includeHidden bool) ([]*menu.WithChildren, error) {
	conditions := []string{"deleted_at IS NULL"}
	var args []interface{}
	argIndex := 1

	if serviceName != "" {
		conditions = append(conditions, fmt.Sprintf("service_name = $%d", argIndex))
		args = append(args, serviceName)
	}
	if !includeInactive {
		conditions = append(conditions, "is_active = true")
	}
	if !includeHidden {
		conditions = append(conditions, "is_visible = true")
	}

	query := fmt.Sprintf(`
		SELECT menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
			service_name, menu_level, sort_order, is_visible, is_active,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_menu WHERE %s ORDER BY menu_level, sort_order
	`, strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get menu tree: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in GetTree")
		}
	}()

	var allMenus []*menu.Menu
	for rows.Next() {
		m, err := r.scanMenuFromRows(rows)
		if err != nil {
			return nil, err
		}
		allMenus = append(allMenus, m)
	}

	return buildMenuTree(allMenus), nil
}

// GetTreeForUser gets the menu tree for a specific user.
func (r *MenuRepository) GetTreeForUser(ctx context.Context, _ uuid.UUID, serviceName string) ([]*menu.WithChildren, error) {
	// For now, return all active visible menus
	// TODO: filter by user permissions
	return r.GetTree(ctx, serviceName, false, false)
}

// AssignPermissions assigns permissions to a menu.
func (r *MenuRepository) AssignPermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	now := time.Now()
	for _, permID := range permissionIDs {
		query := `
			INSERT INTO mst_menu_permission (menu_id, permission_id, created_at, created_by)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING
		`
		if _, err := r.db.ExecContext(ctx, query, menuID, permID, now, assignedBy); err != nil {
			log.Warn().Err(err).
				Str("menu_id", menuID.String()).
				Str("permission_id", permID.String()).
				Msg("failed to assign permission to menu")
		}
	}
	return nil
}

// RemovePermissions removes permissions from a menu.
func (r *MenuRepository) RemovePermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID) error {
	for _, permID := range permissionIDs {
		query := `DELETE FROM mst_menu_permission WHERE menu_id = $1 AND permission_id = $2`
		if _, err := r.db.ExecContext(ctx, query, menuID, permID); err != nil {
			log.Warn().Err(err).
				Str("menu_id", menuID.String()).
				Str("permission_id", permID.String()).
				Msg("failed to remove permission from menu")
		}
	}
	return nil
}

// GetPermissions gets permissions for a menu.
func (r *MenuRepository) GetPermissions(ctx context.Context, menuID uuid.UUID) ([]*role.Permission, error) {
	query := `
		SELECT p.permission_id, p.permission_code, p.permission_name, p.service_name,
			p.module_name, p.action_type, p.is_active,
			p.created_at, p.created_by, p.updated_at, p.updated_by
		FROM mst_permission p
		INNER JOIN mst_menu_permission mp ON mp.permission_id = p.permission_id
		WHERE mp.menu_id = $1 AND p.is_active = true
	`
	rows, err := r.db.QueryContext(ctx, query, menuID)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in GetPermissions")
		}
	}()

	var perms []*role.Permission
	for rows.Next() {
		p, err := scanPermissionFromRows(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, nil
}

// Reorder reorders menus within the same parent.
func (r *MenuRepository) Reorder(ctx context.Context, _ *uuid.UUID, menuIDs []uuid.UUID) error {
	for i, menuID := range menuIDs {
		query := `UPDATE mst_menu SET sort_order = $2 WHERE menu_id = $1`
		if _, err := r.db.ExecContext(ctx, query, menuID, i+1); err != nil {
			log.Warn().Err(err).
				Str("menu_id", menuID.String()).
				Int("sort_order", i+1).
				Msg("failed to reorder menu")
		}
	}
	return nil
}

// Helper functions

func (r *MenuRepository) scanMenu(row *sql.Row) (*menu.Menu, error) {
	var id uuid.UUID
	var parentID *uuid.UUID
	var code, title, iconName, serviceName string
	var url sql.NullString
	var level, sortOrder int
	var isVisible, isActive bool
	var createdAt time.Time
	var createdBy string
	var updatedAt sql.NullTime
	var updatedBy sql.NullString
	var deletedAt sql.NullTime
	var deletedBy sql.NullString

	err := row.Scan(
		&id, &parentID, &code, &title, &url, &iconName,
		&serviceName, &level, &sortOrder, &isVisible, &isActive,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan menu: %w", err)
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}

	urlStr := ""
	if url.Valid {
		urlStr = url.String
	}

	return menu.ReconstructMenu(id, parentID, code, title, urlStr, iconName, serviceName, level, sortOrder, isVisible, isActive, audit), nil
}

func (r *MenuRepository) scanMenuFromRows(rows *sql.Rows) (*menu.Menu, error) {
	var id uuid.UUID
	var parentID *uuid.UUID
	var code, title, iconName, serviceName string
	var url sql.NullString
	var level, sortOrder int
	var isVisible, isActive bool
	var createdAt time.Time
	var createdBy string
	var updatedAt sql.NullTime
	var updatedBy sql.NullString
	var deletedAt sql.NullTime
	var deletedBy sql.NullString

	err := rows.Scan(
		&id, &parentID, &code, &title, &url, &iconName,
		&serviceName, &level, &sortOrder, &isVisible, &isActive,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu: %w", err)
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}

	urlStr := ""
	if url.Valid {
		urlStr = url.String
	}

	return menu.ReconstructMenu(id, parentID, code, title, urlStr, iconName, serviceName, level, sortOrder, isVisible, isActive, audit), nil
}

func scanPermissionFromRows(rows *sql.Rows) (*role.Permission, error) {
	var id uuid.UUID
	var code, name, serviceName, moduleName, actionType string
	var isActive bool
	var createdAt time.Time
	var createdBy string
	var updatedAt sql.NullTime
	var updatedBy sql.NullString

	err := rows.Scan(
		&id, &code, &name, &serviceName, &moduleName, &actionType, &isActive,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan permission: %w", err)
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
	}

	return role.ReconstructPermission(id, code, name, "", serviceName, moduleName, actionType, isActive, audit), nil
}

func buildMenuTree(allMenus []*menu.Menu) []*menu.WithChildren {
	menuMap := make(map[uuid.UUID]*menu.WithChildren)
	var roots []*menu.WithChildren

	// Create nodes
	for _, m := range allMenus {
		menuMap[m.ID()] = &menu.WithChildren{Menu: m, Children: []*menu.WithChildren{}}
	}

	// Build tree
	for _, m := range allMenus {
		node := menuMap[m.ID()]
		if m.ParentID() == nil {
			roots = append(roots, node)
		} else if parent, ok := menuMap[*m.ParentID()]; ok {
			parent.Children = append(parent.Children, node)
		}
	}

	return roots
}

func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

func nullStringToPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}
