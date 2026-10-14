package costroute

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// fakeRatioLoader returns fixed ratio sources and records the graph it saw.
type fakeRatioLoader struct {
	src   costroute.RatioSources
	err   error
	calls int
}

func (f *fakeRatioLoader) LoadRMRatioSources(_ context.Context, _ *costroute.Graph) (costroute.RatioSources, error) {
	f.calls++
	return f.src, f.err
}

// fixedGraphRepo serves a fresh graph per call; every other Repository
// method is unimplemented (nil embedded interface) and must not be called.
type fixedGraphRepo struct {
	costroute.Repository
	build func() *costroute.Graph
}

func (r *fixedGraphRepo) GetGraph(_ context.Context, _ int64) (*costroute.Graph, error) {
	return r.build(), nil
}

const (
	ratioFG   int64 = 100 // ACY, DENIER
	ratioPTY  int64 = 200 // PTY, INPUT; produced at level 2
	ratioPOY  int64 = 300 // upstream of PTY
	ratioMB   int64 = 400 // nested MB product, INPUT
	ratioSpd        = "SPD1"
	ratioOil        = "OIL1"
	ratioDelt       = 1e-9
)

// mixedGraph: level 1 ACY (DENIER) = PTY + spandex + oil + MB; level 2 PTY
// (INPUT) = POY 1.02.
func mixedGraph() *costroute.Graph {
	l1 := mkSeq(1, 1,
		mkProductRm(11, ratioPTY, 1),
		mkGroupRm(12, ratioSpd, 1),
		mkGroupRm(13, ratioOil, 0.03),
		mkProductRm(14, ratioMB, 0.5),
	)
	l1.ProductSysID = ratioFG
	l2 := mkSeq(2, 2, mkProductRm(21, ratioPOY, 1.02))
	l2.ProductSysID = ratioPTY
	return mkGraph(1, ratioFG, "ACY1", l1, l2)
}

func mixedSources() costroute.RatioSources {
	return costroute.RatioSources{
		Modes:          map[int64]string{ratioFG: costroute.RMRatioModeDenier, ratioPTY: costroute.RMRatioModeInput},
		UpstreamDenier: map[int64]float64{ratioPTY: 150, ratioMB: 0},
		GroupDenier:    map[string]float64{ratioSpd: 40},
		GroupUseDraft:  map[string]bool{ratioSpd: true},
		SpandexDraft:   map[int64]float64{ratioFG: 3},
	}
}

func TestGetGraphHandler_RatioLoader_AnnotatesMixedModeGraph(t *testing.T) {
	loader := &fakeRatioLoader{src: mixedSources()}
	h := NewGetGraphHandler(&fixedGraphRepo{build: mixedGraph}).WithRatioLoader(loader)

	g, err := h.Handle(context.Background(), 1, false)
	require.NoError(t, err)
	assert.Equal(t, 1, loader.calls)
	assert.Equal(t, costroute.RMRatioModeDenier, g.Head.RMRatioMode)
	assert.Equal(t, costroute.RMRatioModeDenier, g.Seqs[0].RMRatioMode)
	assert.Equal(t, costroute.RMRatioModeInput, g.Seqs[1].RMRatioMode)

	spd := 40.0 / 3
	pty, sp, oil, mb := g.Seqs[0].Rms[0], g.Seqs[0].Rms[1], g.Seqs[0].Rms[2], g.Seqs[0].Rms[3]
	assert.InDelta(t, 150/(150+spd), pty.ComputedRatio, ratioDelt)
	assert.InDelta(t, pty.ComputedRatio, pty.EffectiveRatio, ratioDelt, "DENIER lines drive EffectiveRatio")
	assert.Equal(t, costroute.RatioSourceDenier, pty.RatioSource)
	assert.InDelta(t, 150, pty.Denier, ratioDelt)
	assert.InDelta(t, spd/(150+spd), sp.ComputedRatio, ratioDelt)
	assert.InDelta(t, 1.0, sp.RouteRmRatio, ratioDelt, "stored ratio untouched")

	assert.Equal(t, costroute.RatioSourceInput, oil.RatioSource)
	assert.InDelta(t, 0.03, oil.ComputedRatio, ratioDelt)
	assert.Zero(t, oil.EffectiveRatio, "non-denier lines keep the flattener/mapper default")

	assert.Equal(t, costroute.RatioSourceDenier, mb.RatioSource)
	assert.Zero(t, mb.ComputedRatio)
	assert.Contains(t, mb.RatioNote, "upstream product 400 has no DENIER")

	poy := g.Seqs[1].Rms[0]
	assert.Empty(t, poy.RatioSource, "INPUT-mode seq: source empty")
	assert.InDelta(t, 1.02, poy.ComputedRatio, ratioDelt)
}

// TestGetGraphHandler_RatioLoader_NestedCompoundingUsesComputedRatio: a
// DENIER-mode line pointing at a nested MB compounds the MB's own rows by the
// computed ratio; the spliced rows keep their own head's (unannotated) values.
func TestGetGraphHandler_RatioLoader_NestedCompoundingUsesComputedRatio(t *testing.T) {
	src := mixedSources()
	src.UpstreamDenier[ratioMB] = 150 // MB line now shares with PTY + spandex
	nested := mkGraph(9, ratioMB, "MB1", func() *costroute.Seq {
		s := mkSeq(91, 1, mkGroupRm(911, "PIG", 0.25))
		s.ProductSysID = ratioMB
		return s
	}())
	flattener := NewNestedMBFlattener(
		&fakeLoader{graphs: map[int64]*costroute.Graph{ratioMB: nested}},
		&fakeChecker{mbIDs: map[int64]bool{ratioMB: true}},
	)
	h := NewGetGraphHandler(&fixedGraphRepo{build: mixedGraph}).
		WithRatioLoader(&fakeRatioLoader{src: src}).
		WithNestedMBFlattener(flattener)

	g, err := h.Handle(context.Background(), 1, true)
	require.NoError(t, err)
	spd := 40.0 / 3
	mbRatio := 150 / (300 + spd)
	assert.InDelta(t, mbRatio, g.Seqs[0].Rms[3].EffectiveRatio, ratioDelt)

	var spliced *costroute.Rm
	for _, s := range g.Seqs {
		if s.OriginHeadID == 9 {
			spliced = s.Rms[0]
		}
	}
	require.NotNil(t, spliced)
	assert.InDelta(t, mbRatio*0.25, spliced.EffectiveRatio, ratioDelt)
	assert.Empty(t, spliced.RatioSource, "spliced rows are not re-annotated")
	assert.Zero(t, spliced.ComputedRatio)
}

func TestGetGraphHandler_NilRatioLoader_Unchanged(t *testing.T) {
	g, err := NewGetGraphHandler(&fixedGraphRepo{build: mixedGraph}).Handle(context.Background(), 1, false)
	require.NoError(t, err)
	assert.Equal(t, mixedGraph(), g)
}

func TestGetGraphHandler_RatioLoaderError(t *testing.T) {
	sentinel := errors.New("boom")
	h := NewGetGraphHandler(&fixedGraphRepo{build: mixedGraph}).WithRatioLoader(&fakeRatioLoader{err: sentinel})
	_, err := h.Handle(context.Background(), 1, false)
	require.ErrorIs(t, err, sentinel)
}
