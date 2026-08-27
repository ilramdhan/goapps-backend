package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// MBLustureRepository implements mblusture.Repository using PostgreSQL.
type MBLustureRepository struct {
	db *DB
}

// NewMBLustureRepository creates a new MBLustureRepository instance.
func NewMBLustureRepository(db *DB) *MBLustureRepository {
	return &MBLustureRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mblusture.Repository = (*MBLustureRepository)(nil)

// Create persists a new lusture row.
func (r *MBLustureRepository) Create(ctx context.Context, e *mblusture.Entity) error {
	const q = `
		INSERT INTO mst_mb_lusture
			(mbl_code, mbl_display_name, mbl_full_description, mbl_category,
			 mbl_is_active, mbl_display_order, mbl_created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING mbl_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		e.Code(), e.DisplayName(), e.FullDescription(), e.Category(),
		e.IsActive(), e.DisplayOrder(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return mblusture.ErrAlreadyExists
		}
		return fmt.Errorf("mb_lusture_repository: create: %w", err)
	}
	return nil
}

// Update persists changes to an existing lusture row.
func (r *MBLustureRepository) Update(ctx context.Context, e *mblusture.Entity) error {
	const q = `
		UPDATE mst_mb_lusture
		SET mbl_display_name = $2, mbl_full_description = $3, mbl_category = $4,
		    mbl_is_active = $5, mbl_display_order = $6,
		    mbl_updated_at = NOW(), mbl_updated_by = $7
		WHERE mbl_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, e.ID(), e.DisplayName(), e.FullDescription(),
		e.Category(), e.IsActive(), e.DisplayOrder(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("mb_lusture_repository: update: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_lusture_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mblusture.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a lusture row by ID.
func (r *MBLustureRepository) Delete(ctx context.Context, id string) error {
	const q = `UPDATE mst_mb_lusture SET deleted_at = NOW() WHERE mbl_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mb_lusture_repository: delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_lusture_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mblusture.ErrNotFound
	}
	return nil
}

// GetByID returns a single active lusture row by ID.
func (r *MBLustureRepository) GetByID(ctx context.Context, id string) (*mblusture.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbl_id = $1 AND deleted_at IS NULL`, id)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mblusture.ErrNotFound
		}
		return nil, fmt.Errorf("mb_lusture_repository: get by id: %w", err)
	}
	return e, nil
}

// GetByCode returns a single active lusture row by its unique code.
func (r *MBLustureRepository) GetByCode(ctx context.Context, code string) (*mblusture.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbl_code = $1 AND deleted_at IS NULL`, code)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mblusture.ErrNotFound
		}
		return nil, fmt.Errorf("mb_lusture_repository: get by code: %w", err)
	}
	return e, nil
}

// ListAll retrieves all non-deleted lustures matching filter, unpaginated (for export).
func (r *MBLustureRepository) ListAll(ctx context.Context, filter mblusture.ExportFilter) ([]*mblusture.Entity, error) {
	where := whereNotDeleted
	args := []any{}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbl_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	listQ := fmt.Sprintf("%s %s ORDER BY mbl_code %s", r.selectCols(), where, sortASC)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mb_lusture_repository: list all: %w", err)
	}
	defer closeRows(rows)

	var out []*mblusture.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_lusture_repository: iterate: %w", err)
	}
	return out, nil
}

// List returns paginated active lusture rows, optionally filtered by a search term matched
// against code and display name.
func (r *MBLustureRepository) List(ctx context.Context, filter mblusture.ListFilter) ([]*mblusture.Entity, int64, error) { //nolint:dupl // Mirrors MBCrossSectionRepository.List — different table/types prevent shared code.
	filter.Validate()

	where := whereNotDeleted
	args := []any{}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND (mbl_code ILIKE $%d OR mbl_display_name ILIKE $%d)", len(args)+1, len(args)+1)
		args = append(args, "%"+filter.Search+"%")
	}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbl_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM mst_mb_lusture " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mb_lusture_repository: count: %w", err)
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
		return nil, 0, fmt.Errorf("mb_lusture_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mblusture.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mb_lusture_repository: iterate: %w", err)
	}
	return out, total, nil
}

func (r *MBLustureRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"code":           "mbl_code",
		"display_name":   "mbl_display_name",
		"category":       "mbl_category",
		sortKeyCreatedAt: "mbl_created_at",
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbl_code"
}

func (r *MBLustureRepository) selectCols() string {
	return `
		SELECT mbl_id, mbl_code, COALESCE(mbl_display_name, ''), COALESCE(mbl_full_description, ''),
		       COALESCE(mbl_category, ''), mbl_is_active, COALESCE(mbl_display_order, 0),
		       mbl_created_at, mbl_created_by,
		       COALESCE(mbl_updated_at::text, ''), COALESCE(mbl_updated_by, ''),
		       COALESCE(deleted_at::text, ''), COALESCE(deleted_by, '')
		FROM mst_mb_lusture
	`
}

type mbLustureDTO struct {
	ID              string
	Code            string
	DisplayName     string
	FullDescription string
	Category        string
	IsActive        bool
	DisplayOrder    int32
	CreatedAt       string
	CreatedBy       string
	UpdatedAt       string
	UpdatedBy       string
	DeletedAt       string
	DeletedBy       string
}

func (d *mbLustureDTO) toEntity() *mblusture.Entity {
	return mblusture.Reconstruct(
		d.ID, d.Code, d.DisplayName, d.FullDescription, d.Category, d.DisplayOrder, d.IsActive,
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy, d.DeletedAt, d.DeletedBy,
	)
}

func (r *MBLustureRepository) scanOne(row *sql.Row) (*mblusture.Entity, error) {
	var d mbLustureDTO
	err := row.Scan(&d.ID, &d.Code, &d.DisplayName, &d.FullDescription, &d.Category,
		&d.IsActive, &d.DisplayOrder, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, err
	}
	return d.toEntity(), nil
}

func (r *MBLustureRepository) scanRow(rows *sql.Rows) (*mblusture.Entity, error) {
	var d mbLustureDTO
	err := rows.Scan(&d.ID, &d.Code, &d.DisplayName, &d.FullDescription, &d.Category,
		&d.IsActive, &d.DisplayOrder, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, fmt.Errorf("mb_lusture_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}
