// Integration test pinning ListProductSysIDs (the export's lean filter
// resolution) to ListResults: for every filter, the ids must be exactly the
// distinct product ids ListResults returns across all of its pages, in the
// same order. LOCAL PostgreSQL only, via internal/testutil/pgcontainer.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const productIDsITPeriod = "202611"

func TestCostResultListProductSysIDsParityIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "cost_result_product_ids_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	seedProductIDsFixture(ctx, t, raw)
	repo := postgres.NewCostResultRepository(postgres.NewDBFromSQL(raw))

	var typeA, typeB int32
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT min(cpt_type_id), max(cpt_type_id) FROM cost_product_type`).Scan(&typeA, &typeB))

	filters := map[string]costcalc.ResultListFilter{
		"period only (actual+forecast, dedup)": {},
		"actual":                               {CalcType: costcalc.CalcTypeActual},
		"forecast":                             {CalcType: costcalc.CalcTypeForecast},
		"status approved":                      {Status: "APPROVED"},
		"status superseded":                    {Status: "SUPERSEDED"},
		"search product code":                  {Search: "PIDS-1"},
		"search erp item name":                 {Search: "nylon yarn"},
		"search erp item code fallback":        {Search: "POY-PIDS-3-for selling"},
		"search shade name":                    {Search: "midnight"},
		"search rm group name":                 {Search: "bright chips"},
		"search rm item name":                  {Search: "titanium"},
		"search rm product ref":                {Search: "PIDS-INT"},
		"search no match":                      {Search: "zz-no-such-thing"},
		"product type":                         {ProductTypeIDs: []int32{typeA}},
		"two product types":                    {ProductTypeIDs: []int32{typeA, typeB}},
		"shade codes":                          {ShadeCodes: []string{"BLK", "RED"}},
		"rm group code":                        {RMGroupCodes: []string{"PIDS-BRT"}},
		"rm group codes":                       {RMGroupCodes: []string{"PIDS-BRT", "PIDS-SD"}},
		"combined":                             {CalcType: costcalc.CalcTypeActual, ShadeCodes: []string{"BLK"}, RMGroupCodes: []string{"PIDS-BRT"}},
		"sort by product code asc":             {SortBy: "productCode", SortOrder: "asc"},
		"sort by total cost desc":              {SortBy: "totalCost", SortOrder: "desc"},
	}
	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			f.Period = productIDsITPeriod
			want := pagedListResultIDs(ctx, t, repo, f)
			got, err := repo.ListProductSysIDs(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	// Sanity: the fixture actually exercises dedup and the RM-name search.
	all, err := repo.ListProductSysIDs(ctx, costcalc.ResultListFilter{Period: productIDsITPeriod})
	require.NoError(t, err)
	assert.Len(t, all, 6)
	for _, s := range []string{"bright chips", "titanium", "PIDS-INT", "nylon yarn"} {
		ids, err := repo.ListProductSysIDs(ctx, costcalc.ResultListFilter{Period: productIDsITPeriod, Search: s})
		require.NoError(t, err)
		assert.NotEmpty(t, ids, "search %q", s)
	}

	// Empty Period resolves to the latest plausible period, like ListResults.
	_, _, latest, err := repo.ListResults(ctx, costcalc.ResultListFilter{PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, productIDsITPeriod, latest)
	got, err := repo.ListProductSysIDs(ctx, costcalc.ResultListFilter{})
	require.NoError(t, err)
	assert.Equal(t, all, got)
}

// pagedListResultIDs pages ListResults with a small page size (so several
// pages are exercised) and returns the distinct product ids in first-seen
// order — exactly what the export's filter resolution used to compute.
func pagedListResultIDs(ctx context.Context, t *testing.T, repo *postgres.CostResultRepository, f costcalc.ResultListFilter) []int64 {
	t.Helper()
	ids := []int64{}
	seen := map[int64]bool{}
	f.PageSize = 3
	fetched := 0
	for page := 1; ; page++ {
		f.Page = page
		items, total, _, err := repo.ListResults(ctx, f)
		require.NoError(t, err)
		for _, it := range items {
			if !seen[it.ProductSysID] {
				seen[it.ProductSysID] = true
				ids = append(ids, it.ProductSysID)
			}
		}
		fetched += len(items)
		if fetched >= total || len(items) == 0 {
			return ids
		}
	}
}

// seedProductIDsFixture creates 6 products (+1 intermediate referenced from RM
// detail) with ACTUAL/FORECAST results, statuses, shades, ERP items and RM
// detail lines of every kind, plus a SUPERSEDED version.
func seedProductIDsFixture(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		_, err := db.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO cst_rm_group_head (group_code, group_name, created_by) VALUES
		('PIDS-BRT', 'Bright Chips PIDS', 'it'), ('PIDS-SD', 'Semi Dull PIDS', 'it')`)
	exec(`INSERT INTO cost_erp_item (cei_item_code, cei_item_name) VALUES
		('PIDS-ITEM-1', 'Nylon Yarn PIDS'), ('PIDS-TIO2', 'Titanium Dioxide PIDS')`)

	var typeA, typeB int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT min(cpt_type_id), max(cpt_type_id) FROM cost_product_type`).Scan(&typeA, &typeB))

	product := func(code string, typeID int, shade, shadeName, erpItem string) (int64, int64) {
		var pid int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
				cpm_shade_code, cpm_shade_name, cpm_erp_item_code, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), 'it', 'it')
			RETURNING cpm_product_sys_id`, code, typeID, "product "+code, shade, shadeName, erpItem).Scan(&pid))
		var hid int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
			VALUES ($1, 'DRAFT', 1, 'it', 'it') RETURNING crh_head_id`, pid).Scan(&hid))
		return pid, hid
	}
	interID, _ := product("PIDS-INT", typeA, "", "", "")
	result := func(pid, hid int64, calcType, status string, version int, total float64, rm string) {
		exec(`INSERT INTO cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type,
				cpc_route_head_id, cpc_version, cpc_cost_per_unit, cpc_total_cost, cpc_status,
				cpc_calculated_by, cpc_calculated_at, cpc_rm_cost_detail)
			VALUES ($1, $2, $3, $4, $5, $6, $6, $7, 'it', NOW() - ($8 || ' minutes')::interval, $9::jsonb)`,
			pid, productIDsITPeriod, calcType, hid, version, total, status, fmt.Sprint(pid%7), rm)
	}
	rmGroup := func(code string) string {
		return fmt.Sprintf(`[{"rm_type":"GROUP","ref_code":%q,"contribution":1}]`, code)
	}
	rmItem := `[{"rm_type":"ITEM","ref_code":"PIDS-TIO2","contribution":2}]`
	rmProduct := fmt.Sprintf(`[{"rm_type":"PRODUCT","ref_code":"product:%d","contribution":3}]`, interID)

	p1, h1 := product("PIDS-1", typeA, "BLK", "Midnight Black", "PIDS-ITEM-1")
	p2, h2 := product("PIDS-2", typeB, "RED", "Red", "")
	p3, h3 := product("PIDS-3", typeA, "WHT", "White", "POY-PIDS-3-for selling")
	p4, h4 := product("PIDS-4", typeB, "BLK", "Midnight Black", "")
	p5, h5 := product("PIDS-5", typeA, "", "", "")
	p6, h6 := product("PIDS-6", typeB, "RED", "Red", "PIDS-ITEM-1")

	result(p1, h1, "ACTUAL", "CALCULATED", 1, 10, rmGroup("PIDS-BRT"))
	result(p1, h1, "FORECAST", "APPROVED", 1, 11, rmGroup("PIDS-SD"))
	result(p2, h2, "ACTUAL", "SUPERSEDED", 1, 20, rmGroup("PIDS-BRT"))
	result(p2, h2, "ACTUAL", "VERIFIED", 2, 21, rmItem)
	result(p3, h3, "ACTUAL", "APPROVED", 1, 30, rmProduct)
	result(p4, h4, "FORECAST", "CALCULATED", 1, 40, rmGroup("PIDS-BRT"))
	result(p4, h4, "ACTUAL", "CALCULATED", 1, 41, `[]`)
	result(p5, h5, "ACTUAL", "APPROVED", 1, 50, rmGroup("PIDS-SD"))
	result(p6, h6, "FORECAST", "VERIFIED", 1, 60, rmItem)
	exec(`UPDATE cst_product_cost SET cpc_rm_cost_detail = NULL WHERE cpc_product_sys_id = $1`, p5)
}
