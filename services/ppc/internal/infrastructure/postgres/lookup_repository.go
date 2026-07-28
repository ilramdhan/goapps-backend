// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/lookup"
)

// LookupRepository implements lookup.Repository using PostgreSQL.
type LookupRepository struct {
	db *DB
}

// NewLookupRepository creates a new LookupRepository.
func NewLookupRepository(db *DB) *LookupRepository {
	return &LookupRepository{db: db}
}

var _ lookup.Repository = (*LookupRepository)(nil)

// Create persists a new lookup and assigns its generated ID.
func (r *LookupRepository) Create(ctx context.Context, entity *lookup.Lookup) error {
	query := `
		INSERT INTO ppc_lookup (
			pl_category, pl_code, pl_label, pl_sort_order,
			pl_is_active, pl_created_at, pl_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING pl_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Category(),
		entity.Code(),
		entity.Label(),
		entity.SortOrder(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return lookup.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create lookup: %w", err)
	}

	*entity = *lookup.Reconstruct(
		id, entity.Category(), entity.Code(), entity.Label(), entity.SortOrder(),
		entity.IsActive(), entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a lookup by its ID.
func (r *LookupRepository) GetByID(ctx context.Context, id int64) (*lookup.Lookup, error) {
	query := `
		SELECT pl_id, pl_category, pl_code, pl_label, pl_sort_order,
			pl_is_active, pl_created_at, pl_created_by, pl_updated_at, pl_updated_by
		FROM ppc_lookup
		WHERE pl_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves lookups with filtering and pagination.
func (r *LookupRepository) List(ctx context.Context, filter lookup.ListFilter) ([]*lookup.Lookup, int64, error) {
	filter.Validate()

	base := `FROM ppc_lookup WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Category != "" {
		base += fmt.Sprintf(` AND pl_category = $%d`, argIdx)
		args = append(args, filter.Category)
		argIdx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (pl_code ILIKE $%d OR pl_label ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND pl_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count lookups: %w", err)
	}

	query := `SELECT pl_id, pl_category, pl_code, pl_label, pl_sort_order, ` +
		`pl_is_active, pl_created_at, pl_created_by, pl_updated_at, pl_updated_by ` +
		base + fmt.Sprintf(` ORDER BY pl_category ASC, pl_sort_order ASC, pl_code ASC LIMIT $%d OFFSET $%d`,
		argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lookups: %w", err)
	}
	defer closeRows(rows)

	var result []*lookup.Lookup
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating lookup rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing lookup.
func (r *LookupRepository) Update(ctx context.Context, entity *lookup.Lookup) error {
	query := `
		UPDATE ppc_lookup
		SET pl_label = $2, pl_sort_order = $3, pl_is_active = $4,
			pl_updated_at = $5, pl_updated_by = $6
		WHERE pl_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Label(),
		entity.SortOrder(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update lookup: %w", err)
	}
	return checkAffected(res, lookup.ErrNotFound)
}

// Delete removes a lookup by its ID.
func (r *LookupRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM ppc_lookup WHERE pl_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete lookup: %w", err)
	}
	return checkAffected(res, lookup.ErrNotFound)
}

func (r *LookupRepository) scanRow(row *sql.Row) (*lookup.Lookup, error) {
	var dto lookupDTO
	err := row.Scan(
		&dto.ID, &dto.Category, &dto.Code, &dto.Label, &dto.SortOrder,
		&dto.IsActive, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, lookup.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan lookup: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *LookupRepository) scanRows(rows *sql.Rows) (*lookup.Lookup, error) {
	var dto lookupDTO
	if err := rows.Scan(
		&dto.ID, &dto.Category, &dto.Code, &dto.Label, &dto.SortOrder,
		&dto.IsActive, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan lookup row: %w", err)
	}
	return dto.toEntity(), nil
}

type lookupDTO struct {
	ID        int64
	Category  string
	Code      string
	Label     string
	SortOrder int32
	IsActive  bool
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt sql.NullTime
	UpdatedBy sql.NullString
}

func (d *lookupDTO) toEntity() *lookup.Lookup {
	return lookup.Reconstruct(
		d.ID, d.Category, d.Code, d.Label, d.SortOrder,
		d.IsActive, d.CreatedAt, d.CreatedBy, nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	)
}
