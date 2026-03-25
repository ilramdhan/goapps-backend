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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// RMCategoryRepository implements rmcategory.Repository interface using PostgreSQL.
type RMCategoryRepository struct {
	db *DB
}

// NewRMCategoryRepository creates a new RMCategoryRepository instance.
func NewRMCategoryRepository(db *DB) *RMCategoryRepository {
	return &RMCategoryRepository{db: db}
}

// Verify interface implementation at compile time.
var _ rmcategory.Repository = (*RMCategoryRepository)(nil)

// Create persists a new RMCategory to the database.
func (r *RMCategoryRepository) Create(ctx context.Context, entity *rmcategory.RMCategory) error {
	query := `
		INSERT INTO mst_rm_category (
			rm_category_id, category_code, category_name, description,
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
		if isRMCategoryUniqueViolation(err) {
			return rmcategory.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create rm category: %w", err)
	}

	return nil
}

// GetByID retrieves an RMCategory by its ID.
func (r *RMCategoryRepository) GetByID(ctx context.Context, id uuid.UUID) (*rmcategory.RMCategory, error) {
	query := `
		SELECT rm_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_rm_category
		WHERE rm_category_id = $1 AND deleted_at IS NULL
	`

	return r.scanRMCategory(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves an RMCategory by its code.
func (r *RMCategoryRepository) GetByCode(ctx context.Context, code rmcategory.Code) (*rmcategory.RMCategory, error) {
	query := `
		SELECT rm_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_rm_category
		WHERE category_code = $1 AND deleted_at IS NULL
	`

	return r.scanRMCategory(r.db.QueryRowContext(ctx, query, code.String()))
}

// List retrieves RMCategories with filtering, searching, and pagination.
func (r *RMCategoryRepository) List(ctx context.Context, filter rmcategory.ListFilter) ([]*rmcategory.RMCategory, int64, error) {
	filter.Validate()

	// Build dynamic query
	baseQuery := `FROM mst_rm_category WHERE deleted_at IS NULL`
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
		return nil, 0, fmt.Errorf("failed to count rm categories: %w", err)
	}

	// Build order clause with sort column mapping
	sortColumnMap := map[string]string{
		"code":       "category_code",
		"name":       "category_name",
		"created_at": "created_at",
	}
	orderColumn := "category_code"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderColumn = mapped
	}
	orderDir := "ASC"
	if strings.ToUpper(filter.SortOrder) == "DESC" {
		orderDir = "DESC"
	}

	// Data query with pagination
	selectQuery := `
		SELECT rm_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
	` + baseQuery + fmt.Sprintf(
		` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderColumn, orderDir, argIndex, argIndex+1,
	)

	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list rm categories: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var categories []*rmcategory.RMCategory
	for rows.Next() {
		entity, err := r.scanRMCategoryFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		categories = append(categories, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating rm category rows: %w", err)
	}

	return categories, total, nil
}

// Update persists changes to an existing RMCategory.
func (r *RMCategoryRepository) Update(ctx context.Context, entity *rmcategory.RMCategory) error {
	query := `
		UPDATE mst_rm_category SET
			category_name = $2,
			description = $3,
			is_active = $4,
			updated_at = $5,
			updated_by = $6
		WHERE rm_category_id = $1 AND deleted_at IS NULL
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
		return fmt.Errorf("failed to update rm category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return rmcategory.ErrNotFound
	}

	return nil
}

// SoftDelete marks an RMCategory as deleted.
func (r *RMCategoryRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_rm_category SET
			deleted_at = $2,
			deleted_by = $3,
			is_active = false
		WHERE rm_category_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to soft delete rm category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return rmcategory.ErrNotFound
	}

	return nil
}

// ExistsByCode checks if an RMCategory with the given code exists.
func (r *RMCategoryRepository) ExistsByCode(ctx context.Context, code rmcategory.Code) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_rm_category WHERE category_code = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code.String()).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check rm category existence: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if an RMCategory with the given ID exists.
func (r *RMCategoryRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_rm_category WHERE rm_category_id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check rm category existence: %w", err)
	}

	return exists, nil
}

// ListAll retrieves all non-deleted RMCategories (for export).
func (r *RMCategoryRepository) ListAll(ctx context.Context, filter rmcategory.ExportFilter) ([]*rmcategory.RMCategory, error) {
	query := `
		SELECT rm_category_id, category_code, category_name, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_rm_category
		WHERE deleted_at IS NULL
	`
	args := []interface{}{}
	argIndex := 1

	// IsActive filter
	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY category_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all rm categories: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var categories []*rmcategory.RMCategory
	for rows.Next() {
		entity, err := r.scanRMCategoryFromRows(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rm category rows: %w", err)
	}

	return categories, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func (r *RMCategoryRepository) scanRMCategory(row *sql.Row) (*rmcategory.RMCategory, error) {
	var dto rmCategoryDTO
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
		return nil, rmcategory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan rm category: %w", err)
	}

	return dto.ToEntity()
}

func (r *RMCategoryRepository) scanRMCategoryFromRows(rows *sql.Rows) (*rmcategory.RMCategory, error) {
	var dto rmCategoryDTO
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
		return nil, fmt.Errorf("failed to scan rm category row: %w", err)
	}

	return dto.ToEntity()
}

// rmCategoryDTO is a data transfer object for database operations.
type rmCategoryDTO struct {
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
func (d *rmCategoryDTO) ToEntity() (*rmcategory.RMCategory, error) {
	code, err := rmcategory.NewCode(d.Code)
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

	return rmcategory.ReconstructRMCategory(
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

// isRMCategoryUniqueViolation checks if the error is a PostgreSQL unique violation.
func isRMCategoryUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	return false
}
