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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// UOMRepository implements uom.Repository interface using PostgreSQL.
type UOMRepository struct {
	db *DB
}

// NewUOMRepository creates a new UOMRepository instance.
func NewUOMRepository(db *DB) *UOMRepository {
	return &UOMRepository{db: db}
}

// Verify interface implementation at compile time.
var _ uom.Repository = (*UOMRepository)(nil)

// Create persists a new UOM to the database.
func (r *UOMRepository) Create(ctx context.Context, entity *uom.UOM) error {
	query := `
		INSERT INTO mst_uom (
			uom_id, uom_code, uom_name, uom_category, description,
			is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code().String(),
		entity.Name(),
		entity.Category().String(),
		entity.Description(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)

	if err != nil {
		if isUniqueViolation(err) {
			return uom.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create uom: %w", err)
	}

	return nil
}

// GetByID retrieves a UOM by its ID.
func (r *UOMRepository) GetByID(ctx context.Context, id uuid.UUID) (*uom.UOM, error) {
	query := `
		SELECT uom_id, uom_code, uom_name, uom_category, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom
		WHERE uom_id = $1 AND deleted_at IS NULL
	`

	return r.scanUOM(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a UOM by its code.
func (r *UOMRepository) GetByCode(ctx context.Context, code uom.Code) (*uom.UOM, error) {
	query := `
		SELECT uom_id, uom_code, uom_name, uom_category, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom
		WHERE uom_code = $1 AND deleted_at IS NULL
	`

	return r.scanUOM(r.db.QueryRowContext(ctx, query, code.String()))
}

// List retrieves UOMs with filtering, searching, and pagination.
func (r *UOMRepository) List(ctx context.Context, filter uom.ListFilter) ([]*uom.UOM, int64, error) {
	filter.Validate()

	// Build dynamic query
	baseQuery := `FROM mst_uom WHERE deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// Search filter
	if filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (
			uom_code ILIKE $%d OR 
			uom_name ILIKE $%d OR 
			description ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// Category filter
	if filter.Category != nil {
		baseQuery += fmt.Sprintf(` AND uom_category = $%d`, argIndex)
		args = append(args, filter.Category.String())
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
		return nil, 0, fmt.Errorf("failed to count uoms: %w", err)
	}

	// Build order clause
	orderColumn := "uom_code"
	switch filter.SortBy {
	case "name":
		orderColumn = "uom_name"
	case "created_at":
		orderColumn = "created_at"
	}
	orderDir := "ASC"
	if strings.ToUpper(filter.SortOrder) == "DESC" {
		orderDir = "DESC"
	}

	// Data query with pagination
	selectQuery := `
		SELECT uom_id, uom_code, uom_name, uom_category, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
	` + baseQuery + fmt.Sprintf(
		` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderColumn, orderDir, argIndex, argIndex+1,
	)

	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list uoms: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log silently as this is cleanup
			_ = err
		}
	}()

	var uoms []*uom.UOM
	for rows.Next() {
		entity, err := r.scanUOMFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		uoms = append(uoms, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating uom rows: %w", err)
	}

	return uoms, total, nil
}

// Update persists changes to an existing UOM.
func (r *UOMRepository) Update(ctx context.Context, entity *uom.UOM) error {
	query := `
		UPDATE mst_uom SET
			uom_name = $2,
			uom_category = $3,
			description = $4,
			is_active = $5,
			updated_at = $6,
			updated_by = $7
		WHERE uom_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.Category().String(),
		entity.Description(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update uom: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return uom.ErrNotFound
	}

	return nil
}

// SoftDelete marks a UOM as deleted.
func (r *UOMRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_uom SET
			deleted_at = $2,
			deleted_by = $3,
			is_active = false
		WHERE uom_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to soft delete uom: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return uom.ErrNotFound
	}

	return nil
}

// ExistsByCode checks if a UOM with the given code exists.
func (r *UOMRepository) ExistsByCode(ctx context.Context, code uom.Code) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_uom WHERE uom_code = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code.String()).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check uom existence: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if a UOM with the given ID exists.
func (r *UOMRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_uom WHERE uom_id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check uom existence: %w", err)
	}

	return exists, nil
}

// ListAll retrieves all non-deleted UOMs (for export).
func (r *UOMRepository) ListAll(ctx context.Context, filter uom.ExportFilter) ([]*uom.UOM, error) {
	query := `
		SELECT uom_id, uom_code, uom_name, uom_category, description,
			   is_active, created_at, created_by, updated_at, updated_by,
			   deleted_at, deleted_by
		FROM mst_uom
		WHERE deleted_at IS NULL
	`
	args := []interface{}{}
	argIndex := 1

	// Category filter
	if filter.Category != nil {
		query += fmt.Sprintf(` AND uom_category = $%d`, argIndex)
		args = append(args, filter.Category.String())
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY uom_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all uoms: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log silently as this is cleanup
			_ = err
		}
	}()

	var uoms []*uom.UOM
	for rows.Next() {
		entity, err := r.scanUOMFromRows(rows)
		if err != nil {
			return nil, err
		}
		uoms = append(uoms, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uom rows: %w", err)
	}

	return uoms, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func (r *UOMRepository) scanUOM(row *sql.Row) (*uom.UOM, error) {
	var dto uomDTO
	err := row.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.Category,
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
		return nil, uom.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan uom: %w", err)
	}

	return dto.ToEntity()
}

func (r *UOMRepository) scanUOMFromRows(rows *sql.Rows) (*uom.UOM, error) {
	var dto uomDTO
	err := rows.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.Category,
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
		return nil, fmt.Errorf("failed to scan uom row: %w", err)
	}

	return dto.ToEntity()
}

// uomDTO is a data transfer object for database operations.
type uomDTO struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Category    string
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
func (d *uomDTO) ToEntity() (*uom.UOM, error) {
	code, err := uom.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}

	category, err := uom.NewCategory(d.Category)
	if err != nil {
		return nil, fmt.Errorf("invalid category from db: %w", err)
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

	return uom.ReconstructUOM(
		d.ID,
		code,
		d.Name,
		category,
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

// isUniqueViolation checks if the error is a PostgreSQL unique violation.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	return false
}
