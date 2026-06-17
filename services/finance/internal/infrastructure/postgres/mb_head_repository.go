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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// MBHeadRepository implements mbhead.Repository using PostgreSQL.
type MBHeadRepository struct {
	db *DB
}

// NewMBHeadRepository creates a new MBHeadRepository instance.
func NewMBHeadRepository(db *DB) *MBHeadRepository {
	return &MBHeadRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbhead.Repository = (*MBHeadRepository)(nil)

// Create persists a new MB Head.
func (r *MBHeadRepository) Create(ctx context.Context, entity *mbhead.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_mb_head (
			mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name,
			mbh_denier, mbh_filament, mbh_dozing,
			mbh_is_active, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		entity.ID(),
		entity.OracleSysID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isMBHeadUniqueViolation(err) {
			return mbhead.ErrAlreadyExists
		}
		return fmt.Errorf("create mb head: %w", err)
	}
	return nil
}

// GetByID retrieves an MB Head by its UUID primary key.
func (r *MBHeadRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbhead.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbh_id = $1 AND deleted_at IS NULL`, id))
}

// GetByMBCosting retrieves an MB Head by its unique mb_costing value.
func (r *MBHeadRepository) GetByMBCosting(ctx context.Context, mbCosting string) (*mbhead.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbh_mb_costing = $1 AND deleted_at IS NULL`, mbCosting))
}

// List retrieves MB Heads with filtering and pagination.
func (r *MBHeadRepository) List(ctx context.Context, filter mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	filter.Validate()

	base := `FROM mst_mb_head WHERE deleted_at IS NULL`
	args := make([]interface{}, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (mbh_mb_costing ILIKE $%d OR mbh_mgt_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND mbh_is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mb heads: %w", err)
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
		return nil, 0, fmt.Errorf("list mb heads: %w", err)
	}
	defer closeRows(rows)

	var items []*mbhead.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mb heads: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing MB Head.
func (r *MBHeadRepository) Update(ctx context.Context, entity *mbhead.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_mb_head SET
			mbh_mb_costing = $2,
			mbh_mgt_name   = $3,
			mbh_denier     = $4,
			mbh_filament   = $5,
			mbh_dozing     = $6,
			mbh_is_active  = $7,
			updated_at     = $8,
			updated_by     = $9
		WHERE mbh_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update mb head: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

// SoftDelete marks an MB Head as deleted.
func (r *MBHeadRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_mb_head SET deleted_at=$2,deleted_by=$3,mbh_is_active=false WHERE mbh_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete mb head: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

// ExistsByMBCosting checks if an MB Head with the given mb_costing exists.
func (r *MBHeadRepository) ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_head WHERE mbh_mb_costing=$1 AND deleted_at IS NULL)`, mbCosting,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by mb_costing: %w", err)
	}
	return exists, nil
}

// ExistsByID checks if an MB Head with the given UUID exists.
func (r *MBHeadRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_head WHERE mbh_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *MBHeadRepository) selectCols() string {
	return `
		SELECT mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name,
		       mbh_denier, mbh_filament, mbh_dozing, mbh_is_active,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_mb_head
	`
}

func (r *MBHeadRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"mbh_mb_costing": "mbh_mb_costing", "mbh_mgt_name": "mbh_mgt_name",
		"mbh_denier": "mbh_denier", sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbh_mb_costing"
}

type mbHeadDTO struct {
	ID          uuid.UUID
	OracleSysID sql.NullString
	MBCosting   string
	MgtName     sql.NullString
	Denier      sql.NullFloat64
	Filament    sql.NullInt64
	Dozing      sql.NullFloat64
	IsActive    bool
	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   sql.NullTime
	UpdatedBy   sql.NullString
	DeletedAt   sql.NullTime
	DeletedBy   sql.NullString
}

func (d *mbHeadDTO) toEntity() *mbhead.Entity {
	return mbhead.Reconstruct(
		d.ID,
		nullableStringPtr(d.OracleSysID),
		d.MBCosting,
		nullableStringPtr(d.MgtName),
		nullableFloat64Ptr(d.Denier),
		nullableIntPtr(d.Filament),
		nullableFloat64Ptr(d.Dozing),
		d.IsActive,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *MBHeadRepository) scanOne(row *sql.Row) (*mbhead.Entity, error) {
	var d mbHeadDTO
	err := row.Scan(
		&d.ID, &d.OracleSysID, &d.MBCosting, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbhead.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan mb head: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBHeadRepository) scanRow(rows *sql.Rows) (*mbhead.Entity, error) {
	var d mbHeadDTO
	err := rows.Scan(
		&d.ID, &d.OracleSysID, &d.MBCosting, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb head row: %w", err)
	}
	return d.toEntity(), nil
}

func isMBHeadUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
