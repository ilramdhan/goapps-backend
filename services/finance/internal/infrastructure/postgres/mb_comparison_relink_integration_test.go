package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func (f *mbasFixture) seedCPP(pid int64, code, text string, ptr *uuid.UUID) {
	f.t.Helper()
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		SELECT $1, id, $3, $4, 'seed', 'seed' FROM mst_parameter WHERE param_code = $2`, pid, code, text, ptr)
}

func (f *mbasFixture) cppDump() string {
	f.t.Helper()
	var s string
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `SELECT COALESCE(string_agg(
		c.cpp_product_sys_id || '|' || p.param_code || '|' || COALESCE(c.cpp_value_text, '') || '|' || COALESCE(CAST(c.cpp_value_numeric AS text), '') || '|' ||
		COALESCE(CAST(c.cpp_value_mb_spin_id AS text), '') || '|' || COALESCE(c.cpp_updated_by, ''), ';' ORDER BY c.cpp_product_sys_id, p.param_code), '')
		FROM cost_product_parameter c JOIN mst_parameter p ON p.id = c.cpp_param_id`).Scan(&s))
	return s
}

func relinkPlan(t *testing.T, f *mbasFixture, pg *postgres.MBComparisonRelink, rows []fill.RelinkRow) []fill.RelinkRow {
	t.Helper()
	prods, err := pg.Products(f.ctx, fill.RelinkProductIDs(rows))
	require.NoError(t, err)
	spins, err := pg.Spins(f.ctx, fill.RelinkSpinIDs(rows, prods))
	require.NoError(t, err)
	return fill.PlanRelink(rows, prods, spins)
}

func TestMBComparisonRelink_Pointer(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_relink")
	pg := postgres.NewMBComparisonRelink(postgres.NewDBFromSQL(f.raw))
	const actor = "mbcmp-staging"
	activeHead, inactiveHead := f.head(true, "R1"), f.head(false, "R1")
	old := f.spin(activeHead, "R1", 1, true, nil)
	inactiveTarget := f.spin(inactiveHead, "R1", 1, false, nil)
	deleted := f.spin(activeHead, "R1", 1, true, nil)
	f.exec(`UPDATE mst_mb_spin SET deleted_at = NOW() WHERE mbs_id = $1`, deleted)
	p1, p2, bystander := f.poy("RLK1", "R1"), f.poy("RLK2", "R1"), f.poy("RLK3", "R1")
	f.seedCPP(p1, "MB_SP_CODE", "CODE1", &old)
	f.seedCPP(p2, "MB_SP_CODE", "CODE2", &old)
	f.seedCPP(bystander, "MB_SP_CODE", "CODE3", &old)
	f.seedCPP(bystander, "MB_SP_DYE", "DYE", nil)

	mk := func(pid int64, target uuid.UUID) fill.RelinkRow {
		return fill.RelinkRow{Row: int(pid), Env: "staging", Product: pid, Kind: fill.RelinkPointer, TargetSpin: target.String()}
	}
	plan := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p1, inactiveTarget), mk(p2, deleted)})
	assert.Equal(t, fill.RelinkPointer, plan[0].Decision, "inactive target is allowed")
	assert.Equal(t, "target_spin_missing_or_deleted", plan[1].Reason)
	plan = plan[:1]
	before := f.cppDump()

	// dry-run writes nothing
	res, err := fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, false)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultWouldApply, res[0].Status, res[0].Reason)
	assert.Equal(t, before, f.cppDump())

	// stale: pointer moved after plan
	other := f.spin(activeHead, "R1", 1, true, nil)
	f.exec(`UPDATE cost_product_parameter SET cpp_value_mb_spin_id = $2 WHERE cpp_product_sys_id = $1`, p1, other)
	res, err = fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultFailed, res[0].Status)
	assert.Equal(t, "changed_since_plan", res[0].Reason)
	f.exec(`UPDATE cost_product_parameter SET cpp_value_mb_spin_id = $2 WHERE cpp_product_sys_id = $1`, p1, old)

	// commit: only p1's pointer changes; text and every other row untouched
	before = f.cppDump()
	res, err = fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultApplied, res[0].Status, res[0].Reason)
	v, ptr, _ := f.cpp(p1, "MB_SP_CODE")
	assert.Equal(t, "CODE1", v)
	assert.Equal(t, inactiveTarget.String(), ptr)
	_, ptr2, _ := f.cpp(p2, "MB_SP_CODE")
	assert.Equal(t, old.String(), ptr2)
	assert.NotEqual(t, before, f.cppDump())
	var changed int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_updated_by = $1`, actor).Scan(&changed))
	assert.Equal(t, 1, changed, "no other cpp row is written")

	// idempotent re-apply (same plan): live pointer already = target
	again, err := fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultAlreadyApplied, again[0].Status, again[0].Reason)

	// revert restores the pointer; a second revert is ALREADY_REVERTED
	rev, err := fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultReverted, rev[0].Status, rev[0].Reason)
	_, ptr, _ = f.cpp(p1, "MB_SP_CODE")
	assert.Equal(t, old.String(), ptr)
	rev, err = fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultAlready, rev[0].Status)

	// revert skips a pointer someone changed after apply
	_, err = fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	f.exec(`UPDATE cost_product_parameter SET cpp_value_mb_spin_id = $2 WHERE cpp_product_sys_id = $1`, p1, other)
	rev, err = fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultChanged, rev[0].Status)
}

func TestMBComparisonRelink_Child(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_relink_child")
	pg := postgres.NewMBComparisonRelink(postgres.NewDBFromSQL(f.raw))
	const actor = "mbcmp-staging"
	inactiveHead := f.head(false, "R2") // RequireLiveHead=false must still allow the child
	parent := f.spin(inactiveHead, "R2", 1.45, false, nil)
	p1, p2, p3 := f.poy("RLC1", "R2"), f.poy("RLC2", "R2"), f.poy("RLC3", "R2")
	for _, p := range []int64{p1, p2, p3} {
		f.seedCPP(p, "MB_SP_CODE", "PARENTCODE", &parent)
	}
	mk := func(pid int64, ldr float64, key string) fill.RelinkRow {
		return fill.RelinkRow{Row: int(pid), Env: "staging", Source: "CONFLICT", Product: pid, Kind: fill.RelinkChild,
			ParentSpin: parent.String(), ChildKey: key, Den: 250, Fil: 144, LDR: ldr}
	}
	plan := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p1, 1.45, "P#2"), mk(p2, 1.45, "P#2"), mk(p3, 2.1, "P#3")})
	for _, r := range plan {
		require.Equal(t, fill.RelinkChild, r.Decision, r.Reason)
	}

	// dry-run: no child, nothing written
	var spinsBefore int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM mst_mb_spin`).Scan(&spinsBefore))
	before := f.cppDump()
	res, err := fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, false)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultWouldApply, res[0].Status, res[0].Reason)
	var n int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM mst_mb_spin`).Scan(&n))
	assert.Equal(t, spinsBefore, n)
	assert.Equal(t, before, f.cppDump())

	// commit: two children (one per key), p1+p2 share one
	res, err = fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	for _, r := range res {
		require.Equal(t, fill.ResultApplied, r.Status, r.Reason)
	}
	assert.True(t, res[0].ChildCreated)
	assert.False(t, res[1].ChildCreated)
	assert.Equal(t, res[0].ChildID, res[1].ChildID)
	assert.NotEqual(t, res[0].ChildID, res[2].ChildID)
	child := uuid.MustParse(res[0].ChildID)
	s := f.readSpin(child)
	assert.InDelta(t, 1.45, s.ldr, 1e-9)
	assert.True(t, s.actual)
	assert.False(t, s.fg.Valid, "no FG")
	_, ptr, _ := f.cpp(p1, "MB_SP_CODE")
	assert.Equal(t, res[0].ChildID, ptr)
	v, _, _ := f.cpp(p1, "MB_SP_CODE")
	assert.Equal(t, "PARENTCODE", v)

	// re-plan after commit: products already on the matching child -> SKIP_SAME
	replan := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p1, 1.45, "P#2")})
	assert.Equal(t, fill.DecisionSkipSame, replan[0].Decision)

	// reuse: a new product, same spec -> existing live child reused, nothing created
	p4 := f.poy("RLC4", "R2")
	f.seedCPP(p4, "MB_SP_CODE", "PARENTCODE", &parent)
	reuse := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p4, 1.45, "P#2")})
	rres, err := fill.ApplyRelink(f.ctx, pg, actor, reuse, 0, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultApplied, rres[0].Status, rres[0].Reason)
	assert.False(t, rres[0].ChildCreated)
	assert.Equal(t, res[0].ChildID, rres[0].ChildID)

	// revert (oldest first input): pointers restored; child P#2 still referenced by p4's NOT-reverted run -> kept
	rev, err := fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	byChild := map[string]string{}
	for _, r := range rev {
		if r.Decision == fill.DecisionChildDelete {
			byChild[r.ChildID] = r.Status
		}
	}
	assert.Equal(t, fill.StatusKept, byChild[res[0].ChildID], "p4 still points at it")
	assert.Equal(t, fill.ResultReverted, byChild[res[2].ChildID], "unreferenced child soft-deleted")
	_, ptr, _ = f.cpp(p1, "MB_SP_CODE")
	assert.Equal(t, parent.String(), ptr)
	_, ptr, _ = f.cpp(p3, "MB_SP_CODE")
	assert.Equal(t, parent.String(), ptr)
	var deleted bool
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT deleted_at IS NOT NULL FROM mst_mb_spin WHERE mbs_id = $1`, res[2].ChildID).Scan(&deleted))
	assert.True(t, deleted)

	// revert the reusing run (p4): the shared child is now unreferenced and every run that recorded it may delete it
	rev, err = fill.RevertRelink(f.ctx, pg, actor, rres)
	require.NoError(t, err)
	var gone bool
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT deleted_at IS NOT NULL FROM mst_mb_spin WHERE mbs_id = $1`, res[0].ChildID).Scan(&gone))
	assert.True(t, gone, "child deleted once the last reference is reverted")
	// a later revert of the first run sees it already deleted
	rev, err = fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	for _, r := range rev {
		if r.Decision == fill.DecisionChildDelete && r.ChildID == res[0].ChildID {
			assert.Equal(t, fill.ResultAlready, r.Status)
		}
	}
}

func TestMBComparisonRelink_FailedFirstRowChildDeletedAndDryRunReuse(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_relink_child2")
	pg := postgres.NewMBComparisonRelink(postgres.NewDBFromSQL(f.raw))
	const actor = "mbcmp-staging"
	parent := f.spin(f.head(false, "R3"), "R3", 1.45, false, nil)
	p1, p2 := f.poy("RLD1", "R3"), f.poy("RLD2", "R3")
	f.seedCPP(p1, "MB_SP_CODE", "PC", &parent)
	f.seedCPP(p2, "MB_SP_CODE", "PC", &parent)
	mk := func(pid int64) fill.RelinkRow {
		return fill.RelinkRow{Row: int(pid), Env: "staging", Source: "CONFLICT", Product: pid, Kind: fill.RelinkChild,
			ParentSpin: parent.String(), ChildKey: "K#2", Den: 250, Fil: 144, LDR: 1.45}
	}
	plan := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p1)})
	// a user moves the pointer after the plan: the row FAILS, but the child was already committed
	other := f.spin(f.head(true, "R3"), "R3", 1, true, nil)
	f.exec(`UPDATE cost_product_parameter SET cpp_value_mb_spin_id = $2 WHERE cpp_product_sys_id = $1`, p1, other)
	res, err := fill.ApplyRelink(f.ctx, pg, actor, plan, 0, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultFailed, res[0].Status)
	require.NotEmpty(t, res[0].ChildID)
	child := res[0].ChildID

	// dry-run with the (still unreferenced, live) child present: WOULD_APPLY, no new spin
	f.exec(`UPDATE cost_product_parameter SET cpp_value_mb_spin_id = $2 WHERE cpp_product_sys_id = $1`, p1, parent)
	plan2 := relinkPlan(t, f, pg, []fill.RelinkRow{mk(p1)})
	var before int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM mst_mb_spin`).Scan(&before))
	dry, err := fill.ApplyRelink(f.ctx, pg, actor, plan2, 0, 10, false)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultWouldApply, dry[0].Status, dry[0].Reason)
	assert.Equal(t, child, dry[0].ChildID)
	assert.False(t, dry[0].ChildCreated)
	var after int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM mst_mb_spin`).Scan(&after))
	assert.Equal(t, before, after)
	var name, pname string
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT c.mbs_mgt_name, p.mbs_mgt_name FROM mst_mb_spin c JOIN mst_mb_spin p ON p.mbs_id = c.mbs_parent_spin_id WHERE c.mbs_id = $1`, child).Scan(&name, &pname))
	assert.Equal(t, pname, name, "child keeps the parent name")

	// reverting the FAILED result alone still removes the orphan child (re-run reuse case: another run reuses it, revert of either run deletes once unreferenced)
	res[0].Env = "staging"
	rev, err := fill.RevertRelink(f.ctx, pg, actor, res)
	require.NoError(t, err)
	require.Len(t, rev, 1)
	assert.Equal(t, fill.ResultReverted, rev[0].Status, rev[0].Reason)
	var deleted bool
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT deleted_at IS NOT NULL FROM mst_mb_spin WHERE mbs_id = $1`, child).Scan(&deleted))
	assert.True(t, deleted)
	_ = p2
}
