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
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// EmployeeGroupRepository implements employeegroup.Repository using PostgreSQL.
type EmployeeGroupRepository struct {
	db *DB
}

// NewEmployeeGroupRepository creates a new EmployeeGroupRepository.
func NewEmployeeGroupRepository(db *DB) *EmployeeGroupRepository {
	return &EmployeeGroupRepository{db: db}
}

// Verify interface implementation at compile time.
var _ employeegroup.Repository = (*EmployeeGroupRepository)(nil)

// Create inserts a new employee group.
func (r *EmployeeGroupRepository) Create(ctx context.Context, eg *employeegroup.EmployeeGroup) error {
	query := `
		INSERT INTO mst_employee_group (
			employee_group_id, code, name, is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		eg.ID(), eg.Code().String(), eg.Name(),
		eg.IsActive(), eg.Audit().CreatedAt, eg.Audit().CreatedBy,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrAlreadyExists
		}
		return fmt.Errorf("failed to insert employee group: %w", err)
	}
	return nil
}

// GetByID retrieves an employee group by ID.
func (r *EmployeeGroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*employeegroup.EmployeeGroup, error) {
	query := r.selectSQL() + ` WHERE employee_group_id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, query, id)
}

// GetByCode retrieves an employee group by code.
func (r *EmployeeGroupRepository) GetByCode(ctx context.Context, code string) (*employeegroup.EmployeeGroup, error) {
	query := r.selectSQL() + ` WHERE code = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, query, code)
}

// Update persists changes to an existing employee group.
func (r *EmployeeGroupRepository) Update(ctx context.Context, eg *employeegroup.EmployeeGroup) error {
	query := `
		UPDATE mst_employee_group SET
			name = $2, is_active = $3, updated_at = $4, updated_by = $5
		WHERE employee_group_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query,
		eg.ID(), eg.Name(), eg.IsActive(), eg.Audit().UpdatedAt, eg.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update employee group: %w", err)
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

// Delete soft-deletes an employee group.
func (r *EmployeeGroupRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_employee_group SET
			is_active = false, deleted_at = $2, deleted_by = $3
		WHERE employee_group_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete employee group: %w", err)
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

// List lists employee groups with pagination, search, and filters.
func (r *EmployeeGroupRepository) List(ctx context.Context, params employeegroup.ListParams) ([]*employeegroup.EmployeeGroup, int64, error) {
	conditions, args := r.buildListWhere(params)
	whereClause := strings.Join(conditions, " AND ")

	// Count
	countQuery := "SELECT COUNT(*) FROM mst_employee_group WHERE " + whereClause
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count employee groups: %w", err)
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
		return nil, 0, fmt.Errorf("failed to list employee groups: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close rows in employee group list")
		}
	}()

	var results []*employeegroup.EmployeeGroup
	for rows.Next() {
		eg, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		results = append(results, eg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating employee group rows: %w", err)
	}
	return results, total, nil
}

// ListAll retrieves all non-deleted employee groups for export.
func (r *EmployeeGroupRepository) ListAll(ctx context.Context, filter employeegroup.ExportFilter) ([]*employeegroup.EmployeeGroup, error) {
	conditions := []string{"deleted_at IS NULL"}
	var args []interface{}
	argPos := 1

	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *filter.IsActive)
		argPos++ //nolint:ineffassign,wastedassign // keep counter consistent for future additions
		_ = argPos
	}

	query := fmt.Sprintf("%s WHERE %s ORDER BY code ASC",
		r.selectSQL(), strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list all employee groups: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close rows in employee group list all")
		}
	}()

	var results []*employeegroup.EmployeeGroup
	for rows.Next() {
		eg, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, sErr
		}
		results = append(results, eg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating employee group export rows: %w", err)
	}
	return results, nil
}

// ExistsByCode returns whether a record with the given code exists.
func (r *EmployeeGroupRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_employee_group WHERE code = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check employee group existence: %w", err)
	}
	return exists, nil
}

// BatchCreate inserts multiple employee groups within a transaction.
func (r *EmployeeGroupRepository) BatchCreate(ctx context.Context, items []*employeegroup.EmployeeGroup) (int, error) {
	count := 0
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, eg := range items {
			query := `
				INSERT INTO mst_employee_group (
					employee_group_id, code, name, is_active, created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6)
			`
			_, err := tx.ExecContext(ctx, query,
				eg.ID(), eg.Code().String(), eg.Name(),
				eg.IsActive(), eg.Audit().CreatedAt, eg.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert employee group %s: %w", eg.Code().String(), err)
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

func (r *EmployeeGroupRepository) selectSQL() string {
	return `
		SELECT employee_group_id, code, name,
			is_active, created_at, created_by, updated_at, updated_by,
			deleted_at, deleted_by
		FROM mst_employee_group
	`
}

func (r *EmployeeGroupRepository) buildListWhere(params employeegroup.ListParams) ([]string, []interface{}) {
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
		argPos++ //nolint:ineffassign,wastedassign // keep counter consistent for future additions
		_ = argPos
	}
	return conditions, args
}

func (r *EmployeeGroupRepository) resolveSort(sortBy, sortOrder string) (string, string) {
	sortColumnMap := map[string]string{
		"code":       "code",
		"name":       "name",
		"created_at": "created_at",
	}
	column := "code"
	if mapped, ok := sortColumnMap[sortBy]; ok {
		column = mapped
	}
	dir := sortASC
	if strings.EqualFold(sortOrder, sortDESC) {
		dir = sortDESC
	}
	return column, dir
}

func (r *EmployeeGroupRepository) queryOne(ctx context.Context, query string, args ...interface{}) (*employeegroup.EmployeeGroup, error) {
	var row employeeGroupRow
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&row.ID, &row.Code, &row.Name,
		&row.IsActive, &row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		&row.DeletedAt, &row.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get employee group: %w", err)
	}
	return row.toDomain()
}

func (r *EmployeeGroupRepository) scanRow(rows *sql.Rows) (*employeegroup.EmployeeGroup, error) {
	var row employeeGroupRow
	if err := rows.Scan(
		&row.ID, &row.Code, &row.Name,
		&row.IsActive, &row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
		&row.DeletedAt, &row.DeletedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan employee group row: %w", err)
	}
	return row.toDomain()
}

// employeeGroupRow is the scan target for SELECT queries.
type employeeGroupRow struct {
	ID        uuid.UUID
	Code      string
	Name      string
	IsActive  bool
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt *time.Time
	UpdatedBy *string
	DeletedAt *time.Time
	DeletedBy *string
}

func (r *employeeGroupRow) toDomain() (*employeegroup.EmployeeGroup, error) {
	code, err := employeegroup.NewCode(r.Code)
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
	return employeegroup.Reconstruct(
		r.ID,
		code,
		r.Name,
		r.IsActive,
		audit,
	), nil
}
