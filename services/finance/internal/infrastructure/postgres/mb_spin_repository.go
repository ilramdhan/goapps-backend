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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBSpinRepository implements mbspin.Repository using PostgreSQL.
type MBSpinRepository struct {
	db *DB
}

// NewMBSpinRepository creates a new MBSpinRepository instance.
func NewMBSpinRepository(db *DB) *MBSpinRepository {
	return &MBSpinRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbspin.Repository = (*MBSpinRepository)(nil)

// Create persists a new MB Spin.
func (r *MBSpinRepository) Create(ctx context.Context, entity *mbspin.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_oracle_sys_id, mbs_orion_item_code, mbs_mbh_id, mbs_mgt_name,
			mbs_denier, mbs_filament, mbs_dozing, mbs_mb_costing,
			mbs_cc, mbs_cost_rate_mkt,
			mbs_is_active, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`,
		entity.ID(),
		entity.OracleSysID(),
		entity.OrionItemCode(),
		entity.HeadID(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.MBCosting(),
		entity.CC(),
		entity.CostRateMkt(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isMBSpinUniqueViolation(err) {
			return mbspin.ErrAlreadyExists
		}
		return fmt.Errorf("create mb spin: %w", err)
	}
	return nil
}

// GetByID retrieves an MB Spin by its UUID primary key.
func (r *MBSpinRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbspin.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_id = $1 AND deleted_at IS NULL`, id))
}

// List retrieves MB Spins with filtering and pagination.
func (r *MBSpinRepository) List(ctx context.Context, filter mbspin.ListFilter) ([]*mbspin.Entity, int64, error) {
	filter.Validate()

	base := whereNotDeleted
	args := make([]interface{}, 0)
	idx := 1

	if filter.HeadID != uuid.Nil {
		base += fmt.Sprintf(` AND mbs_mbh_id = $%d`, idx)
		args = append(args, filter.HeadID)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (mbs_mgt_name ILIKE $%d OR mbs_mb_costing ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND mbs_is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_mb_spin "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mb spins: %w", err)
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
		return nil, 0, fmt.Errorf("list mb spins: %w", err)
	}
	defer closeRows(rows)

	var items []*mbspin.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mb spins: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing MB Spin.
func (r *MBSpinRepository) Update(ctx context.Context, entity *mbspin.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_mb_spin SET
			mbs_mgt_name      = $2,
			mbs_denier        = $3,
			mbs_filament      = $4,
			mbs_dozing        = $5,
			mbs_mb_costing    = $6,
			mbs_cc            = $7,
			mbs_cost_rate_mkt = $8,
			mbs_is_active     = $9,
			updated_at        = $10,
			updated_by        = $11
		WHERE mbs_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.MBCosting(),
		entity.CC(),
		entity.CostRateMkt(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update mb spin: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbspin.ErrNotFound
	}
	return nil
}

// SoftDelete marks an MB Spin as deleted.
func (r *MBSpinRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_mb_spin SET deleted_at=$2,deleted_by=$3,mbs_is_active=false WHERE mbs_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete mb spin: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbspin.ErrNotFound
	}
	return nil
}

// ExistsByID checks if an MB Spin with the given UUID exists.
func (r *MBSpinRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_spin WHERE mbs_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// GetByMBCosting retrieves an MB Spin by its MB costing code.
func (r *MBSpinRepository) GetByMBCosting(ctx context.Context, code string) (*mbspin.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_mb_costing = $1 AND deleted_at IS NULL`, code))
}

// GetByOrionItemCode retrieves the first active MB Spin with the given ORION item code.
func (r *MBSpinRepository) GetByOrionItemCode(ctx context.Context, code string) (*mbspin.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL ORDER BY created_at ASC LIMIT 1`, code))
}

// =============================================================================
// Helpers
// =============================================================================

func (r *MBSpinRepository) selectCols() string {
	return `
		SELECT mbs_id, mbs_oracle_sys_id, mbs_orion_item_code, mbs_mbh_id, mbs_mgt_name,
		       mbs_denier, mbs_filament, mbs_dozing, mbs_mb_costing,
		       mbs_cc, mbs_cost_rate_mkt, mbs_is_active,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_mb_spin
	`
}

func (r *MBSpinRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"mbs_mgt_name": "mbs_mgt_name", "mbs_denier": "mbs_denier",
		sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbs_mgt_name"
}

type mbSpinDTO struct {
	ID             uuid.UUID
	OracleSysID    sql.NullString
	OrionItemCode  sql.NullString
	HeadID         uuid.UUID
	MgtName        string
	Denier         sql.NullFloat64
	Filament       sql.NullInt64
	Dozing         sql.NullFloat64
	MBCosting      sql.NullString
	CC             sql.NullString
	CostRateMkt    sql.NullFloat64
	IsActive       bool
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      sql.NullTime
	UpdatedBy      sql.NullString
	DeletedAt      sql.NullTime
	DeletedBy      sql.NullString
}

func (d *mbSpinDTO) toEntity() *mbspin.Entity {
	return mbspin.Reconstruct(
		d.ID,
		nullableStringPtr(d.OracleSysID),
		nullableStringPtr(d.OrionItemCode),
		d.HeadID,
		d.MgtName,
		nullableFloat64Ptr(d.Denier),
		nullableIntPtr(d.Filament),
		nullableFloat64Ptr(d.Dozing),
		nullableStringPtr(d.MBCosting),
		nullableStringPtr(d.CC),
		nullableFloat64Ptr(d.CostRateMkt),
		d.IsActive,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *MBSpinRepository) scanOne(row *sql.Row) (*mbspin.Entity, error) {
	var d mbSpinDTO
	err := row.Scan(
		&d.ID, &d.OracleSysID, &d.OrionItemCode, &d.HeadID, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.MBCosting,
		&d.CC, &d.CostRateMkt, &d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbspin.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan mb spin: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBSpinRepository) scanRow(rows *sql.Rows) (*mbspin.Entity, error) {
	var d mbSpinDTO
	err := rows.Scan(
		&d.ID, &d.OracleSysID, &d.OrionItemCode, &d.HeadID, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.MBCosting,
		&d.CC, &d.CostRateMkt, &d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb spin row: %w", err)
	}
	return d.toEntity(), nil
}

func isMBSpinUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
