package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

type superbaColourSheetLoader struct {
	sheetFakeLoader
	colours map[int64]string
}

func (f *superbaColourSheetLoader) LoadSuperbaColours(context.Context, []int64) (map[int64]string, error) {
	return f.colours, nil
}

// TestGetRouteCostSheet_SuperbaColourOverridesMBSpDye: only the SUPERBA stage with
// a resolved row shows the colour name; other stages keep their stored value, and
// a loader without the capability changes nothing.
func TestGetRouteCostSheet_SuperbaColourOverridesMBSpDye(t *testing.T) {
	t.Parallel()
	capText := func() map[int64]map[string]string {
		return map[int64]map[string]string{
			11: {"MB_SP_DYE": "stored-11"},
			12: {"MB_SP_DYE": "stored-12"},
			13: {},
		}
	}
	q := GetRouteCostSheetQuery{ProductSysID: 11, Period: "202609", CalcType: calcdomain.CalcTypeActual}
	graphs := map[int64]*costroute.Graph{11: oilSheetGraph()}

	loader := &superbaColourSheetLoader{
		sheetFakeLoader: sheetFakeLoader{graphs: graphs, capText: capText()},
		colours:         map[int64]string{11: "SUPERBA BLUE", 13: "SUPERBA RED"},
	}
	stages, err := NewGetRouteCostSheetHandler(&Service{loader: loader, resultRepo: &sheetFakeResultRepo{}}).Handle(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, stages, 3)
	assert.Equal(t, "SUPERBA BLUE", stages[0].ParamSnapshot["MB_SP_DYE"])
	assert.Equal(t, "stored-12", stages[1].ParamSnapshot["MB_SP_DYE"], "non-SUPERBA stage untouched")
	assert.Equal(t, "SUPERBA RED", stages[2].ParamSnapshot["MB_SP_DYE"], "override also fills an absent value")

	plain := &sheetFakeLoader{graphs: graphs, capText: capText()}
	stages, err = NewGetRouteCostSheetHandler(&Service{loader: plain, resultRepo: &sheetFakeResultRepo{}}).Handle(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, "stored-11", stages[0].ParamSnapshot["MB_SP_DYE"], "no capability -> stored value")
}
