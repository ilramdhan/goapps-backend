package planitem_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// stubRoutes is a scripted RouteProvider: one released route head per product,
// with a call counter so a test can assert how many finance RPCs a cascade
// costs. A product absent from the map has no released route.
type stubRoutes struct {
	byProduct map[int64]*financev1.CostMasterRoute
	calls     int
}

func (s *stubRoutes) GetProductRoute(_ context.Context, sysID int64) (*financev1.GetProductRouteForPPCResponse, error) {
	s.calls++
	return &financev1.GetProductRouteForPPCResponse{Data: s.byProduct[sysID]}, nil
}

// stubGroups maps products to machine groups; an unmapped product returns 0 so
// the caller falls back to the FG's group.
type stubGroups map[int64]int64

func (g stubGroups) MachineGroupForProduct(_ context.Context, sysID int64) (int64, error) {
	return g[sysID], nil
}

// productEdge is one PRODUCT RM edge feeding a stage.
func productEdge(sysID int64) *financev1.CostMasterRouteRm {
	return &financev1.CostMasterRouteRm{RmType: "PRODUCT", RmProductSysId: sysID}
}

// flatRoute builds a single flattened head whose stage i produces chain[i] and
// consumes chain[i+1] — the real shape finance returns for a multi-level route.
func flatRoute(headProduct int64, chain []int64) *financev1.CostMasterRoute {
	route := &financev1.CostMasterRoute{HeadId: 1, ProductSysId: headProduct, RoutingStatus: "COMPLETE"}
	for i := 0; i < len(chain)-1; i++ {
		route.Stages = append(route.Stages, &financev1.CostMasterRouteStage{
			//nolint:gosec // small fixed-size test fixture
			RouteLevel:        int32(i + 1),
			StageProductSysId: chain[i],
			Rms:               []*financev1.CostMasterRouteRm{productEdge(chain[i+1])},
		})
	}
	return route
}

// cascadeService wires a service with a route provider, a machine-group
// resolver and a product lookup, over the shared in-memory repository.
func cascadeService(routes planitemapp.RouteProvider, groups stubGroups, lookup planitemapp.ProductLookup) (*planitemapp.Service, *memRepo) {
	repo := newMemRepo()
	svc := planitemapp.NewService(repo, nil, lookup).
		WithCapacity(fixedCapacity{perDay: 100}).
		WithRoutes(routes).
		WithMachineGroups(groups)
	return svc, repo
}

// The headline gate: one FG_DELIVERY create walks the whole tty→pty→poy→sd
// chain, and every generated level carries its own machine group and its own
// product's shade — not the FG's.
func TestCreate_CascadeWalksFullChainWithOwnGroupAndShade(t *testing.T) {
	const (
		tty = int64(100)
		pty = int64(101)
		poy = int64(102)
		sd  = int64(103)
	)
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		tty: flatRoute(tty, []int64{tty, pty, poy, sd}),
	}}
	groups := stubGroups{tty: 7, pty: 8, poy: 9, sd: 10}
	lookup := stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		tty: {ProductSysId: tty, ProductCode: "TTY-1", ShadeCode: "S-TTY", ShadeName: "TURQUOISE"},
		pty: {ProductSysId: pty, ProductCode: "PTY-1", ShadeCode: "S-PTY", ShadeName: "NATURAL"},
		poy: {ProductSysId: poy, ProductCode: "POY-1", ShadeCode: "S-POY", ShadeName: "RAW"},
		sd:  {ProductSysId: sd, ProductCode: "SD-1", ShadeCode: "S-SD", ShadeName: "SEMI DULL"},
	}}
	svc, _ := cascadeService(routes, groups, lookup)

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)
	require.Len(t, res.Children, 3, "tty must cascade pty, poy and sd")
	assert.Empty(t, res.Warning)

	wantProducts := []int64{pty, poy, sd}
	wantGroups := []int64{8, 9, 10}
	wantShades := []string{"S-PTY", "S-POY", "S-SD"}
	for i, child := range res.Children {
		assert.Equal(t, wantProducts[i], child.CpmProductSysID(), "level %d product", i+1)
		assert.Equal(t, planitemdomain.TypeIntermediate, child.Type())
		assert.Equal(t, wantGroups[i], child.MachineGroupID(), "level %d must use its own machine group", i+1)
		assert.Equal(t, wantShades[i], child.ShadeCode(), "level %d must use its own shade", i+1)
	}
	// Every generated group differs from the FG's, proving the FG group is a
	// fallback rather than a blanket copy.
	assert.Equal(t, int64(7), res.Item.MachineGroupID())
}

// Each level is parented onto the level immediately downstream of it, and the
// whole chain lands in one batch so the ids exist.
func TestCreate_CascadeChainsParentIDs(t *testing.T) {
	const tty, pty, poy = int64(100), int64(101), int64(102)
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		tty: flatRoute(tty, []int64{tty, pty, poy}),
	}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)
	require.Len(t, res.Children, 2)

	require.NotZero(t, res.Item.ID())
	require.NotNil(t, res.Children[0].ParentItemID())
	assert.Equal(t, res.Item.ID(), *res.Children[0].ParentItemID())
	require.NotNil(t, res.Children[1].ParentItemID())
	assert.Equal(t, res.Children[0].ID(), *res.Children[1].ParentItemID())
	// No stale batch-relative index survives persistence.
	assert.Nil(t, res.Children[0].PendingParentIndex())
}

// Deadlines walk backwards by the fixed intermediate lead time, one hop at a
// time.
func TestCreate_CascadeBackDatesDeadlinePerLevel(t *testing.T) {
	const tty, pty, poy = int64(100), int64(101), int64(102)
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		tty: flatRoute(tty, []int64{tty, pty, poy}),
	}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)
	require.Len(t, res.Children, 2)

	lead := planitemdomain.IntermediateLeadTimeDays
	assert.Equal(t, deadline().AddDate(0, 0, -lead), res.Children[0].Deadline())
	assert.Equal(t, deadline().AddDate(0, 0, -2*lead), res.Children[1].Deadline())
}

// A product with no released route is a normal outcome: the FG is created
// alone, with a planner-facing warning naming the product by code.
func TestCreate_CascadeNoRouteReturnsWarningNotError(t *testing.T) {
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		100: {ProductSysId: 100, ProductCode: "TTY-1"},
	}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)

	assert.Empty(t, res.Children)
	assert.Contains(t, res.Warning, "TTY-1")
	assert.Contains(t, res.Warning, "no released route")
	assert.NotContains(t, res.Warning, "100", "planner messages must never carry raw sys ids")
}

// A released route whose only inputs are purchased ITEM / GROUP edges yields
// the FG alone. That is the second silent single-item outcome, and it must
// explain itself rather than look like a cascade that simply found one level.
func TestCreate_CascadeNoProductEdgeReturnsWarning(t *testing.T) {
	const tty = int64(100)
	itemOnly := &financev1.CostMasterRoute{HeadId: 1, ProductSysId: tty, RoutingStatus: "COMPLETE",
		Stages: []*financev1.CostMasterRouteStage{{
			RouteLevel:        1,
			StageProductSysId: tty,
			Rms: []*financev1.CostMasterRouteRm{
				{RmType: "ITEM", RmItemCode: "CHEM-1"},
				{RmType: "GROUP", RmGroupCode: "GRP-1"},
			},
		}}}
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{tty: itemOnly}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		tty: {ProductSysId: tty, ProductCode: "TTY-1"},
	}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)

	assert.Empty(t, res.Children)
	assert.Contains(t, res.Warning, "TTY-1")
	assert.Contains(t, res.Warning, "PRODUCT")
}

// A cyclic route must abort with a domain error naming the product, not hang
// and not truncate silently.
func TestCreate_CascadeCyclicRouteReturnsNamedError(t *testing.T) {
	const a, b = int64(100), int64(101)
	// a consumes b, b consumes a.
	cycle := &financev1.CostMasterRoute{HeadId: 1, ProductSysId: a, Stages: []*financev1.CostMasterRouteStage{
		{RouteLevel: 1, StageProductSysId: a, Rms: []*financev1.CostMasterRouteRm{productEdge(b)}},
		{RouteLevel: 2, StageProductSysId: b, Rms: []*financev1.CostMasterRouteRm{productEdge(a)}},
	}}
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{a: cycle}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		a: {ProductSysId: a, ProductCode: "TTY-1"},
	}})

	_, err := svc.Create(context.Background(), fgCreateCmd())
	require.Error(t, err)

	var cascadeErr *planitemdomain.CascadeError
	require.True(t, errors.As(err, &cascadeErr), "want a CascadeError, got %T", err)
	assert.Equal(t, "TTY-1", cascadeErr.ProductLabel)
	assert.Contains(t, cascadeErr.Reason, "cyclic")
}

// A chain longer than MaxCascadeDepth aborts rather than planning a truncated
// (and therefore under-planned) production chain.
func TestCreate_CascadeDepthGuardAborts(t *testing.T) {
	chain := make([]int64, 0, planitemdomain.MaxCascadeDepth+3)
	for i := 0; i <= planitemdomain.MaxCascadeDepth+1; i++ {
		chain = append(chain, int64(100+i))
	}
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		chain[0]: flatRoute(chain[0], chain),
	}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	_, err := svc.Create(context.Background(), fgCreateCmd())
	require.Error(t, err)

	var cascadeErr *planitemdomain.CascadeError
	require.True(t, errors.As(err, &cascadeErr), "want a CascadeError, got %T", err)
	assert.Contains(t, cascadeErr.Reason, "deeper than the maximum")
}

// A route that fans out past MaxCascadeItems aborts too: the cap is on total
// generated items, independent of depth.
func TestCreate_CascadeItemCapAborts(t *testing.T) {
	const fg = int64(100)
	stage := &financev1.CostMasterRouteStage{RouteLevel: 1, StageProductSysId: fg}
	for i := 0; i <= planitemdomain.MaxCascadeItems; i++ {
		stage.Rms = append(stage.Rms, productEdge(int64(200+i)))
	}
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		fg: {HeadId: 1, ProductSysId: fg, Stages: []*financev1.CostMasterRouteStage{stage}},
	}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	_, err := svc.Create(context.Background(), fgCreateCmd())
	require.Error(t, err)

	var cascadeErr *planitemdomain.CascadeError
	require.True(t, errors.As(err, &cascadeErr), "want a CascadeError, got %T", err)
	assert.Contains(t, cascadeErr.Reason, "more than the maximum")
}

// The walk costs one finance route RPC per node — the FG plus each generated
// level. PPC and finance are separate databases, so this is the unavoidable
// serial cost of a deep chain.
func TestCreate_CascadeIssuesOneRouteRPCPerLevel(t *testing.T) {
	const tty, pty, poy, sd = int64(100), int64(101), int64(102), int64(103)
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{
		tty: flatRoute(tty, []int64{tty, pty, poy, sd}),
	}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)

	assert.Equal(t, 4, routes.calls, "1 for the FG plus 1 per generated level")
	assert.Equal(t, routes.calls, res.RouteLookups)
}

// A non-PRODUCT RM edge (a purchased item or a group placeholder) terminates
// the chain instead of extending it.
func TestCreate_CascadeStopsAtNonProductRM(t *testing.T) {
	const tty, pty = int64(100), int64(101)
	route := &financev1.CostMasterRoute{HeadId: 1, ProductSysId: tty, Stages: []*financev1.CostMasterRouteStage{
		{RouteLevel: 1, StageProductSysId: tty, Rms: []*financev1.CostMasterRouteRm{productEdge(pty)}},
		{RouteLevel: 2, StageProductSysId: pty, Rms: []*financev1.CostMasterRouteRm{
			{RmType: "GROUP", RmGroupCode: "SD"},
			{RmType: "ITEM", RmItemCode: "DYE-01"},
		}},
	}}
	routes := &stubRoutes{byProduct: map[int64]*financev1.CostMasterRoute{tty: route}}
	svc, _ := cascadeService(routes, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)
	require.Len(t, res.Children, 1)
	assert.Equal(t, pty, res.Children[0].CpmProductSysID())
}

// A route lookup failure is a real error: silently planning the FG alone would
// hide a broken finance dependency.
func TestCreate_CascadeRouteLookupErrorFails(t *testing.T) {
	svc, _ := cascadeService(&failingRoutes{}, stubGroups{}, stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	_, err := svc.Create(context.Background(), fgCreateCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch route")
}

type failingRoutes struct{}

func (failingRoutes) GetProductRoute(_ context.Context, sysID int64) (*financev1.GetProductRouteForPPCResponse, error) {
	return nil, fmt.Errorf("finance unavailable for %d", sysID)
}
