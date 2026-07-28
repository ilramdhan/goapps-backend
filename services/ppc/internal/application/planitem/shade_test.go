package planitem_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// stubProducts is a scripted ProductLookup keyed by product sys id.
type stubProducts struct {
	byID map[int64]*financev1.CostMasterProduct
	err  error
}

func (s stubProducts) BatchGetProducts(_ context.Context, sysIDs []int64) ([]*financev1.CostMasterProduct, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]*financev1.CostMasterProduct, 0, len(sysIDs))
	for _, id := range sysIDs {
		if p, ok := s.byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func shadeService(lookup planitemapp.ProductLookup) (*planitemapp.Service, *memRepo) {
	repo := newMemRepo()
	return planitemapp.NewService(repo, nil, lookup).WithCapacity(fixedCapacity{perDay: 100}), repo
}

func TestCreate_ResolvesShadeFromFinanceProduct(t *testing.T) {
	svc, _ := shadeService(stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		100: {ProductSysId: 100, ShadeCode: "5918-01", ShadeName: "TURQUOISE"},
	}})

	res, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)

	assert.Equal(t, "5918-01", res.Item.ShadeCode())
	assert.Equal(t, "TURQUOISE", res.Item.ShadeName())
}

// Shade is descriptive, not structural: an unresolvable product must still
// create the plan item, just without a shade.
func TestCreate_UnknownProductLeavesShadeEmpty(t *testing.T) {
	svc, _ := shadeService(stubProducts{byID: map[int64]*financev1.CostMasterProduct{}})

	res, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)

	assert.Empty(t, res.Item.ShadeCode())
	assert.Empty(t, res.Item.ShadeName())
}

// Without a wired route provider an FG must still be creatable — it just
// yields no upstream items, and says so.
func TestCreate_FGWithoutRouteProviderYieldsWarningNotError(t *testing.T) {
	svc, _ := shadeService(stubProducts{byID: map[int64]*financev1.CostMasterProduct{
		100: {ProductSysId: 100, ShadeCode: "5918-01", ShadeName: "TURQUOISE"},
	}})

	res, err := svc.Create(context.Background(), fgCreateCmd())
	require.NoError(t, err)

	assert.Empty(t, res.Children)
	assert.NotEmpty(t, res.Warning)
	assert.Equal(t, "5918-01", res.Item.ShadeCode())
}

// fgCreateCmd is createCmd retargeted at an FG_DELIVERY item, which is the only
// type that triggers the route cascade.
func fgCreateCmd() planitemapp.CreateCommand {
	cmd := createCmd()
	cmd.Type = planitemdomain.TypeFGDelivery
	demandID := int64(10)
	cmd.DemandID = &demandID
	cmd.ParentItemID = nil
	return cmd
}
