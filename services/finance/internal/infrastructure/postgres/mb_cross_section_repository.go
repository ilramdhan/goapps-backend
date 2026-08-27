package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// MBCrossSectionRepository implements mbcrosssection.Repository using PostgreSQL.
type MBCrossSectionRepository struct {
	db *DB
}

// NewMBCrossSectionRepository creates a new MBCrossSectionRepository instance.
func NewMBCrossSectionRepository(db *DB) *MBCrossSectionRepository {
	return &MBCrossSectionRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbcrosssection.Repository = (*MBCrossSectionRepository)(nil)

// Create persists a new cross-section row.
func (r *MBCrossSectionRepository) Create(ctx context.Context, e *mbcrosssection.Entity) error {
	const q = `
		INSERT INTO mst_mb_cross_section
			(mbcs_code, mbcs_display_name, mbcs_description,
			 mbcs_is_active, mbcs_display_order, mbcs_created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING mbcs_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		e.Code(), e.DisplayName(), e.Description(),
		e.IsActive(), e.DisplayOrder(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return mbcrosssection.ErrAlreadyExists
		}
		return fmt.Errorf("mb_cross_section_repository: create: %w", err)
	}
	return nil
}

// Update persists changes to an existing cross-section row.
func (r *MBCrossSectionRepository) Update(ctx context.Context, e *mbcrosssection.Entity) error {
	const q = `
		UPDATE mst_mb_cross_section
		SET mbcs_display_name = $2, mbcs_description = $3,
		    mbcs_is_active = $4, mbcs_display_order = $5,
		    mbcs_updated_at = NOW(), mbcs_updated_by = $6
		WHERE mbcs_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, e.ID(), e.DisplayName(), e.Description(),
		e.IsActive(), e.DisplayOrder(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("mb_cross_section_repository: update: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_cross_section_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcrosssection.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a cross-section row by ID.
func (r *MBCrossSectionRepository) Delete(ctx context.Context, id, deletedBy string) error {
	const q = `
		UPDATE mst_mb_cross_section
		SET deleted_at = NOW(), deleted_by = $2
		WHERE mbcs_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id, deletedBy)
	if err != nil {
		return fmt.Errorf("mb_cross_section_repository: delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_cross_section_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcrosssection.ErrNotFound
	}
	return nil
}

// GetByID returns a single live cross-section row by ID.
func (r *MBCrossSectionRepository) GetByID(ctx context.Context, id string) (*mbcrosssection.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbcs_id = $1 AND deleted_at IS NULL`, id)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbcrosssection.ErrNotFound
		}
		return nil, fmt.Errorf("mb_cross_section_repository: get by id: %w", err)
	}
	return e, nil
}

// GetByCode returns a single live cross-section row by its unique code.
func (r *MBCrossSectionRepository) GetByCode(ctx context.Context, code string) (*mbcrosssection.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbcs_code = $1 AND deleted_at IS NULL`, code)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbcrosssection.ErrNotFound
		}
		return nil, fmt.Errorf("mb_cross_section_repository: get by code: %w", err)
	}
	return e, nil
}

// List returns paginated live cross-section rows, optionally filtered by a search term
// matched against code and display name.
func (r *MBCrossSectionRepository) List(ctx context.Context, filter mbcrosssection.ListFilter) ([]*mbcrosssection.Entity, int64, error) { //nolint:dupl // Mirrors MBLustureRepository.List — different table/types prevent shared code.
	filter.Validate()

	where := whereNotDeleted
	args := []any{}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND (mbcs_code ILIKE $%d OR mbcs_display_name ILIKE $%d)", len(args)+1, len(args)+1)
		args = append(args, "%"+filter.Search+"%")
	}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbcs_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM mst_mb_cross_section " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_repository: count: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	listQ := fmt.Sprintf("%s %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		r.selectCols(), where, orderCol, dir, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbcrosssection.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_repository: iterate: %w", err)
	}
	return out, total, nil
}

func (r *MBCrossSectionRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"code":           "mbcs_code",
		"display_name":   "mbcs_display_name",
		"display_order":  "mbcs_display_order",
		sortKeyCreatedAt: "mbcs_created_at",
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbcs_display_order"
}

func (r *MBCrossSectionRepository) selectCols() string {
	return `
		SELECT mbcs_id, mbcs_code, COALESCE(mbcs_display_name, ''), COALESCE(mbcs_description, ''),
		       mbcs_is_active, COALESCE(mbcs_display_order, 0),
		       mbcs_created_at, mbcs_created_by,
		       COALESCE(mbcs_updated_at::text, ''), COALESCE(mbcs_updated_by, ''),
		       COALESCE(deleted_at::text, ''), COALESCE(deleted_by, '')
		FROM mst_mb_cross_section
	`
}

type mbCrossSectionDTO struct {
	ID           string
	Code         string
	DisplayName  string
	Description  string
	IsActive     bool
	DisplayOrder int32
	CreatedAt    string
	CreatedBy    string
	UpdatedAt    string
	UpdatedBy    string
	DeletedAt    string
	DeletedBy    string
}

func (d *mbCrossSectionDTO) toEntity() *mbcrosssection.Entity {
	return mbcrosssection.Reconstruct(
		d.ID, d.Code, d.DisplayName, d.Description, d.DisplayOrder, d.IsActive,
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy, d.DeletedAt, d.DeletedBy,
	)
}

func (r *MBCrossSectionRepository) scanOne(row *sql.Row) (*mbcrosssection.Entity, error) {
	var d mbCrossSectionDTO
	err := row.Scan(&d.ID, &d.Code, &d.DisplayName, &d.Description,
		&d.IsActive, &d.DisplayOrder, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, err
	}
	return d.toEntity(), nil
}

func (r *MBCrossSectionRepository) scanRow(rows *sql.Rows) (*mbcrosssection.Entity, error) {
	var d mbCrossSectionDTO
	err := rows.Scan(&d.ID, &d.Code, &d.DisplayName, &d.Description,
		&d.IsActive, &d.DisplayOrder, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, fmt.Errorf("mb_cross_section_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}
