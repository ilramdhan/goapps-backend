package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/group"
)

// BiDashboardGroupRepository implements group.Repository.
type BiDashboardGroupRepository struct {
	db *DB
}

// NewBiDashboardGroupRepository constructs a BiDashboardGroupRepository.
func NewBiDashboardGroupRepository(db *DB) *BiDashboardGroupRepository {
	return &BiDashboardGroupRepository{db: db}
}

var _ group.Repository = (*BiDashboardGroupRepository)(nil)

const selectGroupBase = `
SELECT group_id, group_code, group_name, description, icon, display_order, is_active,
       created_at, created_by, updated_at, updated_by
FROM bi_dashboard_group`

// Create persists a new group row.
func (r *BiDashboardGroupRepository) Create(ctx context.Context, g *group.Group) error {
	const q = `
INSERT INTO bi_dashboard_group (
    group_id, group_code, group_name, description, icon, display_order, is_active,
    created_at, created_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := r.db.ExecContext(ctx, q,
		g.ID(), g.Code(), g.Name(), biNullableString(g.Description()), biNullableString(g.Icon()),
		g.DisplayOrder(), g.IsActive(),
		g.CreatedAt(), nullableUUID(g.CreatedBy()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return group.ErrAlreadyExists
		}
		return fmt.Errorf("insert bi_dashboard_group: %w", err)
	}
	return nil
}

// GetByID looks up by primary key.
func (r *BiDashboardGroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	row := r.db.QueryRowContext(ctx, selectGroupBase+" WHERE group_id = $1", id)
	return r.scanGroup(row.Scan)
}

// GetByCode looks up by business code.
func (r *BiDashboardGroupRepository) GetByCode(ctx context.Context, code string) (*group.Group, error) {
	row := r.db.QueryRowContext(ctx, selectGroupBase+" WHERE group_code = $1", code)
	return r.scanGroup(row.Scan)
}

// List returns groups (optionally including inactive) ordered by display_order then code.
func (r *BiDashboardGroupRepository) List(ctx context.Context, includeInactive bool) ([]*group.Group, error) {
	q := selectGroupBase
	if !includeInactive {
		q += " WHERE is_active = TRUE"
	}
	q += " ORDER BY display_order ASC, group_code ASC"

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*group.Group
	for rows.Next() {
		g, err := r.scanGroup(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Update mutates the group row.
func (r *BiDashboardGroupRepository) Update(ctx context.Context, g *group.Group) error {
	const q = `
UPDATE bi_dashboard_group SET
    group_name = $2, description = $3, icon = $4,
    display_order = $5, is_active = $6,
    updated_at = $7, updated_by = $8
WHERE group_id = $1`
	res, err := r.db.ExecContext(ctx, q,
		g.ID(), g.Name(), biNullableString(g.Description()), biNullableString(g.Icon()),
		g.DisplayOrder(), g.IsActive(),
		g.UpdatedAt(), nullableUUID(g.UpdatedBy()),
	)
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return group.ErrNotFound
	}
	return nil
}

// Delete refuses when active dashboards reference the group, otherwise removes the row.
func (r *BiDashboardGroupRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var refs int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM bi_dashboard WHERE group_id = $1 AND deleted_at IS NULL", id,
	).Scan(&refs); err != nil {
		return fmt.Errorf("count refs: %w", err)
	}
	if refs > 0 {
		return group.ErrInUse
	}
	res, err := r.db.ExecContext(ctx, "DELETE FROM bi_dashboard_group WHERE group_id = $1", id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return group.ErrNotFound
	}
	return nil
}

// scanGroup reads one row into a domain Group.
func (r *BiDashboardGroupRepository) scanGroup(scan scanFunc) (*group.Group, error) {
	var (
		id           uuid.UUID
		code         string
		name         string
		description  sql.NullString
		icon         sql.NullString
		displayOrder int
		isActive     bool
		createdAt    sql.NullTime
		createdBy    uuid.NullUUID
		updatedAt    sql.NullTime
		updatedBy    uuid.NullUUID
	)
	err := scan(&id, &code, &name, &description, &icon, &displayOrder, &isActive,
		&createdAt, &createdBy, &updatedAt, &updatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, group.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan group: %w", err)
	}

	g, err := group.NewGroup(group.NewGroupParams{
		ID:           id,
		Code:         code,
		Name:         name,
		Description:  nullToString(description),
		Icon:         nullToString(icon),
		DisplayOrder: displayOrder,
		IsActive:     isActive,
		CreatedBy:    uuidOrNil(createdBy),
	})
	if err != nil {
		return nil, fmt.Errorf("reconstruct group from db: %w", err)
	}
	g.SetAuditFromHydration(nullTimeOrZero(createdAt), nullTimeOrZero(updatedAt),
		uuidOrNil(createdBy), uuidOrNil(updatedBy))
	return g, nil
}
