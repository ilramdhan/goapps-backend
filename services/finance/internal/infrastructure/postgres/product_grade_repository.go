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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// ProductGradeRepository implements productgrade.Repository using PostgreSQL.
type ProductGradeRepository struct {
	db *DB
}

// NewProductGradeRepository creates a new ProductGradeRepository instance.
func NewProductGradeRepository(db *DB) *ProductGradeRepository {
	return &ProductGradeRepository{db: db}
}

// Verify interface implementation at compile time.
var _ productgrade.Repository = (*ProductGradeRepository)(nil)

// Create persists a new Product Grade.
func (r *ProductGradeRepository) Create(ctx context.Context, entity *productgrade.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_product_grade (
			pg_id, pg_code, pg_name, pg_description,
			bc_perc, non_std_perc, bc_recovery_rate,
			pg_detail_product, pg_grade_label, std_selling_price, sp_value,
			is_active, notes, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`,
		entity.ID(),
		entity.Code(),
		entity.Name(),
		nullableString(entity.Description()),
		entity.BCPerc(),
		entity.NonStdPerc(),
		entity.BCRecoveryRate(),
		nullableString(entity.PgDetailProduct()),
		nullableString(entity.PgGradeLabel()),
		entity.StdSellingPrice(),
		entity.SpValue(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isProductGradeUniqueViolation(err) {
			return productgrade.ErrAlreadyExists
		}
		return fmt.Errorf("create product grade: %w", err)
	}
	return nil
}

// GetByID retrieves a Product Grade by its UUID primary key.
func (r *ProductGradeRepository) GetByID(ctx context.Context, id uuid.UUID) (*productgrade.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE pg_id = $1 AND deleted_at IS NULL`, id))
}

// GetByCode retrieves a Product Grade by its code.
func (r *ProductGradeRepository) GetByCode(ctx context.Context, code string) (*productgrade.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE pg_code = $1 AND deleted_at IS NULL`, code))
}

// List retrieves Product Grades with filtering and pagination.
func (r *ProductGradeRepository) List(ctx context.Context, filter productgrade.ListFilter) ([]*productgrade.Entity, int64, error) {
	filter.Validate()

	base := whereNotDeleted
	args := make([]interface{}, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (pg_code ILIKE $%d OR pg_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_product_grade "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count product grades: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	q := r.selectCols() + base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, dir, idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list product grades: %w", err)
	}
	defer closeRows(rows)

	var items []*productgrade.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate product grades: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing Product Grade.
func (r *ProductGradeRepository) Update(ctx context.Context, entity *productgrade.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_product_grade SET
			pg_name          = $2,
			pg_description   = $3,
			bc_perc          = $4,
			non_std_perc     = $5,
			bc_recovery_rate = $6,
			pg_detail_product= $7,
			pg_grade_label   = $8,
			std_selling_price= $9,
			sp_value         = $10,
			is_active        = $11,
			notes            = $12,
			updated_at       = $13,
			updated_by       = $14
		WHERE pg_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.Name(),
		nullableString(entity.Description()),
		entity.BCPerc(),
		entity.NonStdPerc(),
		entity.BCRecoveryRate(),
		nullableString(entity.PgDetailProduct()),
		nullableString(entity.PgGradeLabel()),
		entity.StdSellingPrice(),
		entity.SpValue(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update product grade: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return productgrade.ErrNotFound
	}
	return nil
}

// SoftDelete marks a Product Grade as deleted.
func (r *ProductGradeRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_product_grade SET deleted_at=$2,deleted_by=$3,is_active=false WHERE pg_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete product grade: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return productgrade.ErrNotFound
	}
	return nil
}

// ExistsByCode checks if a Product Grade with the given code exists.
func (r *ProductGradeRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_product_grade WHERE pg_code=$1 AND deleted_at IS NULL)`, code,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by code: %w", err)
	}
	return exists, nil
}

// ExistsByID checks if a Product Grade with the given UUID exists.
func (r *ProductGradeRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_product_grade WHERE pg_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *ProductGradeRepository) selectCols() string {
	return `
		SELECT pg_id, pg_code, pg_name, pg_description,
		       bc_perc, non_std_perc, bc_recovery_rate,
		       COALESCE(pg_detail_product, ''), COALESCE(pg_grade_label, ''),
		       COALESCE(std_selling_price, 0), COALESCE(sp_value, 0),
		       is_active, notes, created_at, created_by,
		       updated_at, updated_by, deleted_at, deleted_by
		FROM mst_product_grade
	`
}

func (r *ProductGradeRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"pg_code": "pg_code", "pg_name": "pg_name", "bc_perc": "bc_perc",
		"code": "pg_code", "name": "pg_name", sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "pg_code"
}

type productGradeDTO struct {
	ID              uuid.UUID
	Code            string
	Name            string
	Description     sql.NullString
	BCPerc          float64
	NonStdPerc      float64
	BCRecoveryRate  float64
	PgDetailProduct string
	PgGradeLabel    string
	StdSellingPrice float64
	SpValue         float64
	IsActive        bool
	Notes           sql.NullString
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
	DeletedAt       sql.NullTime
	DeletedBy       sql.NullString
}

func (d *productGradeDTO) toEntity() *productgrade.Entity {
	return productgrade.Reconstruct(
		d.ID, d.Code, d.Name, d.Description.String,
		d.BCPerc, d.NonStdPerc, d.BCRecoveryRate,
		d.PgDetailProduct, d.PgGradeLabel,
		d.StdSellingPrice, d.SpValue,
		d.IsActive, d.Notes.String,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *ProductGradeRepository) scanOne(row *sql.Row) (*productgrade.Entity, error) {
	var d productGradeDTO
	err := row.Scan(
		&d.ID, &d.Code, &d.Name, &d.Description,
		&d.BCPerc, &d.NonStdPerc, &d.BCRecoveryRate,
		&d.PgDetailProduct, &d.PgGradeLabel,
		&d.StdSellingPrice, &d.SpValue,
		&d.IsActive, &d.Notes, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, productgrade.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan product grade: %w", err)
	}
	return d.toEntity(), nil
}

func (r *ProductGradeRepository) scanRow(rows *sql.Rows) (*productgrade.Entity, error) {
	var d productGradeDTO
	err := rows.Scan(
		&d.ID, &d.Code, &d.Name, &d.Description,
		&d.BCPerc, &d.NonStdPerc, &d.BCRecoveryRate,
		&d.PgDetailProduct, &d.PgGradeLabel,
		&d.StdSellingPrice, &d.SpValue,
		&d.IsActive, &d.Notes, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan product grade row: %w", err)
	}
	return d.toEntity(), nil
}

func isProductGradeUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
