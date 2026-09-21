package costroute_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// fakeRepoForGetGraph returns a fixed graph from GetGraph and otherwise
// no-ops via the embedded fakeRepoForDup.
type fakeRepoForGetGraph struct {
	fakeRepoForDup
	graph *costroute.Graph
}

func (r *fakeRepoForGetGraph) GetGraph(_ context.Context, _ int64) (*costroute.Graph, error) {
	return r.graph, nil
}

// alwaysFlattens is an app.NestedMBFlattener stand-in that would visibly
// mutate the graph if it were ever invoked -- used to prove includeNestedMB
// gates the call, not just its presence.
func newAlwaysFlattens(t *testing.T) *app.NestedMBFlattener {
	t.Helper()
	// A checker that panics if consulted lets a wiring regression fail loudly
	// instead of silently passing an unrelated assertion.
	return app.NewNestedMBFlattener(panicLoader{}, panicChecker{})
}

type panicLoader struct{}

func (panicLoader) GetLatestGraphByProduct(_ context.Context, _ int64) (*costroute.Graph, error) {
	panic("GetLatestGraphByProduct must not be called when includeNestedMB is false")
}

type panicChecker struct{}

func (panicChecker) MBProductIDs(_ context.Context, _ []int64) (map[int64]bool, error) {
	panic("MBProductIDs must not be called when includeNestedMB is false")
}

// TestGetGraphHandler_IncludeNestedMbUnset_ReturnsRepoGraphUnchanged is the
// plan's required regression test: GetRouteGraph (via GetGraphHandler) with
// include_nested_mb unset/false must be byte-for-byte pre-feature behavior,
// even when a flattener is wired onto the handler (e.g. in production main.go).
func TestGetGraphHandler_IncludeNestedMbUnset_ReturnsRepoGraphUnchanged(t *testing.T) {
	want := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 42, ProductCode: "FG-1"},
		Seqs: []*costroute.Seq{
			{SeqID: 10, RouteLevel: 1, Rms: []*costroute.Rm{
				{RmID: 100, RmType: costroute.RmTypeGroup, RmGroupCode: "GRP-A", RouteRmRatio: 0.6},
			}},
		},
	}
	repo := &fakeRepoForGetGraph{graph: want}
	h := app.NewGetGraphHandler(repo).WithNestedMBFlattener(newAlwaysFlattens(t))

	got, err := h.Handle(context.Background(), 1, false)
	require.NoError(t, err)
	assert.Same(t, want, got, "must return the exact repo graph, not a copy or a flattened variant")
}

// TestGetGraphHandler_NoFlattenerWired_IgnoresIncludeNestedMb proves the flag
// alone can never trigger flattening without an explicitly wired flattener.
func TestGetGraphHandler_NoFlattenerWired_IgnoresIncludeNestedMb(t *testing.T) {
	want := &costroute.Graph{Head: &costroute.Head{HeadID: 1, ProductSysID: 42}}
	repo := &fakeRepoForGetGraph{graph: want}
	h := app.NewGetGraphHandler(repo)

	got, err := h.Handle(context.Background(), 1, true)
	require.NoError(t, err)
	assert.Same(t, want, got)
}
