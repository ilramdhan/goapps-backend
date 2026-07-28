// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/productconfig"
)

// ProductConfigRepository implements productconfig.Repository using PostgreSQL.
type ProductConfigRepository struct {
	db *DB
}

// NewProductConfigRepository creates a new ProductConfigRepository.
func NewProductConfigRepository(db *DB) *ProductConfigRepository {
	return &ProductConfigRepository{db: db}
}

var _ productconfig.Repository = (*ProductConfigRepository)(nil)

// Create persists a new product config and assigns its generated ID.
func (r *ProductConfigRepository) Create(ctx context.Context, entity *productconfig.ProductConfig) error {
	query := `
		INSERT INTO product_ppc_config (
			ppc_cpm_product_sys_id, ppc_is_commodity_watch, ppc_price_sell,
			ppc_machine_group_id, ppc_yield_std, ppc_buffer_rm_pct,
			ppc_ax_yield_pct, ppc_denier, ppc_created_at, ppc_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ppc_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.CpmProductSysID(),
		entity.IsCommodityWatch(),
		floatPtrToNull(entity.PriceSell()),
		int64PtrToNull(entity.MachineGroupID()),
		floatPtrToNull(entity.YieldStd()),
		floatPtrToNull(entity.BufferRmPct()),
		floatPtrToNull(entity.AxYieldPct()),
		floatPtrToNull(entity.Denier()),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return productconfig.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create product config: %w", err)
	}

	*entity = *productconfig.Reconstruct(
		id, entity.CpmProductSysID(), entity.IsCommodityWatch(), entity.PriceSell(),
		entity.MachineGroupID(), entity.YieldStd(), entity.BufferRmPct(), entity.AxYieldPct(),
		entity.Denier(), entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a product config by its ID.
func (r *ProductConfigRepository) GetByID(ctx context.Context, id int64) (*productconfig.ProductConfig, error) {
	query := `
		SELECT ppc_id, ppc_cpm_product_sys_id, ppc_is_commodity_watch, ppc_price_sell,
			ppc_machine_group_id, ppc_yield_std, ppc_buffer_rm_pct, ppc_ax_yield_pct,
			ppc_denier, ppc_created_at, ppc_created_by, ppc_updated_at, ppc_updated_by
		FROM product_ppc_config
		WHERE ppc_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves product configs with filtering and pagination.
func (r *ProductConfigRepository) List(ctx context.Context, filter productconfig.ListFilter) ([]*productconfig.ProductConfig, int64, error) {
	filter.Validate()

	base := `FROM product_ppc_config WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND CAST(ppc_cpm_product_sys_id AS TEXT) ILIKE $%d`, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.CommodityWatchOnly != nil && *filter.CommodityWatchOnly {
		base += ` AND ppc_is_commodity_watch = TRUE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count product configs: %w", err)
	}

	sortColumnMap := map[string]string{
		"cpm_product_sys_id": "ppc_cpm_product_sys_id",
		"price_sell":         "ppc_price_sell",
		"created_at":         "ppc_created_at",
	}
	orderCol := "ppc_cpm_product_sys_id"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ppc_id, ppc_cpm_product_sys_id, ppc_is_commodity_watch, ppc_price_sell, ` +
		`ppc_machine_group_id, ppc_yield_std, ppc_buffer_rm_pct, ppc_ax_yield_pct, ` +
		`ppc_denier, ppc_created_at, ppc_created_by, ppc_updated_at, ppc_updated_by ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list product configs: %w", err)
	}
	defer closeRows(rows)

	var result []*productconfig.ProductConfig
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product config rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing product config.
func (r *ProductConfigRepository) Update(ctx context.Context, entity *productconfig.ProductConfig) error {
	query := `
		UPDATE product_ppc_config
		SET ppc_is_commodity_watch = $2, ppc_price_sell = $3, ppc_machine_group_id = $4,
			ppc_yield_std = $5, ppc_buffer_rm_pct = $6, ppc_ax_yield_pct = $7,
			ppc_denier = $8, ppc_updated_at = $9, ppc_updated_by = $10
		WHERE ppc_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.IsCommodityWatch(),
		floatPtrToNull(entity.PriceSell()),
		int64PtrToNull(entity.MachineGroupID()),
		floatPtrToNull(entity.YieldStd()),
		floatPtrToNull(entity.BufferRmPct()),
		floatPtrToNull(entity.AxYieldPct()),
		floatPtrToNull(entity.Denier()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return productconfig.ErrAlreadyExists
		}
		return fmt.Errorf("failed to update product config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return productconfig.ErrNotFound
	}
	return nil
}

// Delete removes a product config by its ID.
func (r *ProductConfigRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_ppc_config WHERE ppc_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete product config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return productconfig.ErrNotFound
	}
	return nil
}

func (r *ProductConfigRepository) scanRow(row *sql.Row) (*productconfig.ProductConfig, error) {
	var dto productConfigDTO
	err := row.Scan(
		&dto.ID, &dto.CpmProductSysID, &dto.IsCommodityWatch, &dto.PriceSell,
		&dto.MachineGroupID, &dto.YieldStd, &dto.BufferRmPct, &dto.AxYieldPct,
		&dto.Denier, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, productconfig.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan product config: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *ProductConfigRepository) scanRows(rows *sql.Rows) (*productconfig.ProductConfig, error) {
	var dto productConfigDTO
	if err := rows.Scan(
		&dto.ID, &dto.CpmProductSysID, &dto.IsCommodityWatch, &dto.PriceSell,
		&dto.MachineGroupID, &dto.YieldStd, &dto.BufferRmPct, &dto.AxYieldPct,
		&dto.Denier, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan product config row: %w", err)
	}
	return dto.toEntity(), nil
}

// productConfigDTO is the row-mapping type for product_ppc_config.
// NOTE: product_code/product_name are not stored in this table; they are
// denormalized proto fields resolved from finance CPM (plan-03c).
type productConfigDTO struct {
	ID               int64
	CpmProductSysID  int64
	IsCommodityWatch bool
	PriceSell        sql.NullFloat64
	MachineGroupID   sql.NullInt64
	YieldStd         sql.NullFloat64
	BufferRmPct      sql.NullFloat64
	AxYieldPct       sql.NullFloat64
	Denier           sql.NullFloat64
	CreatedAt        time.Time
	CreatedBy        string
	UpdatedAt        sql.NullTime
	UpdatedBy        sql.NullString
}

func (d *productConfigDTO) toEntity() *productconfig.ProductConfig {
	return productconfig.Reconstruct(
		d.ID,
		d.CpmProductSysID,
		d.IsCommodityWatch,
		nullFloatPtr(d.PriceSell),
		nullInt64Ptr(d.MachineGroupID),
		nullFloatPtr(d.YieldStd),
		nullFloatPtr(d.BufferRmPct),
		nullFloatPtr(d.AxYieldPct),
		nullFloatPtr(d.Denier),
		d.CreatedAt,
		d.CreatedBy,
		nullTimePtr(d.UpdatedAt),
		nullStringPtr(d.UpdatedBy),
	)
}

func floatPtrToNull(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

func int64PtrToNull(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// MachineGroupForProduct returns the machine group configured for a product, or
// 0 when the product has no config row or the row leaves the group unset. Used
// by the plan-item cascade to place each route level on its own group; a 0
// result lets the caller fall back rather than fail, because a missing config
// row is a normal state for an upstream intermediate product.
func (r *ProductConfigRepository) MachineGroupForProduct(ctx context.Context, cpmProductSysID int64) (int64, error) {
	const query = `SELECT COALESCE(ppc_machine_group_id, 0) FROM product_ppc_config
		WHERE ppc_cpm_product_sys_id = $1`
	var groupID int64
	if err := r.db.QueryRowContext(ctx, query, cpmProductSysID).Scan(&groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to resolve machine group for product: %w", err)
	}
	return groupID, nil
}
