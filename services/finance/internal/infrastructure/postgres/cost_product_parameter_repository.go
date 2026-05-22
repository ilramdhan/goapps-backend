package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

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
	defer func() { _ = rows.Close() }()

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
		meta                  cpp.ParamMeta
		valueID               sql.NullInt64
		valueNumeric          sql.NullString
		valueText             sql.NullString
		valueFlag             sql.NullBool
		filledAt              sql.NullTime
		filledBy              sql.NullString
		createdAt             sql.NullTime
		createdBy             sql.NullString
		updatedAt             sql.NullTime
		updatedBy             sql.NullString
	)
	if err := rows.Scan(
		&meta.ParamID, &meta.ParamCode, &meta.ParamName, &meta.ParamShortName,
		&meta.DataType, &meta.ParamCategory,
		&meta.UOMCode, &meta.OwnerDepartment,
		&meta.IsRequiredForCosting, &meta.IsPeriodDependent,
		&meta.LookupMasterCode, &meta.DisplayOrder, &meta.DisplayGroup,
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
	defer func() { _ = rows.Close() }()

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
	var displayOrder interface{}
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
	var dispArg interface{}
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
    COALESCE(p.display_group, '')
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
	defer func() { _ = rows.Close() }()

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
			return nil, fmt.Errorf("scan available: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func stringPtrOrNil(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func boolPtrOrNil(p *bool) interface{} {
	if p == nil {
		return nil
	}
	return *p
}
