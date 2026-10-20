// Integration test for rmratioquery.ProductRatioModes resolving the per-product
// RM ratio mode (000579) before the product type mode (000570). LOCAL
// PostgreSQL only, via internal/testutil/pgcontainer. Skipped unless
// INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestProductRatioModesPerProductOverride(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "rm_ratio_mode_per_product_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	typeID := func(code, mode string) int32 {
		var id int32
		require.NoError(t, raw.QueryRowContext(ctx, `
			INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_rm_ratio_mode) VALUES ($1, $1, $2)
			ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_rm_ratio_mode = EXCLUDED.cpt_rm_ratio_mode
			RETURNING cpt_type_id`, code, mode).Scan(&id))
		return id
	}
	inputType, denierType := typeID("ZZRI", "INPUT"), typeID("ZZRD", "DENIER")
	product := func(code string, typ int32, mode any) int64 {
		var id int64
		require.NoError(t, raw.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
				cpm_is_active, cpm_created_by, cpm_updated_by, cpm_rm_ratio_mode)
			VALUES ($1, $2, $4, TRUE, 'itest', 'itest', $3)
			RETURNING cpm_product_sys_id`, code, typ, mode, code).Scan(&id))
		return id
	}
	inNull := product("ZZRM-IN-NULL", inputType, nil)
	inDenier := product("ZZRM-IN-DEN", inputType, "DENIER")
	dnNull := product("ZZRM-DN-NULL", denierType, nil)
	dnInput := product("ZZRM-DN-IN", denierType, "INPUT")

	got, err := rmratioquery.ProductRatioModes(ctx, raw, []int64{inNull, inDenier, dnNull, dnInput, 999999999})
	require.NoError(t, err)
	assert.Equal(t, map[int64]string{
		inNull:   "INPUT",  // NULL → type INPUT (unchanged behavior)
		inDenier: "DENIER", // override beats type INPUT
		dnNull:   "DENIER", // NULL → type DENIER (unchanged behavior)
		dnInput:  "INPUT",  // override beats type DENIER
	}, got, "unknown product is absent")
}
