package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

type fakeColours struct {
	m   map[int64]string
	err error
}

func (f fakeColours) LoadSuperbaColours(context.Context, []int64) (map[int64]string, error) {
	return f.m, f.err
}

func TestApplySuperbaDisplay(t *testing.T) {
	mk := func() []*financev1.RequiredParamEntry {
		return []*financev1.RequiredParamEntry{
			{ParamCode: "MB_SP_DYE", ValueText: "stored"},
			{ParamCode: "OTHER"},
		}
	}
	h := (&CostProductParameterHandler{}).WithSuperbaColours(fakeColours{m: map[int64]string{7: "SUPERBA BLUE"}})

	es := mk()
	h.applySuperbaDisplay(context.Background(), 7, es)
	assert.Equal(t, "SUPERBA BLUE", es[0].DisplayValue)
	assert.Equal(t, "stored", es[0].ValueText, "stored value untouched")
	assert.Empty(t, es[1].DisplayValue, "only MB_SP_DYE gets a display value")

	es = mk()
	h.applySuperbaDisplay(context.Background(), 8, es) // non-SUPERBA / unresolved
	assert.Empty(t, es[0].DisplayValue)

	es = mk()
	(&CostProductParameterHandler{}).applySuperbaDisplay(context.Background(), 7, es) // no resolver
	assert.Empty(t, es[0].DisplayValue)

	es = mk()
	(&CostProductParameterHandler{}).WithSuperbaColours(fakeColours{err: errors.New("db")}).applySuperbaDisplay(context.Background(), 7, es)
	assert.Empty(t, es[0].DisplayValue, "resolver failure degrades to stored value")
}
