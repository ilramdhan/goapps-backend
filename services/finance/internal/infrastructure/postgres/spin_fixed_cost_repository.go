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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

const spinFixedCostTable = "mst_spin_fixed_cost"

// SpinFixedCostRepository implements spinfixedcost.Repository using PostgreSQL.
type SpinFixedCostRepository struct {
	db *DB
}

// NewSpinFixedCostRepository creates a new SpinFixedCostRepository instance.
func NewSpinFixedCostRepository(db *DB) *SpinFixedCostRepository {
	return &SpinFixedCostRepository{db: db}
}

// Verify interface implementation at compile time.
var _ spinfixedcost.Repository = (*SpinFixedCostRepository)(nil)

// Create persists a new Spin Fixed Cost record.
func (r *SpinFixedCostRepository) Create(ctx context.Context, entity *spinfixedcost.Entity) error {
	query := `
		INSERT INTO mst_spin_fixed_cost (
			msfc_id, msfc_period, msfc_common_poy_denier, msfc_poy_production,
			msfc_spin_power_month, msfc_spin_manpower_month,
			msfc_spin_overheads_month, msfc_spin_conssprs_month,
			msfc_is_active, msfc_created_at, msfc_created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Period(),
		entity.CommonPoyDenier(),
		entity.PoyProduction(),
		entity.SpinPowerMonth(),
		entity.SpinManpowerMonth(),
		entity.SpinOverheadsMonth(),
		entity.SpinConssprsMonth(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		// Backstop for the uq_msfc_period_live partial unique index when a concurrent
		// insert slips past the application-layer ExistsByPeriod pre-check.
		if isUniqueViolation(err) {
			return spinfixedcost.ErrDuplicatePeriod
		}
		return fmt.Errorf("create spin fixed cost: %w", err)
	}
	return nil
}

// GetByID retrieves a record by its UUID primary key.
func (r *SpinFixedCostRepository) GetByID(ctx context.Context, id uuid.UUID) (*spinfixedcost.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE msfc_id = $1 AND deleted_at IS NULL`, id))
}

// GetByPeriod retrieves the live record for a YYYYMM period.
func (r *SpinFixedCostRepository) GetByPeriod(ctx context.Context, period string) (*spinfixedcost.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE msfc_period = $1 AND deleted_at IS NULL`, period))
}

// List retrieves records with filtering, searching, and pagination.
func (r *SpinFixedCostRepository) List(ctx context.Context, filter spinfixedcost.ListFilter) ([]*spinfixedcost.Entity, int64, error) {
	filter.Validate()

	base, args := r.buildListWhere(filter)
	idx := len(args) + 1

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+spinFixedCostTable+" "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count spin fixed cost: %w", err)
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
		return nil, 0, fmt.Errorf("list spin fixed cost: %w", err)
	}
	var items []*spinfixedcost.Entity
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
		return nil, 0, fmt.Errorf("close spin fixed cost rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate spin fixed cost: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing record. Period is never updated (immutable).
func (r *SpinFixedCostRepository) Update(ctx context.Context, entity *spinfixedcost.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_spin_fixed_cost SET
			msfc_common_poy_denier    = $2,
			msfc_poy_production       = $3,
			msfc_spin_power_month     = $4,
			msfc_spin_manpower_month  = $5,
			msfc_spin_overheads_month = $6,
			msfc_spin_conssprs_month  = $7,
			msfc_is_active            = $8,
			msfc_updated_at           = $9,
			msfc_updated_by           = $10
		WHERE msfc_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.CommonPoyDenier(),
		entity.PoyProduction(),
		entity.SpinPowerMonth(),
		entity.SpinManpowerMonth(),
		entity.SpinOverheadsMonth(),
		entity.SpinConssprsMonth(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update spin fixed cost: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return spinfixedcost.ErrNotFound
	}
	return nil
}

// SoftDelete marks a record as deleted.
func (r *SpinFixedCostRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_spin_fixed_cost SET deleted_at=$2,deleted_by=$3,msfc_is_active=false
		 WHERE msfc_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete spin fixed cost: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return spinfixedcost.ErrNotFound
	}
	return nil
}

// ExistsByPeriod reports whether a live record exists for the given period.
func (r *SpinFixedCostRepository) ExistsByPeriod(ctx context.Context, period string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_spin_fixed_cost WHERE msfc_period=$1 AND deleted_at IS NULL)`, period,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by period: %w", err)
	}
	return exists, nil
}

// ExistsByID reports whether a live record exists for the given UUID.
func (r *SpinFixedCostRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_spin_fixed_cost WHERE msfc_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// LoadAnchorStats gathers the counts the anchor-row guard needs, excluding the
// candidate row so the numbers describe the post-change state of the table.
func (r *SpinFixedCostRepository) LoadAnchorStats(ctx context.Context, excludeID uuid.UUID) (spinfixedcost.AnchorStats, error) {
	// msfc_period is a zero-padded YYYYMM string, so lexicographic ordering is
	// chronological -- same assumption the calc engine loader makes.
	const q = `
		SELECT
			COUNT(*) FILTER (WHERE msfc_is_active),
			COALESCE(MIN(msfc_period) FILTER (WHERE msfc_is_active), ''),
			COALESCE(BOOL_OR(msfc_period > (
				SELECT msfc_period FROM mst_spin_fixed_cost WHERE msfc_id = $1
			)), FALSE)
		FROM mst_spin_fixed_cost
		WHERE deleted_at IS NULL AND msfc_id <> $1`

	var stats spinfixedcost.AnchorStats
	err := r.db.QueryRowContext(ctx, q, excludeID).Scan(
		&stats.RemainingActiveCount,
		&stats.EarliestRemainingActivePeriod,
		&stats.HasLiveRowAfterCandidate,
	)
	if err != nil {
		return spinfixedcost.AnchorStats{}, fmt.Errorf("load spin fixed cost anchor stats: %w", err)
	}
	return stats, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *SpinFixedCostRepository) buildListWhere(filter spinfixedcost.ListFilter) (string, []any) {
	base := whereNotDeleted
	args := make([]any, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND msfc_period ILIKE $%d`, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.Period != "" {
		base += fmt.Sprintf(` AND msfc_period = $%d`, idx)
		args = append(args, filter.Period)
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND msfc_is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
	}
	return base, args
}

func (r *SpinFixedCostRepository) selectCols() string {
	return `
		SELECT msfc_id, msfc_period, msfc_common_poy_denier, msfc_poy_production,
		       msfc_spin_power_month, msfc_spin_manpower_month,
		       msfc_spin_overheads_month, msfc_spin_conssprs_month,
		       msfc_is_active, msfc_created_at, msfc_created_by,
		       msfc_updated_at, msfc_updated_by, deleted_at, deleted_by
		FROM mst_spin_fixed_cost
	`
}

func (r *SpinFixedCostRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"period":          "msfc_period",
		"msfc_period":     "msfc_period",
		sortKeyCreatedAt:  "msfc_created_at",
		"msfc_created_at": "msfc_created_at",
		"updated_at":      "msfc_updated_at",
		"msfc_updated_at": "msfc_updated_at",
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "msfc_period"
}

type spinFixedCostDTO struct {
	ID                 uuid.UUID
	Period             string
	CommonPoyDenier    float64
	PoyProduction      float64
	SpinPowerMonth     float64
	SpinManpowerMonth  float64
	SpinOverheadsMonth float64
	SpinConssprsMonth  float64
	IsActive           bool
	CreatedAt          time.Time
	CreatedBy          string
	UpdatedAt          sql.NullTime
	UpdatedBy          sql.NullString
	DeletedAt          sql.NullTime
	DeletedBy          sql.NullString
}

func (d *spinFixedCostDTO) toEntity() *spinfixedcost.Entity {
	return spinfixedcost.Reconstruct(spinfixedcost.ReconstructInput{
		ID:                 d.ID,
		Period:             d.Period,
		CommonPoyDenier:    d.CommonPoyDenier,
		PoyProduction:      d.PoyProduction,
		SpinPowerMonth:     d.SpinPowerMonth,
		SpinManpowerMonth:  d.SpinManpowerMonth,
		SpinOverheadsMonth: d.SpinOverheadsMonth,
		SpinConssprsMonth:  d.SpinConssprsMonth,
		IsActive:           d.IsActive,
		CreatedAt:          d.CreatedAt,
		CreatedBy:          d.CreatedBy,
		UpdatedAt:          nullableTimePtr(d.UpdatedAt),
		UpdatedBy:          nullableStringPtr(d.UpdatedBy),
		DeletedAt:          nullableTimePtr(d.DeletedAt),
		DeletedBy:          nullableStringPtr(d.DeletedBy),
	})
}

func (d *spinFixedCostDTO) scanTargets() []any {
	return []any{
		&d.ID, &d.Period, &d.CommonPoyDenier, &d.PoyProduction,
		&d.SpinPowerMonth, &d.SpinManpowerMonth,
		&d.SpinOverheadsMonth, &d.SpinConssprsMonth,
		&d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	}
}

func (r *SpinFixedCostRepository) scanOne(row *sql.Row) (*spinfixedcost.Entity, error) {
	var d spinFixedCostDTO
	err := row.Scan(d.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, spinfixedcost.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan spin fixed cost: %w", err)
	}
	return d.toEntity(), nil
}

func (r *SpinFixedCostRepository) scanRow(rows *sql.Rows) (*spinfixedcost.Entity, error) {
	var d spinFixedCostDTO
	if err := rows.Scan(d.scanTargets()...); err != nil {
		return nil, fmt.Errorf("scan spin fixed cost row: %w", err)
	}
	return d.toEntity(), nil
}
