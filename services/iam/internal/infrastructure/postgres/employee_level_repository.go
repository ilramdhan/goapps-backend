// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// pgUniqueViolationCode is the PostgreSQL SQLSTATE for unique constraint violation.
const pgUniqueViolationCode = "23505"

// isUniqueViolation returns true when the error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}

// EmployeeLevelRepository implements employeelevel.Repository using PostgreSQL.
type EmployeeLevelRepository struct {
	db *DB
}

// NewEmployeeLevelRepository creates a new EmployeeLevelRepository.
func NewEmployeeLevelRepository(db *DB) *EmployeeLevelRepository {
	return &EmployeeLevelRepository{db: db}
}

// Verify interface implementation at compile time.
var _ employeelevel.Repository = (*EmployeeLevelRepository)(nil)

// Create inserts a new employee level.
func (r *EmployeeLevelRepository) Create(ctx context.Context, el *employeelevel.EmployeeLevel) error {
	query := `
		INSERT INTO mst_employee_level (
			employee_level_id, code, name, grade, type, sequence, workflow,
			is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		el.ID(), el.Code().String(), el.Name(), el.Grade(),
		int32(el.Type()), el.Sequence(), int32(el.Workflow()),
		el.IsActive(), el.Audit().CreatedAt, el.Audit().CreatedBy,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrAlreadyExists
		}
		return fmt.Errorf("failed to insert employee level: %w", err)
	}
	return nil
}

// GetByID retrieves an employee level by ID.
func (r *EmployeeLevelRepository) GetByID(ctx context.Context, id uuid.UUID) (*employeelevel.EmployeeLevel, error) {
	query := r.selectSQL() + ` WHERE employee_level_id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, query, id)
}

// GetByCode retrieves an employee level by code.
func (r *EmployeeLevelRepository) GetByCode(ctx context.Context, code string) (*employeelevel.EmployeeLevel, error) {
	query := r.selectSQL() + ` WHERE code = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, query, code)
}

// Update persists changes to an existing employee level.
func (r *EmployeeLevelRepository) Update(ctx context.Context, el *employeelevel.EmployeeLevel) error {
	query := `
		UPDATE mst_employee_level SET
			name = $2, grade = $3, type = $4, sequence = $5, workflow = $6,
			is_active = $7, updated_at = $8, updated_by = $9
		WHERE employee_level_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query,
		el.ID(), el.Name(), el.Grade(), int32(el.Type()), el.Sequence(), int32(el.Workflow()),
		el.IsActive(), el.Audit().UpdatedAt, el.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update employee level: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// Delete soft-deletes an employee level.
func (r *EmployeeLevelRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_employee_level SET
			is_active = false, deleted_at = $2, deleted_by = $3
		WHERE employee_level_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete employee level: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List lists employee levels with pagination, search, and filters.
func (r *EmployeeLevelRepository) List(ctx context.Context, params employeelevel.ListParams) ([]*employeelevel.EmployeeLevel, int64, error) {
	conditions, args := r.buildListWhere(params)
	whereClause := strings.Join(conditions, " AND ")

	// Count
	countQuery := "SELECT COUNT(*) FROM mst_employee_level WHERE " + whereClause
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count employee levels: %w", err)
	}

	// Resolve sort
	sortBy, sortOrder := r.resolveSort(params.SortBy, params.SortOrder)

	offset := (params.Page - 1) * params.PageSize
	argPos := len(args) + 1
	query := fmt.Sprintf(`
		%s WHERE %s
		ORDER BY %s %s, code ASC
		LIMIT $%d OFFSET $%d
	`, r.selectSQL(), whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list employee levels: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close rows in employee level list")
		}
	}()

	var results []*employeelevel.EmployeeLevel
	for rows.Next() {
		el, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		results = append(results, el)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating employee level rows: %w", err)
	}
	return results, total, nil
}

// ListAll retrieves all non-deleted employee levels for export.
func (r *EmployeeLevelRepository) ListAll(ctx context.Context, filter employeelevel.ExportFilter) ([]*employeelevel.EmployeeLevel, error) {
	conditions := []string{"deleted_at IS NULL"}
	var args []interface{}
	argPos := 1

	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *filter.IsActive)
		argPos++
	}
	if filter.Type != nil && *filter.Type != employeelevel.TypeUnspecified {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argPos))
		args = append(args, int32(*filter.Type))
		argPos++
	}
	if filter.Workflow != nil && *filter.Workflow != employeelevel.WorkflowUnspecified {
		conditions = append(conditions, fmt.Sprintf("workflow = $%d", argPos))
		args = append(args, int32(*filter.Workflow))
		argPos++ //nolint:ineffassign,wastedassign // keep counter consistent
		_ = argPos
	}

	query := fmt.Sprintf("%s WHERE %s ORDER BY sequence ASC, code ASC",
		r.selectSQL(), strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all employee levels: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close rows in employee level list all")
		}
	}()

	var results []*employeelevel.EmployeeLevel
	for rows.Next() {
		el, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, sErr
		}
		results = append(results, el)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating employee level export rows: %w", err)
	}
	return results, nil
}

// ExistsByCode returns whether a record with the given code exists.
func (r *EmployeeLevelRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_employee_level WHERE code = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check employee level existence: %w", err)
	}
	return exists, nil
}

// BatchCreate inserts multiple employee levels within a transaction.
func (r *EmployeeLevelRepository) BatchCreate(ctx context.Context, items []*employeelevel.EmployeeLevel) (int, error) {
	count := 0
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, el := range items {
			query := `
				INSERT INTO mst_employee_level (
					employee_level_id, code, name, grade, type, sequence, workflow,
					is_active, created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`
			_, err := tx.ExecContext(ctx, query,
				el.ID(), el.Code().String(), el.Name(), el.Grade(),
				int32(el.Type()), el.Sequence(), int32(el.Workflow()),
				el.IsActive(), el.Audit().CreatedAt, el.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert employee level %s: %w", el.Code().String(), err)
			}
			count++
		}
		return nil
	})
	return count, err
}

// =============================================================================
// Helpers
// =============================================================================

func (r *EmployeeLevelRepository) selectSQL() string {
	return `
		SELECT employee_level_id, code, name, grade, type, sequence, workflow,
			is_active, created_at, created_by, updated_at, updated_by,
			deleted_at, deleted_by
		FROM mst_employee_level
	`
}

func (r *EmployeeLevelRepository) buildListWhere(params employeelevel.ListParams) ([]string, []interface{}) {
	conditions := []string{"deleted_at IS NULL"}
	var args []interface{}
	argPos := 1

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}
	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *params.IsActive)
		argPos++
	}
	if params.Type != nil && *params.Type != employeelevel.TypeUnspecified {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argPos))
		args = append(args, int32(*params.Type))
		argPos++
	}
	if params.Workflow != nil && *params.Workflow != employeelevel.WorkflowUnspecified {
		conditions = append(conditions, fmt.Sprintf("workflow = $%d", argPos))
		args = append(args, int32(*params.Workflow))
		argPos++ //nolint:ineffassign,wastedassign // keep counter consistent for future additions
		_ = argPos
	}
	return conditions, args
}

func (r *EmployeeLevelRepository) resolveSort(sortBy, sortOrder string) (string, string) {
	sortColumnMap := map[string]string{
		"code":       "code",
		"name":       "name",
		"grade":      "grade",
		"sequence":   "sequence",
		"created_at": "created_at",
	}
	column := "sequence"
	if mapped, ok := sortColumnMap[sortBy]; ok {
		column = mapped
	}
	dir := sortASC
	if strings.EqualFold(sortOrder, sortDESC) {
		dir = sortDESC
	}
	return column, dir
}

func (r *EmployeeLevelRepository) queryOne(ctx context.Context, query string, args ...interface{}) (*employeelevel.EmployeeLevel, error) {
	var row employeeLevelRow
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&row.ID, &row.Code, &row.Name, &row.Grade, &row.Type, &row.Sequence, &row.Workflow,
		&row.IsActive, &row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		&row.DeletedAt, &row.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get employee level: %w", err)
	}
	return row.toDomain()
}

func (r *EmployeeLevelRepository) scanRow(rows *sql.Rows) (*employeelevel.EmployeeLevel, error) {
	var row employeeLevelRow
	if err := rows.Scan(
		&row.ID, &row.Code, &row.Name, &row.Grade, &row.Type, &row.Sequence, &row.Workflow,
		&row.IsActive, &row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		&row.DeletedAt, &row.DeletedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan employee level row: %w", err)
	}
	return row.toDomain()
}

// employeeLevelRow is the scan target for SELECT queries.
type employeeLevelRow struct {
	ID        uuid.UUID
	Code      string
	Name      string
	Grade     int32
	Type      int32
	Sequence  int32
	Workflow  int32
	IsActive  bool
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt *time.Time
	UpdatedBy *string
	DeletedAt *time.Time
	DeletedBy *string
}

func (r *employeeLevelRow) toDomain() (*employeelevel.EmployeeLevel, error) {
	code, err := employeelevel.NewCode(r.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}
	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
		DeletedAt: r.DeletedAt,
		DeletedBy: r.DeletedBy,
	}
	return employeelevel.Reconstruct(
		r.ID,
		code,
		r.Name,
		r.Grade,
		employeelevel.Type(r.Type),
		r.Sequence,
		employeelevel.Workflow(r.Workflow),
		r.IsActive,
		audit,
	), nil
}
