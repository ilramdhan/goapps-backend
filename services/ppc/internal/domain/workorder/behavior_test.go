package workorder_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

func newWO(t *testing.T) *workorder.WorkOrder {
	t.Helper()
	wo, err := workorder.New(workorder.NewParams{
		WoNo:       "WO-TXT-1",
		LotNo:      "TXT1",
		AreaCode:   "TXT",
		MachineID:  2,
		CrhHeadID:  10,
		CrhVersion: 1,
		PlanItemID: 1,
		QtyTarget:  1000,
		Deadline:   time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		CreatedBy:  1,
	})
	require.NoError(t, err)
	return wo
}

func TestNew_DefaultsProdCategoryNormalAndDraft(t *testing.T) {
	wo := newWO(t)
	assert.Equal(t, workorder.StatusDraft, wo.Status())
	assert.Equal(t, workorder.ProdCategoryNormal, wo.ProdCategory())
}

func TestNew_RequiresRoute(t *testing.T) {
	_, err := workorder.New(workorder.NewParams{
		WoNo: "WO-1", LotNo: "L1", AreaCode: "TXT", MachineID: 2,
		PlanItemID: 1, QtyTarget: 1000, Deadline: time.Now().Add(time.Hour), CreatedBy: 1,
	})
	assert.ErrorIs(t, err, workorder.ErrInvalidRoute)
}

func TestNew_InvalidRefType(t *testing.T) {
	ref := int64(5)
	_, err := workorder.New(workorder.NewParams{
		WoNo: "WO-1", LotNo: "L1", AreaCode: "TXT", MachineID: 2, CrhHeadID: 10, CrhVersion: 1,
		PlanItemID: 1, QtyTarget: 1000, Deadline: time.Now().Add(time.Hour), CreatedBy: 1,
		RefWoID: &ref, RefType: "BOGUS",
	})
	assert.ErrorIs(t, err, workorder.ErrInvalidRefType)
}

func TestSubmit_Legal(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	assert.Equal(t, workorder.StatusSubmitted, wo.Status())
}

func TestApprove_SequentialPCThenPM(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	now := time.Now()

	done, err := wo.ApprovePC(5, now)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Equal(t, workorder.StatusPCApproved, wo.Status())
	assert.True(t, wo.PCApproved())

	done, err = wo.ApprovePM(6, now)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, workorder.StatusApproved, wo.Status())
	assert.True(t, wo.PMApproved())
}

func TestApprove_PCNotSubmitted_Fails(t *testing.T) {
	wo := newWO(t) // DRAFT
	_, err := wo.ApprovePC(5, time.Now())
	assert.ErrorIs(t, err, workorder.ErrNotSubmitted)
}

func TestApprove_PMBeforePC_Fails(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	_, err := wo.ApprovePM(6, time.Now()) // sequential: PC must go first
	assert.ErrorIs(t, err, workorder.ErrPCApprovalRequired)
	assert.Equal(t, workorder.StatusSubmitted, wo.Status())
}

func TestApprove_InvalidSide(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	_, err := wo.Approve("XX", 5, time.Now())
	assert.ErrorIs(t, err, workorder.ErrInvalidApprovalSide)
}

func TestSnapshots_SetOnApprove(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	_, err := wo.ApprovePC(5, time.Now())
	require.NoError(t, err)
	wo.SetSnapshots(map[string]any{"denier": "150"}, map[string]any{"box": "A"})
	_, err = wo.ApprovePM(6, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "150", wo.SpecSnapshot()["denier"])
	assert.Equal(t, "A", wo.PackingSnapshot()["box"])
}

func TestReject_Legal(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	require.NoError(t, wo.Reject("bad params"))
	assert.Equal(t, workorder.StatusRejected, wo.Status())
	assert.Equal(t, "bad params", wo.PlanChangeNote())
}

func TestReject_EmptyReason(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	assert.ErrorIs(t, wo.Reject(""), workorder.ErrEmptyReason)
}

func TestCancel_Legal(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Cancel("plan dropped"))
	assert.Equal(t, workorder.StatusCancelled, wo.Status())
}

func TestCancel_EmptyReason(t *testing.T) {
	wo := newWO(t)
	assert.ErrorIs(t, wo.Cancel(""), workorder.ErrEmptyReason)
}

func TestUpdate_NonDraftRequiresRevisionReason(t *testing.T) {
	wo := newWO(t)
	require.NoError(t, wo.Submit())
	mid := int64(9)
	err := wo.Update(workorder.UpdateParams{MachineID: &mid})
	assert.ErrorIs(t, err, workorder.ErrNotEditable)

	reason := "PINDAH MC 05"
	require.NoError(t, wo.Update(workorder.UpdateParams{MachineID: &mid, RevisionReason: &reason}))
	assert.Equal(t, int32(1), wo.RevisionNo())
	assert.Equal(t, reason, wo.RevisionReason())
}

func TestTransition_DraftCannotApprove(t *testing.T) {
	wo := newWO(t)
	_, err := wo.Approve(workorder.ApprovalSidePC, 1, time.Now())
	assert.ErrorIs(t, err, workorder.ErrNotSubmitted)
}
