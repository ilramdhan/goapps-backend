package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// CostProductTypeRepository implements costproducttype.Repository using PostgreSQL.
type CostProductTypeRepository struct {
	db *DB
}

// NewCostProductTypeRepository constructs the repo.
func NewCostProductTypeRepository(db *DB) *CostProductTypeRepository {
	return &CostProductTypeRepository{db: db}
}

var _ costproducttype.Repository = (*CostProductTypeRepository)(nil)

// Create persists a new CostProductType and assigns the generated typeID.
func (r *CostProductTypeRepository) Create(ctx context.Context, t *costproducttype.CostProductType) error {
	const q = `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING cpt_type_id
	`
	var id int32
	if err := r.db.QueryRowContext(ctx, q,
		t.TypeCode(), t.TypeName(), t.IsActive(), t.CreatedAt(), t.UpdatedAt(),
	).Scan(&id); err != nil {
		if isProductTypeUniqueViolation(err) {
			return costproducttype.ErrAlreadyExists
		}
		return fmt.Errorf("create cost_product_type: %w", err)
	}
	t.SetID(id)
	return nil
}

// GetByID loads a CostProductType by id.
func (r *CostProductTypeRepository) GetByID(ctx context.Context, id int32) (*costproducttype.CostProductType, error) {
	const q = `
		SELECT cpt_type_id, cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at
		FROM cost_product_type WHERE cpt_type_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, q, id))
}

// GetByCode loads a CostProductType by its unique code.
func (r *CostProductTypeRepository) GetByCode(ctx context.Context, code string) (*costproducttype.CostProductType, error) {
	const q = `
		SELECT cpt_type_id, cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at
		FROM cost_product_type WHERE cpt_type_code = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, q, code))
}

// Update persists changes (type_code immutable).
func (r *CostProductTypeRepository) Update(ctx context.Context, t *costproducttype.CostProductType) error {
	const q = `
		UPDATE cost_product_type
		SET cpt_type_name = $2, cpt_is_active = $3, cpt_updated_at = $4
		WHERE cpt_type_id = $1
	`
	res, err := r.db.ExecContext(ctx, q, t.TypeID(), t.TypeName(), t.IsActive(), t.UpdatedAt())
	if err != nil {
		return fmt.Errorf("update cost_product_type: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costproducttype.ErrNotFound
	}
	return nil
}

// List returns a filtered paginated list.
func (r *CostProductTypeRepository) List(ctx context.Context, f costproducttype.Filter) ([]*costproducttype.CostProductType, int64, error) {
	where := "FROM cost_product_type WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cpt_type_code) LIKE LOWER($%d) OR LOWER(cpt_type_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	switch f.ActiveFilter {
	case "active":
		where += ` AND cpt_is_active = TRUE`
	case "inactive":
		where += ` AND cpt_is_active = FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_product_type: %w", err)
	}

	col := `cpt_type_code`
	switch f.SortBy {
	case "type_name":
		col = `cpt_type_name`
	case "created_at":
		col = `cpt_created_at`
	}
	dir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		dir = "DESC"
	}
	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `
		SELECT cpt_type_id, cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at
		` + where + fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", col, dir, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_product_type: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []*costproducttype.CostProductType{}
	for rows.Next() {
		t, sErr := r.scanRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_product_type: %w", err)
	}
	return items, total, nil
}

// =============================================================================
// scan helpers
// =============================================================================

func (r *CostProductTypeRepository) scanRow(row *sql.Row) (*costproducttype.CostProductType, error) {
	var (
		id                       int32
		code, name               string
		isActive                 bool
		createdAt, updatedAt     time.Time
	)
	if err := row.Scan(&id, &code, &name, &isActive, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costproducttype.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_product_type: %w", err)
	}
	return costproducttype.Reconstruct(id, code, name, isActive, createdAt, updatedAt), nil
}

func (r *CostProductTypeRepository) scanRows(rows *sql.Rows) (*costproducttype.CostProductType, error) {
	var (
		id                       int32
		code, name               string
		isActive                 bool
		createdAt, updatedAt     time.Time
	)
	if err := rows.Scan(&id, &code, &name, &isActive, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan cost_product_type row: %w", err)
	}
	return costproducttype.Reconstruct(id, code, name, isActive, createdAt, updatedAt), nil
}

func isProductTypeUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
