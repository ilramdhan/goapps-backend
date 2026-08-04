package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// CostMasterLookupRepository provides read-only, lightweight master projections
// consumed by the PPC service over the CostMasterLookupService gRPC contract.
// It reuses finance master tables (cost_product_master, cost_product_type,
// cost_route_*, mst_product_grade, mst_parameter, cost_product_parameter) and
// returns flat DTOs — it never mutates and adds no new tables.
type CostMasterLookupRepository struct{ db *DB }

// NewCostMasterLookupRepository constructs the lookup repository.
func NewCostMasterLookupRepository(db *DB) *CostMasterLookupRepository {
	return &CostMasterLookupRepository{db: db}
}

// ErrLookupNotFound is returned by single-row lookups when no matching row
// exists (the handler maps it to a NOT_FOUND BaseResponse).
var ErrLookupNotFound = errors.New("lookup: not found")

// LookupProduct is the flat product projection (cost_product_master + type join).
type LookupProduct struct {
	ProductSysID    int64
	ProductCode     string
	ProductTypeID   int32
	ProductTypeCode string
	ProductTypeName string
	ProductName     string
	ShadeCode       string
	ShadeName       string
	GradeCode       string
	ErpItemCode     string
	ErpGradeCode1   string
	ErpGradeCode2   string
	IsActive        bool
}

// LookupGrade is the flat product-grade projection (mst_product_grade).
type LookupGrade struct {
	PgID         string
	PgCode       string
	PgName       string
	PgGradeLabel string
	IsActive     bool
}

// LookupParamDef is the flat parameter-definition projection (mst_parameter).
type LookupParamDef struct {
	ParamID              string
	ParamCode            string
	ParamName            string
	ParamShortName       string
	DataType             string
	ParamCategory        string
	DisplayGroup         string
	LookupMasterCode     string
	UomID                string
	UomCode              string
	DefaultValue         string
	MinValue             string
	MaxValue             string
	DisplayOrder         int32
	IsRequiredForCosting bool
	IsActive             bool
}

// LookupParamValue is the flat per-product typed parameter value
// (cost_product_parameter + mst_parameter join for code/data_type).
type LookupParamValue struct {
	ProductSysID int64
	ParamID      string
	ParamCode    string
	DataType     string
	ValueNumeric string
	ValueText    string
	ValueFlag    bool
}

// LookupRouteHead is the released route head projection (cost_route_head).
type LookupRouteHead struct {
	HeadID        int64
	ProductSysID  int64
	ProductCode   string
	RoutingStatus string
	Version       int32
}

// LookupRouteStage is a route stage projection (cost_route_seq).
type LookupRouteStage struct {
	SeqID          int64
	ProductSysID   int64
	RouteLevel     int32
	RouteSeq       int32
	RouteName      string
	RouteItemCode  string
	RouteShadeCode string
}

// LookupRouteRm is a route RM-edge projection (cost_route_rm).
type LookupRouteRm struct {
	RmID           int64
	SeqID          int64
	RmType         string
	RmProductSysID int64
	RmItemCode     string
	RmGroupCode    string
	RouteRmRatio   string
	SubType        string
	// Resolved presentation labels (see loadRouteRms) — empty when the master
	// row the edge points at is missing.
	RmCode string
	RmName string
}

const lookupProductCols = `
	cpm.cpm_product_sys_id,
	COALESCE(cpm.cpm_product_code,''),
	cpm.cpm_product_type_id,
	COALESCE(cpt.cpt_type_code,''),
	COALESCE(cpt.cpt_type_name,''),
	COALESCE(cpm.cpm_product_name,''),
	COALESCE(cpm.cpm_shade_code,''),
	COALESCE(cpm.cpm_shade_name,''),
	COALESCE(cpm.cpm_grade_code,''),
	COALESCE(cpm.cpm_erp_item_code,''),
	COALESCE(cpm.cpm_erp_grade_code_1,''),
	COALESCE(cpm.cpm_erp_grade_code_2,''),
	cpm.cpm_is_active`

const lookupProductFrom = `
	FROM cost_product_master cpm
	LEFT JOIN cost_product_type cpt ON cpt.cpt_type_id = cpm.cpm_product_type_id`

func scanLookupProduct(rows *sql.Rows) (LookupProduct, error) {
	var p LookupProduct
	err := rows.Scan(
		&p.ProductSysID, &p.ProductCode, &p.ProductTypeID,
		&p.ProductTypeCode, &p.ProductTypeName, &p.ProductName,
		&p.ShadeCode, &p.ShadeName, &p.GradeCode,
		&p.ErpItemCode, &p.ErpGradeCode1, &p.ErpGradeCode2, &p.IsActive,
	)
	return p, err
}

// GetProduct returns one product projection by sys id. Returns ErrLookupNotFound
// when the product does not exist (caller maps to a NOT_FOUND BaseResponse).
func (r *CostMasterLookupRepository) GetProduct(ctx context.Context, productSysID int64) (*LookupProduct, error) {
	q := `SELECT` + lookupProductCols + lookupProductFrom + `
		WHERE cpm.cpm_product_sys_id = $1`
	rows, err := r.db.QueryContext(ctx, q, productSysID)
	if err != nil {
		return nil, fmt.Errorf("query product lookup: %w", err)
	}
	defer closeRows(rows)
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("scan product lookup: %w", err)
		}
		return nil, ErrLookupNotFound
	}
	p, err := scanLookupProduct(rows)
	if err != nil {
		return nil, fmt.Errorf("scan product lookup: %w", err)
	}
	return &p, nil
}

// BatchGetProducts resolves many product sys ids at once. Missing ids are simply
// absent from the result (order not guaranteed).
func (r *CostMasterLookupRepository) BatchGetProducts(ctx context.Context, ids []int64) ([]LookupProduct, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := `SELECT` + lookupProductCols + lookupProductFrom + `
		WHERE cpm.cpm_product_sys_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query product batch lookup: %w", err)
	}
	defer closeRows(rows)
	out := make([]LookupProduct, 0, len(ids))
	for rows.Next() {
		p, scanErr := scanLookupProduct(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan product batch lookup: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product batch lookup: %w", err)
	}
	return out, nil
}

// ResolveByErpCodes returns every product whose (erp_item_code, shade_code)
// pair matches one of the requested pairs. Matching is trimmed and
// case-insensitive on both components; a NULL shade code is treated as empty.
// Grouping the rows into unique / ambiguous / not-found outcomes is the
// caller's job — a product may legitimately appear more than once per pair.
func (r *CostMasterLookupRepository) ResolveByErpCodes(ctx context.Context, itemCodes, shadeCodes []string) ([]LookupProduct, error) {
	if len(itemCodes) == 0 || len(itemCodes) != len(shadeCodes) {
		return nil, nil
	}
	q := `SELECT` + lookupProductCols + lookupProductFrom + `
		WHERE (UPPER(TRIM(COALESCE(cpm.cpm_erp_item_code,''))), UPPER(TRIM(COALESCE(cpm.cpm_shade_code,''))))
		      IN (SELECT UPPER(TRIM(t.i)), UPPER(TRIM(t.s))
		          FROM UNNEST($1::text[], $2::text[]) AS t(i, s))`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(itemCodes), pq.Array(shadeCodes))
	if err != nil {
		return nil, fmt.Errorf("query product erp-code resolve: %w", err)
	}
	defer closeRows(rows)
	out := make([]LookupProduct, 0, len(itemCodes))
	for rows.Next() {
		p, scanErr := scanLookupProduct(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan product erp-code resolve: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product erp-code resolve: %w", err)
	}
	return out, nil
}

// ListProducts returns a paginated product projection with search + filters.
func (r *CostMasterLookupRepository) ListProducts(
	ctx context.Context, page, pageSize int32, search string, productTypeID int32, shadeCode, activeFilter string,
) ([]LookupProduct, int64, error) {
	var conds []string
	var args []any
	i := 1
	if s := strings.TrimSpace(search); s != "" {
		conds = append(conds, fmt.Sprintf("(cpm.cpm_product_code ILIKE $%d OR cpm.cpm_product_name ILIKE $%d)", i, i+1))
		args = append(args, "%"+s+"%", "%"+s+"%")
		i += 2
	}
	if productTypeID > 0 {
		conds = append(conds, fmt.Sprintf("cpm.cpm_product_type_id = $%d", i))
		args = append(args, productTypeID)
		i++
	}
	if sc := strings.TrimSpace(shadeCode); sc != "" {
		conds = append(conds, fmt.Sprintf("cpm.cpm_shade_code = $%d", i))
		args = append(args, sc)
		i++
	}
	switch activeFilter {
	case filterActive:
		conds = append(conds, "cpm.cpm_is_active = TRUE")
	case filterInactive:
		conds = append(conds, "cpm.cpm_is_active = FALSE")
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	countQ := `SELECT COUNT(*)` + lookupProductFrom + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count product lookup: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * pageSize
	listQ := `SELECT` + lookupProductCols + lookupProductFrom + where +
		fmt.Sprintf(" ORDER BY cpm.cpm_product_code LIMIT $%d OFFSET $%d", i, i+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query product lookup list: %w", err)
	}
	defer closeRows(rows)
	out := make([]LookupProduct, 0, pageSize)
	for rows.Next() {
		p, scanErr := scanLookupProduct(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan product lookup list: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate product lookup list: %w", err)
	}
	return out, total, nil
}

// GetReleasedRoute returns the latest COMPLETE/LOCKED route head for a product
// together with its stages and RM edges. Returns (nil, nil) when none exists.
func (r *CostMasterLookupRepository) GetReleasedRoute(ctx context.Context, productSysID int64) (*LookupRouteHead, []LookupRouteStage, []LookupRouteRm, error) {
	const headQ = `
		SELECT crh.crh_head_id, crh.crh_product_sys_id,
		       COALESCE(cpm.cpm_product_code,''), COALESCE(crh.crh_routing_status,''), crh.crh_version
		FROM cost_route_head crh
		LEFT JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
		WHERE crh.crh_product_sys_id = $1
		  AND crh.crh_deleted_at IS NULL
		  AND crh.crh_routing_status IN ('COMPLETE','LOCKED')
		ORDER BY crh.crh_version DESC
		LIMIT 1`
	var head LookupRouteHead
	err := r.db.QueryRowContext(ctx, headQ, productSysID).Scan(
		&head.HeadID, &head.ProductSysID, &head.ProductCode, &head.RoutingStatus, &head.Version,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("query route head: %w", err)
	}

	stages, err := r.loadRouteStages(ctx, head.HeadID)
	if err != nil {
		return nil, nil, nil, err
	}
	rms, err := r.loadRouteRms(ctx, head.HeadID)
	if err != nil {
		return nil, nil, nil, err
	}
	return &head, stages, rms, nil
}

func (r *CostMasterLookupRepository) loadRouteStages(ctx context.Context, headID int64) ([]LookupRouteStage, error) {
	const q = `
		SELECT crs_seq_id, crs_product_sys_id, crs_route_level, crs_route_seq,
		       COALESCE(crs_route_name,''), COALESCE(crs_route_item_code,''), COALESCE(crs_route_shade_code,'')
		FROM cost_route_seq
		WHERE crs_head_id = $1 AND crs_deleted_at IS NULL
		ORDER BY crs_route_level, crs_route_seq`
	rows, err := r.db.QueryContext(ctx, q, headID)
	if err != nil {
		return nil, fmt.Errorf("query route stages: %w", err)
	}
	defer closeRows(rows)
	var out []LookupRouteStage
	for rows.Next() {
		var s LookupRouteStage
		if scanErr := rows.Scan(&s.SeqID, &s.ProductSysID, &s.RouteLevel, &s.RouteSeq, &s.RouteName, &s.RouteItemCode, &s.RouteShadeCode); scanErr != nil {
			return nil, fmt.Errorf("scan route stage: %w", scanErr)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route stages: %w", err)
	}
	return out, nil
}

func (r *CostMasterLookupRepository) loadRouteRms(ctx context.Context, headID int64) ([]LookupRouteRm, error) {
	// rm_code / rm_name are resolved here rather than by the caller: the master
	// row to join depends on crm_rm_type, and consumers (PPC) live in a separate
	// database with no way to make that join themselves. LEFT JOINs so a route
	// edge pointing at a deleted master still yields the edge with blank labels
	// instead of vanishing from the route.
	const q = `
		SELECT crm.crm_rm_id, crm.crm_seq_id, COALESCE(crm.crm_rm_type,''),
		       COALESCE(crm.crm_rm_product_sys_id,0), COALESCE(crm.crm_rm_item_code,''),
		       COALESCE(crm.crm_rm_group_code,''), crm.crm_route_rm_ratio, COALESCE(crm.crm_sub_type,''),
		       CASE crm.crm_rm_type
		           WHEN 'PRODUCT' THEN COALESCE(cpm.cpm_product_code,'')
		           WHEN 'ITEM'    THEN COALESCE(crm.crm_rm_item_code,'')
		           WHEN 'GROUP'   THEN COALESCE(crm.crm_rm_group_code,'')
		           ELSE ''
		       END AS rm_code,
		       CASE crm.crm_rm_type
		           WHEN 'PRODUCT' THEN COALESCE(cpm.cpm_product_name,'')
		           WHEN 'ITEM'    THEN COALESCE(cei.cei_item_name,'')
		           WHEN 'GROUP'   THEN COALESCE(rgh.group_name,'')
		           ELSE ''
		       END AS rm_name
		FROM cost_route_rm crm
		JOIN cost_route_seq crs ON crs.crs_seq_id = crm.crm_seq_id
		LEFT JOIN cost_product_master cpm
		       ON crm.crm_rm_type = 'PRODUCT' AND cpm.cpm_product_sys_id = crm.crm_rm_product_sys_id
		LEFT JOIN cost_erp_item cei
		       ON crm.crm_rm_type = 'ITEM' AND cei.cei_item_code = crm.crm_rm_item_code
		LEFT JOIN cst_rm_group_head rgh
		       ON crm.crm_rm_type = 'GROUP' AND rgh.group_code = crm.crm_rm_group_code
		      AND rgh.deleted_at IS NULL
		WHERE crs.crs_head_id = $1 AND crs.crs_deleted_at IS NULL
		ORDER BY crm.crm_seq_id, crm.crm_rm_id`
	rows, err := r.db.QueryContext(ctx, q, headID)
	if err != nil {
		return nil, fmt.Errorf("query route rms: %w", err)
	}
	defer closeRows(rows)
	var out []LookupRouteRm
	for rows.Next() {
		var rm LookupRouteRm
		var ratio sql.NullFloat64
		if scanErr := rows.Scan(
			&rm.RmID, &rm.SeqID, &rm.RmType, &rm.RmProductSysID,
			&rm.RmItemCode, &rm.RmGroupCode, &ratio, &rm.SubType,
			&rm.RmCode, &rm.RmName,
		); scanErr != nil {
			return nil, fmt.Errorf("scan route rm: %w", scanErr)
		}
		rm.RouteRmRatio = formatNullFloat(ratio)
		out = append(out, rm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route rms: %w", err)
	}
	return out, nil
}

// ListGrades returns a paginated product-grade projection.
func (r *CostMasterLookupRepository) ListGrades(
	ctx context.Context, page, pageSize int32, search, activeFilter string,
) ([]LookupGrade, int64, error) {
	conds := []string{"deleted_at IS NULL"}
	var args []any
	i := 1
	if s := strings.TrimSpace(search); s != "" {
		conds = append(conds, fmt.Sprintf("(pg_code ILIKE $%d OR pg_name ILIKE $%d)", i, i+1))
		args = append(args, "%"+s+"%", "%"+s+"%")
		i += 2
	}
	switch activeFilter {
	case filterActive:
		conds = append(conds, "is_active = TRUE")
	case filterInactive:
		conds = append(conds, "is_active = FALSE")
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mst_product_grade`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count grade lookup: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * pageSize
	q := `SELECT pg_id, COALESCE(pg_code,''), COALESCE(pg_name,''), COALESCE(pg_grade_label,''), is_active
		FROM mst_product_grade` + where +
		fmt.Sprintf(" ORDER BY pg_code LIMIT $%d OFFSET $%d", i, i+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query grade lookup: %w", err)
	}
	defer closeRows(rows)
	out := make([]LookupGrade, 0, pageSize)
	for rows.Next() {
		var g LookupGrade
		if scanErr := rows.Scan(&g.PgID, &g.PgCode, &g.PgName, &g.PgGradeLabel, &g.IsActive); scanErr != nil {
			return nil, 0, fmt.Errorf("scan grade lookup: %w", scanErr)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate grade lookup: %w", err)
	}
	return out, total, nil
}

// ListParameterDefs returns a paginated mst_parameter projection, filterable by display_group.
func (r *CostMasterLookupRepository) ListParameterDefs(
	ctx context.Context, page, pageSize int32, search, displayGroup, activeFilter string,
) ([]LookupParamDef, int64, error) {
	conds := []string{"p.deleted_at IS NULL"}
	var args []any
	i := 1
	if s := strings.TrimSpace(search); s != "" {
		conds = append(conds, fmt.Sprintf("(p.param_code ILIKE $%d OR p.param_name ILIKE $%d)", i, i+1))
		args = append(args, "%"+s+"%", "%"+s+"%")
		i += 2
	}
	if dg := strings.TrimSpace(displayGroup); dg != "" {
		conds = append(conds, fmt.Sprintf("p.display_group = $%d", i))
		args = append(args, dg)
		i++
	}
	switch activeFilter {
	case filterActive:
		conds = append(conds, "p.is_active = TRUE")
	case filterInactive:
		conds = append(conds, "p.is_active = FALSE")
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	const from = `
		FROM mst_parameter p
		LEFT JOIN mst_uom u ON u.uom_id = p.uom_id`

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count parameter lookup: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * pageSize
	q := `SELECT
		p.id, COALESCE(p.param_code,''), COALESCE(p.param_name,''), COALESCE(p.param_short_name,''),
		COALESCE(p.data_type,''), COALESCE(p.param_category,''), COALESCE(p.display_group,''),
		COALESCE(p.lookup_master_code,''), COALESCE(p.uom_id::text,''), COALESCE(u.uom_code,''),
		p.default_value, p.min_value, p.max_value,
		COALESCE(p.display_order,0), COALESCE(p.is_required_for_costing,FALSE), p.is_active` +
		from + where +
		fmt.Sprintf(" ORDER BY COALESCE(p.display_order,0), p.param_code LIMIT $%d OFFSET $%d", i, i+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query parameter lookup: %w", err)
	}
	defer closeRows(rows)
	out := make([]LookupParamDef, 0, pageSize)
	for rows.Next() {
		var d LookupParamDef
		var def, minV, maxV sql.NullFloat64
		if scanErr := rows.Scan(
			&d.ParamID, &d.ParamCode, &d.ParamName, &d.ParamShortName,
			&d.DataType, &d.ParamCategory, &d.DisplayGroup,
			&d.LookupMasterCode, &d.UomID, &d.UomCode,
			&def, &minV, &maxV,
			&d.DisplayOrder, &d.IsRequiredForCosting, &d.IsActive,
		); scanErr != nil {
			return nil, 0, fmt.Errorf("scan parameter lookup: %w", scanErr)
		}
		d.DefaultValue = formatNullFloat(def)
		d.MinValue = formatNullFloat(minV)
		d.MaxValue = formatNullFloat(maxV)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate parameter lookup: %w", err)
	}
	return out, total, nil
}

// BatchGetParameterValues returns cost_product_parameter typed values for a set
// of products, optionally narrowed to specific param ids.
func (r *CostMasterLookupRepository) BatchGetParameterValues(
	ctx context.Context, productSysIDs []int64, paramIDs []string,
) ([]LookupParamValue, error) {
	if len(productSysIDs) == 0 {
		return nil, nil
	}
	conds := []string{"cpp.cpp_product_sys_id = ANY($1)"}
	args := []any{pq.Array(productSysIDs)}
	if len(paramIDs) > 0 {
		conds = append(conds, "cpp.cpp_param_id = ANY($2)")
		args = append(args, pq.Array(paramIDs))
	}
	q := `SELECT
		cpp.cpp_product_sys_id, cpp.cpp_param_id::text,
		COALESCE(p.param_code,''), COALESCE(p.data_type,''),
		cpp.cpp_value_numeric, COALESCE(cpp.cpp_value_text,''), COALESCE(cpp.cpp_value_flag,FALSE)
		FROM cost_product_parameter cpp
		LEFT JOIN mst_parameter p ON p.id = cpp.cpp_param_id
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY cpp.cpp_product_sys_id, cpp.cpp_param_id`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query parameter values: %w", err)
	}
	defer closeRows(rows)
	var out []LookupParamValue
	for rows.Next() {
		var v LookupParamValue
		var num sql.NullFloat64
		if scanErr := rows.Scan(&v.ProductSysID, &v.ParamID, &v.ParamCode, &v.DataType, &num, &v.ValueText, &v.ValueFlag); scanErr != nil {
			return nil, fmt.Errorf("scan parameter value: %w", scanErr)
		}
		v.ValueNumeric = formatNullFloat(num)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate parameter values: %w", err)
	}
	return out, nil
}

// formatNullFloat renders a nullable numeric as a plain decimal string,
// returning "" for NULL. Matches the decimal-as-string transport convention.
func formatNullFloat(v sql.NullFloat64) string {
	if !v.Valid {
		return ""
	}
	return strconv.FormatFloat(v.Float64, 'f', -1, 64)
}
