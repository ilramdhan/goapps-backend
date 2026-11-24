package postgres_test

import (
	"database/sql"
	"errors"
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
	t.Run("FullCycle", func(t *testing.T) { mbcmpFullCycle(t, f, pg) })
	t.Run("SpinPointerClearedAndRestored", func(t *testing.T) { mbcmpSpinPointer(t, f, pg) })
	t.Run("StalePlanAndInactive", func(t *testing.T) { mbcmpStaleAndInactive(t, f, pg) })
	t.Run("RevertChangedAndLocked", func(t *testing.T) { mbcmpRevertChangedAndLocked(t, f, pg) })
}

func mbcmpPlanApply(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonFill, item string, cells map[string]string, commit bool) (*fill.Plan, []fill.RowResult) {
	t.Helper()
	prods, err := pg.Snapshot(f.ctx)
	require.NoError(t, err)
	plan := fill.BuildPlan([]fill.ExcelRow{{Row: 3, ItemCode: item, Shade: "Y145T", Cells: cells}}, prods, nil)
	res, err := fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, commit)
	require.NoError(t, err)
	return plan, res
}

func (f *mbasFixture) cpp(pid int64, code string) (val string, ptr string, ok bool) {
	var p sql.NullString
	err := f.raw.QueryRowContext(f.ctx, `SELECT COALESCE(CAST(c.cpp_value_numeric AS text), c.cpp_value_text, ''), CAST(c.cpp_value_mb_spin_id AS text)
		FROM cost_product_parameter c JOIN mst_parameter p ON p.id=c.cpp_param_id WHERE p.param_code=$2 AND c.cpp_product_sys_id=$1`, pid, code).Scan(&val, &p)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	require.NoError(f.t, err)
	return val, p.String, true
}

func mbcmpSpinPointer(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonFill) {
	pid := f.poy("POYCMP2", "Y145T")
	spin := f.spin(f.head(true, "Y145T"), "Y145T", 1, true, nil)
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		SELECT $1, id, 'OLDCODE', $2, 'seed', 'seed' FROM mst_parameter WHERE param_code = 'MB_SP_CODE'`, pid, spin)
	cells := map[string]string{"MB_SP_CODE": "NEWCODE"}
	plan, res := mbcmpPlanApply(t, f, pg, "POYCMP2", cells, true)
	assert.Equal(t, 1, plan.SpinPtrCleared)
	assert.Equal(t, spin.String(), plan.Rows[0].SpinPtr)
	require.Equal(t, fill.ResultApplied, res[0].Result, res[0].Detail)
	v, ptr, _ := f.cpp(pid, "MB_SP_CODE")
	assert.Equal(t, "NEWCODE", v)
	assert.Empty(t, ptr, "pointer cleared so a lookup sync resolves the new code")

	rev, err := fill.Revert(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultReverted, rev[0].Result, rev[0].Detail)
	v, ptr, _ = f.cpp(pid, "MB_SP_CODE")
	assert.Equal(t, "OLDCODE", v)
	assert.Equal(t, spin.String(), ptr, "pointer restored")
}

func mbcmpStaleAndInactive(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonFill) {
	pid := f.poy("POYCMP3", "Y145T")
	cells := map[string]string{"MB_SP_DOZING": "2"}
	plan, _ := mbcmpPlanApply(t, f, pg, "POYCMP3", cells, false)
	// value appears between plan and apply (row existed=false in plan) -> changed_since_plan
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT $1, id, 5, 'other', 'other' FROM mst_parameter WHERE param_code = 'MB_SP_DOZING'`, pid)
	res, err := fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultFailed, res[0].Result)
	assert.Contains(t, res[0].Detail, "changed_since_plan")
	v, _, _ := f.cpp(pid, "MB_SP_DOZING")
	assert.Equal(t, "5.000000", v, "stale plan must not overwrite")

	f.exec(`UPDATE cost_product_master SET cpm_is_active = FALSE WHERE cpm_product_sys_id = $1`, pid)
	plan.Rows[0].Before["MB_SP_DOZING"] = "5.000000"
	plan.Rows[0].BeforeExisted["MB_SP_DOZING"] = true
	res, err = fill.Apply(f.ctx, pg, "mbcmp-test", plan.Rows, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, "product_inactive", res[0].Detail)
}

func mbcmpRevertChangedAndLocked(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonFill) {
	pid := f.poy("POYCMP4", "Y145T")
	_, res := mbcmpPlanApply(t, f, pg, "POYCMP4", map[string]string{"MB_SP_DOZING": "2", "RP_DOZING_ACTUAL": "3"}, true)
	require.Equal(t, fill.ResultApplied, res[0].Result, res[0].Detail)

	// locked at revert -> FAILED, nothing restored
	f.exec(`UPDATE cost_product_master SET cpm_is_locked = TRUE WHERE cpm_product_sys_id = $1`, pid)
	rev, err := fill.Revert(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultFailed, rev[0].Result)
	assert.Equal(t, "product_locked", rev[0].Detail)
	_, _, ok := f.cpp(pid, "MB_SP_DOZING")
	assert.True(t, ok)
	f.exec(`UPDATE cost_product_master SET cpm_is_locked = FALSE WHERE cpm_product_sys_id = $1`, pid)

	// RP_DOZING_ACTUAL edited by someone else -> CHANGED_SINCE_APPLY, kept (and its CAPP kept); the other param is reverted
	f.exec(`UPDATE cost_product_parameter SET cpp_value_numeric = 9 WHERE cpp_product_sys_id = $1 AND cpp_param_id = (SELECT id FROM mst_parameter WHERE param_code='RP_DOZING_ACTUAL')`, pid)
	rev, err = fill.Revert(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultChanged, rev[0].Result, rev[0].Detail)
	v, _, _ := f.cpp(pid, "RP_DOZING_ACTUAL")
	assert.Equal(t, "9.000000", v)
	_, _, ok = f.cpp(pid, "MB_SP_DOZING")
	assert.False(t, ok, "unchanged param (no row before) deleted")
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM cost_product_applicable_param WHERE capp_product_sys_id = $1`, pid), "CAPP of the changed param kept")
}

func mbcmpFullCycle(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonFill) {
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
