package costroute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// fakeLoader is a test double for NestedMBGraphLoader.
type fakeLoader struct {
	graphs map[int64]*costroute.Graph
	errs   map[int64]error
	calls  []int64
}

func (f *fakeLoader) GetLatestGraphByProduct(_ context.Context, productSysID int64) (*costroute.Graph, error) {
	f.calls = append(f.calls, productSysID)
	if err, ok := f.errs[productSysID]; ok {
		return nil, err
	}
	if g, ok := f.graphs[productSysID]; ok {
		return g, nil
	}
	return nil, errors.New("fakeLoader: no graph seeded")
}

// fakeChecker is a test double for MBProductChecker. It records how many
// times it was called and the size of each batch, so tests can assert the
// bulk-per-level discipline instead of per-row calls.
type fakeChecker struct {
	mbIDs     map[int64]bool
	callCount int
	callSizes []int
}

func (f *fakeChecker) MBProductIDs(_ context.Context, ids []int64) (map[int64]bool, error) {
	f.callCount++
	f.callSizes = append(f.callSizes, len(ids))
	out := make(map[int64]bool)
	for _, id := range ids {
		if f.mbIDs[id] {
			out[id] = true
		}
	}
	return out, nil
}

func mkGraph(headID, productSysID int64, productCode string, seqs ...*costroute.Seq) *costroute.Graph {
	return &costroute.Graph{
		Head: &costroute.Head{
			HeadID:       headID,
			ProductSysID: productSysID,
			ProductCode:  productCode,
			ProductName:  productCode + "-name",
		},
		Seqs: seqs,
	}
}

func mkSeq(seqID int64, level int32, rms ...*costroute.Rm) *costroute.Seq {
	return &costroute.Seq{SeqID: seqID, RouteLevel: level, Rms: rms}
}

func mkProductRm(rmID, productSysID int64, ratio float64) *costroute.Rm {
	return &costroute.Rm{RmID: rmID, RmType: costroute.RmTypeProduct, RmProductSysID: productSysID, RouteRmRatio: ratio}
}

func mkGroupRm(rmID int64, groupCode string, ratio float64) *costroute.Rm {
	return &costroute.Rm{RmID: rmID, RmType: costroute.RmTypeGroup, RmGroupCode: groupCode, RouteRmRatio: ratio}
}

func allRms(g *costroute.Graph) []*costroute.Rm {
	var out []*costroute.Rm
	for _, s := range g.Seqs {
		out = append(out, s.Rms...)
	}
	return out
}

func TestFlatten_NoNesting_Passthrough(t *testing.T) {
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkGroupRm(100, "GRP-A", 0.6), mkGroupRm(101, "GRP-B", 0.4)),
	)
	loader := &fakeLoader{}
	checker := &fakeChecker{mbIDs: map[int64]bool{}}
	f := NewNestedMBFlattener(loader, checker)

	out, err := f.Flatten(context.Background(), base)
	require.NoError(t, err)
	require.Len(t, out.Seqs, 1)
	require.Len(t, out.Seqs[0].Rms, 2)

	for i, rm := range out.Seqs[0].Rms {
		assert.Equal(t, base.Seqs[0].Rms[i].RouteRmRatio, rm.EffectiveRatio)
		assert.Zero(t, rm.OriginHeadID)
		assert.Zero(t, rm.NestDepth)
		assert.Empty(t, rm.OriginProductCode)
	}
	assert.Empty(t, loader.calls, "loader should never be consulted when nothing is MB-typed")
}

func TestFlatten_SingleLevelSplice_RatioCompounding(t *testing.T) {
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 200, 0.5)),
	)
	nested := mkGraph(2, 200, "MB-200",
		mkSeq(20, 1, mkGroupRm(200, "GRP-X", 0.4)),
	)
	nested.Head.RoutingStatus = costroute.StatusLocked // nested MB routes are normally LOCKED

	loader := &fakeLoader{graphs: map[int64]*costroute.Graph{200: nested}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true}}
	f := NewNestedMBFlattener(loader, checker)

	out, err := f.Flatten(context.Background(), base)
	require.NoError(t, err)
	require.Len(t, out.Seqs, 2, "expected the original seq plus one spliced-in seq")

	spliced := out.Seqs[1]
	assert.Equal(t, int64(2), spliced.OriginHeadID)
	assert.Equal(t, "MB-200", spliced.OriginProductCode)
	assert.Equal(t, int32(1), spliced.NestDepth)
	assert.Greater(t, spliced.RouteLevel, out.Seqs[0].RouteLevel, "spliced stage must render past the base's own max level")

	require.Len(t, spliced.Rms, 1)
	rm := spliced.Rms[0]
	assert.InDelta(t, 0.2, rm.EffectiveRatio, 1e-9, "0.5 (base ratio) * 0.4 (nested ratio) = 0.2")
	assert.Equal(t, int64(2), rm.OriginHeadID)
	assert.Equal(t, "MB-200", rm.OriginProductCode)
	assert.Equal(t, "MB-200-name", rm.OriginProductName)
	assert.Equal(t, int32(1), rm.NestDepth)

	assert.Equal(t, []int64{200}, loader.calls)
	assert.Equal(t, 1, checker.callCount)
}

func TestFlatten_MultiLevelCompounding(t *testing.T) {
	// FG-1 --0.5--> MB-200 --0.4--> MB-300 --0.25--> GRP-Y
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 200, 0.5)),
	)
	level2 := mkGraph(2, 200, "MB-200",
		mkSeq(20, 1, mkProductRm(200, 300, 0.4)),
	)
	level3 := mkGraph(3, 300, "MB-300",
		mkSeq(30, 1, mkGroupRm(300, "GRP-Y", 0.25)),
	)

	loader := &fakeLoader{graphs: map[int64]*costroute.Graph{200: level2, 300: level3}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true, 300: true}}
	f := NewNestedMBFlattener(loader, checker)

	out, err := f.Flatten(context.Background(), base)
	require.NoError(t, err)
	require.Len(t, out.Seqs, 3)

	all := allRms(out)
	require.Len(t, all, 3)

	var deepest *costroute.Rm
	for _, rm := range all {
		if rm.RmType == costroute.RmTypeGroup {
			deepest = rm
		}
	}
	require.NotNil(t, deepest)
	assert.InDelta(t, 0.5*0.4*0.25, deepest.EffectiveRatio, 1e-9)
	assert.Equal(t, int32(2), deepest.NestDepth)
	assert.Equal(t, int64(3), deepest.OriginHeadID)

	// checker was consulted once per BFS level (level1: [200], level2: [300]).
	assert.Equal(t, 2, checker.callCount)
	assert.Equal(t, []int{1, 1}, checker.callSizes)
}

func TestFlatten_CycleDetection_NoInfiniteLoop(t *testing.T) {
	// MB-200's own graph references back to the requesting product (1), which
	// must already be in `visited` and therefore never re-expanded.
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 200, 1.0)),
	)
	nested := mkGraph(2, 200, "MB-200",
		mkSeq(20, 1, mkProductRm(200, 1, 1.0)), // cycle back to product 1
	)

	loader := &fakeLoader{graphs: map[int64]*costroute.Graph{200: nested}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true, 1: true}}
	f := NewNestedMBFlattener(loader, checker)

	done := make(chan struct{})
	var out *costroute.Graph
	var err error
	go func() {
		out, err = f.Flatten(context.Background(), base)
		close(done)
	}()

	select {
	case <-done:
	case <-timeoutCh(t):
		t.Fatal("Flatten did not terminate -- suspected infinite loop on cyclic MB reference")
	}

	require.NoError(t, err)
	require.Len(t, out.Seqs, 2, "only the one nested level should be spliced; the cycle back to product 1 must be skipped")
	assert.NotContains(t, loader.calls, int64(1), "loader must never be asked to load the root product's own graph")
}

func TestFlatten_SelfReferencingNestedMB_NoInfiniteLoop(t *testing.T) {
	// MB-200's own graph references itself.
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 200, 1.0)),
	)
	nested := mkGraph(2, 200, "MB-200",
		mkSeq(20, 1, mkProductRm(200, 200, 1.0)), // self-reference
	)

	loader := &fakeLoader{graphs: map[int64]*costroute.Graph{200: nested}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true}}
	f := NewNestedMBFlattener(loader, checker)

	done := make(chan struct{})
	var out *costroute.Graph
	var err error
	go func() {
		out, err = f.Flatten(context.Background(), base)
		close(done)
	}()

	select {
	case <-done:
	case <-timeoutCh(t):
		t.Fatal("Flatten did not terminate -- suspected infinite loop on self-referencing MB")
	}

	require.NoError(t, err)
	require.Len(t, out.Seqs, 2, "only one splice of MB-200's own graph; its self-reference must not re-expand")
	assert.Equal(t, 1, len(loader.calls), "loader must be called exactly once for product 200")
}

func TestFlatten_MaxDepthCap(t *testing.T) {
	// Build a chain product 1 -> 100 -> 101 -> ... -> 112 (well past maxNestDepth),
	// each one referencing the next with ratio 1.0, terminating in a GROUP row.
	const chainLen = 12
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 100, 1.0)),
	)
	graphs := map[int64]*costroute.Graph{}
	mbIDs := map[int64]bool{}
	for i := int64(100); i < 100+chainLen; i++ {
		next := i + 1
		if i == 100+chainLen-1 {
			graphs[i] = mkGraph(i, i, "MB", mkSeq(i*10, 1, mkGroupRm(i*10, "GRP", 1.0)))
		} else {
			graphs[i] = mkGraph(i, i, "MB", mkSeq(i*10, 1, mkProductRm(i*10, next, 1.0)))
			mbIDs[next] = true
		}
		mbIDs[i] = true
	}

	loader := &fakeLoader{graphs: graphs}
	checker := &fakeChecker{mbIDs: mbIDs}
	f := NewNestedMBFlattener(loader, checker)

	done := make(chan struct{})
	var out *costroute.Graph
	var err error
	go func() {
		out, err = f.Flatten(context.Background(), base)
		close(done)
	}()
	select {
	case <-done:
	case <-timeoutCh(t):
		t.Fatal("Flatten did not terminate within the deadline -- maxNestDepth cap likely broken")
	}
	require.NoError(t, err)

	var maxSeenDepth int32
	for _, rm := range allRms(out) {
		if rm.NestDepth > maxSeenDepth {
			maxSeenDepth = rm.NestDepth
		}
	}
	assert.LessOrEqual(t, maxSeenDepth, int32(maxNestDepth+1), "nest depth must be capped near maxNestDepth")
	assert.Less(t, len(out.Seqs), chainLen+1, "the full un-capped chain must not be fully expanded")
}

func TestFlatten_MissingNestedRoute_DegradesGracefully(t *testing.T) {
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1, mkProductRm(100, 200, 0.5)),
	)
	loader := &fakeLoader{errs: map[int64]error{200: errors.New("no route found")}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true}}
	f := NewNestedMBFlattener(loader, checker)

	out, err := f.Flatten(context.Background(), base)
	require.NoError(t, err, "a load failure for one nested MB must not fail the whole graph fetch")
	require.Len(t, out.Seqs, 1, "no seq should be spliced in for the failed load")

	rm := out.Seqs[0].Rms[0]
	assert.Equal(t, 0.5, rm.EffectiveRatio, "un-flattened row falls back to its own RouteRmRatio")
	assert.Zero(t, rm.OriginHeadID)
}

func TestFlatten_BulkCheckCalledOncePerLevel_NotPerRow(t *testing.T) {
	base := mkGraph(1, 1, "FG-1",
		mkSeq(10, 1,
			mkProductRm(100, 200, 0.3),
			mkProductRm(101, 300, 0.7),
		),
	)
	g200 := mkGraph(2, 200, "MB-200", mkSeq(20, 1, mkGroupRm(200, "GRP-A", 1.0)))
	g300 := mkGraph(3, 300, "MB-300", mkSeq(30, 1, mkGroupRm(300, "GRP-B", 1.0)))

	loader := &fakeLoader{graphs: map[int64]*costroute.Graph{200: g200, 300: g300}}
	checker := &fakeChecker{mbIDs: map[int64]bool{200: true, 300: true}}
	f := NewNestedMBFlattener(loader, checker)

	out, err := f.Flatten(context.Background(), base)
	require.NoError(t, err)
	require.Len(t, out.Seqs, 3)

	assert.Equal(t, 1, checker.callCount, "both rows are on the same BFS level -- one bulk call, not two")
	assert.Equal(t, []int{2}, checker.callSizes)
}

// timeoutCh returns a channel that fires after a short deadline, used as the
// "did not hang" arm of a select alongside Flatten's completion signal. The
// graphs in this file are tiny, so a correct implementation returns in
// microseconds -- this deadline only needs to catch a genuine infinite loop.
func timeoutCh(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(2 * time.Second)
}
