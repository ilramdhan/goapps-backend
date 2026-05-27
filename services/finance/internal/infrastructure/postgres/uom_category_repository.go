// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// UOMCategoryRepository implements uomcategory.Repository interface using PostgreSQL.
type UOMCategoryRepository struct {
	db *DB
}

// NewUOMCategoryRepository creates a new UOMCategoryRepository instance.
func NewUOMCategoryRepository(db *DB) *UOMCategoryRepository {
	return &UOMCategoryRepository{db: db}
}

// Verify interface implementation at compile time.
var _ uomcategory.Repository = (*UOMCategoryRepository)(nil)

// GetCategoryIDByCode resolves a category code to its UUID.
// Implements uom.CategoryRepository interface.
func (r *UOMCategoryRepository) GetCategoryIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	query := `SELECT uom_category_id FROM mst_uom_category WHERE category_code = $1 AND deleted_at IS NULL`

	var id uuid.UUID
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, uomcategory.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to get category id by code: %w", err)
	}

	return id, nil
}

// Create persists a new UOM Category to the database.
func (r *UOMCategoryRepository) Create(ctx context.Context, entity *uomcategory.Category) error {
	query := `
		INSERT INTO mst_uom_category (
			uom_category_id, category_code, category_name, description,
			is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code().String(),
		entity.Name(),
		entity.Description(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)

	if err != nil {
		if isUOMCategoryUniqueViolation(err) {
			return uomcategory.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create uom category: %w", err)
	}

	return nil
}

// GetByID retrieves a UOM Category by its ID.
func (r *UOMCategoryRepository) GetByID(ctx context.Context, id uuid.UUID) (*uomcategory.Category, error) {
	query := `
		SELECT uom_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom_category
		WHERE uom_category_id = $1 AND deleted_at IS NULL
	`

	return r.scanCategory(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a UOM Category by its code.
func (r *UOMCategoryRepository) GetByCode(ctx context.Context, code uomcategory.Code) (*uomcategory.Category, error) {
	query := `
		SELECT uom_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom_category
		WHERE category_code = $1 AND deleted_at IS NULL
	`

	return r.scanCategory(r.db.QueryRowContext(ctx, query, code.String()))
}

// List retrieves UOM Categories with filtering, searching, and pagination.
//
//nolint:dupl // Mirrors RMCategoryRepository.List — different table/types prevent shared code.
func (r *UOMCategoryRepository) List(ctx context.Context, filter uomcategory.ListFilter) ([]*uomcategory.Category, int64, error) {
	filter.Validate()

	baseQuery := `FROM mst_uom_category WHERE deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// Search filter
	if filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (
			category_code ILIKE $%d OR
			category_name ILIKE $%d OR
			description ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		baseQuery += fmt.Sprintf(` AND is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
		argIndex++
	}

	// Count total
	var total int64
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count uom categories: %w", err)
	}

	// Build order clause with sort column mapping
	sortColumnMap := map[string]string{
		"code":           "category_code",
		"name":           "category_name",
		sortKeyCreatedAt: sortKeyCreatedAt,
	}
	orderColumn := "category_code"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderColumn = mapped
	}
	orderDir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		orderDir = sortDESC
	}

	selectQuery := `
		SELECT uom_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
	` + baseQuery + fmt.Sprintf(
		` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderColumn, orderDir, argIndex, argIndex+1,
	)

	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list uom categories: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var categories []*uomcategory.Category
	for rows.Next() {
		entity, err := r.scanCategoryFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		categories = append(categories, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating uom category rows: %w", err)
	}

	return categories, total, nil
}

// Update persists changes to an existing UOM Category.
func (r *UOMCategoryRepository) Update(ctx context.Context, entity *uomcategory.Category) error {
	query := `
		UPDATE mst_uom_category SET
			category_name = $2,
			description = $3,
			is_active = $4,
			updated_at = $5,
			updated_by = $6
		WHERE uom_category_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.Description(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update uom category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return uomcategory.ErrNotFound
	}

	return nil
}

// SoftDelete marks a UOM Category as deleted.
func (r *UOMCategoryRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_uom_category SET
			deleted_at = $2,
			deleted_by = $3,
			is_active = false
		WHERE uom_category_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to soft delete uom category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return uomcategory.ErrNotFound
	}

	return nil
}

// ExistsByCode checks if a UOM Category with the given code exists.
func (r *UOMCategoryRepository) ExistsByCode(ctx context.Context, code uomcategory.Code) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_uom_category WHERE category_code = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code.String()).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check uom category existence: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if a UOM Category with the given ID exists.
func (r *UOMCategoryRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_uom_category WHERE uom_category_id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check uom category existence: %w", err)
	}

	return exists, nil
}

// ListAll retrieves all non-deleted UOM Categories (for export).
func (r *UOMCategoryRepository) ListAll(ctx context.Context, filter uomcategory.ExportFilter) ([]*uomcategory.Category, error) {
	query := `
		SELECT uom_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom_category
		WHERE deleted_at IS NULL
	`
	args := []interface{}{}
	argIndex := 1

	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY category_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all uom categories: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var categories []*uomcategory.Category
	for rows.Next() {
		entity, err := r.scanCategoryFromRows(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uom category rows: %w", err)
	}

	return categories, nil
}

// IsInUse checks if a UOM Category is referenced by any UOM.
func (r *UOMCategoryRepository) IsInUse(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_uom WHERE uom_category_id = $1 AND deleted_at IS NULL)`

	var inUse bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&inUse); err != nil {
		return false, fmt.Errorf("failed to check uom category usage: %w", err)
	}

	return inUse, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func (r *UOMCategoryRepository) scanCategory(row *sql.Row) (*uomcategory.Category, error) {
	var dto uomCategoryDTO
	err := row.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.Description,
		&dto.IsActive,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, uomcategory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan uom category: %w", err)
	}

	return dto.ToEntity()
}

func (r *UOMCategoryRepository) scanCategoryFromRows(rows *sql.Rows) (*uomcategory.Category, error) {
	var dto uomCategoryDTO
	err := rows.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.Description,
		&dto.IsActive,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan uom category row: %w", err)
	}

	return dto.ToEntity()
}

// uomCategoryDTO is a data transfer object for database operations.
type uomCategoryDTO struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Description sql.NullString
	IsActive    bool
	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   sql.NullTime
	UpdatedBy   sql.NullString
	DeletedAt   sql.NullTime
	DeletedBy   sql.NullString
}

// ToEntity converts DTO to domain entity.
func (d *uomCategoryDTO) ToEntity() (*uomcategory.Category, error) {
	code, err := uomcategory.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}

	var description string
	if d.Description.Valid {
		description = d.Description.String
	}

	var updatedAt *time.Time
	if d.UpdatedAt.Valid {
		updatedAt = &d.UpdatedAt.Time
	}

	var updatedBy *string
	if d.UpdatedBy.Valid {
		updatedBy = &d.UpdatedBy.String
	}

	var deletedAt *time.Time
	if d.DeletedAt.Valid {
		deletedAt = &d.DeletedAt.Time
	}

	var deletedBy *string
	if d.DeletedBy.Valid {
		deletedBy = &d.DeletedBy.String
	}

	return uomcategory.ReconstructCategory(
		d.ID,
		code,
		d.Name,
		description,
		d.IsActive,
		d.CreatedAt,
		d.CreatedBy,
		updatedAt,
		updatedBy,
		deletedAt,
		deletedBy,
	), nil
}

// isUOMCategoryUniqueViolation checks if the error is a PostgreSQL unique violation.
func isUOMCategoryUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	return false
}
