package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// MBCrossSectionFactorRepository implements mbcrosssection.FactorRepository using PostgreSQL.
type MBCrossSectionFactorRepository struct {
	db *DB
}

// NewMBCrossSectionFactorRepository creates a new MBCrossSectionFactorRepository instance.
func NewMBCrossSectionFactorRepository(db *DB) *MBCrossSectionFactorRepository {
	return &MBCrossSectionFactorRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbcrosssection.FactorRepository = (*MBCrossSectionFactorRepository)(nil)

// Create persists a new conversion factor row.
func (r *MBCrossSectionFactorRepository) Create(ctx context.Context, e *mbcrosssection.FactorEntity) error {
	const q = `
		INSERT INTO mst_mb_cross_section_factor
			(mbcf_from_code, mbcf_to_code, mbcf_factor, mbcf_operation,
			 mbcf_note, mbcf_is_active, mbcf_created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING mbcf_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		e.FromCode(), e.ToCode(), e.Factor(), e.Operation(),
		e.Note(), e.IsActive(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return mbcrosssection.ErrFactorAlreadyExists
		}
		return fmt.Errorf("mb_cross_section_factor_repository: create: %w", err)
	}
	return nil
}

// Update persists changes to an existing conversion factor row. The (from_code, to_code)
// pair is the row's identity and is never updated.
func (r *MBCrossSectionFactorRepository) Update(ctx context.Context, e *mbcrosssection.FactorEntity) error {
	const q = `
		UPDATE mst_mb_cross_section_factor
		SET mbcf_factor = $2, mbcf_operation = $3, mbcf_note = $4,
		    mbcf_is_active = $5, mbcf_updated_at = NOW(), mbcf_updated_by = $6
		WHERE mbcf_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, e.ID(), e.Factor(), e.Operation(),
		e.Note(), e.IsActive(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("mb_cross_section_factor_repository: update: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_cross_section_factor_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcrosssection.ErrFactorNotFound
	}
	return nil
}

// Delete soft-deletes a conversion factor row by ID.
func (r *MBCrossSectionFactorRepository) Delete(ctx context.Context, id, deletedBy string) error {
	const q = `
		UPDATE mst_mb_cross_section_factor
		SET deleted_at = NOW(), deleted_by = $2
		WHERE mbcf_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id, deletedBy)
	if err != nil {
		return fmt.Errorf("mb_cross_section_factor_repository: delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_cross_section_factor_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcrosssection.ErrFactorNotFound
	}
	return nil
}

// GetByID returns a single live conversion factor row by ID.
func (r *MBCrossSectionFactorRepository) GetByID(ctx context.Context, id string) (*mbcrosssection.FactorEntity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbcf_id = $1 AND deleted_at IS NULL`, id)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbcrosssection.ErrFactorNotFound
		}
		return nil, fmt.Errorf("mb_cross_section_factor_repository: get by id: %w", err)
	}
	return e, nil
}

// GetByPair returns the live conversion factor for an ordered (from_code, to_code) pair.
func (r *MBCrossSectionFactorRepository) GetByPair(ctx context.Context, fromCode, toCode string) (*mbcrosssection.FactorEntity, error) {
	row := r.db.QueryRowContext(ctx,
		r.selectCols()+` WHERE mbcf_from_code = $1 AND mbcf_to_code = $2 AND deleted_at IS NULL`,
		fromCode, toCode)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbcrosssection.ErrFactorNotFound
		}
		return nil, fmt.Errorf("mb_cross_section_factor_repository: get by pair: %w", err)
	}
	return e, nil
}

// List returns paginated live conversion factor rows.
func (r *MBCrossSectionFactorRepository) List(ctx context.Context, filter mbcrosssection.FactorListFilter) ([]*mbcrosssection.FactorEntity, int64, error) {
	filter.Validate()

	where := whereNotDeleted
	args := []any{}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND (mbcf_from_code ILIKE $%d OR mbcf_to_code ILIKE $%d)", len(args)+1, len(args)+1)
		args = append(args, "%"+filter.Search+"%")
	}
	if filter.FromCode != "" {
		where += fmt.Sprintf(" AND mbcf_from_code = $%d", len(args)+1)
		args = append(args, filter.FromCode)
	}
	if filter.ToCode != "" {
		where += fmt.Sprintf(" AND mbcf_to_code = $%d", len(args)+1)
		args = append(args, filter.ToCode)
	}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbcf_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM mst_mb_cross_section_factor " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_factor_repository: count: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	listQ := fmt.Sprintf("%s %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		r.selectCols(), where, orderCol, dir, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_factor_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbcrosssection.FactorEntity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mb_cross_section_factor_repository: iterate: %w", err)
	}
	return out, total, nil
}

func (r *MBCrossSectionFactorRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"from_code":      "mbcf_from_code",
		"to_code":        "mbcf_to_code",
		"factor":         "mbcf_factor",
		sortKeyCreatedAt: "mbcf_created_at",
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbcf_from_code"
}

func (r *MBCrossSectionFactorRepository) selectCols() string {
	return `
		SELECT mbcf_id, mbcf_from_code, mbcf_to_code, mbcf_factor, mbcf_operation,
		       COALESCE(mbcf_note, ''), mbcf_is_active,
		       mbcf_created_at, mbcf_created_by,
		       COALESCE(mbcf_updated_at::text, ''), COALESCE(mbcf_updated_by, ''),
		       COALESCE(deleted_at::text, ''), COALESCE(deleted_by, '')
		FROM mst_mb_cross_section_factor
	`
}

type mbCrossSectionFactorDTO struct {
	ID        string
	FromCode  string
	ToCode    string
	Factor    float64
	Operation string
	Note      string
	IsActive  bool
	CreatedAt string
	CreatedBy string
	UpdatedAt string
	UpdatedBy string
	DeletedAt string
	DeletedBy string
}

func (d *mbCrossSectionFactorDTO) toEntity() *mbcrosssection.FactorEntity {
	return mbcrosssection.ReconstructFactor(
		d.ID, d.FromCode, d.ToCode, d.Factor, d.Operation, d.Note, d.IsActive,
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy, d.DeletedAt, d.DeletedBy,
	)
}

func (r *MBCrossSectionFactorRepository) scanOne(row *sql.Row) (*mbcrosssection.FactorEntity, error) {
	var d mbCrossSectionFactorDTO
	err := row.Scan(&d.ID, &d.FromCode, &d.ToCode, &d.Factor, &d.Operation,
		&d.Note, &d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, err
	}
	return d.toEntity(), nil
}

func (r *MBCrossSectionFactorRepository) scanRow(rows *sql.Rows) (*mbcrosssection.FactorEntity, error) {
	var d mbCrossSectionFactorDTO
	err := rows.Scan(&d.ID, &d.FromCode, &d.ToCode, &d.Factor, &d.Operation,
		&d.Note, &d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, fmt.Errorf("mb_cross_section_factor_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}
