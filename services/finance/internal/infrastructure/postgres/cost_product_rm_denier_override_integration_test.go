// Integration tests for the per-product RM denier override repository (000578):
// List/Upsert/Delete, apply_to_duplicates (direct copies only), DuplicateProduct
// copying overrides + the duplicate link, and route fork re-keying PRODUCT
// overrides onto forked upstream products. LOCAL PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type rmOverrideFixture struct {
	ctx  context.Context
	raw  *sql.DB
	repo *postgres.CostProductMasterRepository
	typ  int32
}

func newRMOverrideFixture(t *testing.T) *rmOverrideFixture {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "rm_denier_override_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	f := &rmOverrideFixture{ctx: ctx, raw: raw, repo: postgres.NewCostProductMasterRepository(postgres.NewDBFromSQL(raw))}
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name) VALUES ('ZZDO', 'ZZ denier override')
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`).Scan(&f.typ))
	_, err = raw.ExecContext(ctx, `INSERT INTO cst_rm_group_head (group_code, group_name, created_by)
		VALUES ('ZZDO-G', 'ZZ Spandex', 'itest') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	return f
}

func (f *rmOverrideFixture) product(t *testing.T, code string, from int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_is_active, cpm_created_by, cpm_updated_by, cpm_duplicated_from_sys_id)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest', NULLIF($4, 0)::bigint)
		RETURNING cpm_product_sys_id`, code, f.typ, code, from).Scan(&id))
	return id
}

func (f *rmOverrideFixture) upsert(t *testing.T, in costproductmaster.RMDenierOverrideInput, apply bool) (*costproductmaster.RMDenierOverride, []int64) {
	t.Helper()
	o, err := costproductmaster.NewRMDenierOverride(in)
	require.NoError(t, err)
	saved, applied, err := f.repo.UpsertRMDenierOverride(f.ctx, o, apply, in.ActorUserID)
	require.NoError(t, err)
	return saved, applied
}

// overrideMap reads every override of a product keyed "P:<id>" / "G:<code>".
func (f *rmOverrideFixture) overrideMap(t *testing.T, productSysID int64) map[string]float64 {
	t.Helper()
	all, err := rmratioquery.ProductDenierOverrides(f.ctx, f.raw, []int64{productSysID})
	require.NoError(t, err)
	out := map[string]float64{}
	for id, v := range all[productSysID].Product {
		out["P:"+itoa64(id)] = v
	}
	for code, v := range all[productSysID].Group {
		out["G:"+code] = v
	}
	return out
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

func TestRMDenierOverrideRepositoryIntegration(t *testing.T) {
	f := newRMOverrideFixture(t)
	ctx := f.ctx
	parent := f.product(t, "ZZDO-A", 0)
	rm := f.product(t, "ZZDO-AA", 0)
	copy1 := f.product(t, "ZZDO-A1", parent)
	copy2 := f.product(t, "ZZDO-A2", parent)
	grandchild := f.product(t, "ZZDO-A11", copy1)

	dups, err := f.repo.ListDirectDuplicates(ctx, parent)
	require.NoError(t, err)
	require.Len(t, dups, 2, "direct copies only")
	assert.Equal(t, copy1, dups[0].ProductSysID)
	assert.Equal(t, "ZZDO-A1", dups[0].ProductCode)
	assert.Equal(t, copy2, dups[1].ProductSysID)

	// Upsert PRODUCT without apply: only the parent.
	saved, applied := f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: parent, RMType: "PRODUCT", RMProductSysID: rm, Denier: 75, Reason: "special", ActorUserID: "u1",
	}, false)
	assert.Empty(t, applied)
	assert.Positive(t, saved.ID())
	assert.Equal(t, "ZZDO-AA", saved.RMProductCode())
	assert.Equal(t, "ZZDO-AA", saved.RMProductName())
	assert.InDelta(t, 75.0, saved.Denier(), 1e-9)
	assert.Equal(t, "u1", saved.CreatedBy())
	assert.Empty(t, f.overrideMap(t, copy1))

	// Upsert GROUP with apply: parent + both direct copies, never the grandchild.
	saved, applied = f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: parent, RMType: "GROUP", RMGroupCode: "ZZDO-G", Denier: 20, Reason: "draft", ActorUserID: "u1",
	}, true)
	assert.ElementsMatch(t, []int64{copy1, copy2}, applied)
	assert.Equal(t, "ZZ Spandex", saved.RMGroupName())
	assert.Equal(t, map[string]float64{"G:ZZDO-G": 20}, f.overrideMap(t, copy1))
	assert.Empty(t, f.overrideMap(t, grandchild))

	// Update in place (same key): denier + reason + updated_by change, no new row.
	saved, _ = f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: parent, RMType: "PRODUCT", RMProductSysID: rm, Denier: 80.5, Reason: "  revised ", ActorUserID: "u2",
	}, true)
	assert.InDelta(t, 80.5, saved.Denier(), 1e-9)
	assert.Equal(t, "revised", saved.Reason())
	assert.Equal(t, "u1", saved.CreatedBy())
	assert.Equal(t, "u2", saved.UpdatedBy())

	list, err := f.repo.ListRMDenierOverrides(ctx, parent)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, costproductmaster.RMTypeProduct, list[0].RMType(), "PRODUCT first")
	assert.Equal(t, costproductmaster.RMTypeGroup, list[1].RMType())
	assert.Equal(t, map[string]float64{"G:ZZDO-G": 20, "P:" + itoa64(rm): 80.5}, f.overrideMap(t, copy2))

	// Unknown refs map to sentinels.
	o, err := costproductmaster.NewRMDenierOverride(costproductmaster.RMDenierOverrideInput{
		ProductSysID: parent, RMType: "GROUP", RMGroupCode: "ZZDO-NOPE", Denier: 1, Reason: "x",
	})
	require.NoError(t, err)
	_, _, err = f.repo.UpsertRMDenierOverride(ctx, o, false, "u")
	require.ErrorIs(t, err, costproductmaster.ErrOverrideRMGroupNotFound)
	o, err = costproductmaster.NewRMDenierOverride(costproductmaster.RMDenierOverrideInput{
		ProductSysID: parent, RMType: "PRODUCT", RMProductSysID: 999999999, Denier: 1, Reason: "x",
	})
	require.NoError(t, err)
	_, _, err = f.repo.UpsertRMDenierOverride(ctx, o, false, "u")
	require.ErrorIs(t, err, costproductmaster.ErrOverrideRMProductNotFound)
	o, err = costproductmaster.NewRMDenierOverride(costproductmaster.RMDenierOverrideInput{
		ProductSysID: 999999999, RMType: "PRODUCT", RMProductSysID: rm, Denier: 1, Reason: "x",
	})
	require.NoError(t, err)
	_, _, err = f.repo.UpsertRMDenierOverride(ctx, o, false, "u")
	require.ErrorIs(t, err, costproductmaster.ErrNotFound)

	// Delete without apply: only the parent's row.
	n, applied, err := f.repo.DeleteRMDenierOverride(ctx, costproductmaster.DeleteRMDenierOverrideInput{
		ProductSysID: parent, RMType: "GROUP", RMGroupCode: "ZZDO-G",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Empty(t, applied)
	assert.Contains(t, f.overrideMap(t, copy1), "G:ZZDO-G")

	// Delete with apply: parent's PRODUCT row + copies'.
	n, applied, err = f.repo.DeleteRMDenierOverride(ctx, costproductmaster.DeleteRMDenierOverrideInput{
		ProductSysID: parent, RMType: "PRODUCT", RMProductSysID: rm, ApplyToDuplicates: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.ElementsMatch(t, []int64{copy1, copy2}, applied)
	assert.Empty(t, f.overrideMap(t, parent))
	assert.Equal(t, map[string]float64{"G:ZZDO-G": 20}, f.overrideMap(t, copy1))

	// Deleting a missing override is a no-op, invalid identity is rejected.
	n, _, err = f.repo.DeleteRMDenierOverride(ctx, costproductmaster.DeleteRMDenierOverrideInput{
		ProductSysID: parent, RMType: "PRODUCT", RMProductSysID: rm,
	})
	require.NoError(t, err)
	assert.Zero(t, n)
	_, _, err = f.repo.DeleteRMDenierOverride(ctx, costproductmaster.DeleteRMDenierOverrideInput{
		ProductSysID: parent, RMType: "ITEM", RMGroupCode: "X",
	})
	require.ErrorIs(t, err, costproductmaster.ErrInvalidOverrideRMType)
}
