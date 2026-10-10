package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type stubSuperbaProvider struct{ data map[string]mbsource.Resolution }

func (s stubSuperbaProvider) Name() string { return "stub" }
func (s stubSuperbaProvider) ResolveByShades(_ context.Context, shades []string) (map[string]mbsource.Resolution, error) {
	out := map[string]mbsource.Resolution{}
	for _, sh := range shades {
		if r, ok := s.data[mbsource.NormalizeShade(sh)]; ok {
			out[mbsource.NormalizeShade(sh)] = r
		}
	}
	return out, nil
}

func TestFillFromMBSpin_SuperbaShadeFallback(t *testing.T) {
	dye := "SKY BLUE"
	prov := stubSuperbaProvider{data: map[string]mbsource.Resolution{
		"MC-0547": {SPCode: "MC-0547", DyeName: dye, Children: map[string]mbsource.ChildValue{mbsource.ParamSPDye: {Text: &dye}}},
	}}
	repo := &resolveFillFakeRepo{byOrionErr: mbspin.ErrNotFound, byMBCostingErr: mbspin.ErrNotFound}

	t.Run("superba shade fills MB_SP_DYE instead of NotFound", func(t *testing.T) {
		h := &YarnLookupFillHandler{mbSpinRepo: repo}
		h.WithSuperbaFallback(prov)
		resp, err := h.fillFromMBSpin(context.Background(), "mc-0547", "MB_SP_CODE")
		require.NoError(t, err)
		assert.True(t, resp.GetBase().GetIsSuccess())
		assert.Equal(t, "SKY BLUE", resp.GetTextFills()[mbsource.ParamSPDye])
		assert.Empty(t, resp.GetNumericFills())
	})

	t.Run("unknown key still reports not found", func(t *testing.T) {
		h := &YarnLookupFillHandler{mbSpinRepo: repo}
		h.WithSuperbaFallback(prov)
		resp, err := h.fillFromMBSpin(context.Background(), "NOPE", "MB_SP_CODE")
		require.NoError(t, err)
		assert.False(t, resp.GetBase().GetIsSuccess())
	})

	t.Run("without fallback behaviour is unchanged", func(t *testing.T) {
		h := &YarnLookupFillHandler{mbSpinRepo: repo}
		resp, err := h.fillFromMBSpin(context.Background(), "MC-0547", "MB_SP_CODE")
		require.NoError(t, err)
		assert.False(t, resp.GetBase().GetIsSuccess())
	})
}
