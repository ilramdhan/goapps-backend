// Package orchestrator builds and coordinates cost calculation jobs.
package orchestrator

import (
	"context"
	"database/sql"
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

// resolveInitialSet returns the product_sys_ids to start traversal from.
func (b *DagBuilder) resolveInitialSet(ctx context.Context, in ScopeInput) ([]int64, error) {
	switch in.Scope {
	case costcalc.ScopeSingleProduct:
		if in.ProductSysID == 0 {
			return nil, fmt.Errorf("product_sys_id required for SINGLE_PRODUCT")
		}
		return []int64{in.ProductSysID}, nil

	case costcalc.ScopeSingleRoute:
		if in.RouteHeadID == 0 {
			return nil, fmt.Errorf("route_head_id required for SINGLE_ROUTE")
		}
		return b.productsOfRouteHead(ctx, in.RouteHeadID)

	case costcalc.ScopeFiltered:
		return b.productsByType(ctx, in.ProductTypeIDFilter)

	case costcalc.ScopeAll:
		return b.allActiveProducts(ctx)
	default:
		return nil, fmt.Errorf("unknown scope: %s", in.Scope)
	}
}

// allActiveProducts returns every product that has an active (COMPLETE or LOCKED) route head.
func (b *DagBuilder) allActiveProducts(ctx context.Context) ([]int64, error) {
	const q = `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		ORDER BY crh.crh_product_sys_id
	`
	return b.scanInt64s(ctx, q)
}

// productsByType returns active-route products of a specific product type.
func (b *DagBuilder) productsByType(ctx context.Context, typeID int32) ([]int64, error) {
	if typeID == 0 {
		return nil, fmt.Errorf("product_type_id_filter required for FILTERED scope")
	}
	const q = `
		SELECT DISTINCT crh.crh_product_sys_id
		FROM cost_route_head crh
		JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
		WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		  AND crh.crh_deleted_at IS NULL
		  AND cpm.cpm_product_type_id = $1
		ORDER BY crh.crh_product_sys_id
	`
	return b.scanInt64s(ctx, q, typeID)
}

// productsOfRouteHead returns the FG product for a specific route head.
func (b *DagBuilder) productsOfRouteHead(ctx context.Context, headID int64) ([]int64, error) {
	const q = `SELECT crh_product_sys_id FROM cost_route_head WHERE crh_head_id = $1 AND crh_deleted_at IS NULL`
	return b.scanInt64s(ctx, q, headID)
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
