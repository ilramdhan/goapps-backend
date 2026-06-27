package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costerp"
)

// CostErpRepository implements costerp.Repository.
type CostErpRepository struct{ db *DB }

// NewCostErpRepository constructs the repository.
func NewCostErpRepository(db *DB) *CostErpRepository {
	return &CostErpRepository{db: db}
}

var _ costerp.Repository = (*CostErpRepository)(nil)

// =============================================================================
// Items
// =============================================================================

// scanItem scans a full cost_erp_item row (10 columns including audit fields).
// Columns must be: cei_item_id, cei_item_code, cei_item_name, cei_item_type,
// cei_is_active, cei_synced_at, cei_created_at, cei_updated_at,
// cei_created_by, cei_updated_by.
func scanItem(row interface {
	Scan(...any) error
}) (*costerp.Item, error) {
	it := &costerp.Item{}
	var itemName, itemType sql.NullString
	var syncedAt time.Time
	var createdAt, updatedAt time.Time
	var createdBy, updatedBy sql.NullString
	if err := row.Scan(
		&it.ItemID, &it.ItemCode, &itemName, &itemType, &it.IsActive, &syncedAt,
		&createdAt, &updatedAt, &createdBy, &updatedBy,
	); err != nil {
		return nil, err
	}
	it.ItemName = itemName.String
	it.ItemType = itemType.String
	it.SyncedAt = syncedAt
	it.CreatedAt = createdAt
	it.UpdatedAt = updatedAt
	it.CreatedBy = createdBy.String
	it.UpdatedBy = updatedBy.String
	return it, nil
}

const itemSelectCols = `cei_item_id, cei_item_code, cei_item_name, cei_item_type,
	cei_is_active, cei_synced_at, cei_created_at, cei_updated_at,
	cei_created_by, cei_updated_by`

// ListItems returns a filtered paginated list of cost_erp_item rows.
func (r *CostErpRepository) ListItems(ctx context.Context, f costerp.ItemFilter) ([]*costerp.Item, int64, error) {
	where := "FROM cost_erp_item WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cei_item_code) LIKE LOWER($%d) OR LOWER(cei_item_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.ItemType != "" {
		where += fmt.Sprintf(` AND cei_item_type = $%d`, idx)
		args = append(args, f.ItemType)
		idx++
	}
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND cei_is_active = TRUE`
	case filterInactive:
		where += ` AND cei_is_active = FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_erp_item: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + itemSelectCols + `
		` + where + fmt.Sprintf(` ORDER BY cei_item_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_erp_item: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	items := []*costerp.Item{}
	for rows.Next() {
		it, sErr := scanItem(rows)
		if sErr != nil {
			return nil, 0, fmt.Errorf("scan cost_erp_item: %w", sErr)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_erp_item: %w", err)
	}
	return items, total, nil
}

// GetItem loads a single cost_erp_item by id.
func (r *CostErpRepository) GetItem(ctx context.Context, itemID int64) (*costerp.Item, error) {
	q := `SELECT ` + itemSelectCols + `
		FROM cost_erp_item WHERE cei_item_id = $1`
	it, err := scanItem(r.db.QueryRowContext(ctx, q, itemID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costerp.ErrNotFound
		}
		return nil, fmt.Errorf("get cost_erp_item: %w", err)
	}
	return it, nil
}

// =============================================================================
// Grades
// =============================================================================

// ListGrades returns a paginated list of cost_erp_grade rows.
func (r *CostErpRepository) ListGrades(ctx context.Context, f costerp.LookupFilter) ([]*costerp.Grade, int64, error) { //nolint:dupl // parallel to ListShades but distinct entity/table
	where := "FROM cost_erp_grade WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(ceg_grade_code) LIKE LOWER($%d) OR LOWER(ceg_grade_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND ceg_is_active = TRUE`
	case filterInactive:
		where += ` AND ceg_is_active = FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_erp_grade: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `
		SELECT ceg_grade_id,ceg_grade_code,ceg_grade_name,ceg_is_active,ceg_synced_at
		` + where + fmt.Sprintf(` ORDER BY ceg_grade_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_erp_grade: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	items := []*costerp.Grade{}
	for rows.Next() {
		g := &costerp.Grade{}
		var name sql.NullString
		var syncedAt time.Time
		if sErr := rows.Scan(&g.GradeID, &g.GradeCode, &name, &g.IsActive, &syncedAt); sErr != nil {
			return nil, 0, fmt.Errorf("scan cost_erp_grade: %w", sErr)
		}
		g.GradeName = name.String
		g.SyncedAt = syncedAt
		items = append(items, g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_erp_grade: %w", err)
	}
	return items, total, nil
}

// =============================================================================
// Shades
// =============================================================================

// ListShades returns a paginated list of cost_erp_shade rows.
func (r *CostErpRepository) ListShades(ctx context.Context, f costerp.LookupFilter) ([]*costerp.Shade, int64, error) { //nolint:dupl // parallel to ListGrades but distinct entity/table
	where := "FROM cost_erp_shade WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(ces_shade_code) LIKE LOWER($%d) OR LOWER(ces_shade_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND ces_is_active = TRUE`
	case filterInactive:
		where += ` AND ces_is_active = FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_erp_shade: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `
		SELECT ces_shade_id,ces_shade_code,ces_shade_name,ces_is_active,ces_synced_at
		` + where + fmt.Sprintf(` ORDER BY ces_shade_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_erp_shade: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	items := []*costerp.Shade{}
	for rows.Next() {
		s := &costerp.Shade{}
		var name sql.NullString
		var syncedAt time.Time
		if sErr := rows.Scan(&s.ShadeID, &s.ShadeCode, &name, &s.IsActive, &syncedAt); sErr != nil {
			return nil, 0, fmt.Errorf("scan cost_erp_shade: %w", sErr)
		}
		s.ShadeName = name.String
		s.SyncedAt = syncedAt
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_erp_shade: %w", err)
	}
	return items, total, nil
}
