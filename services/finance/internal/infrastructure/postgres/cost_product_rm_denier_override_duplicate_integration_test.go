// Integration tests for RM denier overrides (000578) across product duplicate
// and route fork: DuplicateProduct copies overrides + records the duplicate
// link (always, D2); a NEW_PRODUCT route fork copies them onto every forked
// product and re-keys PRODUCT overrides onto forked upstream products (D3).
// LOCAL PostgreSQL only, via internal/testutil/pgcontainer. Skipped unless
// INTEGRATION_TEST=true.
package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestRMDenierOverrideDuplicateProductIntegration(t *testing.T) {
	f := newRMOverrideFixture(t)
	ctx := f.ctx
	src := f.product(t, "ZZDO-S", 0)
	rm := f.product(t, "ZZDO-SR", 0)
	f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: src, RMType: "PRODUCT", RMProductSysID: rm, Denier: 70, Reason: "p", ActorUserID: "u",
	}, false)
	f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: src, RMType: "GROUP", RMGroupCode: "ZZDO-G", Denier: 22, Reason: "g", ActorUserID: "u",
	}, false)

	// CopyParams=false still copies overrides (D2) and records the link.
	out, err := f.repo.DuplicateProduct(ctx, costproductmaster.DuplicateInput{
		ProductSysID: src, NewCodePrefix: "ZZDO-SD", CopyParams: false, ActorUserID: "dup",
	})
	require.NoError(t, err)
	assert.Equal(t, f.overrideMap(t, src), f.overrideMap(t, out.NewProductSysID))
	var from sql.NullInt64
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT cpm_duplicated_from_sys_id FROM cost_product_master WHERE cpm_product_sys_id = $1`,
		out.NewProductSysID).Scan(&from))
	assert.Equal(t, src, from.Int64)
	list, err := f.repo.ListRMDenierOverrides(ctx, out.NewProductSysID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "dup", list[0].CreatedBy())

	dups, err := f.repo.ListDirectDuplicates(ctx, src)
	require.NoError(t, err)
	require.Len(t, dups, 1)
	assert.Equal(t, out.NewProductSysID, dups[0].ProductSysID)
}

func TestRMDenierOverrideRouteForkIntegration(t *testing.T) {
	f := newRMOverrideFixture(t)
	ctx := f.ctx
	fg := f.product(t, "ZZDO-FG", 0)
	poy := f.product(t, "ZZDO-POY", 0)
	ext := f.product(t, "ZZDO-EXT", 0) // RM product not in the route: never forked

	var headID, seqL1, seqL2 int64
	require.NoError(t, f.raw.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'itest', 'itest') RETURNING crh_head_id`, fg).Scan(&headID))
	require.NoError(t, f.raw.QueryRowContext(ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, fg).Scan(&seqL1))
	require.NoError(t, f.raw.QueryRowContext(ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 2, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, poy).Scan(&seqL2))
	_, err := f.raw.ExecContext(ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 1.0, 'itest', 'itest')`, seqL1, fg, poy)
	require.NoError(t, err)
	_, err = f.raw.ExecContext(ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, 'ZZDO-G', 'GROUP', 1.0, 'itest', 'itest')`, seqL2, poy)
	require.NoError(t, err)

	f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: fg, RMType: "PRODUCT", RMProductSysID: poy, Denier: 75, Reason: "poy", ActorUserID: "u",
	}, false)
	f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: fg, RMType: "PRODUCT", RMProductSysID: ext, Denier: 50, Reason: "ext", ActorUserID: "u",
	}, false)
	f.upsert(t, costproductmaster.RMDenierOverrideInput{
		ProductSysID: poy, RMType: "GROUP", RMGroupCode: "ZZDO-G", Denier: 20, Reason: "g", ActorUserID: "u",
	}, false)

	routeRepo := postgres.NewCostRouteRepository(postgres.NewDBFromSQL(f.raw))
	out, err := routeRepo.DuplicateRoute(ctx, costroute.DuplicateInput{
		SourceHeadID: headID, IncludeRouting: true, IncludeUpstream: true,
		NewCodePrefix: "ZZDO-FK", ActorUserID: "fork",
	})
	require.NoError(t, err)

	var newPOY int64
	require.NoError(t, f.raw.QueryRowContext(ctx, `
		SELECT crs_product_sys_id FROM cost_route_seq
		WHERE crs_head_id = $1 AND crs_route_level = 2`, out.NewHeadID).Scan(&newPOY))
	require.NotEqual(t, poy, newPOY)

	// FG fork: POY override re-keyed to the forked POY; EXT override kept as-is.
	assert.Equal(t, map[string]float64{"P:" + itoa64(newPOY): 75, "P:" + itoa64(ext): 50},
		f.overrideMap(t, out.NewProductSysID))
	assert.Equal(t, map[string]float64{"G:ZZDO-G": 20}, f.overrideMap(t, newPOY))
	// Source products untouched.
	assert.Equal(t, map[string]float64{"P:" + itoa64(poy): 75, "P:" + itoa64(ext): 50}, f.overrideMap(t, fg))

	for _, pair := range [][2]int64{{out.NewProductSysID, fg}, {newPOY, poy}} {
		var from sql.NullInt64
		require.NoError(t, f.raw.QueryRowContext(ctx,
			`SELECT cpm_duplicated_from_sys_id FROM cost_product_master WHERE cpm_product_sys_id = $1`, pair[0]).Scan(&from))
		assert.Equal(t, pair[1], from.Int64)
	}

	// SAME_PRODUCT fork creates no product: overrides unchanged, no new copies.
	before := f.overrideMap(t, fg)
	_, err = f.raw.ExecContext(ctx, `UPDATE cost_route_head SET crh_routing_status = 'LOCKED' WHERE crh_head_id = $1`, out.NewHeadID)
	require.NoError(t, err)
	_, err = routeRepo.DuplicateRoute(ctx, costroute.DuplicateInput{
		SourceHeadID: headID, IncludeRouting: true, ActorUserID: "fork",
		TargetMode: costroute.DuplicateTargetModeSameProduct,
	})
	require.NoError(t, err)
	assert.Equal(t, before, f.overrideMap(t, fg))
	dups, err := f.repo.ListDirectDuplicates(ctx, fg)
	require.NoError(t, err)
	assert.Len(t, dups, 1)
}
