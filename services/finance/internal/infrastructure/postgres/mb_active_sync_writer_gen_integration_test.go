package postgres_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// draftHead inserts a head in the given entry status, without cost product, carrying the
// fields the Validate auto-gen copies down (shade, orion code, optional FG / boughtout).
// The head gets its own machine with a fixed cost: the frozen MB_MACHINE_FIXED_TOTAL
// param of the auto-generated cost product needs one (same as the supersede flow test).
func (f *mbasFixture) draftHead(status, shade, orion string, fg *int64, boughtout bool) uuid.UUID {
	f.t.Helper()
	id, machine := uuid.New(), uuid.New()
	f.exec(`INSERT INTO mst_machine (mc_id, mc_code, mc_name, mc_tot_fxd_cst, created_by) VALUES ($1, $2, 'itest machine', 1000, 'itest')`,
		machine, "MC-"+machine.String()[:8])
	f.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_is_active, mbh_check_status, mbh_current_version,
			mbh_entry_status, mbh_is_boughtout, mbh_shade_code, mbh_orion_item_code, mbh_fg_product_id, mbh_machine_id, created_by)
		VALUES ($1, CAST($2 AS text), CAST($2 AS text), TRUE, 'Valid', 1, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, 'itest')`,
		id, "GEN-"+id.String()[:8], status, boughtout, shade, orion, fg, machine)
	return id
}

// withCostProduct gives the head an (already generated) MB cost product: K7 precondition.
func (f *mbasFixture) withCostProduct(head uuid.UUID) int64 {
	f.t.Helper()
	cp := f.poy("MBCP-"+head.String()[:8], "")
	f.exec(`UPDATE mst_mb_head SET mbh_cost_product_id = $2, mbh_entry_status = 'VALIDATED' WHERE mbh_id = $1`, head, cp)
	return cp
}

type genSpinRow struct {
	parent, orion      *string
	costProduct, fg    *int64
	shade              string
	active, superseded bool
}

func (f *mbasFixture) headSpins(head uuid.UUID) map[uuid.UUID]genSpinRow {
	f.t.Helper()
	rows, err := f.raw.QueryContext(f.ctx, `
		SELECT mbs_id, CAST(mbs_parent_spin_id AS text), mbs_orion_item_code, mbs_cost_product_id, mbs_fg_product_id,
		       COALESCE(mbs_shade_code, ''), mbs_is_active, mbs_superseded_by_spin_id IS NOT NULL
		  FROM mst_mb_spin WHERE mbs_mbh_id = $1 AND deleted_at IS NULL`, head)
	require.NoError(f.t, err)
	defer func() { require.NoError(f.t, rows.Close()) }()
	out := map[uuid.UUID]genSpinRow{}
	for rows.Next() {
		var id uuid.UUID
		var r genSpinRow
		require.NoError(f.t, rows.Scan(&id, &r.parent, &r.orion, &r.costProduct, &r.fg, &r.shade, &r.active, &r.superseded))
		out[id] = r
	}
	require.NoError(f.t, rows.Err())
	return out
}

func (f *mbasFixture) headState(head uuid.UUID) (status string, costProduct int64) {
	f.t.Helper()
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx,
		`SELECT mbh_entry_status, COALESCE(mbh_cost_product_id, 0) FROM mst_mb_head WHERE mbh_id = $1`, head).Scan(&status, &costProduct))
	return status, costProduct
}

func TestMBActiveSyncWriter_GenerateRootSpin(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_gen")

	t.Run("head with cost product and no spin: root built like auto-gen, rerun skipped", func(t *testing.T) {
		fg := f.poy("FG-GEN1", "SHG1")
		head := f.draftHead(mbhead.StatusDraft, "SHG1", "OR-GEN1", &fg, false)
		cp := f.withCostProduct(head)

		res, err := f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: head, Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		spins := f.headSpins(head)
		require.Len(t, spins, 1)
		root := uuid.MustParse(res.SpinID)
		got, ok := spins[root]
		require.True(t, ok)
		require.Nil(t, got.parent, "root spin")
		require.Equal(t, cp, *got.costProduct)
		require.Equal(t, fg, *got.fg, "root inherits the head FG")
		require.Equal(t, "SHG1", got.shade)
		require.Equal(t, "OR-GEN1", *got.orion, "free head orion code inherited (mbAutoGenSpinOrionCode)")
		require.NotEmpty(t, res.BeforeJSON)
		require.NotEmpty(t, res.AfterJSON)

		res, err = f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: head, Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
		require.Equal(t, root.String(), res.SpinID)
		require.Len(t, f.headSpins(head), 1)
	})

	t.Run("orion code already on a live spin: root gets none", func(t *testing.T) {
		other := f.head(true, "SHX")
		otherSpin := f.spin(other, "SHX", 6, true, nil)
		f.setOrion(otherSpin, "OR-TAKEN")
		head := f.draftHead(mbhead.StatusDraft, "SHG2", "OR-TAKEN", nil, false)
		f.withCostProduct(head)

		res, err := f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: head, Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.Nil(t, f.headSpins(head)[uuid.MustParse(res.SpinID)].orion)
	})

	t.Run("live spin with same shade+FG elsewhere: fg_conflict, nothing written", func(t *testing.T) {
		fg := f.poy("FG-GEN3", "SHG3")
		_, _ = f.newTargetWithFG("SHG3", fg)
		head := f.draftHead(mbhead.StatusDraft, "shg3", "", &fg, false)
		f.withCostProduct(head)

		res, err := f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: head, Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonFGConflict)
		require.Empty(t, f.headSpins(head))
	})

	t.Run("head without cost product: stale_plan (EnsureValidated path)", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusDraft, "SHG4", "", nil, false)
		res, err := f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: head, Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan)
		require.Empty(t, f.headSpins(head))
	})

	t.Run("missing head: stale_plan", func(t *testing.T) {
		res, err := f.w.GenerateRootSpin(f.ctx, mbactivesync.GenerateRootInput{HeadID: uuid.New(), Actor: "itest"})
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan)
	})
}

// newTargetWithFG creates an active head + live spin with the given shade and FG.
func (f *mbasFixture) newTargetWithFG(shade string, fg int64) (head, spin uuid.UUID) {
	head = f.head(true, shade)
	spin = f.spin(head, shade, 6, true, &fg)
	return head, spin
}

// newITHeadWorkflow wires the adapter exactly like cmd/sync-mb-active newHeadWorkflow.
func newITHeadWorkflow(f *mbasFixture) *mbactivesync.HandlerHeadWorkflow {
	db := postgres.NewDBFromSQL(f.raw)
	compositionRepo := postgres.NewMBCompositionRepository(db)
	headRepo := postgres.NewMBHeadRepository(db, compositionRepo)
	return mbactivesync.NewHandlerHeadWorkflow(headRepo, postgres.NewMBSpinRepository(db),
		appmbhead.NewSubmitHandlerWithComposition(headRepo, compositionRepo),
		appmbhead.NewValidateHandlerWithComposition(headRepo, postgres.NewMBParamRepository(db), compositionRepo))
}

func TestHandlerHeadWorkflow_EnsureValidated(t *testing.T) {
	f := newMBASFixture(t, "mbas_head_workflow")
	wf := newITHeadWorkflow(f)

	t.Run("DRAFT head without cost product: Submit→Validate creates cost product + root", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusDraft, "SHW1", "OR-W1", nil, false)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		status, cp := f.headState(head)
		require.Equal(t, mbhead.StatusValidated, status)
		require.NotZero(t, cp)
		spins := f.headSpins(head)
		require.Len(t, spins, 1)
		root := uuid.MustParse(res.SpinID)
		require.Equal(t, cp, *spins[root].costProduct)
		require.Nil(t, spins[root].parent)
		require.Equal(t, 1, f.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'SUBMITTED'`, head))

		res, err = wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
		require.Equal(t, root.String(), res.SpinID)
		require.Len(t, f.headSpins(head), 1)
	})

	t.Run("boughtout DRAFT validates directly (no submit)", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusDraft, "SHW2", "", nil, true)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		status, cp := f.headState(head)
		require.Equal(t, mbhead.StatusValidated, status)
		require.NotZero(t, cp)
		require.Zero(t, f.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'SUBMITTED'`, head))
	})

	t.Run("REJECTED head: FAILED validate_gate, nothing written", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusRejected, "SHW3", "", nil, false)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonValidateGate)
		status, cp := f.headState(head)
		require.Equal(t, mbhead.StatusRejected, status)
		require.Zero(t, cp)
		require.Empty(t, f.headSpins(head))
	})

	t.Run("boughtout UNLOCK_REQUESTED head: FAILED validate_gate (UI)", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusUnlockRequested, "SHW3B", "", nil, true)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonValidateGate)
		status, cp := f.headState(head)
		require.Equal(t, mbhead.StatusUnlockRequested, status)
		require.Zero(t, cp)
	})

	t.Run("composition gate fails: FAILED validate_gate, head stays DRAFT", func(t *testing.T) {
		t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
		head := f.draftHead(mbhead.StatusDraft, "SHW4", "", nil, false)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonValidateGate)
		status, cp := f.headState(head)
		require.Equal(t, mbhead.StatusDraft, status)
		require.Zero(t, cp)
	})

	t.Run("same shade+FG live elsewhere: confirm required, then CS pair supersedes", func(t *testing.T) {
		fg := f.poy("FG-W5", "SHW5")
		oldHead := f.draftHead(mbhead.StatusDraft, "SHW5", "", &fg, false)
		res, err := wf.EnsureValidated(f.ctx, oldHead, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		oldSpin := uuid.MustParse(res.SpinID)

		newHead := f.draftHead(mbhead.StatusDraft, "SHW5", "", &fg, false)
		res, err = wf.EnsureValidated(f.ctx, newHead, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonSupersedeConfirmRequired)
		status, cp := f.headState(newHead)
		require.Equal(t, mbhead.StatusSubmitted, status, "submit committed; validate rolled back")
		require.Zero(t, cp)
		require.Empty(t, f.headSpins(newHead))

		// Rerun with the plan's CS pair: only Validate runs (no second submit).
		res, err = wf.EnsureValidated(f.ctx, newHead, "itest", true)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		status, cp = f.headState(newHead)
		require.Equal(t, mbhead.StatusValidated, status)
		require.NotZero(t, cp)
		require.Equal(t, 1, f.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'SUBMITTED'`, newHead))
		require.True(t, f.spinSup(oldSpin).supersededBy.Valid, "old spin superseded by the new root")

		var after struct {
			Batch      *uuid.UUID  `json:"supersede_batch_id"`
			Superseded []uuid.UUID `json:"superseded_spin_ids"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.AfterJSON), &after))
		require.NotNil(t, after.Batch)
		require.Equal(t, []uuid.UUID{oldSpin}, after.Superseded)
	})

	t.Run("head with cost product: skipped (K7 handles the root)", func(t *testing.T) {
		head := f.draftHead(mbhead.StatusDraft, "SHW6", "", nil, false)
		f.withCostProduct(head)
		res, err := wf.EnsureValidated(f.ctx, head, "itest", false)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
		require.Empty(t, res.SpinID)
	})

	t.Run("missing head: stale_plan", func(t *testing.T) {
		res, err := wf.EnsureValidated(f.ctx, uuid.New(), "itest", false)
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan)
	})
}
