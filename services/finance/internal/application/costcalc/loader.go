package costcalc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// loaderKind* constants identify a bulk loader stage for metrics labels.
const (
	loaderKindProducts = "products"
	loaderKindRoutes   = "routes"
	loaderKindCAPP     = "capp"
	loaderKindFormulas = "formulas"
	loaderKindRMCosts  = "rmcosts"
	loaderKindUpstream = "upstream"
)

// observeLoad observes bulk loader latency under the given kind label.
func observeLoad(kind string, start time.Time) {
	metrics.BulkLoadSeconds.WithLabelValues(kind).Observe(time.Since(start).Seconds())
}

// ProductLoader bulk-loads everything computeProduct needs for a chunk of products.
// All methods MUST be safe to call concurrently across chunks; the default
// implementation only reads from the connection pool.
type ProductLoader interface {
	LoadProducts(ctx context.Context, ids []int64) (map[int64]*costproductmaster.CostProductMaster, error)
	LoadRoutesByProducts(ctx context.Context, productSysIDs []int64) (map[int64]*costroute.Graph, error)
	LoadCAPP(ctx context.Context, productSysIDs []int64) (map[int64]map[string]float64, error)
	LoadFormulas(ctx context.Context, productSysIDs []int64) (map[int64][]Formula, error)
	LoadRMCosts(ctx context.Context, itemCodes []string, period string, calcType string) (map[string]float64, error)
	LoadUpstreamCosts(ctx context.Context, productSysIDs []int64, period, calcType string) (map[int64]float64, error)
}

type productLoader struct {
	db *sql.DB
}

// NewProductLoader constructs the default bulk loader implementation.
func NewProductLoader(db *sql.DB) ProductLoader {
	return &productLoader{db: db}
}

// =============================================================================
// LoadProducts
// =============================================================================

// LoadProducts hydrates cost_product_master rows for the given sys IDs.
func (l *productLoader) LoadProducts(ctx context.Context, ids []int64) (map[int64]*costproductmaster.CostProductMaster, error) {
	defer observeLoad(loaderKindProducts, time.Now())
	out := map[int64]*costproductmaster.CostProductMaster{}
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT cpm_product_sys_id, cpm_product_code, cpm_product_type_id, cpm_product_name,
		       COALESCE(cpm_shade_code, ''), COALESCE(cpm_grade_code, ''), COALESCE(cpm_description, ''),
		       COALESCE(cpm_erp_item_code, ''), COALESCE(cpm_erp_grade_code_1, ''), COALESCE(cpm_erp_grade_code_2, ''),
		       cpm_erp_linked_at, COALESCE(cpm_erp_linked_by, ''),
		       cpm_is_active,
		       cpm_created_at, cpm_created_by, cpm_updated_at, COALESCE(cpm_updated_by, ''),
		       COALESCE(cpm_shade_name,''), COALESCE(cpm_flex_01,''), COALESCE(cpm_flex_02,''), COALESCE(cpm_flex_03,'')
		FROM cost_product_master
		WHERE cpm_product_sys_id = ANY($1)`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("load products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			sysID        int64
			code         string
			typeID       int32
			name         string
			shade, grade string
			desc         string
			erpItem      string
			erpG1, erpG2 string
			erpAt        sql.NullTime
			erpBy        string
			active       bool
			createdAt    time.Time
			createdBy    string
			updatedAt    time.Time
			updatedBy    string
			shadeName    string
			flex01       string
			flex02       string
			flex03       string
		)
		if scanErr := rows.Scan(
			&sysID, &code, &typeID, &name, &shade, &grade, &desc,
			&erpItem, &erpG1, &erpG2, &erpAt, &erpBy,
			&active, &createdAt, &createdBy, &updatedAt, &updatedBy,
			&shadeName, &flex01, &flex02, &flex03,
		); scanErr != nil {
			return nil, fmt.Errorf("scan product row: %w", scanErr)
		}
		var erpAtPtr *time.Time
		if erpAt.Valid {
			t := erpAt.Time
			erpAtPtr = &t
		}
		out[sysID] = costproductmaster.Reconstruct(
			sysID, code, typeID, name, shade, grade, desc,
			erpItem, erpG1, erpG2, erpAtPtr, erpBy,
			active, createdAt, createdBy, updatedAt, updatedBy,
			shadeName, flex01, flex02, flex03,
		)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product rows: %w", err)
	}
	return out, nil
}

// =============================================================================
// LoadRoutesByProducts
// =============================================================================

// LoadRoutesByProducts returns the active route Graph keyed by the REQUESTED
// product sys id. Two cases (self-contained DAG model, migration 000244):
//
//  1. Requested product is the head of its own route_head (FG-level products).
//  2. Requested product is an intermediate that exists only as a seq inside
//     another FG's route_head — return the SAME owning Graph keyed by the
//     intermediate's product_sys_id (so the engine's `routes[productID]`
//     lookup works for intermediates too).
//
// Same product_sys_id may appear in multiple FGs' routes; we pick any one
// (DISTINCT ON arbitrary tiebreak) — engine only reads the seq matching this
// product, so all sibling intermediates are present + correct in either graph.
func (l *productLoader) LoadRoutesByProducts(ctx context.Context, productSysIDs []int64) (map[int64]*costroute.Graph, error) { //nolint:gocognit,gocyclo // single-pass bulk row assembly
	defer observeLoad(loaderKindRoutes, time.Now())
	out := map[int64]*costroute.Graph{}
	if len(productSysIDs) == 0 {
		return out, nil
	}

	// 1. Resolve each requested product to its owning head_id (via crh OR crs).
	const resolveQ = `
		SELECT DISTINCT ON (product_sys_id) product_sys_id, head_id
		FROM (
		  SELECT crh.crh_product_sys_id AS product_sys_id, crh.crh_head_id AS head_id, 0 AS rank
		  FROM cost_route_head crh
		  WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		    AND crh.crh_deleted_at IS NULL
		    AND crh.crh_product_sys_id = ANY($1)
		  UNION ALL
		  SELECT crs.crs_product_sys_id AS product_sys_id, crs.crs_head_id AS head_id, 1 AS rank
		  FROM cost_route_seq crs
		  JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
		  WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		    AND crh.crh_deleted_at IS NULL
		    AND crs.crs_deleted_at IS NULL
		    AND crs.crs_product_sys_id = ANY($1)
		) t
		ORDER BY product_sys_id, rank ASC, head_id DESC`
	rRows, err := l.db.QueryContext(ctx, resolveQ, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("resolve product to head: %w", err)
	}
	productToHead := map[int64]int64{}
	headIDSet := map[int64]struct{}{}
	for rRows.Next() {
		var pid, hid int64
		if scanErr := rRows.Scan(&pid, &hid); scanErr != nil {
			if e := rRows.Close(); e != nil {
				_ = e
			}
			return nil, fmt.Errorf("scan product→head: %w", scanErr)
		}
		productToHead[pid] = hid
		headIDSet[hid] = struct{}{}
	}
	if rErr := rRows.Err(); rErr != nil {
		if e := rRows.Close(); e != nil {
			_ = e
		}
		return nil, fmt.Errorf("iterate product→head: %w", rErr)
	}
	if e := rRows.Close(); e != nil {
		_ = e
	}
	if len(headIDSet) == 0 {
		return out, nil
	}
	headIDs := make([]int64, 0, len(headIDSet))
	for hid := range headIDSet {
		headIDs = append(headIDs, hid)
	}

	// 2. Load full Head rows for those heads.
	const headQ = `
		SELECT crh_head_id, crh_product_sys_id, crh_routing_status, crh_version,
		       COALESCE(crh_promoted_from_draft_id, 0), COALESCE(crh_cyl_type_id, 0),
		       COALESCE(crh_notes, ''),
		       crh_created_at, crh_created_by, crh_updated_at, COALESCE(crh_updated_by, '')
		FROM cost_route_head
		WHERE crh_head_id = ANY($1)
		  AND crh_deleted_at IS NULL`
	headRows, err := l.db.QueryContext(ctx, headQ, pq.Array(headIDs))
	if err != nil {
		return nil, fmt.Errorf("load route heads: %w", err)
	}
	headsByID := map[int64]*costroute.Head{}
	productByHeadID := map[int64]int64{} // FG product for the route — not used in output keying anymore
	scanHeadIDs := make([]int64, 0)
	if err := scanRouteHeads(headRows, headsByID, productByHeadID, &scanHeadIDs); err != nil {
		return nil, err
	}

	// 3. Seqs for those heads.
	seqs, seqsByHeadID, err := l.loadSeqsForHeads(ctx, headIDs)
	if err != nil {
		return nil, err
	}

	// 4. Rms for those heads.
	if err := l.loadRmsForHeads(ctx, headIDs, seqs); err != nil {
		return nil, err
	}

	// 5. Assemble Graphs keyed by REQUESTED product_sys_id (intermediates +
	//    FG point at the same Graph instance for shared head).
	for pid, hid := range productToHead {
		h, ok := headsByID[hid]
		if !ok {
			continue
		}
		out[pid] = &costroute.Graph{
			Head: h,
			Seqs: seqsByHeadID[hid],
		}
	}
	return out, nil
}

func scanRouteHeads(rows *sql.Rows, headsByID map[int64]*costroute.Head, productByHeadID map[int64]int64, headIDs *[]int64) error {
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		h := &costroute.Head{}
		if err := rows.Scan(
			&h.HeadID, &h.ProductSysID, &h.RoutingStatus, &h.Version,
			&h.PromotedFromDraftID, &h.CylTypeID, &h.Notes,
			&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
		); err != nil {
			return fmt.Errorf("scan route head: %w", err)
		}
		headsByID[h.HeadID] = h
		productByHeadID[h.HeadID] = h.ProductSysID
		*headIDs = append(*headIDs, h.HeadID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate route heads: %w", err)
	}
	return nil
}

func (l *productLoader) loadSeqsForHeads(ctx context.Context, headIDs []int64) (map[int64]*costroute.Seq, map[int64][]*costroute.Seq, error) {
	const seqQ = `
		SELECT crs_seq_id, crs_head_id, crs_product_sys_id,
		       crs_route_level, crs_route_seq,
		       COALESCE(crs_route_name, ''), COALESCE(crs_route_item_code, ''),
		       COALESCE(crs_route_shade_code, ''), COALESCE(crs_route_shade_name, ''),
		       crs_position_x, crs_position_y
		FROM cost_route_seq
		WHERE crs_head_id = ANY($1)
		  AND crs_deleted_at IS NULL
		ORDER BY crs_head_id, crs_route_level, crs_route_seq`
	rows, err := l.db.QueryContext(ctx, seqQ, pq.Array(headIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("load route seqs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	seqByID := map[int64]*costroute.Seq{}
	seqsByHeadID := map[int64][]*costroute.Seq{}
	for rows.Next() {
		s := &costroute.Seq{}
		if err := rows.Scan(
			&s.SeqID, &s.HeadID, &s.ProductSysID,
			&s.RouteLevel, &s.RouteSeq,
			&s.RouteName, &s.RouteItemCode, &s.RouteShadeCode, &s.RouteShadeName,
			&s.PositionX, &s.PositionY,
		); err != nil {
			return nil, nil, fmt.Errorf("scan route seq: %w", err)
		}
		seqByID[s.SeqID] = s
		seqsByHeadID[s.HeadID] = append(seqsByHeadID[s.HeadID], s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate route seqs: %w", err)
	}
	return seqByID, seqsByHeadID, nil
}

func (l *productLoader) loadRmsForHeads(ctx context.Context, headIDs []int64, seqsByID map[int64]*costroute.Seq) error {
	const rmQ = `
		SELECT rm.crm_rm_id, rm.crm_seq_id, rm.crm_parent_product_sys_id, rm.crm_rm_type,
		       COALESCE(rm.crm_rm_product_sys_id, 0), COALESCE(rm.crm_rm_item_code, ''), COALESCE(rm.crm_rm_group_code, ''),
		       COALESCE(rm.crm_route_rm_name, ''), COALESCE(rm.crm_route_rm_item_code, ''),
		       COALESCE(rm.crm_route_rm_shade_code, ''), COALESCE(rm.crm_route_rm_shade_name, ''),
		       rm.crm_route_rm_ratio,
		       COALESCE(rm.crm_uom_id, 0), COALESCE(rm.crm_sub_type, ''), COALESCE(rm.crm_notes, '')
		FROM cost_route_rm rm
		JOIN cost_route_seq s ON s.crs_seq_id = rm.crm_seq_id
		WHERE s.crs_head_id = ANY($1)
		ORDER BY rm.crm_seq_id, rm.crm_rm_id`
	rows, err := l.db.QueryContext(ctx, rmQ, pq.Array(headIDs))
	if err != nil {
		return fmt.Errorf("load route rms: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		rm := &costroute.Rm{}
		if err := rows.Scan(
			&rm.RmID, &rm.SeqID, &rm.ParentProductSysID, &rm.RmType,
			&rm.RmProductSysID, &rm.RmItemCode, &rm.RmGroupCode,
			&rm.RouteRmName, &rm.RouteRmItemCode, &rm.RouteRmShadeCode, &rm.RouteRmShadeName,
			&rm.RouteRmRatio,
			&rm.UomID, &rm.SubType, &rm.Notes,
		); err != nil {
			return fmt.Errorf("scan route rm: %w", err)
		}
		if seq, ok := seqsByID[rm.SeqID]; ok {
			seq.Rms = append(seq.Rms, rm)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate route rms: %w", err)
	}
	return nil
}

// =============================================================================
// LoadCAPP
// =============================================================================

// LoadCAPP returns the per-product map of paramCode → numeric value for every
// applicable parameter that has a NUMBER-typed value persisted in
// cost_product_parameter. Missing params simply don't appear in the inner map;
// computeProduct surfaces ErrMissingCAPPValue if a formula references one.
func (l *productLoader) LoadCAPP(ctx context.Context, productSysIDs []int64) (map[int64]map[string]float64, error) {
	defer observeLoad(loaderKindCAPP, time.Now())
	out := map[int64]map[string]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT capp.capp_product_sys_id, mp.param_code, cpp.cpp_value_numeric
		FROM cost_product_applicable_param capp
		JOIN mst_parameter mp ON mp.id = capp.capp_param_id
		JOIN cost_product_parameter cpp
		     ON cpp.cpp_product_sys_id = capp.capp_product_sys_id
		    AND cpp.cpp_param_id = capp.capp_param_id
		WHERE capp.capp_product_sys_id = ANY($1)
		  AND cpp.cpp_value_numeric IS NOT NULL
		  AND mp.deleted_at IS NULL`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load CAPP values: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			paramCode    string
			val          float64
		)
		if err := rows.Scan(&productSysID, &paramCode, &val); err != nil {
			return nil, fmt.Errorf("scan CAPP row: %w", err)
		}
		inner, ok := out[productSysID]
		if !ok {
			inner = map[string]float64{}
			out[productSysID] = inner
		}
		inner[paramCode] = val
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate CAPP rows: %w", err)
	}
	return out, nil
}

// =============================================================================
// LoadFormulas
// =============================================================================

// LoadFormulas returns the set of active formulas (mst_formula + formula_param)
// in topologically-sorted order, keyed per product.
//
// NOTE: Per-product formula applicability filtering is not yet implemented.
// All active formulas apply to every product in productSysIDs. A future
// sub-phase will narrow this based on CAPP / parameter ownership. — S8b.5.
func (l *productLoader) LoadFormulas(ctx context.Context, productSysIDs []int64) (map[int64][]Formula, error) {
	defer observeLoad(loaderKindFormulas, time.Now())
	out := map[int64][]Formula{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	formulas, err := l.loadActiveFormulas(ctx)
	if err != nil {
		return nil, err
	}
	sorted, err := topoSortFormulas(formulas)
	if err != nil {
		return nil, err
	}
	for _, id := range productSysIDs {
		// Each product references the same slice for now; computeProduct is
		// read-only against it. Future per-product filtering will swap in a
		// product-specific subset.
		out[id] = sorted
	}
	return out, nil
}

func (l *productLoader) loadActiveFormulas(ctx context.Context) ([]Formula, error) {
	const q = `
		SELECT f.id, f.formula_code, f.formula_name, f.expression,
		       rp.param_code AS result_param_code
		FROM mst_formula f
		JOIN mst_parameter rp ON rp.id = f.result_param_id
		WHERE f.deleted_at IS NULL
		  AND f.is_active = TRUE
		  AND rp.deleted_at IS NULL`
	rows, err := l.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("load formulas: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	type row struct {
		id          string
		code        string
		name        string
		expr        string
		resultParam string
	}
	heads := []row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.code, &r.name, &r.expr, &r.resultParam); err != nil {
			return nil, fmt.Errorf("scan formula: %w", err)
		}
		heads = append(heads, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate formulas: %w", err)
	}
	if len(heads) == 0 {
		return nil, nil
	}

	inputsByFormulaID, err := l.loadFormulaInputs(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Formula, 0, len(heads))
	for i, r := range heads {
		out = append(out, Formula{
			FormulaCode:     r.code,
			FormulaName:     r.name,
			Expression:      r.expr,
			ResultParamCode: r.resultParam,
			InputParamCodes: inputsByFormulaID[r.id],
			SortOrder:       i,
		})
	}
	return out, nil
}

// loadFormulaInputs returns map[formulaID][]paramCode using a single query.
func (l *productLoader) loadFormulaInputs(ctx context.Context) (map[string][]string, error) {
	const q = `
		SELECT fp.formula_id, p.param_code, fp.sort_order
		FROM formula_param fp
		JOIN mst_parameter p ON p.id = fp.param_id
		WHERE p.deleted_at IS NULL
		ORDER BY fp.formula_id, fp.sort_order`
	rows, err := l.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("load formula inputs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := map[string][]string{}
	for rows.Next() {
		var (
			formulaID string
			paramCode string
			sortOrder int
		)
		if err := rows.Scan(&formulaID, &paramCode, &sortOrder); err != nil {
			return nil, fmt.Errorf("scan formula input: %w", err)
		}
		out[formulaID] = append(out[formulaID], paramCode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate formula inputs: %w", err)
	}
	return out, nil
}

// topoSortFormulas returns formulas in evaluation order (deepest dependencies
// first) using Kahn's algorithm. Returns calcdomain.ErrCycleDetected wrapped
// with context if a cycle is found.
func topoSortFormulas(fs []Formula) ([]Formula, error) { //nolint:gocognit,gocyclo // Kahn topological sort, cohesive
	if len(fs) == 0 {
		return fs, nil
	}
	// Index by result param code.
	byResult := make(map[string]int, len(fs))
	for i, f := range fs {
		// If two formulas produce the same result_param_code (shouldn't happen
		// per the unique index added in migration 000006) we keep the first.
		if _, exists := byResult[f.ResultParamCode]; !exists {
			byResult[f.ResultParamCode] = i
		}
	}

	// Build adjacency: edge from dependency → dependent. In-degree counts
	// inputs that are themselves produced by another formula in the set.
	inDegree := make([]int, len(fs))
	adj := make([][]int, len(fs))
	for i, f := range fs {
		for _, input := range f.InputParamCodes {
			depIdx, ok := byResult[input]
			if !ok {
				// Input is a CAPP / external param — not a formula edge.
				continue
			}
			if depIdx == i {
				continue
			}
			adj[depIdx] = append(adj[depIdx], i)
			inDegree[i]++
		}
	}

	queue := make([]int, 0, len(fs))
	for i, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, i)
		}
	}

	sorted := make([]Formula, 0, len(fs))
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		f := fs[n]
		f.SortOrder = len(sorted)
		sorted = append(sorted, f)
		for _, next := range adj[n] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(fs) {
		return nil, fmt.Errorf("topoSortFormulas: %w", calcdomain.ErrCycleDetected)
	}
	return sorted, nil
}

// =============================================================================
// LoadRMCosts
// =============================================================================

// LoadRMCosts returns the landed cost per RM identity. The returned map key is
// "<rm_code>|<item_code>" so callers can look up either a GROUP row (item_code
// empty → trailing pipe) or a specific ITEM row.
//
// itemCodes here is overloaded for input filtering — the engine passes both
// item codes (for ITEM-type RMs) and group codes (for GROUP-type RMs) since
// cst_rm_cost stores them all in rm_code.
func (l *productLoader) LoadRMCosts(ctx context.Context, itemCodes []string, period string, calcType string) (map[string]float64, error) {
	defer observeLoad(loaderKindRMCosts, time.Now())
	out := map[string]float64{}
	if len(itemCodes) == 0 || period == "" {
		return out, nil
	}
	const q = `
		SELECT rm_code, COALESCE(item_code, ''),
		       CASE $3
		           WHEN 'ACTUAL'   THEN COALESCE(cost_val,  0)
		           WHEN 'FORECAST' THEN COALESCE(cost_mark, 0)
		           WHEN 'SELLING'  THEN COALESCE(cost_sim,  0)
		           ELSE COALESCE(cost_val, 0)
		       END
		FROM cst_rm_cost
		WHERE period = $1
		  AND rm_code = ANY($2)`
	rows, err := l.db.QueryContext(ctx, q, period, pq.Array(itemCodes), calcType)
	if err != nil {
		return nil, fmt.Errorf("load RM costs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			rmCode   string
			itemCode string
			val      float64
		)
		if err := rows.Scan(&rmCode, &itemCode, &val); err != nil {
			return nil, fmt.Errorf("scan RM cost row: %w", err)
		}
		out[rmCode+"|"+itemCode] = val
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RM cost rows: %w", err)
	}
	return out, nil
}

// =============================================================================
// LoadUpstreamCosts
// =============================================================================

// LoadUpstreamCosts returns the per-unit cost for upstream products that were
// already calculated in prior waves of the same job. Rows in SUPERSEDED status
// are excluded.
func (l *productLoader) LoadUpstreamCosts(ctx context.Context, productSysIDs []int64, period, calcType string) (map[int64]float64, error) {
	defer observeLoad(loaderKindUpstream, time.Now())
	out := map[int64]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	if period == "" || calcType == "" {
		return nil, errors.New("LoadUpstreamCosts: period and calcType are required")
	}
	const q = `
		SELECT cpc_product_sys_id, cpc_cost_per_unit
		FROM cst_product_cost
		WHERE cpc_product_sys_id = ANY($1)
		  AND cpc_period = $2
		  AND cpc_calculation_type = $3
		  AND cpc_status <> 'SUPERSEDED'`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(productSysIDs), period, calcType)
	if err != nil {
		return nil, fmt.Errorf("load upstream costs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			cost         float64
		)
		if err := rows.Scan(&productSysID, &cost); err != nil {
			return nil, fmt.Errorf("scan upstream cost row: %w", err)
		}
		out[productSysID] = cost
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream cost rows: %w", err)
	}
	return out, nil
}
