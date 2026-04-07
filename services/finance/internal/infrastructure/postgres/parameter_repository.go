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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// ParameterRepository implements parameter.Repository interface using PostgreSQL.
type ParameterRepository struct {
	db *DB
}

// NewParameterRepository creates a new ParameterRepository instance.
func NewParameterRepository(db *DB) *ParameterRepository {
	return &ParameterRepository{db: db}
}

// Verify interface implementation at compile time.
var _ parameter.Repository = (*ParameterRepository)(nil)

// Create persists a new Parameter to the database.
func (r *ParameterRepository) Create(ctx context.Context, entity *parameter.Parameter) error {
	query := `
		INSERT INTO mst_parameter (
			id, param_code, param_name, param_short_name,
			data_type, param_category, uom_id,
			default_value, min_value, max_value,
			is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code().String(),
		entity.Name(),
		entity.ShortName(),
		entity.DataType().String(),
		entity.ParamCategory().String(),
		entity.UOMID(),
		entity.DefaultValue(),
		entity.MinValue(),
		entity.MaxValue(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)

	if err != nil {
		if isParameterUniqueViolation(err) {
			return parameter.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create parameter: %w", err)
	}

	return nil
}

// selectWithUOMJoin returns the base SELECT query that joins with mst_uom.
func selectWithUOMJoin() string {
	return `
		SELECT p.id, p.param_code, p.param_name, p.param_short_name,
			   p.data_type, p.param_category, p.uom_id,
			   COALESCE(u.uom_code, '') AS uom_code,
			   COALESCE(u.uom_name, '') AS uom_name,
			   p.default_value, p.min_value, p.max_value,
			   p.is_active, p.created_at, p.created_by,
			   p.updated_at, p.updated_by, p.deleted_at, p.deleted_by
		FROM mst_parameter p
		LEFT JOIN mst_uom u ON p.uom_id = u.uom_id AND u.deleted_at IS NULL
	`
}

// GetByID retrieves a Parameter by its ID (with UOM join).
func (r *ParameterRepository) GetByID(ctx context.Context, id uuid.UUID) (*parameter.Parameter, error) {
	query := selectWithUOMJoin() + ` WHERE p.id = $1 AND p.deleted_at IS NULL`
	return r.scanParameter(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a Parameter by its code (with UOM join).
func (r *ParameterRepository) GetByCode(ctx context.Context, code parameter.Code) (*parameter.Parameter, error) {
	query := selectWithUOMJoin() + ` WHERE p.param_code = $1 AND p.deleted_at IS NULL`
	return r.scanParameter(r.db.QueryRowContext(ctx, query, code.String()))
}

// List retrieves Parameters with filtering, searching, and pagination.
func (r *ParameterRepository) List(ctx context.Context, filter parameter.ListFilter) ([]*parameter.Parameter, int64, error) {
	filter.Validate()

	// Build dynamic query
	baseQuery := `FROM mst_parameter p
		LEFT JOIN mst_uom u ON p.uom_id = u.uom_id AND u.deleted_at IS NULL
		WHERE p.deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// Search filter
	if filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (
			p.param_code ILIKE $%d OR
			p.param_name ILIKE $%d OR
			p.param_short_name ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// DataType filter
	if filter.DataType != nil {
		baseQuery += fmt.Sprintf(` AND p.data_type = $%d`, argIndex)
		args = append(args, filter.DataType.String())
		argIndex++
	}

	// ParamCategory filter
	if filter.ParamCategory != nil {
		baseQuery += fmt.Sprintf(` AND p.param_category = $%d`, argIndex)
		args = append(args, filter.ParamCategory.String())
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		baseQuery += fmt.Sprintf(` AND p.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
		argIndex++
	}

	// Count total
	var total int64
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count parameters: %w", err)
	}

	// Build order clause with sort column mapping
	sortColumnMap := map[string]string{
		"code":       "p.param_code",
		"name":       "p.param_name",
		"category":   "p.param_category",
		"data_type":  "p.data_type",
		"created_at": "p.created_at",
	}
	orderColumn := "p.param_code"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderColumn = mapped
	}
	orderDir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		orderDir = sortDESC
	}

	// Data query with pagination
	selectQuery := `
		SELECT p.id, p.param_code, p.param_name, p.param_short_name,
			   p.data_type, p.param_category, p.uom_id,
			   COALESCE(u.uom_code, '') AS uom_code,
			   COALESCE(u.uom_name, '') AS uom_name,
			   p.default_value, p.min_value, p.max_value,
			   p.is_active, p.created_at, p.created_by,
			   p.updated_at, p.updated_by, p.deleted_at, p.deleted_by
	` + baseQuery + fmt.Sprintf(
		` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderColumn, orderDir, argIndex, argIndex+1,
	)

	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list parameters: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var params []*parameter.Parameter
	for rows.Next() {
		entity, err := r.scanParameterFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		params = append(params, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating parameter rows: %w", err)
	}

	return params, total, nil
}

// Update persists changes to an existing Parameter.
func (r *ParameterRepository) Update(ctx context.Context, entity *parameter.Parameter) error {
	query := `
		UPDATE mst_parameter SET
			param_name = $2,
			param_short_name = $3,
			data_type = $4,
			param_category = $5,
			uom_id = $6,
			default_value = $7,
			min_value = $8,
			max_value = $9,
			is_active = $10,
			updated_at = $11,
			updated_by = $12
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.ShortName(),
		entity.DataType().String(),
		entity.ParamCategory().String(),
		entity.UOMID(),
		entity.DefaultValue(),
		entity.MinValue(),
		entity.MaxValue(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update parameter: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return parameter.ErrNotFound
	}

	return nil
}

// SoftDelete marks a Parameter as deleted.
func (r *ParameterRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_parameter SET
			deleted_at = $2,
			deleted_by = $3,
			is_active = false
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to soft delete parameter: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return parameter.ErrNotFound
	}

	return nil
}

// ExistsByCode checks if a Parameter with the given code exists.
func (r *ParameterRepository) ExistsByCode(ctx context.Context, code parameter.Code) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code.String()).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check parameter existence: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if a Parameter with the given ID exists.
func (r *ParameterRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_parameter WHERE id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check parameter existence: %w", err)
	}

	return exists, nil
}

// ListAll retrieves all non-deleted Parameters (for export).
func (r *ParameterRepository) ListAll(ctx context.Context, filter parameter.ExportFilter) ([]*parameter.Parameter, error) {
	query := selectWithUOMJoin() + ` WHERE p.deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// DataType filter
	if filter.DataType != nil {
		query += fmt.Sprintf(` AND p.data_type = $%d`, argIndex)
		args = append(args, filter.DataType.String())
		argIndex++
	}

	// ParamCategory filter
	if filter.ParamCategory != nil {
		query += fmt.Sprintf(` AND p.param_category = $%d`, argIndex)
		args = append(args, filter.ParamCategory.String())
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND p.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY p.param_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all parameters: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var params []*parameter.Parameter
	for rows.Next() {
		entity, err := r.scanParameterFromRows(rows)
		if err != nil {
			return nil, err
		}
		params = append(params, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating parameter rows: %w", err)
	}

	return params, nil
}

// ResolveUOMCode resolves a UOM code to its UUID. Returns ErrUOMNotFound if not found.
func (r *ParameterRepository) ResolveUOMCode(ctx context.Context, uomCode string) (*uuid.UUID, error) {
	query := `SELECT uom_id FROM mst_uom WHERE uom_code = $1 AND deleted_at IS NULL`

	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, query, uomCode).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, parameter.ErrUOMNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve uom code: %w", err)
	}

	return &id, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func (r *ParameterRepository) scanParameter(row *sql.Row) (*parameter.Parameter, error) {
	var dto parameterDTO
	err := row.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.ShortName,
		&dto.DataType,
		&dto.ParamCategory,
		&dto.UOMID,
		&dto.UOMCode,
		&dto.UOMName,
		&dto.DefaultValue,
		&dto.MinValue,
		&dto.MaxValue,
		&dto.IsActive,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, parameter.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan parameter: %w", err)
	}

	return dto.ToEntity()
}

func (r *ParameterRepository) scanParameterFromRows(rows *sql.Rows) (*parameter.Parameter, error) {
	var dto parameterDTO
	err := rows.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.ShortName,
		&dto.DataType,
		&dto.ParamCategory,
		&dto.UOMID,
		&dto.UOMCode,
		&dto.UOMName,
		&dto.DefaultValue,
		&dto.MinValue,
		&dto.MaxValue,
		&dto.IsActive,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan parameter row: %w", err)
	}

	return dto.ToEntity()
}

// parameterDTO is a data transfer object for database operations.
type parameterDTO struct {
	ID            uuid.UUID
	Code          string
	Name          string
	ShortName     string
	DataType      string
	ParamCategory string
	UOMID         *uuid.UUID
	UOMCode       string
	UOMName       string
	DefaultValue  sql.NullString
	MinValue      sql.NullString
	MaxValue      sql.NullString
	IsActive      bool
	CreatedAt     time.Time
	CreatedBy     string
	UpdatedAt     sql.NullTime
	UpdatedBy     sql.NullString
	DeletedAt     sql.NullTime
	DeletedBy     sql.NullString
}

// ToEntity converts DTO to domain entity.
func (d *parameterDTO) ToEntity() (*parameter.Parameter, error) {
	code, err := parameter.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}

	dataType, err := parameter.NewDataType(d.DataType)
	if err != nil {
		return nil, fmt.Errorf("invalid data_type from db: %w", err)
	}

	paramCategory, err := parameter.NewParamCategory(d.ParamCategory)
	if err != nil {
		return nil, fmt.Errorf("invalid param_category from db: %w", err)
	}

	var defaultValue, minValue, maxValue *string
	if d.DefaultValue.Valid {
		v := trimDecimalZeros(d.DefaultValue.String)
		defaultValue = &v
	}
	if d.MinValue.Valid {
		v := trimDecimalZeros(d.MinValue.String)
		minValue = &v
	}
	if d.MaxValue.Valid {
		v := trimDecimalZeros(d.MaxValue.String)
		maxValue = &v
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

	return parameter.ReconstructParameter(
		d.ID,
		code,
		d.Name,
		d.ShortName,
		dataType,
		paramCategory,
		d.UOMID,
		d.UOMCode,
		d.UOMName,
		defaultValue,
		minValue,
		maxValue,
		d.IsActive,
		d.CreatedAt,
		d.CreatedBy,
		updatedAt,
		updatedBy,
		deletedAt,
		deletedBy,
	), nil
}

// isParameterUniqueViolation checks if the error is a PostgreSQL unique violation.
func isParameterUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	return false
}

// trimDecimalZeros trims trailing zeros from a decimal string.
// "100.500000" → "100.5", "0.000000" → "0", "999.000000" → "999".
func trimDecimalZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
