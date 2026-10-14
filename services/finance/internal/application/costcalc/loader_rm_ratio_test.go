package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadRMRatioInputs_Integration exercises LoadProductRMRatioModes and
// LoadRMGroupDeniers against a real database migrated to >= 000570.
func TestLoadRMRatioInputs_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitDB(db, 10*time.Second))
	t.Cleanup(func() { _ = db.Close() })

	const actor = "loader-rm-ratio-test"
	prefix := strings.ToUpper(uniqueCodePrefix(t, "R"))
	typeDenier, typeInput := prefix[:5], "Q"+prefix[1:5]

	insertType := func(code, mode string) int {
		var id int
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_rm_ratio_mode)
			VALUES ($1, 'loader rm ratio test', $2) RETURNING cpt_type_id`, code, mode).Scan(&id))
		return id
	}
	denierTypeID := insertType(typeDenier, "DENIER")
	inputTypeID := insertType(typeInput, "INPUT")

	insertProduct := func(suffix string, typeID int) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, 'loader rm ratio test', $3, $3) RETURNING cpm_product_sys_id`,
			prefix+suffix, typeID, actor).Scan(&id))
		return id
	}
	pDenier := insertProduct("-D", denierTypeID)
	pInput := insertProduct("-I", inputTypeID)

	grpDenier, grpNull, grpDeleted := prefix+"-SPD", prefix+"-OIL", prefix+"-DEL"
	_, err = db.ExecContext(ctx, `
		INSERT INTO cst_rm_group_head (group_code, group_name, denier, created_by) VALUES
			($1, $1, 40, $4), ($2, $2, NULL, $4), ($3, $3, 70, $4)`, grpDenier, grpNull, grpDeleted, actor)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE cst_rm_group_head SET deleted_at = NOW() WHERE group_code = $1`, grpDeleted)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = ANY($1)`, int64ToArray([]int64{pDenier, pInput}))
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id IN ($1, $2)`, denierTypeID, inputTypeID)
		_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_group_head WHERE created_by = $1`, actor)
	})

	loader := NewProductLoader(db)

	modes, err := loader.LoadProductRMRatioModes(ctx, []int64{pDenier, pInput, -1})
	require.NoError(t, err)
	assert.Equal(t, map[int64]string{pDenier: RMRatioModeDenier, pInput: RMRatioModeInput}, modes)

	deniers, err := loader.LoadRMGroupDeniers(ctx, []string{grpDenier, grpNull, grpDeleted, "NOPE"})
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{grpDenier: 40}, deniers,
		"NULL denier, soft-deleted and unknown groups are absent")

	// Upstream DENIER is read from the product master (CAPP + cost_product_parameter).
	var denierParamID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT id FROM mst_parameter WHERE param_code = 'DENIER' AND deleted_at IS NULL`).Scan(&denierParamID))
	_, err = db.ExecContext(ctx, `
		INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
		VALUES ($1, $3, FALSE, $4), ($2, $3, FALSE, $4)`, pDenier, pInput, denierParamID, actor)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, 75, $3, $3)`, pDenier, denierParamID, actor)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = ANY($1)`, int64ToArray([]int64{pDenier, pInput}))
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id = ANY($1)`, int64ToArray([]int64{pDenier, pInput}))
	})
	upDeniers, err := loader.LoadUpstreamDeniers(ctx, []int64{pDenier, pInput, -1})
	require.NoError(t, err)
	assert.Equal(t, map[int64]float64{pDenier: 75}, upDeniers, "applicable but unfilled and unknown products are absent")
	emptyUp, err := loader.LoadUpstreamDeniers(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, emptyUp)

	emptyModes, err := loader.LoadProductRMRatioModes(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, emptyModes)
	emptyDeniers, err := loader.LoadRMGroupDeniers(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, emptyDeniers)
}
