package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// =============================================================================
// Fakes — only the two ports GetRouteCostSheetHandler actually touches are
// exercised; the rest of each interface panics so an accidental new dependency
// fails loudly instead of silently returning zero values.
// =============================================================================

type sheetFakeLoader struct {
	ProductLoader
	graphs   map[int64]*costroute.Graph
	products map[int64]*costproductmaster.CostProductMaster
	capText  map[int64]map[string]string
}

func (f *sheetFakeLoader) LoadRoutesByProducts(_ context.Context, _ []int64) (map[int64]*costroute.Graph, error) {
	return f.graphs, nil
}

func (f *sheetFakeLoader) LoadProducts(_ context.Context, _ []int64) (map[int64]*costproductmaster.CostProductMaster, error) {
	return f.products, nil
}

func (f *sheetFakeLoader) LoadCAPPText(_ context.Context, _ []int64) (map[int64]map[string]string, error) {
	return f.capText, nil
}

type sheetFakeResultRepo struct {
	calcdomain.ResultRepository
	byProduct map[int64]*calcdomain.Result
}

func (f *sheetFakeResultRepo) ListByProductIDsPeriodType(
	_ context.Context, ids []int64, _ string, _ calcdomain.CalculationType,
) (map[int64]*calcdomain.Result, error) {
	out := map[int64]*calcdomain.Result{}
	for _, id := range ids {
		if r, ok := f.byProduct[id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

// sheetResult builds a minimal CALCULATED result carrying only a param snapshot.
func sheetResult(productSysID int64, snapshot string) *calcdomain.Result {
	return calcdomain.NewResult(
		productSysID, "202604", calcdomain.CalcTypeActual, 1, 1,
		0, 0, 0, 0, 0, "IDR",
		nil, nil, []byte(snapshot), nil, "",
		0, "tester",
		0, 0, 0, 0, 0, 0, 0,
	)
}

// sevenStageGraph returns a 7-stage route deliberately out of (level, seq) order
// so the handler's own sort is what produces the expected ordering.
func sevenStageGraph() *costroute.Graph {
	order := []struct {
		level, seq int32
		id         int64
	}{
		{3, 1, 703}, {1, 2, 502}, {2, 1, 601},
		{1, 1, 501}, {2, 2, 602}, {4, 1, 800}, {1, 3, 503},
	}
	seqs := make([]*costroute.Seq, 0, len(order))
	for _, o := range order {
		seqs = append(seqs, &costroute.Seq{
			SeqID:         o.id,
			ProductSysID:  o.id,
			RouteLevel:    o.level,
			RouteSeq:      o.seq,
			RouteName:     "STAGE",
			RouteItemCode: "ITEM-" + string(rune('A'+o.id%7)),
		})
	}
	return &costroute.Graph{Seqs: seqs}
}

func newSheetHandler(graph *costroute.Graph, results map[int64]*calcdomain.Result) *GetRouteCostSheetHandler {
	svc := &Service{
		loader:     &sheetFakeLoader{graphs: map[int64]*costroute.Graph{800: graph}},
		resultRepo: &sheetFakeResultRepo{byProduct: results},
	}
	return NewGetRouteCostSheetHandler(svc)
}

// =============================================================================
// Tests
// =============================================================================

func TestGetRouteCostSheet_InvalidInput_Errors(t *testing.T) {
	t.Parallel()
	h := NewGetRouteCostSheetHandler(nil)

	_, err := h.Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 0, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.Error(t, err, "non-positive product id must be rejected")

	_, err = h.Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "2026", CalcType: calcdomain.CalcTypeActual,
	})
	require.Error(t, err, "period must be exactly 6 chars")
}

func TestGetRouteCostSheet_SevenStages_ReturnedInRouteOrder(t *testing.T) {
	t.Parallel()
	graph := sevenStageGraph()
	results := map[int64]*calcdomain.Result{}
	for _, s := range graph.Seqs {
		results[s.ProductSysID] = sheetResult(s.ProductSysID, `{"RM_RATE":12.5}`)
	}

	stages, err := newSheetHandler(graph, results).Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)
	require.Len(t, stages, 7)

	want := [][2]int32{{1, 1}, {1, 2}, {1, 3}, {2, 1}, {2, 2}, {3, 1}, {4, 1}}
	for i, w := range want {
		require.Equal(t, w[0], stages[i].RouteLevel, "stage %d level", i)
		require.Equal(t, w[1], stages[i].RouteSeq, "stage %d seq", i)
		require.True(t, stages[i].HasCost)
		require.Equal(t, "12.5", stages[i].ParamSnapshot["RM_RATE"])
	}
}

func TestGetRouteCostSheet_MissingMiddleStage_YieldsEmptyColumn(t *testing.T) {
	t.Parallel()
	graph := sevenStageGraph()
	results := map[int64]*calcdomain.Result{}
	for _, s := range graph.Seqs {
		if s.ProductSysID == 601 { // level 2, seq 1 — never calculated
			continue
		}
		results[s.ProductSysID] = sheetResult(s.ProductSysID, `{"RM_RATE":12.5}`)
	}

	stages, err := newSheetHandler(graph, results).Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)
	require.Len(t, stages, 7, "a missing cost row must not drop the column")

	withCost := 0
	for _, st := range stages {
		if st.ProductSysID == 601 {
			require.False(t, st.HasCost)
			require.Empty(t, st.ParamSnapshot, "absent params stay absent — never a fabricated zero")
			require.Equal(t, int32(2), st.RouteLevel)
			continue
		}
		require.True(t, st.HasCost)
		withCost++
	}
	require.Equal(t, 6, withCost)
}

func TestGetRouteCostSheet_NoRoute_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	svc := &Service{
		loader:     &sheetFakeLoader{graphs: map[int64]*costroute.Graph{}},
		resultRepo: &sheetFakeResultRepo{},
	}
	stages, err := NewGetRouteCostSheetHandler(svc).Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)
	require.Empty(t, stages)
}

// Text-valued params never reach cpc_param_snapshot — the evaluator scope is
// float-only — so they come from master data via LoadCAPPText. This covers the
// two properties that follow from that: they land even on an uncalculated
// stage, and they win over a same-code numeric (a stored label beats the 0 that
// buildInitialScope zero-fills for an absent numeric).
func TestGetRouteCostSheet_TextParams_FromMasterData(t *testing.T) {
	t.Parallel()
	graph := sevenStageGraph()
	results := map[int64]*calcdomain.Result{}
	for _, s := range graph.Seqs {
		if s.ProductSysID == 601 { // never calculated
			continue
		}
		results[s.ProductSysID] = sheetResult(s.ProductSysID, `{"RM_RATE":12.5,"MC_NAME":0}`)
	}

	svc := &Service{
		loader: &sheetFakeLoader{
			graphs: map[int64]*costroute.Graph{800: graph},
			capText: map[int64]map[string]string{
				501: {"MC_NAME": "D Line", "NS_LOSS_TYPE": "NA"},
				601: {"MC_NAME": "K Line"},
			},
		},
		resultRepo: &sheetFakeResultRepo{byProduct: results},
	}

	stages, err := NewGetRouteCostSheetHandler(svc).Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)

	byID := map[int64]RouteCostSheetStage{}
	for _, st := range stages {
		byID[st.ProductSysID] = st
	}

	calculated := byID[501]
	require.True(t, calculated.HasCost)
	require.Equal(t, "D Line", calculated.ParamSnapshot["MC_NAME"], "text must override the zero-filled numeric")
	require.Equal(t, "NA", calculated.ParamSnapshot["NS_LOSS_TYPE"])
	require.Equal(t, "12.5", calculated.ParamSnapshot["RM_RATE"], "numeric params are untouched")

	uncalculated := byID[601]
	require.False(t, uncalculated.HasCost)
	require.Equal(t, "K Line", uncalculated.ParamSnapshot["MC_NAME"],
		"master-data labels exist independently of a cost row")

	// No master text for 703, so the zero-filled numeric survives — the known
	// buildInitialScope deviation from Decision #8 (absent prints "-"), tracked
	// as backend finding I-1 rather than papered over here.
	require.Equal(t, "0", byID[703].ParamSnapshot["MC_NAME"],
		"without master text the zero-filled numeric leaks through")
}

func TestGetRouteCostSheet_CorruptSnapshot_DegradesOneColumn(t *testing.T) {
	t.Parallel()
	graph := sevenStageGraph()
	results := map[int64]*calcdomain.Result{}
	for _, s := range graph.Seqs {
		snap := `{"RM_RATE":12.5}`
		if s.ProductSysID == 703 {
			snap = `{not json`
		}
		results[s.ProductSysID] = sheetResult(s.ProductSysID, snap)
	}

	stages, err := newSheetHandler(graph, results).Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 800, Period: "202604", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err, "one corrupt snapshot must not fail the whole sheet")
	require.Len(t, stages, 7)
	for _, st := range stages {
		if st.ProductSysID == 703 {
			require.Empty(t, st.ParamSnapshot)
			continue
		}
		require.Equal(t, "12.5", st.ParamSnapshot["RM_RATE"])
	}
}
