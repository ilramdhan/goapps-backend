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
			uom_id, uom_code, uom_name, uom_category_id, description,
			is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code().String(),
		entity.Name(),
		entity.CategoryID(),
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
		SELECT u.uom_id, u.uom_code, u.uom_name, u.uom_category_id,
			   c.category_code, c.category_name,
			   u.description, u.is_active, u.created_at, u.created_by,
			   u.updated_at, u.updated_by, u.deleted_at, u.deleted_by
		FROM mst_uom u
		JOIN mst_uom_category c ON u.uom_category_id = c.uom_category_id
		WHERE u.uom_id = $1 AND u.deleted_at IS NULL
	`

	return r.scanUOM(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a UOM by its code.
func (r *UOMRepository) GetByCode(ctx context.Context, code uom.Code) (*uom.UOM, error) {
	query := `
		SELECT u.uom_id, u.uom_code, u.uom_name, u.uom_category_id,
			   c.category_code, c.category_name,
			   u.description, u.is_active, u.created_at, u.created_by,
			   u.updated_at, u.updated_by, u.deleted_at, u.deleted_by
		FROM mst_uom u
		JOIN mst_uom_category c ON u.uom_category_id = c.uom_category_id
		WHERE u.uom_code = $1 AND u.deleted_at IS NULL
	`

	return r.scanUOM(r.db.QueryRowContext(ctx, query, code.String()))
}

// List retrieves UOMs with filtering, searching, and pagination.
func (r *UOMRepository) List(ctx context.Context, filter uom.ListFilter) ([]*uom.UOM, int64, error) {
	filter.Validate()

	// Build dynamic query with JOIN
	baseQuery := `FROM mst_uom u JOIN mst_uom_category c ON u.uom_category_id = c.uom_category_id WHERE u.deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// Search filter
	if filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (
			u.uom_code ILIKE $%d OR
			u.uom_name ILIKE $%d OR
			u.description ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// Category filter
	if filter.CategoryID != nil {
		baseQuery += fmt.Sprintf(` AND u.uom_category_id = $%d`, argIndex)
		args = append(args, *filter.CategoryID)
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		baseQuery += fmt.Sprintf(` AND u.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
		argIndex++
	}

	// Count total
	var total int64
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count uoms: %w", err)
	}

	// Build order clause with sort column mapping
	sortColumnMap := map[string]string{
		"code":           "u.uom_code",
		"name":           "u.uom_name",
		"category":       "c.category_code",
		sortKeyCreatedAt: "u.created_at",
	}
	orderColumn := "u.uom_code"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderColumn = mapped
	}
	orderDir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		orderDir = sortDESC
	}

	// Data query with pagination
	selectQuery := `
		SELECT u.uom_id, u.uom_code, u.uom_name, u.uom_category_id,
			   c.category_code, c.category_name,
			   u.description, u.is_active, u.created_at, u.created_by,
			   u.updated_at, u.updated_by, u.deleted_at, u.deleted_by
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
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
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
			uom_category_id = $3,
			description = $4,
			is_active = $5,
			updated_at = $6,
			updated_by = $7
		WHERE uom_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.CategoryID(),
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
		SELECT u.uom_id, u.uom_code, u.uom_name, u.uom_category_id,
			   c.category_code, c.category_name,
			   u.description, u.is_active, u.created_at, u.created_by,
			   u.updated_at, u.updated_by, u.deleted_at, u.deleted_by
		FROM mst_uom u
		JOIN mst_uom_category c ON u.uom_category_id = c.uom_category_id
		WHERE u.deleted_at IS NULL
	`
	args := []interface{}{}
	argIndex := 1

	// Category filter
	if filter.CategoryID != nil {
		query += fmt.Sprintf(` AND u.uom_category_id = $%d`, argIndex)
		args = append(args, *filter.CategoryID)
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND u.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY u.uom_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all uoms: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
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
		&dto.CategoryID,
		&dto.CategoryCode,
		&dto.CategoryName,
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
		&dto.CategoryID,
		&dto.CategoryCode,
		&dto.CategoryName,
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
	ID           uuid.UUID
	Code         string
	Name         string
	CategoryID   uuid.UUID
	CategoryCode string
	CategoryName string
	Description  sql.NullString
	IsActive     bool
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    sql.NullTime
	UpdatedBy    sql.NullString
	DeletedAt    sql.NullTime
	DeletedBy    sql.NullString
}

// ToEntity converts DTO to domain entity.
func (d *uomDTO) ToEntity() (*uom.UOM, error) {
	code, err := uom.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}

	categoryInfo := uom.NewCategoryInfo(d.CategoryID, d.CategoryCode, d.CategoryName)

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
		categoryInfo,
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

// isUniqueViolation checks if the error is a PostgreSQL unique violation (code 23505).
// Handles both lib/pq and pgx/v5/stdlib error types.
func isUniqueViolation(err error) bool {
	// lib/pq driver error type.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	// pgx/v5/stdlib driver error type (pgconn.PgError).
	// Use interface matching to avoid importing pgconn directly.
	type pgError interface {
		SQLState() string
	}
	var pgErr pgError
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
