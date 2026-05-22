package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costrmtype"
)

// CostRmTypeRepository implements costrmtype.Repository.
type CostRmTypeRepository struct{ db *DB }

// NewCostRmTypeRepository constructs the repository.
func NewCostRmTypeRepository(db *DB) *CostRmTypeRepository {
	return &CostRmTypeRepository{db: db}
}

var _ costrmtype.Repository = (*CostRmTypeRepository)(nil)

// Create persists a new RM type and assigns the generated id.
func (r *CostRmTypeRepository) Create(ctx context.Context, t *costrmtype.CostRmType) error {
	const q = `
		INSERT INTO cost_rm_type (
			crmt_type_code,crmt_type_name,crmt_reference_target,crmt_allow_sub_sequence,
			crmt_is_active,crmt_created_at,crmt_updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING crmt_type_id`
	var id int32
	if err := r.db.QueryRowContext(ctx, q,
		t.TypeCode(), t.TypeName(), t.ReferenceTarget(), t.AllowSubSequence(),
		t.IsActive(), t.CreatedAt(), t.UpdatedAt(),
	).Scan(&id); err != nil {
		if isRmTypeUniqueViolation(err) {
			return costrmtype.ErrAlreadyExists
		}
		return fmt.Errorf("create cost_rm_type: %w", err)
	}
	t.SetID(id)
	return nil
}

// GetByID loads an RM type by id.
func (r *CostRmTypeRepository) GetByID(ctx context.Context, id int32) (*costrmtype.CostRmType, error) {
	const q = `
		SELECT crmt_type_id,crmt_type_code,crmt_type_name,crmt_reference_target,
		       crmt_allow_sub_sequence,crmt_is_active,crmt_created_at,crmt_updated_at
		FROM cost_rm_type WHERE crmt_type_id=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, id))
}

// GetByCode loads by code.
func (r *CostRmTypeRepository) GetByCode(ctx context.Context, code string) (*costrmtype.CostRmType, error) {
	const q = `
		SELECT crmt_type_id,crmt_type_code,crmt_type_name,crmt_reference_target,
		       crmt_allow_sub_sequence,crmt_is_active,crmt_created_at,crmt_updated_at
		FROM cost_rm_type WHERE crmt_type_code=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, code))
}

// Update mutates the name + active flag (other fields immutable).
func (r *CostRmTypeRepository) Update(ctx context.Context, t *costrmtype.CostRmType) error {
	const q = `
		UPDATE cost_rm_type
		SET crmt_type_name=$2, crmt_is_active=$3, crmt_updated_at=$4
		WHERE crmt_type_id=$1`
	res, err := r.db.ExecContext(ctx, q, t.TypeID(), t.TypeName(), t.IsActive(), t.UpdatedAt())
	if err != nil {
		return fmt.Errorf("update cost_rm_type: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costrmtype.ErrNotFound
	}
	return nil
}

// List returns a filtered paginated list.
func (r *CostRmTypeRepository) List(ctx context.Context, f costrmtype.Filter) ([]*costrmtype.CostRmType, int64, error) {
	where := "FROM cost_rm_type WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(crmt_type_code) LIKE LOWER($%d) OR LOWER(crmt_type_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.ReferenceTarget != "" {
		where += fmt.Sprintf(` AND crmt_reference_target=$%d`, idx)
		args = append(args, f.ReferenceTarget)
		idx++
	}
	switch f.ActiveFilter {
	case "active":
		where += ` AND crmt_is_active=TRUE`
	case "inactive":
		where += ` AND crmt_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_rm_type: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `
		SELECT crmt_type_id,crmt_type_code,crmt_type_name,crmt_reference_target,
		       crmt_allow_sub_sequence,crmt_is_active,crmt_created_at,crmt_updated_at
		` + where + fmt.Sprintf(` ORDER BY crmt_type_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_rm_type: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []*costrmtype.CostRmType{}
	for rows.Next() {
		t, sErr := r.scanRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_rm_type: %w", err)
	}
	return items, total, nil
}

// =============================================================================
// scan helpers
// =============================================================================

func (r *CostRmTypeRepository) scanRow(row *sql.Row) (*costrmtype.CostRmType, error) {
	var (
		id                                int32
		code, name, target                string
		allowSub, isActive                bool
		createdAt, updatedAt              time.Time
	)
	if err := row.Scan(&id, &code, &name, &target, &allowSub, &isActive, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costrmtype.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_rm_type: %w", err)
	}
	return costrmtype.Reconstruct(id, code, name, target, allowSub, isActive, createdAt, updatedAt), nil
}

func (r *CostRmTypeRepository) scanRows(rows *sql.Rows) (*costrmtype.CostRmType, error) {
	var (
		id                                int32
		code, name, target                string
		allowSub, isActive                bool
		createdAt, updatedAt              time.Time
	)
	if err := rows.Scan(&id, &code, &name, &target, &allowSub, &isActive, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan cost_rm_type row: %w", err)
	}
	return costrmtype.Reconstruct(id, code, name, target, allowSub, isActive, createdAt, updatedAt), nil
}

func isRmTypeUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
