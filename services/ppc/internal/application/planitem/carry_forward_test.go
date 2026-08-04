package planitem_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

const (
	sourceMonth = "2026-08"
	targetMonth = "2026-09"
)

// seedItem persists one plan item in the given status and returns it. Deadline
// is fixed at 2026-08-15, so its own month is sourceMonth.
func seedItem(t *testing.T, repo *memRepo, status string, qty float64) *planitemdomain.PlanItem {
	t.Helper()
	demandID := int64(42)
	item, err := planitemdomain.New(planitemdomain.NewParams{
		CpmProductSysID: 100,
		Type:            planitemdomain.TypeFGDelivery,
		DemandID:        &demandID,
		QtyTarget:       qty,
		Deadline:        deadline(),
		RMSource:        planitemdomain.RMSourceStore,
		MachineGroupID:  7,
		ShadeCode:       "S-1",
		ShadeName:       "TURQUOISE",
		Notes:           "keep me",
		CreatedBy:       1,
	})
	require.NoError(t, err)
	advanceTo(t, item, status)
	require.NoError(t, repo.Create(context.Background(), item))
	return item
}

// advanceTo walks a freshly-created DRAFT item forward to the wanted status one
// legal transition at a time. The state machine rejects a jump (DRAFT straight
// to IN_PROGRESS is not a transition), so the path has to be walked.
func advanceTo(t *testing.T, item *planitemdomain.PlanItem, want string) {
	t.Helper()
	if want == planitemdomain.StatusDraft {
		return
	}
	lifecycle := []string{
		planitemdomain.StatusConfirmed,
		planitemdomain.StatusInProgress,
		planitemdomain.StatusCompleted,
		planitemdomain.StatusClosed,
	}
	for _, step := range lifecycle {
		next := step
		_, err := item.Update(planitemdomain.UpdateParams{Status: &next})
		require.NoError(t, err, "advancing to %s", next)
		if next == want {
			return
		}
	}
	t.Fatalf("unknown plan item status %q", want)
}

func carryService(repo *memRepo) *planitemapp.Service {
	return planitemapp.NewService(repo, nil, nil).WithCapacity(fixedCapacity{perDay: 100})
}

func carryCmd(id int64) planitemapp.ProcessPlanCarryForwardCommand {
	return planitemapp.ProcessPlanCarryForwardCommand{
		SourcePlanItemID: id,
		Action:           planitemdomain.CarryActionAsIs,
		TargetMonth:      targetMonth,
		ActedBy:          9,
	}
}

// ── Eligible statuses ────────────────────────────────────────────────────────

// DRAFT and CONFIRMED carry; the other three do not. This is the T1 decision
// made executable: IN_PROGRESS is excluded because its unfinished work belongs
// to the work-order carry scope, and carrying both would plan it twice.
func TestProcessPlanCarryForward_StatusEligibility(t *testing.T) {
	cases := []struct {
		status    string
		wantCarry bool
	}{
		{planitemdomain.StatusDraft, true},
		{planitemdomain.StatusConfirmed, true},
		{planitemdomain.StatusInProgress, false},
		{planitemdomain.StatusCompleted, false},
		{planitemdomain.StatusClosed, false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			repo := newMemRepo()
			item := seedItem(t, repo, tc.status, 500)

			child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))

			if !tc.wantCarry {
				require.ErrorIs(t, err, planitemdomain.ErrNotCarryCandidate)
				assert.Nil(t, child)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, child)
			assert.Equal(t, targetMonth, child.Month())
		})
	}
}

// ── CARRY_AS_IS ──────────────────────────────────────────────────────────────

// S-2.2: the carried item preserves demand link, product, machine intent and
// remaining qty, and is traceable back to its source.
func TestProcessPlanCarryForward_AsIs_PreservesEverything(t *testing.T) {
	repo := newMemRepo()
	machineID := int64(55)
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	preferred := machineID
	_, err := item.Update(planitemdomain.UpdateParams{PreferredMachineID: &preferred})
	require.NoError(t, err)

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))
	require.NoError(t, err)
	require.NotNil(t, child)

	assert.Equal(t, targetMonth, child.Month())
	assert.Equal(t, item.CpmProductSysID(), child.CpmProductSysID())
	require.NotNil(t, child.DemandID())
	assert.Equal(t, *item.DemandID(), *child.DemandID(), "the demand link must survive the carry")
	assert.Equal(t, item.MachineGroupID(), child.MachineGroupID())
	require.NotNil(t, child.PreferredMachineID())
	assert.Equal(t, machineID, *child.PreferredMachineID(), "machine intent must survive the carry")
	assert.Equal(t, 500.0, child.QtyTarget())
	assert.Equal(t, item.ShadeCode(), child.ShadeCode())
	assert.Equal(t, item.Deadline(), child.Deadline())

	// Traceability (S-2.2): the child names its source.
	require.NotNil(t, child.CarryFromItemID())
	assert.Equal(t, item.ID(), *child.CarryFromItemID())
	assert.Equal(t, planitemdomain.CarryActionAsIs, child.CarryAction())

	// The source is deliberately left alone: its work orders still point at it,
	// and its own month's plan stays an accurate record.
	assert.Equal(t, planitemdomain.StatusConfirmed, item.Status())
	assert.Equal(t, sourceMonth, item.Month())
}

// A new deadline is honoured and the month override still parks the item in the
// requested target month rather than the one the deadline implies.
func TestProcessPlanCarryForward_NewDeadline_MonthStillTarget(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusDraft, 500)
	newDeadline := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

	cmd := carryCmd(item.ID())
	cmd.NewDeadline = &newDeadline
	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.NoError(t, err)

	assert.Equal(t, newDeadline, child.Deadline())
	assert.Equal(t, targetMonth, child.Month(), "the requested target month wins over the deadline's own month")
}

// ── Quantity arithmetic / no double-count against the demand ─────────────────

// The core S-2.2 rule. A plan item half-covered by a work order carries only
// the uncovered half: the covered half is production already committed
// downstream of the same demand, and carrying it whole would book it twice.
func TestProcessPlanCarryForward_AsIs_CarriesOnlyUncoveredQty(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.coverage = map[int64]planitemdomain.Coverage{
		item.ID(): {QtyCovered: 300, WorkOrderCount: 1},
	}

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))
	require.NoError(t, err)

	assert.Equal(t, 200.0, child.QtyTarget(), "500 target less 300 already covered by a work order")
}

// Fully covered leaves nothing to carry, and says so rather than creating a
// zero-quantity plan item.
func TestProcessPlanCarryForward_FullyCovered_Rejected(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.coverage = map[int64]planitemdomain.Coverage{
		item.ID(): {QtyCovered: 500, WorkOrderCount: 2},
	}

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))

	require.ErrorIs(t, err, planitemdomain.ErrNothingToCarry)
	assert.Nil(t, child)
}

// Over-contribution (a work order that delivered more than its share) must not
// produce a negative carry.
func TestProcessPlanCarryForward_OverCovered_ClampsToNothingToCarry(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.coverage = map[int64]planitemdomain.Coverage{item.ID(): {QtyCovered: 720, WorkOrderCount: 1}}

	_, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))
	require.ErrorIs(t, err, planitemdomain.ErrNothingToCarry)
}

// ── PARTIAL_CARRY ────────────────────────────────────────────────────────────

func TestProcessPlanCarryForward_Partial_CarriesRequestedQty(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)

	cmd := carryCmd(item.ID())
	cmd.Action = planitemdomain.CarryActionPartial
	qty := 120.0
	cmd.CarryQty = &qty

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, 120.0, child.QtyTarget())
	assert.Equal(t, planitemdomain.CarryActionPartial, child.CarryAction())
}

// A partial carry is measured against the UNCOVERED quantity, not the target —
// otherwise it becomes a back door around the double-count rule.
func TestProcessPlanCarryForward_Partial_CannotExceedUncovered(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.coverage = map[int64]planitemdomain.Coverage{item.ID(): {QtyCovered: 400, WorkOrderCount: 1}}

	cmd := carryCmd(item.ID())
	cmd.Action = planitemdomain.CarryActionPartial
	qty := 150.0 // under the 500 target, but over the 100 uncovered
	cmd.CarryQty = &qty

	_, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.ErrorIs(t, err, planitemdomain.ErrCarryQtyExceedsUncovered)
}

func TestProcessPlanCarryForward_Partial_RejectsNonPositiveQty(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusDraft, 500)

	for _, qty := range []*float64{nil, ptrFloat64(0), ptrFloat64(-5)} {
		cmd := carryCmd(item.ID())
		cmd.Action = planitemdomain.CarryActionPartial
		cmd.CarryQty = qty
		_, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
		require.ErrorIs(t, err, planitemdomain.ErrInvalidQty)
	}
}

// ── Double-carry prevention (S-2.4) ──────────────────────────────────────────

func TestProcessPlanCarryForward_AlreadyCarried_Rejected(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.coverage = map[int64]planitemdomain.Coverage{
		item.ID(): {CarriedToMonths: []string{targetMonth}},
	}

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), carryCmd(item.ID()))

	require.ErrorIs(t, err, planitemdomain.ErrAlreadyCarried)
	assert.Nil(t, child)
}

// ── CANCEL ───────────────────────────────────────────────────────────────────

// CANCEL closes the item and creates nothing, and the decision is written to
// the plan-change log so the month-start run is auditable (S-2.4).
func TestProcessPlanCarryForward_Cancel_ClosesAndLogs(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)

	cmd := carryCmd(item.ID())
	cmd.Action = planitemdomain.CarryActionCancel

	child, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.NoError(t, err)
	assert.Nil(t, child, "cancel creates nothing in the target month")
	assert.Equal(t, planitemdomain.StatusClosed, item.Status())

	require.Len(t, repo.logs, 1)
	assert.Equal(t, "status", repo.logs[0].Field)
	assert.Equal(t, planitemdomain.StatusConfirmed, repo.logs[0].Before)
	assert.Equal(t, planitemdomain.StatusClosed, repo.logs[0].After)
	assert.Equal(t, int64(9), repo.logs[0].ChangedBy)
	assert.NotEmpty(t, repo.logs[0].Reason)
}

// ── Guards ───────────────────────────────────────────────────────────────────

func TestProcessPlanCarryForward_UnknownAction_Rejected(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusDraft, 500)

	cmd := carryCmd(item.ID())
	cmd.Action = "DEFER" // a demand action with no plan-item meaning

	_, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.ErrorIs(t, err, planitemdomain.ErrInvalidCarryAction)
}

func TestProcessPlanCarryForward_SameMonth_Rejected(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusDraft, 500)

	cmd := carryCmd(item.ID())
	cmd.TargetMonth = sourceMonth

	_, err := carryService(repo).ProcessPlanCarryForward(context.Background(), cmd)
	require.ErrorIs(t, err, planitemdomain.ErrSameMonth)
}

func TestProcessPlanCarryForward_MissingItem_NotFound(t *testing.T) {
	_, err := carryService(newMemRepo()).ProcessPlanCarryForward(context.Background(), carryCmd(999))
	require.ErrorIs(t, err, planitemdomain.ErrNotFound)
}

// ── Candidate list ───────────────────────────────────────────────────────────

// QtyUncovered is what the UI shows and what CARRY_AS_IS carries; they must be
// the same number, computed once in the domain.
func TestCarryCandidate_QtyUncovered(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)

	cases := []struct {
		covered float64
		want    float64
	}{
		{0, 500},
		{200, 300},
		{500, 0},
		{620, 0}, // clamped, never negative
	}
	for _, tc := range cases {
		c := &planitemdomain.CarryCandidate{
			Item:     item,
			Coverage: planitemdomain.Coverage{QtyCovered: tc.covered},
		}
		assert.Equal(t, tc.want, c.QtyUncovered())
	}
}

func TestListCarryCandidates_PassesThroughRepository(t *testing.T) {
	repo := newMemRepo()
	item := seedItem(t, repo, planitemdomain.StatusConfirmed, 500)
	repo.candidates = []*planitemdomain.CarryCandidate{{
		Item:        item,
		Coverage:    planitemdomain.Coverage{QtyCovered: 100, WorkOrderCount: 1},
		DemandLabel: "CTR-2026-001",
	}}

	got, err := carryService(repo).ListCarryCandidates(context.Background(), sourceMonth, targetMonth)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 400.0, got[0].QtyUncovered())
	assert.Equal(t, "CTR-2026-001", got[0].DemandLabel, "the demand is named, never surfaced as an id")
}
