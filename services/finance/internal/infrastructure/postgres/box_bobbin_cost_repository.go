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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// BoxBobbinCostRepository implements boxbobbincost.Repository using PostgreSQL.
type BoxBobbinCostRepository struct {
	db *DB
}

// NewBoxBobbinCostRepository creates a new BoxBobbinCostRepository.
func NewBoxBobbinCostRepository(db *DB) *BoxBobbinCostRepository {
	return &BoxBobbinCostRepository{db: db}
}

// Verify interface implementation at compile time.
var _ boxbobbincost.Repository = (*BoxBobbinCostRepository)(nil)

// Create persists a new Entity.
func (r *BoxBobbinCostRepository) Create(ctx context.Context, entity *boxbobbincost.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_box_bobbin_cost (
			bbc_id, bbc_code, bbc_name, bbc_type, no_of_bob,
			is_active, notes,
			bbn_reuse, box_reuse, box_cost, bobin_cost, box_cost_val, bobin_cost_val,
			bbn_reuse_val, box_reuse_val,
			created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`,
		entity.ID(),
		entity.Code(),
		entity.Name(),
		entity.BBCType(),
		entity.NoOfBob(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.BbnReuse(),
		entity.BoxReuse(),
		entity.BoxCost(),
		entity.BobinCost(),
		entity.BoxCostVal(),
		entity.BobinCostVal(),
		entity.BbnReuseVal(),
		entity.BoxReuseVal(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isBoxBobbinCostUniqueViolation(err) {
			return boxbobbincost.ErrAlreadyExists
		}
		return fmt.Errorf("create box bobbin cost: %w", err)
	}
	return nil
}

// GetByID retrieves an Entity by UUID primary key (includes latest rates).
func (r *BoxBobbinCostRepository) GetByID(ctx context.Context, id uuid.UUID) (*boxbobbincost.Entity, error) {
	entity, err := r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE bbc_id = $1 AND deleted_at IS NULL`, id))
	if err != nil {
		return nil, err
	}
	rates, err := r.ListRates(ctx, entity.ID())
	if err != nil {
		return nil, fmt.Errorf("load rates: %w", err)
	}
	return boxbobbincost.Reconstruct(
		entity.ID(), entity.Code(), entity.Name(), entity.BBCType(), entity.NoOfBob(),
		entity.IsActive(), rates, entity.Notes(),
		entity.BbnReuse(), entity.BoxReuse(), entity.BoxCost(), entity.BobinCost(), entity.BoxCostVal(), entity.BobinCostVal(),
		entity.BbnReuseVal(), entity.BoxReuseVal(),
		entity.CreatedAt(), entity.CreatedBy(),
		entity.UpdatedAt(), entity.UpdatedBy(),
		entity.DeletedAt(), entity.DeletedBy(),
	), nil
}

// GetByCode retrieves an Entity by its unique code.
func (r *BoxBobbinCostRepository) GetByCode(ctx context.Context, code string) (*boxbobbincost.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE bbc_code = $1 AND deleted_at IS NULL`, code))
}

// List retrieves entities with filtering and pagination.
func (r *BoxBobbinCostRepository) List(ctx context.Context, filter boxbobbincost.ListFilter) ([]*boxbobbincost.Entity, int64, error) {
	filter.Validate()

	base := whereNotDeleted
	args := make([]interface{}, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (bbc_code ILIKE $%d OR bbc_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.BBCType != "" {
		base += fmt.Sprintf(` AND bbc_type = $%d`, idx)
		args = append(args, filter.BBCType)
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_box_bobbin_cost "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count box bobbin costs: %w", err)
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
		return nil, 0, fmt.Errorf("list box bobbin costs: %w", err)
	}
	defer closeRows(rows)

	var items []*boxbobbincost.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate box bobbin costs: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing Entity.
func (r *BoxBobbinCostRepository) Update(ctx context.Context, entity *boxbobbincost.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_box_bobbin_cost SET
			bbc_name      = $2,
			bbc_type      = $3,
			no_of_bob     = $4,
			is_active     = $5,
			notes         = $6,
			bbn_reuse     = $7,
			box_reuse     = $8,
			box_cost      = $9,
			bobin_cost    = $10,
			box_cost_val  = $11,
			bobin_cost_val= $12,
			bbn_reuse_val = $13,
			box_reuse_val = $14,
			updated_at    = $15,
			updated_by    = $16
		WHERE bbc_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.Name(),
		entity.BBCType(),
		entity.NoOfBob(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.BbnReuse(),
		entity.BoxReuse(),
		entity.BoxCost(),
		entity.BobinCost(),
		entity.BoxCostVal(),
		entity.BobinCostVal(),
		entity.BbnReuseVal(),
		entity.BoxReuseVal(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update box bobbin cost: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return boxbobbincost.ErrNotFound
	}
	return nil
}

// Delete soft-deletes an Entity by UUID.
func (r *BoxBobbinCostRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_box_bobbin_cost SET deleted_at=$2,deleted_by=$3,is_active=false WHERE bbc_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete box bobbin cost: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return boxbobbincost.ErrNotFound
	}
	return nil
}

// ListRates retrieves all active rate entries for a parent entity.
func (r *BoxBobbinCostRepository) ListRates(ctx context.Context, parentID uuid.UUID) ([]*boxbobbincost.RateEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT bbcr_id, bbcr_bbc_id, bbcr_period,
		       bbcr_bob_rate_mkt, bbcr_box_rate_mkt, bbcr_bob_rate_val, bbcr_box_rate_val,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_box_bobbin_cost_rate
		WHERE bbcr_bbc_id = $1 AND deleted_at IS NULL
		ORDER BY bbcr_period DESC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list rates: %w", err)
	}
	defer closeRows(rows)

	var rates []*boxbobbincost.RateEntry
	for rows.Next() {
		r2, scanErr := r.scanRateRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rates = append(rates, r2)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rates: %w", err)
	}
	return rates, nil
}

// CreateRate persists a new rate entry.
func (r *BoxBobbinCostRepository) CreateRate(ctx context.Context, rate *boxbobbincost.RateEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_box_bobbin_cost_rate (
			bbcr_id, bbcr_bbc_id, bbcr_period,
			bbcr_bob_rate_mkt, bbcr_box_rate_mkt, bbcr_bob_rate_val, bbcr_box_rate_val,
			created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		rate.ID(),
		rate.ParentID(),
		rate.Period(),
		rate.BobRateMkt(),
		rate.BoxRateMkt(),
		rate.BobRateVal(),
		rate.BoxRateVal(),
		rate.CreatedAt(),
		rate.CreatedBy(),
	)
	if err != nil {
		if isBoxBobbinCostUniqueViolation(err) {
			return boxbobbincost.ErrDuplicatePeriod
		}
		return fmt.Errorf("create rate: %w", err)
	}
	return nil
}

// DeleteRate soft-deletes a single rate entry by UUID.
func (r *BoxBobbinCostRepository) DeleteRate(ctx context.Context, rateID uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_box_bobbin_cost_rate SET deleted_at=$2,deleted_by=$3 WHERE bbcr_id=$1 AND deleted_at IS NULL`,
		rateID, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("delete rate: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return boxbobbincost.ErrNotFound
	}
	return nil
}

// ExistsByCode checks if a non-deleted entity with the given code exists.
func (r *BoxBobbinCostRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_box_bobbin_cost WHERE bbc_code=$1 AND deleted_at IS NULL)`, code,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by code: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *BoxBobbinCostRepository) selectCols() string {
	return `
		SELECT bbc_id, bbc_code, bbc_name, bbc_type, no_of_bob,
		       is_active, notes,
		       bbn_reuse, box_reuse, box_cost, bobin_cost, box_cost_val, bobin_cost_val,
		       bbn_reuse_val, box_reuse_val,
		       created_at, created_by,
		       updated_at, updated_by, deleted_at, deleted_by
		FROM mst_box_bobbin_cost
	`
}

func (r *BoxBobbinCostRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"bbc_code": "bbc_code", "bbc_name": "bbc_name", "bbc_type": "bbc_type",
		"code": "bbc_code", "name": "bbc_name", sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "bbc_code"
}

type boxBobbinCostDTO struct {
	ID           uuid.UUID
	Code         string
	Name         string
	BBCType      string
	NoOfBob      int
	IsActive     bool
	Notes        sql.NullString
	BbnReuse     sql.NullFloat64
	BoxReuse     sql.NullFloat64
	BoxCost      sql.NullFloat64
	BobinCost    sql.NullFloat64
	BoxCostVal   sql.NullFloat64
	BobinCostVal sql.NullFloat64
	BbnReuseVal  sql.NullFloat64
	BoxReuseVal  sql.NullFloat64
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    sql.NullTime
	UpdatedBy    sql.NullString
	DeletedAt    sql.NullTime
	DeletedBy    sql.NullString
}

func (d *boxBobbinCostDTO) toEntity() *boxbobbincost.Entity {
	return boxbobbincost.Reconstruct(
		d.ID, d.Code, d.Name, d.BBCType, d.NoOfBob, d.IsActive, nil, d.Notes.String,
		nullableFloat64Ptr(d.BbnReuse), nullableFloat64Ptr(d.BoxReuse),
		nullableFloat64Ptr(d.BoxCost), nullableFloat64Ptr(d.BobinCost),
		nullableFloat64Ptr(d.BoxCostVal), nullableFloat64Ptr(d.BobinCostVal),
		nullableFloat64Ptr(d.BbnReuseVal), nullableFloat64Ptr(d.BoxReuseVal),
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *BoxBobbinCostRepository) scanOne(row *sql.Row) (*boxbobbincost.Entity, error) {
	var d boxBobbinCostDTO
	err := row.Scan(
		&d.ID, &d.Code, &d.Name, &d.BBCType, &d.NoOfBob,
		&d.IsActive, &d.Notes,
		&d.BbnReuse, &d.BoxReuse, &d.BoxCost, &d.BobinCost, &d.BoxCostVal, &d.BobinCostVal,
		&d.BbnReuseVal, &d.BoxReuseVal,
		&d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, boxbobbincost.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan box bobbin cost: %w", err)
	}
	return d.toEntity(), nil
}

func (r *BoxBobbinCostRepository) scanRow(rows *sql.Rows) (*boxbobbincost.Entity, error) {
	var d boxBobbinCostDTO
	err := rows.Scan(
		&d.ID, &d.Code, &d.Name, &d.BBCType, &d.NoOfBob,
		&d.IsActive, &d.Notes,
		&d.BbnReuse, &d.BoxReuse, &d.BoxCost, &d.BobinCost, &d.BoxCostVal, &d.BobinCostVal,
		&d.BbnReuseVal, &d.BoxReuseVal,
		&d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan box bobbin cost row: %w", err)
	}
	return d.toEntity(), nil
}

func (r *BoxBobbinCostRepository) scanRateRow(rows *sql.Rows) (*boxbobbincost.RateEntry, error) {
	var (
		id, parentID           uuid.UUID
		period                 string
		bobRateMkt, boxRateMkt float64
		bobRateVal, boxRateVal sql.NullFloat64
		createdAt              time.Time
		createdBy              string
		updatedAt              sql.NullTime
		updatedBy              sql.NullString
		deletedAt              sql.NullTime
		deletedBy              sql.NullString
	)
	err := rows.Scan(
		&id, &parentID, &period,
		&bobRateMkt, &boxRateMkt, &bobRateVal, &boxRateVal,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan rate row: %w", err)
	}
	return boxbobbincost.ReconstructRateEntry(
		id, parentID, period, bobRateMkt, boxRateMkt,
		nullableFloat64Ptr(bobRateVal), nullableFloat64Ptr(boxRateVal),
		createdAt, createdBy,
		nullableTimePtr(updatedAt), nullableStringPtr(updatedBy),
		nullableTimePtr(deletedAt), nullableStringPtr(deletedBy),
	), nil
}

func isBoxBobbinCostUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
