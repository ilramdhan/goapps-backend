// Integration parity test: the route graph's computed RM ratio for a
// DENIER-mode (ACY-like) product equals the calc engine's ratio for the same
// master data. Gated by INTEGRATION_TEST=true.
//
// SAFETY: fixtures use a unique ITRRL prefix and are hard-deleted on cleanup.
package postgres_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costrouteapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestRouteRMRatioLoader_MatchesCalcEngine(t *testing.T) { //nolint:funlen // end-to-end fixture
	raw, db := openRRDDB(t)
	ctx := context.Background()
	const actor = "itest-rrl"
	n := rand.IntN(1_000_000)
	prefix := fmt.Sprintf("ITRRL%06d", n)
	typeCode := fmt.Sprintf("L%04d", n%10000)
	grp := prefix + "-SPD"

	var typeID int
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_rm_ratio_mode)
		VALUES ($1, 'itest rrl', 'DENIER') RETURNING cpt_type_id`, typeCode).Scan(&typeID))
	insertProduct := func(suffix string) int64 {
		var id int64
		require.NoError(t, raw.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, 'itest rrl', $3, $3) RETURNING cpm_product_sys_id`, prefix+suffix, typeID, actor).Scan(&id))
		return id
	}
	fg, pty := insertProduct("-ACY"), insertProduct("-PTY")
	_, err := raw.ExecContext(ctx, `
		INSERT INTO cst_rm_group_head (group_code, group_name, denier, created_by) VALUES ($1, $1, 40, $2)`, grp, actor)
	require.NoError(t, err)

	setParam := func(product int64, code string, v float64) {
		_, err := raw.ExecContext(ctx, `
			INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
			SELECT $1, id, FALSE, $3 FROM mst_parameter WHERE param_code = $2 AND deleted_at IS NULL
			ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`, product, code, actor)
		require.NoError(t, err)
		_, err = raw.ExecContext(ctx, `
			INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
			SELECT $1, id, $3, $4, $4 FROM mst_parameter WHERE param_code = $2 AND deleted_at IS NULL`, product, code, v, actor)
		require.NoError(t, err)
	}
	setParam(pty, costroute.ParamCodeDenier, 150)
	setParam(fg, costroute.ParamCodeSpandexDraft, 3)

	var headID, seqID int64
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'COMPLETE', 1, $2, $2) RETURNING crh_head_id`, fg, actor).Scan(&headID))
	require.NoError(t, raw.QueryRowContext(ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, $3, $3) RETURNING crs_seq_id`, headID, fg, actor).Scan(&seqID))
	_, err = raw.ExecContext(ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type, crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 1, $4, $4)`, seqID, fg, pty, actor)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type, crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'GROUP', 1, $4, $4)`, seqID, fg, grp, actor)
	require.NoError(t, err)

	t.Cleanup(func() {
		ids := []int64{fg, pty}
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_route_rm WHERE crm_seq_id = $1`, seqID)
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_route_seq WHERE crs_head_id = $1`, headID)
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, headID)
		for _, id := range ids {
			_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = $1`, id)
			_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id = $1`, id)
			_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, id)
		}
		_, _ = raw.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id = $1`, typeID)
		_, _ = raw.ExecContext(ctx, `DELETE FROM cst_rm_group_head WHERE group_code = $1`, grp)
	})

	// Route graph side.
	h := costrouteapp.NewGetGraphHandler(postgres.NewCostRouteRepository(db)).
		WithRatioLoader(postgres.NewRouteRMRatioLoader(db))
	g, err := h.Handle(ctx, headID, false)
	require.NoError(t, err)
	require.Len(t, g.Seqs, 1)
	assert.Equal(t, costroute.RMRatioModeDenier, g.Head.RMRatioMode)
	routeRatio := map[string]float64{}
	for _, rm := range g.Seqs[0].Rms {
		assert.Equal(t, costroute.RatioSourceDenier, rm.RatioSource)
		routeRatio[rm.RmType] = rm.ComputedRatio
	}
	spd := 40.0 / 3
	assert.InDelta(t, 150/(150+spd), routeRatio[costroute.RmTypeProduct], 1e-9)
	assert.InDelta(t, spd/(150+spd), routeRatio[costroute.RmTypeGroup], 1e-9)

	// Calc engine side, fed by the engine's own bulk loader.
	loader := costcalc.NewProductLoader(raw)
	routes, err := loader.LoadRoutesByProducts(ctx, []int64{fg})
	require.NoError(t, err)
	capp, err := loader.LoadCAPP(ctx, []int64{fg})
	require.NoError(t, err)
	modes, err := loader.LoadProductRMRatioModes(ctx, []int64{fg})
	require.NoError(t, err)
	groupDeniers, err := loader.LoadRMGroupDeniers(ctx, []string{grp})
	require.NoError(t, err)
	upDeniers, err := loader.LoadUpstreamDeniers(ctx, []int64{pty})
	require.NoError(t, err)

	out, err := costcalc.ComputeProduct(ctx, costcalc.ComputeInput{
		ProductSysID: fg,
		Period:       "202609",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        routes[fg],
		CAPP:         capp[fg],
		Formulas: []costcalc.Formula{{
			FormulaCode: "F_FINAL", FormulaName: "Final", Expression: "COST_RM_TOTAL",
			ResultParamCode: costcalc.ScopeKeyFinalCost, InputParamCodes: []string{costcalc.ScopeKeyCostRMTotal},
		}},
		RMCosts:        map[string]costcalc.RMCostRates{grp + "|": {CostVal: 100}},
		UpstreamCosts:  map[int64]float64{pty: 40},
		EvalCache:      evaluator.NewCache(),
		RMRatioMode:    modes[fg],
		RMGroupDenier:  groupDeniers,
		UpstreamDenier: upDeniers,
	})
	require.NoError(t, err)
	require.Len(t, out.RMCostDetail, 2)
	for _, d := range out.RMCostDetail {
		assert.InDelta(t, routeRatio[d.RMType], d.Ratio, 1e-12, "route graph ratio == calc engine ratio for %s", d.RMType)
	}
}
