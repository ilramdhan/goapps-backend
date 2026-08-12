// Package orchestrator builds and coordinates cost calculation jobs.
package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/pkg/costcalc"
)

// DagBuilder walks routes to build a product-level dependency graph.
// Nodes are product_sys_ids; edges are PRODUCT-type RM links (downstream -> upstream).
type DagBuilder struct {
	db *sql.DB
}

// NewDagBuilder constructs a DAG builder.
func NewDagBuilder(db *sql.DB) *DagBuilder { return &DagBuilder{db: db} }

// ScopeInput describes what set of products to seed traversal with.
type ScopeInput struct {
	Scope               costcalc.JobScope
	ProductSysID        int64
	RouteHeadID         int64
	ProductTypeIDFilter int32
	Period              string // unused by builder but threaded for future filters
}

// Build resolves the initial product set per scope, then transitively traverses
// PRODUCT-type RMs to include upstream dependencies. Returns the graph and the
// sorted list of all product_sys_ids discovered.
func (b *DagBuilder) Build(ctx context.Context, in ScopeInput) (*costcalc.DependencyGraph, []int64, error) {
	initial, err := b.resolveInitialSet(ctx, in)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve initial set: %w", err)
	}
	g := costcalc.NewDependencyGraph()
	if len(initial) == 0 {
		return g, nil, nil
	}

	visited := map[int64]bool{}
	frontier := append([]int64{}, initial...)

	for len(frontier) > 0 {
		batch := make([]int64, 0, len(frontier))
		for _, pid := range frontier {
			if visited[pid] {
				continue
			}
			visited[pid] = true
			g.AddNode(pid)
			batch = append(batch, pid)
		}
		if len(batch) == 0 {
			break
		}

		edges, err := b.loadProductRMEdges(ctx, batch)
		if err != nil {
			return nil, nil, fmt.Errorf("load edges for batch: %w", err)
		}

		var nextFrontier []int64
		for _, e := range edges {
			g.AddEdge(e.downstream, e.upstream)
			if !visited[e.upstream] {
				nextFrontier = append(nextFrontier, e.upstream)
			}
		}
		frontier = nextFrontier
	}

	return g, g.Nodes(), nil
}

type edge struct {
	downstream int64 // product produced by the sequence containing the RM line
	upstream   int64 // the product referenced as RM
}

// typeCodeMB is the cost_product_type.cpt_type_code of Master Batch products.
//
// MB is computed exclusively by the finance MB_BATCH path (mbbatch.RunMBBatch,
// reached from the "MB Push to Head" page), which computes all three calc types in
// one dependency-ordered run and then pushes the results into cst_mb_cost. Letting
// the generic calc engine also compute MB gives two writers for the same
// (product, period, calc_type): a calc job supersedes the APPROVED cst_product_cost
// row that a push already consumed, leaving cst_mb_cost pointing at a SUPERSEDED
// source while its value stays stale. MB is therefore excluded from every calc-job
// scope — see excludeMBPredicate.
//
// Non-MB products are unaffected: MB is never referenced as a PRODUCT-type route RM
// by any other product type (only MB routes reference MB). Downstream consumers such
// as yarn read MB prices through cst_mb_cost via LoadMBCosts, a formula path that
// does not involve the route DAG at all.
const typeCodeMB = "MB"

// excludeMBPredicate is the SQL fragment that drops MB-typed products from a product
// selection. It is a NOT EXISTS rather than a JOIN so it can be appended to queries
// that do not already join the product master, and so a product without a master row
// is kept rather than silently dropped.
const excludeMBPredicate = `
	  AND NOT EXISTS (
	    SELECT 1
	    FROM cost_product_master mb_pm
	    JOIN cost_product_type   mb_pt ON mb_pt.cpt_type_id = mb_pm.cpm_product_type_id
	    WHERE mb_pm.cpm_product_sys_id = %s
	      AND mb_pt.cpt_type_code = '` + typeCodeMB + `'
	  )`

// ErrMBScopeNotAllowed is returned when a calc job explicitly targets MB products.
// Unlike the silent filtering applied to ALL, an explicit MB request is a user
// mistake worth surfacing: the job would otherwise report SUCCESS having calculated
// nothing.
var ErrMBScopeNotAllowed = errors.New(
	"MB products are calculated by the MB Batch path (MB Push to Head), not by calc jobs")

// resolveInitialSet returns the product_sys_ids to start traversal from.
func (b *DagBuilder) resolveInitialSet(ctx context.Context, in ScopeInput) ([]int64, error) {
	switch in.Scope {
	case costcalc.ScopeSingleProduct:
		if in.ProductSysID == 0 {
			return nil, fmt.Errorf("product_sys_id required for SINGLE_PRODUCT")
		}
		isMB, err := b.isMBProduct(ctx, in.ProductSysID)
		if err != nil {
			return nil, err
		}
		if isMB {
			return nil, fmt.Errorf("product %d: %w", in.ProductSysID, ErrMBScopeNotAllowed)
		}
		return []int64{in.ProductSysID}, nil

	case costcalc.ScopeSingleRoute:
		if in.RouteHeadID == 0 {
			return nil, fmt.Errorf("route_head_id required for SINGLE_ROUTE")
		}
		return b.productsOfRouteHead(ctx, in.RouteHeadID)

	case costcalc.ScopeFiltered:
		isMB, err := b.isMBProductType(ctx, in.ProductTypeIDFilter)
		if err != nil {
			return nil, err
		}
		if isMB {
			return nil, ErrMBScopeNotAllowed
		}
		return b.productsByType(ctx, in.ProductTypeIDFilter)

	case costcalc.ScopeAll:
		return b.allActiveProducts(ctx)
	default:
		return nil, fmt.Errorf("unknown scope: %s", in.Scope)
	}
}

// allActiveProducts returns every product that has an active (COMPLETE or LOCKED)
// route head, excluding MB (see typeCodeMB). Under ALL the exclusion is silent —
// the user asked for "everything the calc engine owns", and MB is not part of that.
func (b *DagBuilder) allActiveProducts(ctx context.Context) ([]int64, error) {
	q := `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL` +
		fmt.Sprintf(excludeMBPredicate, "crh.crh_product_sys_id") + `
		ORDER BY crh.crh_product_sys_id
	`
	return b.scanInt64s(ctx, q)
}

// productsByType returns active-route products of a specific product type. The MB
// type itself is rejected earlier in resolveInitialSet; the predicate here is
// belt-and-braces for the case where two type rows share the MB code.
func (b *DagBuilder) productsByType(ctx context.Context, typeID int32) ([]int64, error) {
	if typeID == 0 {
		return nil, fmt.Errorf("product_type_id_filter required for FILTERED scope")
	}
	q := `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND cpm.cpm_product_type_id = $1` +
		fmt.Sprintf(excludeMBPredicate, "crh.crh_product_sys_id") + `
		ORDER BY crh.crh_product_sys_id
	`
	return b.scanInt64s(ctx, q, typeID)
}

// productsOfRouteHead returns the FG product for a specific route head, unless that
// product is an MB.
func (b *DagBuilder) productsOfRouteHead(ctx context.Context, headID int64) ([]int64, error) {
	q := `SELECT crh_product_sys_id FROM cost_route_head
	      WHERE crh_head_id = $1 AND crh_deleted_at IS NULL` +
		fmt.Sprintf(excludeMBPredicate, "crh_product_sys_id")
	return b.scanInt64s(ctx, q, headID)
}

// isMBProduct reports whether a single product is MB-typed.
func (b *DagBuilder) isMBProduct(ctx context.Context, productSysID int64) (bool, error) {
	const q = `
		SELECT EXISTS (
		  SELECT 1
		  FROM cost_product_master pm
		  JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		  WHERE pm.cpm_product_sys_id = $1 AND pt.cpt_type_code = $2
		)`
	var isMB bool
	if err := b.db.QueryRowContext(ctx, q, productSysID, typeCodeMB).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB product %d: %w", productSysID, err)
	}
	return isMB, nil
}

// isMBProductType reports whether a product type id is the MB type.
func (b *DagBuilder) isMBProductType(ctx context.Context, typeID int32) (bool, error) {
	if typeID == 0 {
		return false, nil // productsByType raises the "filter required" error.
	}
	const q = `SELECT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_id = $1 AND cpt_type_code = $2)`
	var isMB bool
	if err := b.db.QueryRowContext(ctx, q, typeID, typeCodeMB).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB product type %d: %w", typeID, err)
	}
	return isMB, nil
}

// loadProductRMEdges returns the PRODUCT-type RM edges (downstream -> upstream)
// for the given set of product_sys_ids.
//
// An edge is only followed when the upstream RM target is itself a calc-able
// product, i.e. it has an active (COMPLETE/LOCKED) route head of its own.
// A PRODUCT-type RM whose target has no active route is a raw cost INPUT, not a
// calc target: it carries a known price (RM cost) rather than a computed route
// cost, so it must NOT become a graph node. Without this guard such a target
// would be added as a headless node and later fail the cal_job_product insert
// (cjp_route_head_id NOT NULL), aborting the whole job.
//
// Deliberately NOT filtered by typeCodeMB. The MB exclusion applies to what a job
// TARGETS, not to dependency resolution: if some non-MB product ever does reference
// an MB as a PRODUCT-type RM, that MB is a genuine upstream input and must still be
// computed, exactly as it is today, or the referencing product would fail with
// ErrMissingUpstreamCost. No such reference exists in current data — MB routes are
// the only routes that reference MB — so this branch is unreachable in practice and
// the seed-set filters above carry the whole exclusion.
func (b *DagBuilder) loadProductRMEdges(ctx context.Context, productSysIDs []int64) ([]edge, error) {
	const q = `
		SELECT crs.crs_product_sys_id, crm.crm_rm_product_sys_id
		FROM cost_route_head crh
		JOIN cost_route_seq crs ON crs.crs_head_id = crh.crh_head_id
		JOIN cost_route_rm  crm ON crm.crm_seq_id  = crs.crs_seq_id
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at    IS NULL
		  AND crs.crs_deleted_at    IS NULL
		  AND crs.crs_product_sys_id = ANY($1)
		  AND crm.crm_rm_type        = 'PRODUCT'
		  AND crm.crm_rm_product_sys_id IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM cost_route_head up
		    WHERE up.crh_product_sys_id = crm.crm_rm_product_sys_id
		      AND up.crh_routing_status IN ('COMPLETE','LOCKED')
		      AND up.crh_deleted_at IS NULL
		  )
	`
	rows, err := b.db.QueryContext(ctx, q, pq.Array(productSysIDs))
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := rows.Close(); e != nil {
			_ = e
		}
	}()

	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.downstream, &e.upstream); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// scanInt64s is a helper for "SELECT one BIGINT column".
func (b *DagBuilder) scanInt64s(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := b.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := rows.Close(); e != nil {
			_ = e
		}
	}()

	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
