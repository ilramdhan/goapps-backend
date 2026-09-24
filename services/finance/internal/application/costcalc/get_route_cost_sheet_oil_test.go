package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// oilNameSheetLoader adds the OilGroupNameLoader capability to sheetFakeLoader.
type oilNameSheetLoader struct {
	sheetFakeLoader
	names    map[string]string
	gotCodes []string
}

func (f *oilNameSheetLoader) LoadRMGroupNames(_ context.Context, codes []string) (map[string]string, error) {
	f.gotCodes = append(f.gotCodes, codes...)
	return f.names, nil
}

func oilSheetGraph() *costroute.Graph {
	return &costroute.Graph{Seqs: []*costroute.Seq{
		{SeqID: 1, ProductSysID: 11, RouteLevel: 1, RouteSeq: 1},
		{SeqID: 2, ProductSysID: 12, RouteLevel: 2, RouteSeq: 1},
		{SeqID: 3, ProductSysID: 13, RouteLevel: 3, RouteSeq: 1},
	}}
}

// TestGetRouteCostSheet_OilGroupNameResolved (D18): each stage gets the RM
// group name behind its OIL_NAME code, codes are looked up once each, and the
// snapshot keeps the code so the gRPC read model is unchanged.
func TestGetRouteCostSheet_OilGroupNameResolved(t *testing.T) {
	t.Parallel()
	loader := &oilNameSheetLoader{
		sheetFakeLoader: sheetFakeLoader{
			graphs: map[int64]*costroute.Graph{11: oilSheetGraph()},
			capText: map[int64]map[string]string{
				11: {"OIL_NAME": "202006101"},
				12: {"OIL_NAME": " 202006101 "},
				13: {"OIL_NAME": "UNKNOWN1"},
			},
		},
		names: map[string]string{"202006101": "CONING OIL"},
	}
	h := NewGetRouteCostSheetHandler(&Service{loader: loader, resultRepo: &sheetFakeResultRepo{}})

	stages, err := h.Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 11, Period: "202609", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)
	require.Len(t, stages, 3)

	assert.ElementsMatch(t, []string{"202006101", "UNKNOWN1"}, loader.gotCodes)
	assert.Equal(t, "CONING OIL", stages[0].OilGroupName)
	assert.Equal(t, "CONING OIL", stages[1].OilGroupName, "stored code is trimmed before lookup")
	assert.Empty(t, stages[2].OilGroupName, "unknown code resolves to no name (exporter falls back to the code)")
	assert.Equal(t, "202006101", stages[0].ParamSnapshot["OIL_NAME"], "snapshot keeps the code")
}

// TestGetRouteCostSheet_LoaderWithoutNameCapability: a loader lacking
// OilGroupNameLoader leaves OilGroupName empty rather than failing.
func TestGetRouteCostSheet_LoaderWithoutNameCapability(t *testing.T) {
	t.Parallel()
	loader := &sheetFakeLoader{
		graphs:  map[int64]*costroute.Graph{11: oilSheetGraph()},
		capText: map[int64]map[string]string{11: {"OIL_NAME": "202006101"}},
	}
	h := NewGetRouteCostSheetHandler(&Service{loader: loader, resultRepo: &sheetFakeResultRepo{}})

	stages, err := h.Handle(context.Background(), GetRouteCostSheetQuery{
		ProductSysID: 11, Period: "202609", CalcType: calcdomain.CalcTypeActual,
	})
	require.NoError(t, err)
	require.Len(t, stages, 3)
	for _, s := range stages {
		assert.Empty(t, s.OilGroupName)
	}
}
