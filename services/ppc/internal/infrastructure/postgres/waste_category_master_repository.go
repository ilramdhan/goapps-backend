// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/wastecategory"
)

// WasteCategoryRepository implements wastecategory.Repository using PostgreSQL.
type WasteCategoryRepository struct {
	db *DB
}

// NewWasteCategoryRepository creates a new WasteCategoryRepository.
func NewWasteCategoryRepository(db *DB) *WasteCategoryRepository {
	return &WasteCategoryRepository{db: db}
}

var _ wastecategory.Repository = (*WasteCategoryRepository)(nil)

// Create persists a new waste category and assigns its generated ID.
func (r *WasteCategoryRepository) Create(ctx context.Context, entity *wastecategory.Category) error {
	query := `
		INSERT INTO waste_category_master (
			wcm_area, wcm_type, wcm_code, wcm_name, wcm_grade_target,
			wcm_is_active, wcm_sort_order, wcm_created_at, wcm_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING wcm_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Area().String(),
		entity.Type(),
		entity.Code(),
		entity.Name(),
		nullableString(entity.GradeTarget()),
		entity.IsActive(),
		entity.SortOrder(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return wastecategory.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create waste category: %w", err)
	}

	*entity = *wastecategory.Reconstruct(
		id, entity.Area(), entity.Type(), entity.Code(), entity.Name(), entity.GradeTarget(),
		entity.IsActive(), entity.SortOrder(), entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a waste category by its ID.
func (r *WasteCategoryRepository) GetByID(ctx context.Context, id int64) (*wastecategory.Category, error) {
	query := `
		SELECT wcm_id, wcm_area, wcm_type, wcm_code, wcm_name, wcm_grade_target,
			wcm_is_active, wcm_sort_order, wcm_created_at, wcm_created_by, wcm_updated_at, wcm_updated_by
		FROM waste_category_master
		WHERE wcm_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves waste categories with filtering and pagination.
func (r *WasteCategoryRepository) List(ctx context.Context, filter wastecategory.ListFilter) ([]*wastecategory.Category, int64, error) {
	filter.Validate()

	base := `FROM waste_category_master WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (wcm_code ILIKE $%d OR wcm_name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.Area != "" {
		base += fmt.Sprintf(` AND wcm_area = $%d`, argIdx)
		args = append(args, filter.Area)
		argIdx++
	}
	if filter.Type != "" {
		base += fmt.Sprintf(` AND wcm_type = $%d`, argIdx)
		args = append(args, filter.Type)
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND wcm_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count waste categories: %w", err)
	}

	sortColumnMap := map[string]string{
		"sort_order": "wcm_sort_order",
		"code":       "wcm_code",
		"name":       "wcm_name",
		"area":       "wcm_area",
		"type":       "wcm_type",
		"created_at": "wcm_created_at",
	}
	orderCol := "wcm_sort_order"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT wcm_id, wcm_area, wcm_type, wcm_code, wcm_name, wcm_grade_target, ` +
		`wcm_is_active, wcm_sort_order, wcm_created_at, wcm_created_by, wcm_updated_at, wcm_updated_by ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list waste categories: %w", err)
	}
	defer closeRows(rows)

	var result []*wastecategory.Category
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating waste category rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing waste category.
func (r *WasteCategoryRepository) Update(ctx context.Context, entity *wastecategory.Category) error {
	query := `
		UPDATE waste_category_master
		SET wcm_name = $2, wcm_grade_target = $3, wcm_is_active = $4, wcm_sort_order = $5,
			wcm_updated_at = $6, wcm_updated_by = $7
		WHERE wcm_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		nullableString(entity.GradeTarget()),
		entity.IsActive(),
		entity.SortOrder(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return wastecategory.ErrAlreadyExists
		}
		return fmt.Errorf("failed to update waste category: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return wastecategory.ErrNotFound
	}
	return nil
}

// Delete removes a waste category by its ID.
func (r *WasteCategoryRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM waste_category_master WHERE wcm_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete waste category: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return wastecategory.ErrNotFound
	}
	return nil
}

func (r *WasteCategoryRepository) scanRow(row *sql.Row) (*wastecategory.Category, error) {
	var dto wasteCategoryDTO
	err := row.Scan(
		&dto.ID, &dto.Area, &dto.Type, &dto.Code, &dto.Name, &dto.GradeTarget,
		&dto.IsActive, &dto.SortOrder, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, wastecategory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan waste category: %w", err)
	}
	return dto.toEntity()
}

func (r *WasteCategoryRepository) scanRows(rows *sql.Rows) (*wastecategory.Category, error) {
	var dto wasteCategoryDTO
	if err := rows.Scan(
		&dto.ID, &dto.Area, &dto.Type, &dto.Code, &dto.Name, &dto.GradeTarget,
		&dto.IsActive, &dto.SortOrder, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan waste category row: %w", err)
	}
	return dto.toEntity()
}

type wasteCategoryDTO struct {
	ID          int64
	Area        string
	Type        string
	Code        string
	Name        string
	GradeTarget sql.NullString
	IsActive    bool
	SortOrder   int32
	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   sql.NullTime
	UpdatedBy   sql.NullString
}

func (d *wasteCategoryDTO) toEntity() (*wastecategory.Category, error) {
	a, err := area.New(d.Area)
	if err != nil {
		return nil, fmt.Errorf("invalid area from db: %w", err)
	}
	return wastecategory.Reconstruct(
		d.ID, a, d.Type, d.Code, d.Name, nullStringPtr(d.GradeTarget),
		d.IsActive, d.SortOrder, d.CreatedAt, d.CreatedBy, nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	), nil
}

// nullableString converts an optional string pointer to a sql.NullString for insertion.
func nullableString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}
