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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// FormulaRepository implements formula.Repository interface using PostgreSQL.
type FormulaRepository struct {
	db *DB
}

// NewFormulaRepository creates a new FormulaRepository instance.
func NewFormulaRepository(db *DB) *FormulaRepository {
	return &FormulaRepository{db: db}
}

// Verify interface implementation at compile time.
var _ formula.Repository = (*FormulaRepository)(nil)

// Create persists a new Formula with its input parameters using a transaction.
func (r *FormulaRepository) Create(ctx context.Context, entity *formula.Formula) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
			}
		}
	}()

	// Insert formula
	formulaQuery := `
		INSERT INTO mst_formula (
			id, formula_code, formula_name, formula_type, expression,
			result_param_id, description, version, is_active,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = tx.ExecContext(ctx, formulaQuery,
		entity.ID(),
		entity.Code().String(),
		entity.Name(),
		entity.FormulaType().String(),
		entity.Expression(),
		entity.ResultParamID(),
		entity.Description(),
		entity.Version(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isFormulaUniqueViolation(err) {
			return formula.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create formula: %w", err)
	}

	// Insert input params
	if err = r.insertFormulaParams(ctx, tx, entity.ID(), entity.InputParams()); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// insertFormulaParams inserts formula_param rows within a transaction.
func (r *FormulaRepository) insertFormulaParams(ctx context.Context, tx *sql.Tx, formulaID uuid.UUID, params []*formula.Param) error {
	if len(params) == 0 {
		return nil
	}

	paramQuery := `
		INSERT INTO formula_param (id, formula_id, param_id, sort_order)
		VALUES ($1, $2, $3, $4)
	`
	for _, p := range params {
		_, err := tx.ExecContext(ctx, paramQuery, p.ID(), formulaID, p.ParamID(), p.SortOrder())
		if err != nil {
			return fmt.Errorf("failed to insert formula param: %w", err)
		}
	}

	return nil
}

// GetByID retrieves a Formula by its ID (with joins).
func (r *FormulaRepository) GetByID(ctx context.Context, id uuid.UUID) (*formula.Formula, error) {
	query := formulaSelectQuery() + ` WHERE f.id = $1 AND f.deleted_at IS NULL`
	entity, err := r.scanFormula(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}

	// Load input params
	params, err := r.loadFormulaParams(ctx, id)
	if err != nil {
		return nil, err
	}

	return r.reconstructWithParams(entity, params), nil
}

// GetByCode retrieves a Formula by its code (with joins).
func (r *FormulaRepository) GetByCode(ctx context.Context, code formula.Code) (*formula.Formula, error) {
	query := formulaSelectQuery() + ` WHERE f.formula_code = $1 AND f.deleted_at IS NULL`
	entity, err := r.scanFormula(r.db.QueryRowContext(ctx, query, code.String()))
	if err != nil {
		return nil, err
	}

	// Load input params
	params, err := r.loadFormulaParams(ctx, entity.ID())
	if err != nil {
		return nil, err
	}

	return r.reconstructWithParams(entity, params), nil
}

// List retrieves Formulas with filtering, searching, and pagination.
func (r *FormulaRepository) List(ctx context.Context, filter formula.ListFilter) ([]*formula.Formula, int64, error) {
	filter.Validate()

	baseQuery := `FROM mst_formula f
		LEFT JOIN mst_parameter rp ON f.result_param_id = rp.id AND rp.deleted_at IS NULL
		WHERE f.deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	// Search filter
	if filter.Search != "" {
		baseQuery += fmt.Sprintf(` AND (
			f.formula_code ILIKE $%d OR
			f.formula_name ILIKE $%d OR
			f.expression ILIKE $%d
		)`, argIndex, argIndex, argIndex)
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// FormulaType filter
	if filter.FormulaType != nil {
		baseQuery += fmt.Sprintf(` AND f.formula_type = $%d`, argIndex)
		args = append(args, filter.FormulaType.String())
		argIndex++
	}

	// IsActive filter
	if filter.IsActive != nil {
		baseQuery += fmt.Sprintf(` AND f.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
		argIndex++
	}

	// Count total
	var total int64
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count formulas: %w", err)
	}

	// Build order clause
	sortColumnMap := map[string]string{
		"code":           "f.formula_code",
		"name":           "f.formula_name",
		"type":           "f.formula_type",
		"version":        "f.version",
		sortKeyCreatedAt: "f.created_at",
	}
	orderColumn := "f.formula_code"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderColumn = mapped
	}
	orderDir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		orderDir = sortDESC
	}

	selectQuery := `
		SELECT f.id, f.formula_code, f.formula_name, f.formula_type, f.expression,
			   f.result_param_id,
			   COALESCE(rp.param_code, '') AS result_param_code,
			   COALESCE(rp.param_name, '') AS result_param_name,
			   f.description, f.version, f.is_active,
			   f.created_at, f.created_by,
			   f.updated_at, f.updated_by, f.deleted_at, f.deleted_by
	` + baseQuery + fmt.Sprintf(
		` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderColumn, orderDir, argIndex, argIndex+1,
	)

	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list formulas: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var formulas []*formula.Formula
	for rows.Next() {
		entity, scanErr := r.scanFormulaFromRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		formulas = append(formulas, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating formula rows: %w", err)
	}

	return formulas, total, nil
}

// Update persists changes to an existing Formula and replaces input params.
func (r *FormulaRepository) Update(ctx context.Context, entity *formula.Formula) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
			}
		}
	}()

	query := `
		UPDATE mst_formula SET
			formula_name = $2,
			formula_type = $3,
			expression = $4,
			result_param_id = $5,
			description = $6,
			version = $7,
			is_active = $8,
			updated_at = $9,
			updated_by = $10
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.FormulaType().String(),
		entity.Expression(),
		entity.ResultParamID(),
		entity.Description(),
		entity.Version(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update formula: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return formula.ErrNotFound
	}

	// Replace input params: delete old, insert new
	_, err = tx.ExecContext(ctx, `DELETE FROM formula_param WHERE formula_id = $1`, entity.ID())
	if err != nil {
		return fmt.Errorf("failed to delete old formula params: %w", err)
	}

	if err = r.insertFormulaParams(ctx, tx, entity.ID(), entity.InputParams()); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SoftDelete marks a Formula as deleted.
func (r *FormulaRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_formula SET
			deleted_at = $2,
			deleted_by = $3,
			is_active = false
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to soft delete formula: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return formula.ErrNotFound
	}

	return nil
}

// ExistsByCode checks if a Formula with the given code exists.
func (r *FormulaRepository) ExistsByCode(ctx context.Context, code formula.Code) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_formula WHERE formula_code = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, code.String()).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check formula existence: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if a Formula with the given ID exists.
func (r *FormulaRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_formula WHERE id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check formula existence: %w", err)
	}

	return exists, nil
}

// ListAll retrieves all non-deleted Formulas (for export).
func (r *FormulaRepository) ListAll(ctx context.Context, filter formula.ExportFilter) ([]*formula.Formula, error) {
	query := formulaSelectQuery() + ` WHERE f.deleted_at IS NULL`
	args := []interface{}{}
	argIndex := 1

	if filter.FormulaType != nil {
		query += fmt.Sprintf(` AND f.formula_type = $%d`, argIndex)
		args = append(args, filter.FormulaType.String())
		argIndex++
	}

	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND f.is_active = $%d`, argIndex)
		args = append(args, *filter.IsActive)
	}

	query += ` ORDER BY f.formula_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all formulas: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var formulas []*formula.Formula
	for rows.Next() {
		entity, scanErr := r.scanFormulaFromRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		formulas = append(formulas, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating formula rows: %w", err)
	}

	return formulas, nil
}

// ResultParamUsedByOther checks if a result_param_id is used by another formula.
func (r *FormulaRepository) ResultParamUsedByOther(ctx context.Context, resultParamID uuid.UUID, excludeID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(
		SELECT 1 FROM mst_formula
		WHERE result_param_id = $1 AND id != $2 AND deleted_at IS NULL
	)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, resultParamID, excludeID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check result param usage: %w", err)
	}

	return exists, nil
}

// ResolveParamCode resolves a parameter code to its UUID.
func (r *FormulaRepository) ResolveParamCode(ctx context.Context, paramCode string) (*uuid.UUID, error) {
	query := `SELECT id FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL`

	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, query, paramCode).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, formula.ErrInputParamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve param code: %w", err)
	}

	return &id, nil
}

// ParamExistsByID checks if a parameter with the given ID exists.
func (r *FormulaRepository) ParamExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_parameter WHERE id = $1 AND deleted_at IS NULL)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check param existence: %w", err)
	}

	return exists, nil
}

// =============================================================================
// Private helpers
// =============================================================================

func formulaSelectQuery() string {
	return `
		SELECT f.id, f.formula_code, f.formula_name, f.formula_type, f.expression,
			   f.result_param_id,
			   COALESCE(rp.param_code, '') AS result_param_code,
			   COALESCE(rp.param_name, '') AS result_param_name,
			   f.description, f.version, f.is_active,
			   f.created_at, f.created_by,
			   f.updated_at, f.updated_by, f.deleted_at, f.deleted_by
		FROM mst_formula f
		LEFT JOIN mst_parameter rp ON f.result_param_id = rp.id AND rp.deleted_at IS NULL
	`
}

func (r *FormulaRepository) loadFormulaParams(ctx context.Context, formulaID uuid.UUID) ([]*formula.Param, error) {
	query := `
		SELECT fp.id, fp.param_id,
			   COALESCE(p.param_code, '') AS param_code,
			   COALESCE(p.param_name, '') AS param_name,
			   fp.sort_order
		FROM formula_param fp
		LEFT JOIN mst_parameter p ON fp.param_id = p.id AND p.deleted_at IS NULL
		WHERE fp.formula_id = $1
		ORDER BY fp.sort_order ASC
	`

	rows, err := r.db.QueryContext(ctx, query, formulaID)
	if err != nil {
		return nil, fmt.Errorf("failed to load formula params: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var params []*formula.Param
	for rows.Next() {
		var id, paramID uuid.UUID
		var paramCode, paramName string
		var sortOrder int

		if err := rows.Scan(&id, &paramID, &paramCode, &paramName, &sortOrder); err != nil {
			return nil, fmt.Errorf("failed to scan formula param: %w", err)
		}

		params = append(params, formula.ReconstructParam(id, paramID, paramCode, paramName, sortOrder))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating formula param rows: %w", err)
	}

	return params, nil
}

// formulaDTO is a data transfer object for formula database operations.
type formulaDTO struct {
	ID              uuid.UUID
	Code            string
	Name            string
	FormulaType     string
	Expression      string
	ResultParamID   uuid.UUID
	ResultParamCode string
	ResultParamName string
	Description     string
	Version         int
	IsActive        bool
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
	DeletedAt       sql.NullTime
	DeletedBy       sql.NullString
}

func (d *formulaDTO) toEntity(inputParams []*formula.Param) (*formula.Formula, error) {
	code, err := formula.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid formula code from DB: %w", err)
	}
	ft, err := formula.NewType(d.FormulaType)
	if err != nil {
		return nil, fmt.Errorf("invalid formula type from DB: %w", err)
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

	return formula.ReconstructFormula(
		d.ID, code, d.Name, ft, d.Expression,
		d.ResultParamID, d.ResultParamCode, d.ResultParamName,
		d.Description, d.Version, d.IsActive,
		inputParams,
		d.CreatedAt, d.CreatedBy,
		updatedAt, updatedBy, deletedAt, deletedBy,
	), nil
}

func (r *FormulaRepository) scanFormula(row *sql.Row) (*formula.Formula, error) {
	var dto formulaDTO
	err := row.Scan(
		&dto.ID, &dto.Code, &dto.Name, &dto.FormulaType, &dto.Expression,
		&dto.ResultParamID, &dto.ResultParamCode, &dto.ResultParamName,
		&dto.Description, &dto.Version, &dto.IsActive,
		&dto.CreatedAt, &dto.CreatedBy,
		&dto.UpdatedAt, &dto.UpdatedBy, &dto.DeletedAt, &dto.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, formula.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan formula: %w", err)
	}

	return dto.toEntity(nil)
}

func (r *FormulaRepository) scanFormulaFromRows(rows *sql.Rows) (*formula.Formula, error) {
	var dto formulaDTO
	err := rows.Scan(
		&dto.ID, &dto.Code, &dto.Name, &dto.FormulaType, &dto.Expression,
		&dto.ResultParamID, &dto.ResultParamCode, &dto.ResultParamName,
		&dto.Description, &dto.Version, &dto.IsActive,
		&dto.CreatedAt, &dto.CreatedBy,
		&dto.UpdatedAt, &dto.UpdatedBy, &dto.DeletedAt, &dto.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan formula row: %w", err)
	}

	return dto.toEntity(nil)
}

func (r *FormulaRepository) reconstructWithParams(entity *formula.Formula, params []*formula.Param) *formula.Formula {
	// Code and Type are already validated from the original entity — safe to reuse directly.
	return formula.ReconstructFormula(
		entity.ID(), entity.Code(), entity.Name(), entity.FormulaType(), entity.Expression(),
		entity.ResultParamID(), entity.ResultParamCode(), entity.ResultParamName(),
		entity.Description(), entity.Version(), entity.IsActive(),
		params,
		entity.CreatedAt(), entity.CreatedBy(),
		entity.UpdatedAt(), entity.UpdatedBy(), entity.DeletedAt(), entity.DeletedBy(),
	)
}

func isFormulaUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
