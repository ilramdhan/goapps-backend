// Integration test for ListActiveSnapshotPeriods (MB recompute after re-validate):
// SUPERSEDED-only periods are excluded, any calc type counts, products are deduped per
// period. LOCAL PostgreSQL only, via internal/testutil/pgcontainer.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestCostResultListActiveSnapshotPeriodsIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "cost_result_active_periods_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	seedProductIDsFixture(ctx, t, raw) // period 202611; PIDS-2 has SUPERSEDED v1 + VERIFIED v2

	id := func(code string) (pid, hid int64) {
		require.NoError(t, raw.QueryRowContext(ctx, `
			SELECT p.cpm_product_sys_id, h.crh_head_id FROM cost_product_master p
			JOIN cost_route_head h ON h.crh_product_sys_id = p.cpm_product_sys_id
			WHERE p.cpm_product_code = $1`, code).Scan(&pid, &hid))
		return pid, hid
	}
	p1, h1 := id("PIDS-1")
	p2, h2 := id("PIDS-2")
	p6, _ := id("PIDS-6")
	add := func(pid, hid int64, period, status string) {
		_, err := raw.ExecContext(ctx, `INSERT INTO cst_product_cost (cpc_product_sys_id, cpc_period,
			cpc_calculation_type, cpc_route_head_id, cpc_version, cpc_cost_per_unit, cpc_total_cost,
			cpc_status, cpc_calculated_by, cpc_calculated_at)
			VALUES ($1, $2, 'ACTUAL', $3, 1, 1, 1, $4, 'it', NOW())`, pid, period, hid, status)
		require.NoError(t, err)
	}
	add(p1, h1, "202610", "APPROVED")
	add(p2, h2, "202610", "SUPERSEDED") // SUPERSEDED-only period: excluded for p2

	repo := postgres.NewCostResultRepository(postgres.NewDBFromSQL(raw))
	got, err := repo.ListActiveSnapshotPeriods(ctx, []int64{p1, p2, p6})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{p1, p2, p6}, got["202611"], "202611: p1 (2 calc types, deduped), p2 (VERIFIED), p6 (FORECAST only)")
	assert.ElementsMatch(t, []int64{p1}, got["202610"])
	assert.Len(t, got, 2)

	empty, err := repo.ListActiveSnapshotPeriods(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
