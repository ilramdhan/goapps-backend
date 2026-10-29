// Migration round-trip test for 000599 (F_YARN_RM_LANDED ACTUAL cascade gains
// the PP fallback: CL,SL,FL -> CL,SL,FL,PP). Local PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preRMLandedPP599 uint64 = 598
	rmLandedPP599Ver uint64 = 599

	rmLandedExpr519 = "ACTUAL:CL,SL,FL;FORECAST:SP,PP,FP;SELLING:SP,PP,FP"
	rmLandedExpr599 = "ACTUAL:CL,SL,FL,PP;FORECAST:SP,PP,FP;SELLING:SP,PP,FP"
)

func TestRMLandedActualPOFallback599Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "rm_landed_actual_pp_599_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), rmLandedPP599Ver)
	require.NoError(t, m.UpTo(ctx, preRMLandedPP599))

	expression := func() string {
		var expr string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT expression FROM mst_formula WHERE formula_code = 'F_YARN_RM_LANDED'`).Scan(&expr))
		return expr
	}
	assert.Equal(t, rmLandedExpr519, expression())

	require.NoError(t, m.UpTo(ctx, rmLandedPP599Ver))
	assert.Equal(t, rmLandedExpr599, expression())

	// Idempotent re-run of the up SQL itself.
	sqlUp, err := os.ReadFile("000599_rm_landed_actual_po_fallback.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assert.Equal(t, rmLandedExpr599, expression())

	require.NoError(t, m.DownTo(ctx, preRMLandedPP599))
	assert.Equal(t, rmLandedExpr519, expression())

	require.NoError(t, m.UpTo(ctx, rmLandedPP599Ver))
	assert.Equal(t, rmLandedExpr599, expression())
}
