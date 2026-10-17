package costproductmaster_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

func TestNewRMDenierOverride(t *testing.T) {
	base := costproductmaster.RMDenierOverrideInput{
		ProductSysID: 10, RMType: "product", RMProductSysID: 20, Denier: 75, Reason: "  special  ", ActorUserID: "u",
	}
	o, err := costproductmaster.NewRMDenierOverride(base)
	require.NoError(t, err)
	assert.Equal(t, costproductmaster.RMTypeProduct, o.RMType())
	assert.Equal(t, int64(20), o.RMProductSysID())
	assert.Empty(t, o.RMGroupCode())
	assert.Equal(t, "special", o.Reason())
	assert.InDelta(t, 75.0, o.Denier(), 1e-9)

	g := base
	g.RMType, g.RMProductSysID, g.RMGroupCode = "GROUP", 0, " G1 "
	o, err = costproductmaster.NewRMDenierOverride(g)
	require.NoError(t, err)
	assert.Equal(t, "G1", o.RMGroupCode())
	assert.Zero(t, o.RMProductSysID())

	cases := map[string]struct {
		mut  func(*costproductmaster.RMDenierOverrideInput)
		want error
	}{
		"no owner":           {func(i *costproductmaster.RMDenierOverrideInput) { i.ProductSysID = 0 }, costproductmaster.ErrNotFound},
		"item type":          {func(i *costproductmaster.RMDenierOverrideInput) { i.RMType = "ITEM" }, costproductmaster.ErrInvalidOverrideRMType},
		"product no id":      {func(i *costproductmaster.RMDenierOverrideInput) { i.RMProductSysID = 0 }, costproductmaster.ErrInvalidOverrideRMIdentity},
		"product with group": {func(i *costproductmaster.RMDenierOverrideInput) { i.RMGroupCode = "G" }, costproductmaster.ErrInvalidOverrideRMIdentity},
		"product self":       {func(i *costproductmaster.RMDenierOverrideInput) { i.RMProductSysID = 10 }, costproductmaster.ErrInvalidOverrideRMIdentity},
		"group with product": {func(i *costproductmaster.RMDenierOverrideInput) { i.RMType = "GROUP"; i.RMGroupCode = "G" }, costproductmaster.ErrInvalidOverrideRMIdentity},
		"group no code":      {func(i *costproductmaster.RMDenierOverrideInput) { i.RMType = "GROUP"; i.RMProductSysID = 0 }, costproductmaster.ErrInvalidOverrideRMIdentity},
		"zero denier":        {func(i *costproductmaster.RMDenierOverrideInput) { i.Denier = 0 }, costproductmaster.ErrInvalidOverrideDenier},
		"negative denier":    {func(i *costproductmaster.RMDenierOverrideInput) { i.Denier = -1 }, costproductmaster.ErrInvalidOverrideDenier},
		"blank reason":       {func(i *costproductmaster.RMDenierOverrideInput) { i.Reason = "  " }, costproductmaster.ErrInvalidOverrideReason},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			c.mut(&in)
			_, err := costproductmaster.NewRMDenierOverride(in)
			assert.ErrorIs(t, err, c.want)
		})
	}
}
