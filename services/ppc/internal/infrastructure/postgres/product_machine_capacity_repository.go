// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/capacity"
)

// CapacityRepository implements capacity.Repository using PostgreSQL.
type CapacityRepository struct {
	db *DB
}

// NewCapacityRepository creates a new CapacityRepository.
func NewCapacityRepository(db *DB) *CapacityRepository {
	return &CapacityRepository{db: db}
}

var _ capacity.Repository = (*CapacityRepository)(nil)

const capacitySelectColumns = `pmc.pmc_id, pmc.pmc_cpm_product_sys_id, pmc.pmc_machine_id, m.machine_no, ` +
	`pmc.pmc_prod_per_day, pmc.pmc_efficiency_pct, ` +
	`pmc.pmc_created_at, pmc.pmc_created_by, pmc.pmc_updated_at, pmc.pmc_updated_by`

// Create persists a new capacity and assigns its generated ID.
func (r *CapacityRepository) Create(ctx context.Context, entity *capacity.Capacity) error {
	query := `
		INSERT INTO product_machine_capacity (
			pmc_cpm_product_sys_id, pmc_machine_id,
			pmc_prod_per_day, pmc_efficiency_pct, pmc_created_at, pmc_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING pmc_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.CpmProductSysID(),
		entity.MachineID(),
		floatPtrToNull(entity.ProdPerDay()),
		floatPtrToNull(entity.EfficiencyPct()),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return capacity.ErrAlreadyExists
		}
		if isForeignKeyViolation(err) {
			return capacity.ErrInvalidMachine
		}
		return fmt.Errorf("failed to create product machine capacity: %w", err)
	}

	*entity = *capacity.Reconstruct(
		id, entity.CpmProductSysID(), entity.MachineID(), entity.MachineNo(),
		entity.ProdPerDay(), entity.EfficiencyPct(),
		entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a capacity by its ID.
func (r *CapacityRepository) GetByID(ctx context.Context, id int64) (*capacity.Capacity, error) {
	query := `SELECT ` + capacitySelectColumns + `
		FROM product_machine_capacity pmc
		LEFT JOIN machine m ON pmc.pmc_machine_id = m.machine_id
		WHERE pmc.pmc_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves capacities with filtering and pagination.
func (r *CapacityRepository) List(ctx context.Context, filter capacity.ListFilter) ([]*capacity.Capacity, int64, error) {
	filter.Validate()

	base := `FROM product_machine_capacity pmc
		LEFT JOIN machine m ON pmc.pmc_machine_id = m.machine_id
		WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.CpmProductSysID != nil {
		base += fmt.Sprintf(` AND pmc.pmc_cpm_product_sys_id = $%d`, argIdx)
		args = append(args, *filter.CpmProductSysID)
		argIdx++
	}
	if filter.MachineID != nil {
		base += fmt.Sprintf(` AND pmc.pmc_machine_id = $%d`, argIdx)
		args = append(args, *filter.MachineID)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count product machine capacities: %w", err)
	}

	sortColumnMap := map[string]string{
		"cpm_product_sys_id": "pmc.pmc_cpm_product_sys_id",
		"machine_id":         "pmc.pmc_machine_id",
		"created_at":         "pmc.pmc_created_at",
	}
	orderCol := "pmc.pmc_cpm_product_sys_id"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + capacitySelectColumns + ` ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list product machine capacities: %w", err)
	}
	defer closeRows(rows)

	var result []*capacity.Capacity
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product machine capacity rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing capacity.
func (r *CapacityRepository) Update(ctx context.Context, entity *capacity.Capacity) error {
	query := `
		UPDATE product_machine_capacity
		SET pmc_prod_per_day = $2, pmc_efficiency_pct = $3,
			pmc_updated_at = $4, pmc_updated_by = $5
		WHERE pmc_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		floatPtrToNull(entity.ProdPerDay()),
		floatPtrToNull(entity.EfficiencyPct()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update product machine capacity: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return capacity.ErrNotFound
	}
	return nil
}

// Delete removes a capacity by its ID.
func (r *CapacityRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_machine_capacity WHERE pmc_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete product machine capacity: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return capacity.ErrNotFound
	}
	return nil
}

func (r *CapacityRepository) scanRow(row *sql.Row) (*capacity.Capacity, error) {
	var dto capacityDTO
	err := dto.scan(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, capacity.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan product machine capacity: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *CapacityRepository) scanRows(rows *sql.Rows) (*capacity.Capacity, error) {
	var dto capacityDTO
	if err := dto.scan(rows.Scan); err != nil {
		return nil, fmt.Errorf("failed to scan product machine capacity row: %w", err)
	}
	return dto.toEntity(), nil
}

// capacityDTO is the row-mapping type for product_machine_capacity.
type capacityDTO struct {
	ID              int64
	CpmProductSysID int64
	MachineID       int64
	MachineNo       sql.NullString
	ProdPerDay      sql.NullFloat64
	EfficiencyPct   sql.NullFloat64
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
}

func (d *capacityDTO) scan(scan func(...any) error) error {
	return scan(
		&d.ID, &d.CpmProductSysID, &d.MachineID, &d.MachineNo,
		&d.ProdPerDay, &d.EfficiencyPct,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
}

func (d *capacityDTO) toEntity() *capacity.Capacity {
	var machineNo string
	if d.MachineNo.Valid {
		machineNo = d.MachineNo.String
	}
	return capacity.Reconstruct(
		d.ID,
		d.CpmProductSysID,
		d.MachineID,
		machineNo,
		nullFloatPtr(d.ProdPerDay),
		nullFloatPtr(d.EfficiencyPct),
		d.CreatedAt,
		d.CreatedBy,
		nullTimePtr(d.UpdatedAt),
		nullStringPtr(d.UpdatedBy),
	)
}

// DailyCapacity returns the total effective daily output, in product units,
// available to a machine group for a product: the sum of per-machine
// prod_per_day across the group's active machines, de-rated by each machine's
// efficiency percentage when one is recorded.
//
// A group with no capacity rows for the product returns 0 with no error —
// capacity master data is incomplete for many pairs and planning must not
// block on it.
func (r *CapacityRepository) DailyCapacity(ctx context.Context, cpmProductSysID, machineGroupID int64) (float64, error) {
	const query = `
		SELECT COALESCE(SUM(pmc.pmc_prod_per_day * COALESCE(pmc.pmc_efficiency_pct, 100) / 100.0), 0)
		FROM product_machine_capacity pmc
		JOIN machine m ON m.machine_id = pmc.pmc_machine_id
		WHERE pmc.pmc_cpm_product_sys_id = $1
		  AND m.machine_group_id = $2
		  AND m.machine_is_active = TRUE
	`
	var perDay float64
	if err := r.db.QueryRowContext(ctx, query, cpmProductSysID, machineGroupID).Scan(&perDay); err != nil {
		return 0, fmt.Errorf("failed to resolve daily capacity: %w", err)
	}
	return perDay, nil
}
