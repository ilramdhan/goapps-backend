package postgres

import (
	"context"
	"database/sql"
	"fmt"

	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	domainmbhead "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBRecipeFullExportRepository serves the P12 denormalized full-recipe export (items C1 + C2).
//
// ⛔ READ-ONLY BY CONTRACT. This type owns exactly one SELECT and holds no write method.
// cst_mb_cost and cost_product_master are joined for display only; the export must never
// mutate a cost row.
type MBRecipeFullExportRepository struct {
	db *DB
}

// NewMBRecipeFullExportRepository creates a new MBRecipeFullExportRepository.
func NewMBRecipeFullExportRepository(db *DB) *MBRecipeFullExportRepository {
	return &MBRecipeFullExportRepository{db: db}
}

var _ appmbhead.RecipeFullReader = (*MBRecipeFullExportRepository)(nil)

// recipeFullQuery denormalizes head → composition (LEFT JOIN, so a head with no composition
// rows still yields exactly one row) → cst_mb_cost (LEFT JOIN LATERAL, at most ONE cost row
// per head so the row count stays n_composition, not n_composition × n_cost_type).
//
// $1 = is_active filter or NULL (NULL disables the filter)
// $2 = period or ” (empty resolves to the latest active period for that head)
// $3 = cost type (already defaulted by the application layer)
// $4 = derived check status filter, or ” (empty disables the filter — ALL rows,
//
//	NULL-status heads INCLUDED, which is the pre-filter behavior byte for byte).
//	A non-empty value necessarily EXCLUDES NULL heads: `NULL = 'Approved'` is NULL,
//	never TRUE. No cast is needed — mbh_check_status_calc is VARCHAR(50) and $4 is
//	passed as a Go string, so both sides are already text.
//	⚠ Only THREE of the six values the CHECK constraint allows are produced by
//	DeriveCheckStatus today (Boughtout, Approved, Waiting). Filtering by "Current",
//	"Outdated" or "Rejected" is valid SQL but returns ZERO ROWS until the matching
//	user gates are decided. ⛔ Not a bug.
//
// $5 = IncludeRejected flag (bool). false (default) excludes heads whose workflow
//
//	status (h.mbh_entry_status, NOT h.mbh_check_status_calc — the two are unrelated,
//	see mbhead.ExportFilter's doc comment) is $6. ⭐ §11 item 140: this predicate
//	previously did not exist at all, so a REJECTED head leaked into every export.
//
// $6 = the literal workflow-status value to exclude when $5 is false, passed as a
//
//	parameter (mbhead.StatusRejected) rather than interpolated into the SQL text.
const recipeFullQuery = `
SELECT
    h.mbh_mb_costing,
    h.mbh_mgt_name,
    h.mbh_code,
    COALESCE(h.mbh_dev_code, ''),
    h.mbh_vs_number,
    h.mbh_no_of_process,
    COALESCE(h.mbh_shade_code, ''),
    COALESCE(h.mbh_shade_name, ''),
    s2.mbhs_shade_code,
    s2.mbhs_shade_name,
    s3.mbhs_shade_code,
    s3.mbhs_shade_name,
    h.mbh_denier,
    h.mbh_filament,
    COALESCE(h.mbh_cross_section, ''),
    COALESCE(h.mbh_lusture_code, ''),
    h.mbh_ldr_prsn,
    h.mbh_dozing,
    h.mbh_status,
    h.mbh_check_status,
    h.mbh_check_status_calc,
    h.mbh_final_product,
    h.mbh_is_boughtout,
    h.mbh_entry_status,
    c.mbcm_seq_no,
    c.mbcm_source_type,
    g.group_code,
    ref.mbh_mb_costing,
    c.mbcm_composition_pct::text,
    c.mbcm_is_carrier,
    mbc.mbc_period,
    mbc.mbc_cost_type,
    mbc.mbc_cost_value::text,
    to_char(mbc.mbc_pushed_at, 'YYYY-MM-DD HH24:MI:SS'),
    COALESCE(h.mbh_cost_product_id, 0),
    p.cpm_product_code,
    h.mbh_cost_generated_at
FROM mst_mb_head h
LEFT JOIN mst_mb_head_shade s2
       ON s2.mbhs_mbh_id = h.mbh_id AND s2.mbhs_seq_no = 1 AND s2.deleted_at IS NULL
LEFT JOIN mst_mb_head_shade s3
       ON s3.mbhs_mbh_id = h.mbh_id AND s3.mbhs_seq_no = 2 AND s3.deleted_at IS NULL
LEFT JOIN mst_mb_composition c
       ON c.mbcm_mbh_id = h.mbh_id AND c.deleted_at IS NULL
LEFT JOIN cst_rm_group_head g
       ON g.group_head_id = c.mbcm_group_head_id
LEFT JOIN mst_mb_head ref
       ON ref.mbh_id = c.mbcm_mb_ref_mbh_id
LEFT JOIN cost_product_master p
       ON p.cpm_product_sys_id = h.mbh_cost_product_id
LEFT JOIN LATERAL (
    SELECT x.mbc_period, x.mbc_cost_type, x.mbc_cost_value, x.mbc_pushed_at
    FROM cst_mb_cost x
    WHERE x.mbc_mbh_id = h.mbh_id
      AND x.mbc_is_active = TRUE
      AND x.mbc_cost_type = $3
      AND ($2 = '' OR x.mbc_period = $2)
    ORDER BY x.mbc_period DESC
    LIMIT 1
) mbc ON TRUE
WHERE h.deleted_at IS NULL
  AND ($1::boolean IS NULL OR h.mbh_is_active = $1::boolean)
  AND ($4 = '' OR h.mbh_check_status_calc = $4)
  AND ($5::boolean OR h.mbh_entry_status != $6)
ORDER BY h.mbh_mb_costing ASC, c.mbcm_seq_no ASC NULLS FIRST`

// ListRecipeFullRows executes the denormalized read. One row per composition line; heads
// without composition rows yield one row with the composition columns left absent.
func (r *MBRecipeFullExportRepository) ListRecipeFullRows(
	ctx context.Context, filter appmbhead.RecipeFullFilter,
) ([]appmbhead.RecipeFullRow, error) {
	var active sql.NullBool
	if filter.IsActive != nil {
		active = sql.NullBool{Bool: *filter.IsActive, Valid: true}
	}

	// ⚠ Argument order MUST match the $N placeholders: $1 active, $2 period, $3 cost
	// type, $4 derived check status, $5 include-rejected flag, $6 the workflow-status
	// literal excluded when $5 is false.
	rows, err := r.db.QueryContext(
		ctx, recipeFullQuery, active, filter.Period, filter.CostType, filter.CheckStatusCalc,
		filter.IncludeRejected, domainmbhead.StatusRejected,
	)
	if err != nil {
		return nil, fmt.Errorf("mb_recipe_full_export_repository: list rows: %w", err)
	}
	defer closeRows(rows)

	var out []appmbhead.RecipeFullRow
	for rows.Next() {
		row, scanErr := scanRecipeFullRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_recipe_full_export_repository: iterate rows: %w", err)
	}
	return out, nil
}

// recipeFullDTO mirrors recipeFullQuery's column list one-for-one. Every nullable column is a
// sql.Null* so absence survives into the domain row as nil rather than collapsing to a zero
// value (D13).
type recipeFullDTO struct {
	MBCosting                        string
	MgtName, Code                    sql.NullString
	DevCode                          string
	VSNumber, NoOfProcess            sql.NullString
	ShadeCode, ShadeName             string
	Shade2Code, Shade2Name           sql.NullString
	Shade3Code, Shade3Name           sql.NullString
	Denier                           sql.NullFloat64
	Filament                         sql.NullInt64
	CrossSection, LustureCode        string
	LdrPct, Dozing                   sql.NullFloat64
	Status, CheckStatusLegacy        sql.NullString
	CheckStatusCalc, FinalProduct    sql.NullString
	IsBoughtout                      bool
	EntryStatus                      string
	CompSeqNo                        sql.NullInt64
	CompSourceType, CompGroupCode    sql.NullString
	CompMBRefCosting, CompPct        sql.NullString
	CompIsCarrier                    sql.NullBool
	CostPeriod, CostType             sql.NullString
	CostValue, CostPushedAt          sql.NullString
	CostProductSysID                 int64
	CostProductCode, CostGeneratedAt sql.NullString
}

func scanRecipeFullRow(rows *sql.Rows) (appmbhead.RecipeFullRow, error) {
	var d recipeFullDTO
	err := rows.Scan(
		&d.MBCosting, &d.MgtName, &d.Code, &d.DevCode, &d.VSNumber, &d.NoOfProcess,
		&d.ShadeCode, &d.ShadeName,
		&d.Shade2Code, &d.Shade2Name, &d.Shade3Code, &d.Shade3Name,
		&d.Denier, &d.Filament, &d.CrossSection, &d.LustureCode,
		&d.LdrPct, &d.Dozing, &d.Status, &d.CheckStatusLegacy, &d.CheckStatusCalc,
		&d.FinalProduct, &d.IsBoughtout, &d.EntryStatus,
		&d.CompSeqNo, &d.CompSourceType, &d.CompGroupCode, &d.CompMBRefCosting,
		&d.CompPct, &d.CompIsCarrier,
		&d.CostPeriod, &d.CostType, &d.CostValue, &d.CostPushedAt,
		&d.CostProductSysID, &d.CostProductCode, &d.CostGeneratedAt,
	)
	if err != nil {
		return appmbhead.RecipeFullRow{}, fmt.Errorf("mb_recipe_full_export_repository: scan row: %w", err)
	}
	return d.toRow(), nil
}

func (d *recipeFullDTO) toRow() appmbhead.RecipeFullRow {
	row := appmbhead.RecipeFullRow{
		MBCosting:         d.MBCosting,
		MgtName:           nullStrPtr(d.MgtName),
		Code:              nullStrPtr(d.Code),
		DevCode:           d.DevCode,
		VSNumber:          nullStrPtr(d.VSNumber),
		NoOfProcess:       nullStrPtr(d.NoOfProcess),
		ShadeCode:         d.ShadeCode,
		ShadeName:         d.ShadeName,
		Shade2Code:        nullStrPtr(d.Shade2Code),
		Shade2Name:        nullStrPtr(d.Shade2Name),
		Shade3Code:        nullStrPtr(d.Shade3Code),
		Shade3Name:        nullStrPtr(d.Shade3Name),
		Denier:            nullFloatPtr(d.Denier),
		Filament:          nullInt32Ptr(d.Filament),
		CrossSection:      d.CrossSection,
		LustureCode:       d.LustureCode,
		LdrPct:            nullFloatPtr(d.LdrPct),
		Dozing:            nullFloatPtr(d.Dozing),
		Status:            nullStrPtr(d.Status),
		CheckStatusLegacy: nullStrPtr(d.CheckStatusLegacy),
		CheckStatusCalc:   nullStrPtr(d.CheckStatusCalc),
		FinalProduct:      nullStrPtr(d.FinalProduct),
		IsBoughtout:       d.IsBoughtout,
		EntryStatus:       d.EntryStatus,
		CompSourceType:    nullStrPtr(d.CompSourceType),
		CompRMGroupCode:   nullStrPtr(d.CompGroupCode),
		CompMBRefCosting:  nullStrPtr(d.CompMBRefCosting),
		CompPct:           nullStrPtr(d.CompPct),
		CostPeriod:        nullStrPtr(d.CostPeriod),
		CostType:          nullStrPtr(d.CostType),
		CostValue:         nullStrPtr(d.CostValue),
		CostPushedAt:      nullStrPtr(d.CostPushedAt),
		CostProductSysID:  d.CostProductSysID,
		CostProductCode:   nullStrPtr(d.CostProductCode),
		CostGeneratedAt:   nullStrPtr(d.CostGeneratedAt),
	}
	if d.CompSeqNo.Valid {
		v := safeconv.Int64ToInt32(d.CompSeqNo.Int64)
		row.CompSeqNo = &v
	}
	row.CompIsCarrier = nullBoolPtr(d.CompIsCarrier)
	return row
}

// nullInt32Ptr converts a nullable int64 column into an *int. Uniquely named because
// nullStrPtr/nullFloatPtr/nullBoolPtr already exist package-wide and are reused here.
func nullInt32Ptr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}
