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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// InterminglingRepository implements intermingling.Repository using PostgreSQL.
type InterminglingRepository struct {
	db *DB
}

// NewInterminglingRepository creates a new InterminglingRepository instance.
func NewInterminglingRepository(db *DB) *InterminglingRepository {
	return &InterminglingRepository{db: db}
}

// Verify interface implementation at compile time.
var _ intermingling.Repository = (*InterminglingRepository)(nil)

// Create persists a new Intermingling record.
func (r *InterminglingRepository) Create(ctx context.Context, entity *intermingling.Entity) error {
	query := `
		INSERT INTO mst_intermingling (
			intm_id, intm_code, intm_name, intm_cost_per_kg,
			is_active, notes, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code(),
		entity.Name(),
		entity.CostPerKg(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isInterminglingUniqueViolation(err) {
			return intermingling.ErrAlreadyExists
		}
		return fmt.Errorf("create intermingling: %w", err)
	}
	return nil
}

// GetByID retrieves a record by its UUID primary key.
func (r *InterminglingRepository) GetByID(ctx context.Context, id uuid.UUID) (*intermingling.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE intm_id = $1 AND deleted_at IS NULL`, id))
}

// GetByCode retrieves a record by its unique code.
func (r *InterminglingRepository) GetByCode(ctx context.Context, code string) (*intermingling.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE intm_code = $1 AND deleted_at IS NULL`, code))
}

// List retrieves records with filtering, searching, and pagination.
func (r *InterminglingRepository) List(ctx context.Context, filter intermingling.ListFilter) ([]*intermingling.Entity, int64, error) {
	filter.Validate()

	base := `FROM mst_intermingling WHERE deleted_at IS NULL`
	args := make([]any, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (intm_code ILIKE $%d OR intm_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count intermingling: %w", err)
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
		return nil, 0, fmt.Errorf("list intermingling: %w", err)
	}
	var items []*intermingling.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, 0, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, 0, fmt.Errorf("close intermingling rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate intermingling: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing record.
func (r *InterminglingRepository) Update(ctx context.Context, entity *intermingling.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_intermingling SET
			intm_name        = $2,
			intm_cost_per_kg = $3,
			is_active        = $4,
			notes            = $5,
			updated_at       = $6,
			updated_by       = $7
		WHERE intm_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.Name(),
		entity.CostPerKg(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update intermingling: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return intermingling.ErrNotFound
	}
	return nil
}

// SoftDelete marks a record as deleted.
func (r *InterminglingRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_intermingling SET deleted_at=$2,deleted_by=$3,is_active=false WHERE intm_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete intermingling: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return intermingling.ErrNotFound
	}
	return nil
}

// ExistsByCode checks if a record with the given code exists.
func (r *InterminglingRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_intermingling WHERE intm_code=$1 AND deleted_at IS NULL)`, code,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by code: %w", err)
	}
	return exists, nil
}

// ExistsByID checks if a record with the given UUID exists.
func (r *InterminglingRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_intermingling WHERE intm_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *InterminglingRepository) selectCols() string {
	return `
		SELECT intm_id, intm_code, intm_name, intm_cost_per_kg,
		       is_active, notes, created_at, created_by,
		       updated_at, updated_by, deleted_at, deleted_by
		FROM mst_intermingling
	`
}

func (r *InterminglingRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"intm_code": "intm_code", "intm_name": "intm_name",
		"intm_cost_per_kg": "intm_cost_per_kg", "code": "intm_code", "name": "intm_name",
		sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "intm_code"
}

type interminglingDTO struct {
	ID        uuid.UUID
	Code      string
	Name      string
	CostPerKg float64
	IsActive  bool
	Notes     sql.NullString
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt sql.NullTime
	UpdatedBy sql.NullString
	DeletedAt sql.NullTime
	DeletedBy sql.NullString
}

func (d *interminglingDTO) toEntity() *intermingling.Entity {
	return intermingling.Reconstruct(
		d.ID, d.Code, d.Name, d.CostPerKg, d.IsActive, d.Notes.String,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *InterminglingRepository) scanOne(row *sql.Row) (*intermingling.Entity, error) {
	var d interminglingDTO
	err := row.Scan(
		&d.ID, &d.Code, &d.Name, &d.CostPerKg,
		&d.IsActive, &d.Notes, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, intermingling.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan intermingling: %w", err)
	}
	return d.toEntity(), nil
}

func (r *InterminglingRepository) scanRow(rows *sql.Rows) (*intermingling.Entity, error) {
	var d interminglingDTO
	err := rows.Scan(
		&d.ID, &d.Code, &d.Name, &d.CostPerKg,
		&d.IsActive, &d.Notes, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan intermingling row: %w", err)
	}
	return d.toEntity(), nil
}

func isInterminglingUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
