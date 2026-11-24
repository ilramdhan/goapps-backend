package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// TestMBComparisonFill_Integration: snapshot -> plan -> preflight -> dry-run -> commit (CPP+CAPP) -> revert.
func TestMBComparisonFill_Integration(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_fill")
	pg := postgres.NewMBComparisonFill(postgres.NewDBFromSQL(f.raw))
	pid := f.poy("POYCMP1", "Y-145 T")
	// existing CPP for dozing (no CAPP), so the plan must change it and count "CPP without CAPP"
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT $1, id, 1.39, 'seed', 'seed' FROM mst_parameter WHERE param_code = 'MB_SP_DOZING'`, pid)

	// preflight: migration 000616 ran in the template, formulas carry its marker
	require.NoError(t, pg.Preflight(f.ctx))

	prods, err := pg.Snapshot(f.ctx)
	require.NoError(t, err)
	cells := map[string]string{"MB_SP_CODE": "CMB1", "MB_SP_DOZING": "1.45", "CONV_FACTOR_ACTUAL": "1.6743", "RP_DOZING_ACTUAL": "1.6743"}
	plan := fill.BuildPlan([]fill.ExcelRow{{Row: 3, ItemCode: "POYCMP1", Shade: "Y145T", Cells: cells}}, prods, nil)
	require.Equal(t, fill.DecisionApply, plan.Rows[0].Decision, plan.Rows[0].Reason)
	assert.Equal(t, 1, plan.CPPWithoutCAPP)
	assert.Equal(t, "1.390000", plan.Rows[0].Before["MB_SP_DOZING"])

	// dry-run writes nothing
	res, err := fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, false)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultWouldApply, res[0].Result)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM cost_product_parameter c JOIN mst_parameter p ON p.id=c.cpp_param_id WHERE p.param_code='RP_DOZING_ACTUAL'`))

	// commit
	res, err = fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultApplied, res[0].Result, res[0].Detail)
	assert.Len(t, res[0].CAPPInserted, 4)
	var v float64
	var by string
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT c.cpp_value_numeric, c.cpp_filled_by FROM cost_product_parameter c JOIN mst_parameter p ON p.id=c.cpp_param_id
		WHERE p.param_code='RP_DOZING_ACTUAL' AND c.cpp_product_sys_id=$1`, pid).Scan(&v, &by))
	assert.InDelta(t, 1.6743, v, 1e-9)
	assert.Equal(t, "mbcmp-test", by)

	// re-plan -> everything same
	prods, err = pg.Snapshot(f.ctx)
	require.NoError(t, err)
	plan2 := fill.BuildPlan([]fill.ExcelRow{{Row: 3, ItemCode: "POYCMP1", Shade: "Y145T", Cells: cells}}, prods, nil)
	assert.Equal(t, fill.DecisionSkipSame, plan2.Rows[0].Decision)

	// locked product -> FAILED at apply even from a stale plan
	f.exec(`UPDATE cost_product_master SET cpm_is_locked = TRUE WHERE cpm_product_sys_id = $1`, pid)
	stale, err := fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultFailed, stale[0].Result)
	f.exec(`UPDATE cost_product_master SET cpm_is_locked = FALSE WHERE cpm_product_sys_id = $1`, pid)

	// revert restores dozing 1.39, deletes the new CPPs and the inserted CAPPs
	rev, err := fill.Revert(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultReverted, rev[0].Result, rev[0].Detail)
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT c.cpp_value_numeric FROM cost_product_parameter c JOIN mst_parameter p ON p.id=c.cpp_param_id
		WHERE p.param_code='MB_SP_DOZING' AND c.cpp_product_sys_id=$1`, pid).Scan(&v))
	assert.InDelta(t, 1.39, v, 1e-9)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM cost_product_applicable_param WHERE capp_created_by = 'mbcmp-test'`))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_product_sys_id = $1`, pid))

	rev, err = fill.Revert(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultAlready, rev[0].Result)
}
