package planitem

import (
	"context"
	"fmt"
	"time"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// rmTypeProduct is the cost_route_rm discriminator marking an RM edge that
// points at another manufactured product (as opposed to a purchased ITEM or a
// GROUP placeholder). Only PRODUCT edges continue the cascade.
const rmTypeProduct = "PRODUCT"

// unknownProduct labels a product whose code could not be resolved. Sys ids are
// never surfaced to planners.
const unknownProduct = "(unknown product)"

// RouteProvider fetches a product's released route from finance. PPC and
// finance are separate databases, so a multi-level chain is walked with one
// call per level — there is no join available. May be nil, in which case no
// cascade is generated at all.
type RouteProvider interface {
	GetProductRoute(ctx context.Context, sysID int64) (*financev1.GetProductRouteForPPCResponse, error)
}

// WithRoutes attaches the finance route provider used to cascade upstream
// INTERMEDIATE plan items. Without one, creating an FG item yields that item
// alone.
func (s *Service) WithRoutes(r RouteProvider) *Service {
	s.routes = r
	return s
}

// MachineGroupResolver maps a product to the machine group that produces it.
// Each cascade level is planned on its own group; without a resolver every
// level falls back to the FG's group, which is the defect this replaces.
type MachineGroupResolver interface {
	MachineGroupForProduct(ctx context.Context, cpmProductSysID int64) (int64, error)
}

// WithMachineGroups attaches the per-product machine-group resolver used to
// place each cascade level on its own group.
func (s *Service) WithMachineGroups(r MachineGroupResolver) *Service {
	s.machineGroups = r
	return s
}

// cascadeResult is the outcome of a route walk: the generated items ordered
// FG-first by increasing route level, plus a non-fatal warning when the product
// has no route to walk.
type cascadeResult struct {
	items   []*planitemdomain.PlanItem
	warning string
	// rpcCalls counts finance route lookups issued, for observability: a deep
	// chain costs one serial RPC per hop.
	rpcCalls int
}

// walkState threads guard bookkeeping through the recursion.
type walkState struct {
	visited  map[int64]bool
	items    []*planitemdomain.PlanItem
	fg       *planitemdomain.PlanItem
	rpcCalls int
}

// cascadeRoute walks the FG product's released route upstream and builds one
// INTERMEDIATE plan item per level, each carrying its own machine group, its
// own product's shade, and its own lead time.
//
// A product with no released route is not an error: it yields the FG alone plus
// a warning, because a product without a route must still be plannable.
func (s *Service) cascadeRoute(ctx context.Context, fg *planitemdomain.PlanItem) (cascadeResult, error) {
	if s.routes == nil {
		return cascadeResult{warning: "No upstream plan items were generated: the finance route lookup is not " +
			"available, so this product's route could not be read."}, nil
	}
	route, err := s.fetchRoute(ctx, fg.CpmProductSysID())
	if err != nil {
		return cascadeResult{}, err
	}
	st := &walkState{visited: map[int64]bool{fg.CpmProductSysID(): true}, fg: fg, rpcCalls: 1}
	if route == nil {
		return cascadeResult{
			rpcCalls: st.rpcCalls,
			warning: fmt.Sprintf(
				"No upstream plan items were generated: product %s has no released route. "+
					"Finance only serves a route once its status is COMPLETE or LOCKED — a DRAFT route is ignored. "+
					"Release the route in finance, then delete and re-create this plan item to cascade its levels.",
				s.productLabel(ctx, fg.CpmProductSysID()),
			),
		}, nil
	}
	// A released head with no PRODUCT-typed RM edge at the FG stage is the
	// other silent single-item outcome: the route exists but every input is a
	// purchased ITEM or a GROUP placeholder, which terminates the walk.
	if len(upstreamProductsOf(route, fg.CpmProductSysID())) == 0 {
		return cascadeResult{
			rpcCalls: st.rpcCalls,
			warning: fmt.Sprintf(
				"No upstream plan items were generated: the released route for product %s has no semi-finished "+
					"(PRODUCT) input at its final stage, so there is no upstream level to plan.",
				s.productLabel(ctx, fg.CpmProductSysID()),
			),
		}, nil
	}

	// The FG occupies index 0 of the batch; every level-1 item parents onto it.
	if err := s.walkLevel(ctx, route, fg.CpmProductSysID(), 0, 0, st); err != nil {
		return cascadeResult{}, err
	}
	return cascadeResult{items: st.items, rpcCalls: st.rpcCalls}, nil
}

// walkLevel descends one level: for the stage of route that produces
// downstreamProduct it builds an INTERMEDIATE item per PRODUCT RM edge, then
// recurses into each upstream product.
//
// depth counts hops from the FG; parentIndex is the batch position of the item
// these children hang off (0 = the FG itself).
func (s *Service) walkLevel(
	ctx context.Context,
	route *financev1.CostMasterRoute,
	downstreamProduct int64,
	depth, parentIndex int,
	st *walkState,
) error {
	upstreams := upstreamProductsOf(route, downstreamProduct)
	if len(upstreams) == 0 {
		return nil
	}
	if depth >= planitemdomain.MaxCascadeDepth {
		return planitemdomain.NewCascadeError(
			fmt.Sprintf("route is deeper than the maximum of %d levels", planitemdomain.MaxCascadeDepth),
			s.productLabel(ctx, downstreamProduct),
		)
	}
	for _, upstream := range upstreams {
		if err := s.addUpstream(ctx, upstream, route, depth, parentIndex, st); err != nil {
			return err
		}
	}
	return nil
}

// addUpstream materializes one upstream product as a plan item and recurses
// into its route. Cycles are cut here, before any work is done.
//
// inherited is the route the caller was walking. A finance route head describes
// the whole flattened chain for its own FG, so when an upstream product has no
// released head of its own the walk continues inside the inherited head rather
// than stopping — otherwise a 13-level flattened route would yield one item.
func (s *Service) addUpstream(
	ctx context.Context,
	upstream int64,
	inherited *financev1.CostMasterRoute,
	depth, parentIndex int,
	st *walkState,
) error {
	if st.visited[upstream] {
		return planitemdomain.NewCascadeError("route is cyclic", s.productLabel(ctx, upstream))
	}
	if len(st.items) >= planitemdomain.MaxCascadeItems {
		return planitemdomain.NewCascadeError(
			fmt.Sprintf("route generates more than the maximum of %d plan items", planitemdomain.MaxCascadeItems),
			s.productLabel(ctx, upstream),
		)
	}
	st.visited[upstream] = true

	ownRoute, err := s.fetchRoute(ctx, upstream)
	if err != nil {
		return err
	}
	st.rpcCalls++
	next := ownRoute
	if next == nil {
		next = inherited
	}

	item, err := s.buildIntermediate(ctx, buildParams{
		product:        upstream,
		parentIndex:    parentIndex,
		parentQty:      st.fg.QtyTarget(),
		parentDeadline: st.parentDeadline(parentIndex),
		route:          ownRoute,
		fg:             st.fg,
	})
	if err != nil {
		return err
	}
	st.items = append(st.items, item)
	// Batch index of the item just appended: index 0 is the FG.
	childIndex := len(st.items)

	return s.walkLevel(ctx, next, upstream, depth+1, childIndex, st)
}

// parentDeadline returns the deadline of the batch item at the given index,
// where index 0 is the FG.
func (st *walkState) parentDeadline(parentIndex int) time.Time {
	if parentIndex == 0 {
		return st.fg.Deadline()
	}
	return st.items[parentIndex-1].Deadline()
}

// upstreamProductsOf returns, in stage order, the products feeding the stage
// that produces downstreamProduct. Non-PRODUCT RM edges (purchased ITEM rows,
// GROUP placeholders) terminate the chain and are skipped.
//
// The stage is selected by the product it produces rather than by level number,
// which keeps the walk correct for both flattened multi-level heads and
// single-stage per-product heads.
func upstreamProductsOf(route *financev1.CostMasterRoute, downstreamProduct int64) []int64 {
	var out []int64
	for _, stage := range route.GetStages() {
		if stage.GetStageProductSysId() != downstreamProduct {
			continue
		}
		for _, rm := range stage.GetRms() {
			if rm.GetRmType() != rmTypeProduct || rm.GetRmProductSysId() <= 0 {
				continue
			}
			out = append(out, rm.GetRmProductSysId())
		}
	}
	return out
}

// buildParams carries everything one cascade level needs.
type buildParams struct {
	product        int64
	parentIndex    int
	parentQty      float64
	parentDeadline time.Time
	route          *financev1.CostMasterRoute
	fg             *planitemdomain.PlanItem
}

// buildIntermediate constructs one INTERMEDIATE plan item for a cascade level.
//
// Quantity is the parent's quantity verbatim: there is no yield model yet, so
// no loss or scrap is applied. That is a deliberate deferral — modeling yield
// needs per-route yield master data that does not exist today, and inventing a
// factor would be worse than a documented 1:1.
func (s *Service) buildIntermediate(ctx context.Context, p buildParams) (*planitemdomain.PlanItem, error) {
	shadeCode, shadeName := s.resolveShade(ctx, p.product)
	deadline := p.parentDeadline.AddDate(0, 0, -s.resolveLeadTime(p.route))
	idx := p.parentIndex
	item, err := planitemdomain.New(planitemdomain.NewParams{
		CpmProductSysID:    p.product,
		Type:               planitemdomain.TypeIntermediate,
		PendingParentIndex: &idx,
		QtyTarget:          p.parentQty,
		Deadline:           deadline,
		RMSource:           p.fg.RMSource(),
		ShadeCode:          shadeCode,
		ShadeName:          shadeName,
		MachineGroupID:     s.resolveMachineGroup(ctx, p.product, p.fg.MachineGroupID()),
		CreatedBy:          p.fg.CreatedBy(),
	})
	if err != nil {
		return nil, err
	}
	s.applyDerivedTimeline(ctx, item)
	return item, nil
}

// resolveLeadTime returns the back-dating, in days, between a cascade level and
// the level below it. The finance route model carries no lead-time column
// today, so every level uses the fixed IntermediateLeadTimeDays fallback; the
// route is taken as a parameter so a per-route value can later be honored
// without touching any caller.
func (s *Service) resolveLeadTime(_ *financev1.CostMasterRoute) int {
	return planitemdomain.IntermediateLeadTimeDays
}

// resolveMachineGroup returns the machine group that produces a given product,
// falling back to the FG's group when no per-product mapping exists. The
// pre-cascade code applied that fallback unconditionally; demoting it to a
// fallback is the fix.
func (s *Service) resolveMachineGroup(ctx context.Context, product, fallback int64) int64 {
	if s.machineGroups == nil {
		return fallback
	}
	groupID, err := s.machineGroups.MachineGroupForProduct(ctx, product)
	if err != nil || groupID <= 0 {
		return fallback
	}
	return groupID
}

// fetchRoute returns a product's released route, or nil when it has none.
// A missing route is a normal outcome, not an error.
func (s *Service) fetchRoute(ctx context.Context, sysID int64) (*financev1.CostMasterRoute, error) {
	resp, err := s.routes.GetProductRoute(ctx, sysID)
	if err != nil {
		return nil, fmt.Errorf("fetch route for cascade: %w", err)
	}
	return resp.GetData(), nil
}

// productLabel resolves a product's code for planner-facing messages.
func (s *Service) productLabel(ctx context.Context, sysID int64) string {
	if p := s.LookupProducts(ctx, []int64{sysID})[sysID]; p != nil && p.GetProductCode() != "" {
		return p.GetProductCode()
	}
	return unknownProduct
}
