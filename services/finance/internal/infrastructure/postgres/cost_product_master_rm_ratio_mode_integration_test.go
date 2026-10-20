// Integration tests for the per-product RM ratio mode (000579) on the product
// master repository: read (Get/List with effective mode), SaveRMRatioMode
// set/clear, and DuplicateProduct copying cpm_rm_ratio_mode. LOCAL PostgreSQL
// only, via internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
)

func TestCostProductMasterRMRatioModeIntegration(t *testing.T) {
	f := newRMOverrideFixture(t) // type ZZDO, cpt_rm_ratio_mode default INPUT
	ctx := f.ctx
	a := f.product(t, "ZZRMM-A", 0)

	// Default: no override, effective = type mode.
	p, err := f.repo.GetBySysID(ctx, a)
	require.NoError(t, err)
	assert.Empty(t, p.RMRatioModeOverride())
	assert.Equal(t, "INPUT", p.EffectiveRMRatioMode())

	// Set DENIER.
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	require.NoError(t, f.repo.SaveRMRatioMode(ctx, costproductmaster.RMRatioModeWrite{
		ProductSysID: a, Override: "DENIER", UpdatedAt: at, UpdatedBy: "u1",
	}))
	p, err = f.repo.GetByCode(ctx, "ZZRMM-A")
	require.NoError(t, err)
	assert.Equal(t, "DENIER", p.RMRatioModeOverride())
	assert.Equal(t, "DENIER", p.EffectiveRMRatioMode())
	assert.Equal(t, "u1", p.UpdatedBy())
	modes, err := rmratioquery.ProductRatioModes(ctx, f.raw, []int64{a})
	require.NoError(t, err)
	assert.Equal(t, "DENIER", modes[a], "engine/graph query sees the override")

	items, _, err := f.repo.List(ctx, costproductmaster.Filter{Search: "ZZRMM-A"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "DENIER", items[0].RMRatioModeOverride())

	// Duplicate copies the override (like RM denier overrides).
	out, err := f.repo.DuplicateProduct(ctx, costproductmaster.DuplicateInput{
		ProductSysID: a, NewCodePrefix: "ZZRMM-D", ActorUserID: "u1",
	})
	require.NoError(t, err)
	dup, err := f.repo.GetBySysID(ctx, out.NewProductSysID)
	require.NoError(t, err)
	assert.Equal(t, "DENIER", dup.RMRatioModeOverride())

	// Clear: "" stores NULL, effective follows the type again.
	require.NoError(t, f.repo.SaveRMRatioMode(ctx, costproductmaster.RMRatioModeWrite{
		ProductSysID: a, Override: "", UpdatedAt: at, UpdatedBy: "u2",
	}))
	var raw sql.NullString
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT cpm_rm_ratio_mode FROM cost_product_master WHERE cpm_product_sys_id = $1`, a).Scan(&raw))
	assert.False(t, raw.Valid)
	p, err = f.repo.GetBySysID(ctx, a)
	require.NoError(t, err)
	assert.Empty(t, p.RMRatioModeOverride())
	assert.Equal(t, "INPUT", p.EffectiveRMRatioMode())

	// Duplicate of a NULL-mode product stays NULL.
	out, err = f.repo.DuplicateProduct(ctx, costproductmaster.DuplicateInput{
		ProductSysID: a, NewCodePrefix: "ZZRMM-E", ActorUserID: "u1",
	})
	require.NoError(t, err)
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT cpm_rm_ratio_mode FROM cost_product_master WHERE cpm_product_sys_id = $1`, out.NewProductSysID).Scan(&raw))
	assert.False(t, raw.Valid)

	// Unknown product.
	err = f.repo.SaveRMRatioMode(ctx, costproductmaster.RMRatioModeWrite{ProductSysID: 999999999, Override: "INPUT", UpdatedAt: at, UpdatedBy: "u"})
	require.ErrorIs(t, err, costproductmaster.ErrNotFound)
}
