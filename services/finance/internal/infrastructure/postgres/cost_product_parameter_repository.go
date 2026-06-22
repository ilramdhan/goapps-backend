package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// CostProductParameterRepository implements cpp.Repository.
type CostProductParameterRepository struct {
	db *DB
}

// NewCostProductParameterRepository wires the repo.
func NewCostProductParameterRepository(db *DB) *CostProductParameterRepository {
	return &CostProductParameterRepository{db: db}
}

var _ cpp.Repository = (*CostProductParameterRepository)(nil)

// selectListSQL drives from CAPP_ (per-product applicable params), joins
// mst_parameter for metadata + LEFT JOIN cost_product_parameter for the value.
// Per-product `capp_is_required` overrides mst_parameter.is_required_for_costing.
const selectListSQL = `
SELECT
    p.id, p.param_code, p.param_name, p.param_short_name,
    p.data_type, p.param_category,
    COALESCE(u.uom_code, '') AS uom_code,
    COALESCE(p.owner_department, '') AS owner_department,
    a.capp_is_required AS is_required_for_costing,
    p.is_period_dependent,
    COALESCE(p.lookup_master_code, '') AS lookup_master_code,
    COALESCE(a.capp_display_order, p.display_order) AS display_order,
    COALESCE(p.display_group, '') AS display_group,
    COALESCE(p.lookup_fill_group_code, '') AS lookup_fill_group_code,
    c.cpp_value_id, c.cpp_value_numeric::text, c.cpp_value_text, c.cpp_value_flag,
    c.cpp_filled_at, c.cpp_filled_by,
    c.cpp_created_at, c.cpp_created_by, c.cpp_updated_at, c.cpp_updated_by
FROM cost_product_applicable_param a
JOIN mst_parameter p
       ON p.id = a.capp_param_id
LEFT JOIN mst_uom u
       ON u.uom_id = p.uom_id AND u.deleted_at IS NULL
LEFT JOIN cost_product_parameter c
       ON c.cpp_param_id = p.id AND c.cpp_product_sys_id = a.capp_product_sys_id
WHERE a.capp_product_sys_id = $1
  AND p.deleted_at IS NULL
  AND p.is_active = TRUE
  AND p.is_period_dependent = FALSE
`

// ListForProduct implements Repository.
func (r *CostProductParameterRepository) ListForProduct(ctx context.Context, productSysID int64, requiredOnly bool) ([]cpp.RequiredEntry, error) {
	query := selectListSQL
	if requiredOnly {
		query += " AND a.capp_is_required = TRUE"
	}
	query += " ORDER BY COALESCE(p.display_group, ''), COALESCE(a.capp_display_order, p.display_order), p.param_code"

	rows, err := r.db.QueryContext(ctx, query, productSysID)
	if err != nil {
		return nil, fmt.Errorf("list product params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var entries []cpp.RequiredEntry
	for rows.Next() {
		entry, scanErr := scanRequiredEntry(rows, productSysID)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product params: %w", err)
	}
	return entries, nil
}

func scanRequiredEntry(rows *sql.Rows, productSysID int64) (cpp.RequiredEntry, error) {
	var (
		meta         cpp.ParamMeta
		valueID      sql.NullInt64
		valueNumeric sql.NullString
		valueText    sql.NullString
		valueFlag    sql.NullBool
		filledAt     sql.NullTime
		filledBy     sql.NullString
		createdAt    sql.NullTime
		createdBy    sql.NullString
		updatedAt    sql.NullTime
		updatedBy    sql.NullString
	)
	if err := rows.Scan(
		&meta.ParamID, &meta.ParamCode, &meta.ParamName, &meta.ParamShortName,
		&meta.DataType, &meta.ParamCategory,
		&meta.UOMCode, &meta.OwnerDepartment,
		&meta.IsRequiredForCosting, &meta.IsPeriodDependent,
		&meta.LookupMasterCode, &meta.DisplayOrder, &meta.DisplayGroup,
		&meta.LookupFillGroupCode,
		&valueID, &valueNumeric, &valueText, &valueFlag,
		&filledAt, &filledBy,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	); err != nil {
		return cpp.RequiredEntry{}, fmt.Errorf("scan required entry: %w", err)
	}

	entry := cpp.RequiredEntry{Meta: meta}
	if !valueID.Valid {
		return entry, nil
	}

	v := &cpp.Value{
		ValueID:      valueID.Int64,
		ProductSysID: productSysID,
		ParamID:      meta.ParamID,
		FilledBy:     filledBy.String,
		CreatedBy:    createdBy.String,
	}
	if valueNumeric.Valid {
		s := valueNumeric.String
		v.ValueNumeric = &s
	}
	if valueText.Valid {
		s := valueText.String
		v.ValueText = &s
	}
	if valueFlag.Valid {
		b := valueFlag.Bool
		v.ValueFlag = &b
	}
	if filledAt.Valid {
		v.FilledAt = filledAt.Time
	}
	if createdAt.Valid {
		v.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		t := updatedAt.Time
		v.UpdatedAt = &t
	}
	if updatedBy.Valid {
		s := updatedBy.String
		v.UpdatedBy = &s
	}
	entry.Value = v
	return entry, nil
}

// GetMeta returns the joined snapshot for a single parameter.
func (r *CostProductParameterRepository) GetMeta(ctx context.Context, paramID uuid.UUID) (*cpp.ParamMeta, error) {
	const q = `
SELECT
    p.id, p.param_code, p.param_name, p.param_short_name,
    p.data_type, p.param_category,
    COALESCE(u.uom_code, ''),
    COALESCE(p.owner_department, ''),
    p.is_required_for_costing,
    p.is_period_dependent,
    COALESCE(p.lookup_master_code, ''),
    p.display_order,
    COALESCE(p.display_group, '')
FROM mst_parameter p
LEFT JOIN mst_uom u ON u.uom_id = p.uom_id AND u.deleted_at IS NULL
WHERE p.id = $1 AND p.deleted_at IS NULL AND p.is_active = TRUE
`
	var meta cpp.ParamMeta
	err := r.db.QueryRowContext(ctx, q, paramID).Scan(
		&meta.ParamID, &meta.ParamCode, &meta.ParamName, &meta.ParamShortName,
		&meta.DataType, &meta.ParamCategory,
		&meta.UOMCode, &meta.OwnerDepartment,
		&meta.IsRequiredForCosting, &meta.IsPeriodDependent,
		&meta.LookupMasterCode, &meta.DisplayOrder, &meta.DisplayGroup,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, cpp.ErrParamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get param meta: %w", err)
	}
	return &meta, nil
}

// ProductExists checks cost_product_master for the product.
func (r *CostProductParameterRepository) ProductExists(ctx context.Context, productSysID int64) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM cost_product_master WHERE cpm_product_sys_id = $1)`
	var ok bool
	if err := r.db.QueryRowContext(ctx, q, productSysID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check product exists: %w", err)
	}
	return ok, nil
}

// Upsert inserts or updates a single CPP_ row. The param must first be marked
// applicable in cost_product_applicable_param (CAPP) — otherwise this returns
// ErrParamNotApplicable so the UI can guide the user to add it first.
func (r *CostProductParameterRepository) Upsert(ctx context.Context, v *cpp.Value) error {
	const checkApplicable = `
SELECT 1 FROM cost_product_applicable_param
WHERE capp_product_sys_id = $1 AND capp_param_id = $2
`
	var dummy int
	checkErr := r.db.QueryRowContext(ctx, checkApplicable, v.ProductSysID, v.ParamID).Scan(&dummy)
	if errors.Is(checkErr, sql.ErrNoRows) {
		return cpp.ErrParamNotApplicable
	}
	if checkErr != nil {
		return fmt.Errorf("check applicable: %w", checkErr)
	}

	const q = `
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric, cpp_value_text, cpp_value_flag,
    cpp_filled_at, cpp_filled_by,
    cpp_created_at, cpp_created_by
) VALUES ($1, $2, $3::numeric, $4, $5, $6, $7, $6, $7)
ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
    cpp_value_numeric = EXCLUDED.cpp_value_numeric,
    cpp_value_text    = EXCLUDED.cpp_value_text,
    cpp_value_flag    = EXCLUDED.cpp_value_flag,
    cpp_filled_at     = EXCLUDED.cpp_filled_at,
    cpp_filled_by     = EXCLUDED.cpp_filled_by,
    cpp_updated_at    = EXCLUDED.cpp_filled_at,
    cpp_updated_by    = EXCLUDED.cpp_filled_by
RETURNING cpp_value_id
`
	now := time.Now()
	if v.FilledAt.IsZero() {
		v.FilledAt = now
	}
	return r.db.QueryRowContext(ctx, q,
		v.ProductSysID,
		v.ParamID,
		stringPtrOrNil(v.ValueNumeric),
		stringPtrOrNil(v.ValueText),
		boolPtrOrNil(v.ValueFlag),
		v.FilledAt,
		v.FilledBy,
	).Scan(&v.ValueID)
}

// Delete clears a single CPP_ row.
func (r *CostProductParameterRepository) Delete(ctx context.Context, productSysID int64, paramID uuid.UUID) error {
	const q = `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`
	res, err := r.db.ExecContext(ctx, q, productSysID, paramID)
	if err != nil {
		return fmt.Errorf("delete product param: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return cpp.ErrNotFound
	}
	return nil
}

// MissingRequired returns per-product required params without a bound value.
// Drives from CAPP_ (per-product applicable list) — global mst_parameter flag
// is no longer the source of truth.
func (r *CostProductParameterRepository) MissingRequired(ctx context.Context, productSysID int64) ([]cpp.ParamMeta, error) {
	const q = `
SELECT
    p.id, p.param_code, p.param_name, p.param_short_name,
    p.data_type, p.param_category,
    COALESCE(u.uom_code, ''),
    COALESCE(p.owner_department, ''),
    a.capp_is_required,
    p.is_period_dependent,
    COALESCE(p.lookup_master_code, ''),
    COALESCE(a.capp_display_order, p.display_order),
    COALESCE(p.display_group, '')
FROM cost_product_applicable_param a
JOIN mst_parameter p ON p.id = a.capp_param_id
LEFT JOIN mst_uom u ON u.uom_id = p.uom_id AND u.deleted_at IS NULL
LEFT JOIN cost_product_parameter c
       ON c.cpp_param_id = p.id AND c.cpp_product_sys_id = a.capp_product_sys_id
WHERE a.capp_product_sys_id = $1
  AND a.capp_is_required = TRUE
  AND p.deleted_at IS NULL
  AND p.is_active = TRUE
  AND p.is_period_dependent = FALSE
  AND p.param_category <> 'CALCULATED'   -- engine-filled, not user-filled
  AND c.cpp_value_id IS NULL
ORDER BY COALESCE(p.display_group, ''), COALESCE(a.capp_display_order, p.display_order), p.param_code
`
	rows, err := r.db.QueryContext(ctx, q, productSysID)
	if err != nil {
		return nil, fmt.Errorf("missing required params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var out []cpp.ParamMeta
	for rows.Next() {
		var m cpp.ParamMeta
		if err := rows.Scan(
			&m.ParamID, &m.ParamCode, &m.ParamName, &m.ParamShortName,
			&m.DataType, &m.ParamCategory,
			&m.UOMCode, &m.OwnerDepartment,
			&m.IsRequiredForCosting, &m.IsPeriodDependent,
			&m.LookupMasterCode, &m.DisplayOrder, &m.DisplayGroup,
		); err != nil {
			return nil, fmt.Errorf("scan missing: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// =============================================================================
// CAPP_ (cost_product_applicable_param) operations
// =============================================================================

// AddApplicable marks a param as applicable for a product. Default capp_is_required
// follows mst_parameter.is_required_for_costing if Applicability.IsRequired is FALSE
// AND the global flag is TRUE (auto-default convenience).
func (r *CostProductParameterRepository) AddApplicable(ctx context.Context, a *cpp.Applicability) error {
	const q = `
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order,
    capp_created_at, capp_created_by
) VALUES ($1, $2, $3, $4, NOW(), $5)
ON CONFLICT (capp_product_sys_id, capp_param_id) DO UPDATE SET
    capp_is_required    = EXCLUDED.capp_is_required,
    capp_display_order  = EXCLUDED.capp_display_order,
    capp_updated_at     = NOW(),
    capp_updated_by     = EXCLUDED.capp_created_by
RETURNING capp_id
`
	var displayOrder any
	if a.DisplayOrder != nil {
		displayOrder = *a.DisplayOrder
	}
	return r.db.QueryRowContext(ctx, q,
		a.ProductSysID, a.ParamID, a.IsRequired, displayOrder, a.CreatedBy,
	).Scan(&a.CappID)
}

// RemoveApplicable removes a param from a product's applicable list.
// The CASCADE on CPP_ is NOT defined — we delete the value row explicitly first.
func (r *CostProductParameterRepository) RemoveApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				_ = rbErr
			}
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`, productSysID, paramID); err != nil {
		return fmt.Errorf("delete cpp: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id = $1 AND capp_param_id = $2`, productSysID, paramID)
	if err != nil {
		return fmt.Errorf("delete capp: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return cpp.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// UpdateApplicable patches the per-product override fields.
func (r *CostProductParameterRepository) UpdateApplicable(
	ctx context.Context, productSysID int64, paramID uuid.UUID,
	isRequired *bool, displayOrder *int32, updatedBy string,
) error {
	const q = `
UPDATE cost_product_applicable_param SET
    capp_is_required   = COALESCE($3, capp_is_required),
    capp_display_order = $4,
    capp_updated_at    = NOW(),
    capp_updated_by    = $5
WHERE capp_product_sys_id = $1 AND capp_param_id = $2
`
	var dispArg any
	if displayOrder != nil {
		dispArg = *displayOrder
	}
	res, err := r.db.ExecContext(ctx, q, productSysID, paramID, isRequired, dispArg, updatedBy)
	if err != nil {
		return fmt.Errorf("update capp: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return cpp.ErrNotFound
	}
	return nil
}

// ListAvailableParams returns mst_parameter rows that are NOT YET applicable to
// the product (so they can be shown in the "Add parameter" picker).
func (r *CostProductParameterRepository) ListAvailableParams(ctx context.Context, productSysID int64) ([]cpp.ParamMeta, error) {
	const q = `
SELECT
    p.id, p.param_code, p.param_name, p.param_short_name,
    p.data_type, p.param_category,
    COALESCE(u.uom_code, ''),
    COALESCE(p.owner_department, ''),
    p.is_required_for_costing,
    p.is_period_dependent,
    COALESCE(p.lookup_master_code, ''),
    p.display_order,
    COALESCE(p.display_group, ''),
    COALESCE(p.lookup_fill_group_code, '')
FROM mst_parameter p
LEFT JOIN mst_uom u ON u.uom_id = p.uom_id AND u.deleted_at IS NULL
WHERE p.deleted_at IS NULL
  AND p.is_active = TRUE
  AND p.is_period_dependent = FALSE
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param a
      WHERE a.capp_product_sys_id = $1 AND a.capp_param_id = p.id
  )
ORDER BY COALESCE(p.display_group, ''), p.display_order, p.param_code
`
	rows, err := r.db.QueryContext(ctx, q, productSysID)
	if err != nil {
		return nil, fmt.Errorf("list available: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var out []cpp.ParamMeta
	for rows.Next() {
		var m cpp.ParamMeta
		if err := rows.Scan(
			&m.ParamID, &m.ParamCode, &m.ParamName, &m.ParamShortName,
			&m.DataType, &m.ParamCategory,
			&m.UOMCode, &m.OwnerDepartment,
			&m.IsRequiredForCosting, &m.IsPeriodDependent,
			&m.LookupMasterCode, &m.DisplayOrder, &m.DisplayGroup,
			&m.LookupFillGroupCode,
		); err != nil {
			return nil, fmt.Errorf("scan available: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func stringPtrOrNil(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// CountApplicableForProducts returns the number of non-CALCULATED applicable params
// for all products in the slice. Used to set cft_total_params when creating fill tasks.
// CALCULATED params are excluded because they are computed by the calc engine and
// must not block fill submission.
func (r *CostProductParameterRepository) CountApplicableForProducts(ctx context.Context, productSysIDs []int64) (int32, error) {
	if len(productSysIDs) == 0 {
		return 0, nil
	}
	var n int32
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		   FROM cost_product_applicable_param ca
		   JOIN mst_parameter mp ON mp.id = ca.capp_param_id
		                        AND mp.param_category != 'CALCULATED'
		  WHERE ca.capp_product_sys_id = ANY($1)`,
		pq.Array(productSysIDs),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count applicable params: %w", err)
	}
	return n, nil
}

func boolPtrOrNil(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}

// GetParamIDByCode resolves mst_parameter.param_code → UUID.
func (r *CostProductParameterRepository) GetParamIDByCode(ctx context.Context, paramCode string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL AND is_active = TRUE LIMIT 1`,
		paramCode,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, cpp.ErrParamNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get param id by code: %w", err)
	}
	return id, nil
}

// GetProductSysIDByCode resolves cost_product_master.cpm_product_code → sys_id.
func (r *CostProductParameterRepository) GetProductSysIDByCode(ctx context.Context, productCode string) (int64, error) {
	var sysID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code = $1 LIMIT 1`,
		productCode,
	).Scan(&sysID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, cpp.ErrProductNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get product sys id by code: %w", err)
	}
	return sysID, nil
}

// ListApplicable returns all CAPP rows for a product joined with product code and param code.
func (r *CostProductParameterRepository) ListApplicable(ctx context.Context, productSysID int64) ([]cpp.CAPPRow, error) {
	const q = `
SELECT m.cpm_product_code, p.param_code, a.capp_is_required, a.capp_display_order
FROM cost_product_applicable_param a
JOIN mst_parameter p ON p.id = a.capp_param_id AND p.deleted_at IS NULL
JOIN cost_product_master m ON m.cpm_product_sys_id = a.capp_product_sys_id
WHERE a.capp_product_sys_id = $1
ORDER BY a.capp_display_order, p.param_code
`
	rows, err := r.db.QueryContext(ctx, q, productSysID)
	if err != nil {
		return nil, fmt.Errorf("list applicable: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var out []cpp.CAPPRow
	for rows.Next() {
		var row cpp.CAPPRow
		var dispOrder sql.NullInt32
		if err := rows.Scan(&row.ProductCode, &row.ParamCode, &row.IsRequired, &dispOrder); err != nil {
			return nil, fmt.Errorf("scan capp row: %w", err)
		}
		if dispOrder.Valid {
			v := dispOrder.Int32
			row.DisplayOrder = &v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAllApplicable returns all CAPP rows across all products.
func (r *CostProductParameterRepository) ListAllApplicable(ctx context.Context) ([]cpp.CAPPRow, error) {
	const q = `
SELECT m.cpm_product_code, p.param_code, a.capp_is_required, a.capp_display_order
FROM cost_product_applicable_param a
JOIN mst_parameter p ON p.id = a.capp_param_id AND p.deleted_at IS NULL
JOIN cost_product_master m ON m.cpm_product_sys_id = a.capp_product_sys_id
ORDER BY m.cpm_product_code, a.capp_display_order, p.param_code
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all applicable: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var out []cpp.CAPPRow
	for rows.Next() {
		var row cpp.CAPPRow
		var dispOrder sql.NullInt32
		if err := rows.Scan(&row.ProductCode, &row.ParamCode, &row.IsRequired, &dispOrder); err != nil {
			return nil, fmt.Errorf("scan capp row: %w", err)
		}
		if dispOrder.Valid {
			v := dispOrder.Int32
			row.DisplayOrder = &v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAllValues returns all CPP rows across all products.
func (r *CostProductParameterRepository) ListAllValues(ctx context.Context) ([]cpp.CPPRow, error) {
	const q = `
SELECT m.cpm_product_code, p.param_code,
       c.cpp_value_numeric::text, c.cpp_value_text, c.cpp_value_flag
FROM cost_product_parameter c
JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.deleted_at IS NULL
JOIN cost_product_master m ON m.cpm_product_sys_id = c.cpp_product_sys_id
ORDER BY m.cpm_product_code, p.param_code
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all cpp values: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var out []cpp.CPPRow
	for rows.Next() {
		var row cpp.CPPRow
		var vn, vt sql.NullString
		var vf sql.NullBool
		if err := rows.Scan(&row.ProductCode, &row.ParamCode, &vn, &vt, &vf); err != nil {
			return nil, fmt.Errorf("scan cpp row: %w", err)
		}
		if vn.Valid {
			row.ValueNumeric = &vn.String
		}
		if vt.Valid {
			row.ValueText = &vt.String
		}
		if vf.Valid {
			row.ValueFlag = &vf.Bool
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// AddApplicableWithChildren inserts trigger + all child CAPP rows in a single transaction.
// ON CONFLICT DO NOTHING ensures idempotency.
func (r *CostProductParameterRepository) AddApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, createdBy string, fillGroupChildren []uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				_ = rbErr
			}
		}
	}()

	allIDs := make([]uuid.UUID, 0, 1+len(fillGroupChildren))
	allIDs = append(allIDs, triggerParamID)
	allIDs = append(allIDs, fillGroupChildren...)

	const q = `INSERT INTO cost_product_applicable_param
	               (capp_product_sys_id, capp_param_id, capp_created_by)
	           VALUES ($1, $2, $3)
	           ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`
	for _, paramID := range allIDs {
		if _, execErr := tx.ExecContext(ctx, q, productSysID, paramID, createdBy); execErr != nil {
			return fmt.Errorf("insert capp %s: %w", paramID, execErr)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// GetRemovePreview returns trigger + child display info for the confirm-remove dialog.
func (r *CostProductParameterRepository) GetRemovePreview(ctx context.Context, productSysID int64, paramID uuid.UUID) (cpp.RemovePreview, error) {
	const trigQ = `SELECT param_code, param_name
	               FROM mst_parameter WHERE id = $1 AND deleted_at IS NULL`
	var preview cpp.RemovePreview
	if err := r.db.QueryRowContext(ctx, trigQ, paramID).Scan(&preview.TriggerParamCode, &preview.TriggerParamName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return preview, cpp.ErrParamNotFound
		}
		return preview, fmt.Errorf("get trigger param: %w", err)
	}

	const childQ = `
SELECT p.param_code, p.param_name,
       COALESCE(c.cpp_value_numeric::text, c.cpp_value_text, '') AS current_val
FROM cost_product_applicable_param capp
JOIN mst_parameter p ON p.id = capp.capp_param_id
LEFT JOIN cost_product_parameter c
       ON c.cpp_product_sys_id = capp.capp_product_sys_id
      AND c.cpp_param_id = capp.capp_param_id
WHERE capp.capp_product_sys_id = $1
  AND p.lookup_fill_group_code = $2
  AND p.deleted_at IS NULL`
	rows, err := r.db.QueryContext(ctx, childQ, productSysID, preview.TriggerParamCode)
	if err != nil {
		return preview, fmt.Errorf("get child params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var c cpp.ChildPreview
		if scanErr := rows.Scan(&c.ParamCode, &c.ParamName, &c.CurrentValue); scanErr != nil {
			return preview, fmt.Errorf("scan child: %w", scanErr)
		}
		preview.Children = append(preview.Children, c)
	}
	return preview, rows.Err()
}

// scanParamIDRows drains a rows result set of uuid.UUID values.
func scanParamIDRows(rows *sql.Rows) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("scan param id (close: %w): %w", closeErr, err)
			}
			return nil, fmt.Errorf("scan param id: %w", err)
		}
		ids = append(ids, id)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close rows: %w", closeErr)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate param ids: %w", rowsErr)
	}
	return ids, nil
}

// RemoveApplicableWithChildren removes trigger + all child CAPP rows + their CPP values in one tx.
func (r *CostProductParameterRepository) RemoveApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, _ string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				_ = rbErr
			}
		}
	}()

	// Get trigger param_code for fill-group lookup.
	var trigParamCode string
	if err = tx.QueryRowContext(ctx, `SELECT param_code FROM mst_parameter WHERE id = $1 AND deleted_at IS NULL`, triggerParamID).Scan(&trigParamCode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cpp.ErrParamNotFound
		}
		return fmt.Errorf("get trigger param code: %w", err)
	}

	// Collect all param IDs to remove: trigger + fill-group children currently in CAPP.
	const collectQ = `
SELECT capp.capp_param_id
FROM cost_product_applicable_param capp
JOIN mst_parameter p ON p.id = capp.capp_param_id
WHERE capp.capp_product_sys_id = $1
  AND (capp.capp_param_id = $2 OR p.lookup_fill_group_code = $3)
  AND p.deleted_at IS NULL`
	rows, err := tx.QueryContext(ctx, collectQ, productSysID, triggerParamID, trigParamCode)
	if err != nil {
		return fmt.Errorf("collect param ids: %w", err)
	}
	paramIDs, err := scanParamIDRows(rows)
	if err != nil {
		return err
	}
	if len(paramIDs) == 0 {
		return cpp.ErrNotFound
	}

	// Delete CPP values for all collected params.
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = ANY($2)`,
		productSysID, pq.Array(paramIDs),
	); err != nil {
		return fmt.Errorf("delete cpp values: %w", err)
	}

	// Delete CAPP rows for all collected params.
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id = $1 AND capp_param_id = ANY($2)`,
		productSysID, pq.Array(paramIDs),
	); err != nil {
		return fmt.Errorf("delete capp rows: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// GetParamCodeByID resolves a param UUID to its param_code string.
func (r *CostProductParameterRepository) GetParamCodeByID(ctx context.Context, paramID uuid.UUID) (string, error) {
	const q = `SELECT param_code FROM mst_parameter WHERE id = $1 AND deleted_at IS NULL`
	var code string
	if err := r.db.QueryRowContext(ctx, q, paramID).Scan(&code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", cpp.ErrParamNotFound
		}
		return "", fmt.Errorf("get param code by id: %w", err)
	}
	return code, nil
}

// GetCurrentValueAsText returns the current stored CPP value as a human-readable string.
// Returns empty string when no value exists for the given (productSysID, paramID) pair.
func (r *CostProductParameterRepository) GetCurrentValueAsText(ctx context.Context, productSysID int64, paramID uuid.UUID) (string, error) {
	const q = `
SELECT cpp_value_numeric::text, cpp_value_text, cpp_value_flag
FROM cost_product_parameter
WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`

	var vn, vt sql.NullString
	var vf sql.NullBool
	err := r.db.QueryRowContext(ctx, q, productSysID, paramID).Scan(&vn, &vt, &vf)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get current cpp value: %w", err)
	}
	switch {
	case vn.Valid:
		return vn.String, nil
	case vt.Valid:
		return vt.String, nil
	case vf.Valid:
		if vf.Bool {
			return "true", nil
		}
		return "false", nil
	default:
		return "", nil
	}
}

// BulkUpsertValues upserts CPP value rows in a single transaction with batches of 200.
func (r *CostProductParameterRepository) BulkUpsertValues(ctx context.Context, items []cpp.CPPUpsertInput, actor string) (inserted, updated int, err error) {
	const batchSize = 200
	now := time.Now()

	tx, txErr := r.db.BeginTx(ctx, nil)
	if txErr != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", txErr)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		ins, upd, batchErr := upsertCPPBatch(ctx, tx, items[start:end], actor, now)
		if batchErr != nil {
			return 0, 0, fmt.Errorf("upsert CPP batch: %w", batchErr)
		}
		inserted += ins
		updated += upd
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return 0, 0, fmt.Errorf("commit CPP upsert: %w", commitErr)
	}
	return inserted, updated, nil
}

func upsertCPPBatch(ctx context.Context, tx *sql.Tx, items []cpp.CPPUpsertInput, actor string, now time.Time) (inserted, updated int, err error) {
	const q = `
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric, cpp_value_text, cpp_value_flag,
    cpp_filled_at, cpp_filled_by,
    cpp_created_at, cpp_created_by, cpp_updated_at, cpp_updated_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$8,$9)
ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
    cpp_value_numeric = EXCLUDED.cpp_value_numeric,
    cpp_value_text    = EXCLUDED.cpp_value_text,
    cpp_value_flag    = EXCLUDED.cpp_value_flag,
    cpp_filled_at     = EXCLUDED.cpp_filled_at,
    cpp_filled_by     = EXCLUDED.cpp_filled_by,
    cpp_updated_at    = EXCLUDED.cpp_updated_at,
    cpp_updated_by    = EXCLUDED.cpp_updated_by
RETURNING (xmax = 0)::int`

	for _, item := range items {
		var wasInserted int
		if scanErr := tx.QueryRowContext(ctx, q,
			item.ProductSysID, item.ParamID,
			item.ValueNumeric, item.ValueText, item.ValueFlag,
			item.FilledAt, item.FilledBy,
			now, actor,
		).Scan(&wasInserted); scanErr != nil {
			return 0, 0, fmt.Errorf("upsert cpp row: %w", scanErr)
		}
		if wasInserted == 1 {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, nil
}

// BulkUpsertApplicable upserts CAPP rows in a single transaction with batches of 200.
func (r *CostProductParameterRepository) BulkUpsertApplicable(ctx context.Context, items []cpp.CAPPUpsertInput, actor string) (inserted, updated int, err error) {
	const batchSize = 200
	now := time.Now()

	tx, txErr := r.db.BeginTx(ctx, nil)
	if txErr != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", txErr)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		ins, upd, batchErr := upsertCAPPBatch(ctx, tx, items[start:end], actor, now)
		if batchErr != nil {
			return 0, 0, fmt.Errorf("upsert CAPP batch: %w", batchErr)
		}
		inserted += ins
		updated += upd
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return 0, 0, fmt.Errorf("commit CAPP upsert: %w", commitErr)
	}
	return inserted, updated, nil
}

func upsertCAPPBatch(ctx context.Context, tx *sql.Tx, items []cpp.CAPPUpsertInput, actor string, now time.Time) (inserted, updated int, err error) {
	const q = `
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order,
    capp_created_at, capp_created_by, capp_updated_at, capp_updated_by
) VALUES ($1,$2,$3,$4,$5,$6,$5,$6)
ON CONFLICT (capp_product_sys_id, capp_param_id) DO UPDATE SET
    capp_is_required   = EXCLUDED.capp_is_required,
    capp_display_order = EXCLUDED.capp_display_order,
    capp_updated_at    = EXCLUDED.capp_updated_at,
    capp_updated_by    = EXCLUDED.capp_updated_by
RETURNING (xmax = 0)::int`

	for _, item := range items {
		var wasInserted int
		if scanErr := tx.QueryRowContext(ctx, q,
			item.ProductSysID, item.ParamID, item.IsRequired, item.DisplayOrder,
			now, actor,
		).Scan(&wasInserted); scanErr != nil {
			return 0, 0, fmt.Errorf("upsert capp row: %w", scanErr)
		}
		if wasInserted == 1 {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, nil
}

// ListAllParams returns all non-deleted mst_parameter rows for map preloading.
func (r *CostProductParameterRepository) ListAllParams(ctx context.Context) ([]cpp.ParamMeta, error) {
	const q = `
SELECT p.id, p.param_code, p.param_name, p.param_short_name,
       p.data_type, p.param_category,
       COALESCE(u.uom_code, '') AS uom_code,
       COALESCE(p.owner_department, ''),
       p.is_required_for_costing, p.is_period_dependent,
       COALESCE(p.lookup_master_code, ''),
       COALESCE(p.display_order, 0),
       COALESCE(p.display_group, ''),
       COALESCE(p.lookup_fill_group_code, ''),
       COALESCE(p.lookup_source_column, '')
FROM mst_parameter p
LEFT JOIN mst_uom u ON u.uom_id = p.uom_id AND u.deleted_at IS NULL
WHERE p.deleted_at IS NULL
ORDER BY p.param_code`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var out []cpp.ParamMeta
	for rows.Next() {
		var m cpp.ParamMeta
		if scanErr := rows.Scan(
			&m.ParamID, &m.ParamCode, &m.ParamName, &m.ParamShortName,
			&m.DataType, &m.ParamCategory,
			&m.UOMCode, &m.OwnerDepartment,
			&m.IsRequiredForCosting, &m.IsPeriodDependent,
			&m.LookupMasterCode, &m.DisplayOrder, &m.DisplayGroup,
			&m.LookupFillGroupCode, &m.LookupSourceColumn,
		); scanErr != nil {
			return nil, fmt.Errorf("scan param meta: %w", scanErr)
		}
		out = append(out, m)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate params: %w", rowsErr)
	}
	return out, nil
}
