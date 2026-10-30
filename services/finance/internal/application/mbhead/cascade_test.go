package mbhead_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// fakeLookupSyncTrigger records every after-commit trigger call. Calls arrive on a
// detached goroutine, so tests wait with require.Eventually.
type fakeLookupSyncTrigger struct {
	mu       sync.Mutex
	spins    []uuid.UUID
	spinErr  error
	actorSet map[string]struct{}
}

func (f *fakeLookupSyncTrigger) TriggerSpinSync(_ context.Context, spinID uuid.UUID, actor string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spins = append(f.spins, spinID)
	f.recordActor(actor)
	return f.spinErr
}

func (f *fakeLookupSyncTrigger) recordActor(actor string) {
	if f.actorSet == nil {
		f.actorSet = map[string]struct{}{}
	}
	f.actorSet[actor] = struct{}{}
}

func (f *fakeLookupSyncTrigger) calls() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.spins...)
}

// validatedHeadWithProduct is a head whose Validate already auto-generated cost
// product 1001 on machine validatedHeadMachine, carrying its own throughput/no_of_process.
var validatedHeadMachine = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func validatedHeadWithProduct() *mbheaddomain.Entity {
	machine := validatedHeadMachine
	return mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbheaddomain.StatusValidated, false, 1, nil,
		"", "", "", "", "", "",
		1001, nil, "",
		nil, nil, nil, nil, nil,
		strPtr("20"), "55", "T",
		&machine,
	)
}

func TestUpdateHandler_Cascade_MachineChange_RefreezesAndTriggersPerSpin(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	params := new(MockParamRepository)
	trigger := &fakeLookupSyncTrigger{}
	head := validatedHeadWithProduct()
	spinIDs := []uuid.UUID{uuid.New(), uuid.New()}

	repo.On("GetByID", ctx, head.ID()).Return(head, nil)
	params.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	var snap *mbheaddomain.ParamSnapshot
	repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", mock.AnythingOfType("*mbhead.ParamSnapshot")).
		Run(func(args mock.Arguments) { snap = args.Get(3).(*mbheaddomain.ParamSnapshot) }).
		Return(spinIDs, nil)

	newMachine := uuid.New()
	got, err := mbhead.NewUpdateHandler(repo, nil).WithParamRepo(params).WithLookupSyncTrigger(trigger).
		Handle(ctx, mbhead.UpdateCommand{ID: head.ID(), MachineID: &newMachine, UpdatedBy: "editor"})
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, snap, "machine_id feeds MACHINE_MB_FIXED_TOTAL, so the CPP must be re-frozen")
	assert.Equal(t, "55", snap.ThroughputPerHour, "head override must win, as in Refreeze")
	assert.Equal(t, "T", snap.NoOfProcess)
	assert.Equal(t, "55", got.ParamThroughputPerHour(), "snapshot must be frozen onto the entity for the CPP write")

	require.Eventually(t, func() bool { return len(trigger.calls()) == len(spinIDs) }, time.Second, 5*time.Millisecond)
	assert.ElementsMatch(t, spinIDs, trigger.calls())
	assert.Contains(t, trigger.actorSet, "editor")
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestUpdateHandler_Cascade_NonCPPEdit_NoRefreeze(t *testing.T) {
	sameMachine := validatedHeadMachine
	denier := 75.0
	cases := map[string]mbhead.UpdateCommand{
		"cosmetic mgt_name":     {MgtName: strPtr("typo fixed")},
		"spec denier":           {Denier: &denier},
		"same machine resent":   {MachineID: &sameMachine},
		"live no_of_process":    {NoOfProcess: strPtr("3")},
		"costing rename":        {MBCosting: strPtr("MB002")},
		"shade code and name":   {ShadeCode: strPtr("S1"), ShadeName: strPtr("Shade 1")},
		"final product + vs no": {MBHFinalProduct: strPtr("FP"), VSNumber: strPtr("VS-1")},
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := new(MockRepository)
			params := new(MockParamRepository)
			trigger := &fakeLookupSyncTrigger{}
			head := validatedHeadWithProduct()
			spinID := uuid.New()

			repo.On("GetByID", ctx, head.ID()).Return(head, nil)
			repo.On("ExistsByMBCosting", ctx, mock.Anything).Return(false, nil).Maybe()
			repo.On("ExistsByVSNumber", ctx, mock.Anything, mock.Anything).Return(false, nil).Maybe()
			repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).
				Return([]uuid.UUID{spinID}, nil)

			cmd.ID, cmd.UpdatedBy = head.ID(), "editor"
			got, err := mbhead.NewUpdateHandler(repo, nil).WithParamRepo(params).WithLookupSyncTrigger(trigger).
				Handle(ctx, cmd)
			require.NoError(t, err)
			assert.Equal(t, "55", got.ParamThroughputPerHour(), "frozen snapshot untouched")

			require.Eventually(t, func() bool { return len(trigger.calls()) == 1 }, time.Second, 5*time.Millisecond)
			params.AssertNotCalled(t, "ListActive", mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestUpdateHandler_Cascade_TriggerQueuedEvenWhenReplaceShadesFails(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	trigger := &fakeLookupSyncTrigger{}
	head := validatedHeadWithProduct()
	spinID := uuid.New()
	boom := errors.New("shade write failed")

	repo.On("GetByID", ctx, head.ID()).Return(head, nil)
	repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).
		Return([]uuid.UUID{spinID}, nil)
	repo.On("ReplaceShades", ctx, head.ID(), mock.Anything, "editor").Return(boom)

	_, err := mbhead.NewUpdateHandler(repo, nil).WithLookupSyncTrigger(trigger).
		Handle(ctx, mbhead.UpdateCommand{ID: head.ID(), ReplaceAdditionalShades: true, UpdatedBy: "editor"})
	require.ErrorIs(t, err, boom)

	// The cascade was already committed, so the spin sync is queued regardless.
	require.Eventually(t, func() bool { return len(trigger.calls()) == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []uuid.UUID{spinID}, trigger.calls())
}

func TestUpdateHandler_Cascade_TriggerErrorDoesNotFailUpdate(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	trigger := &fakeLookupSyncTrigger{spinErr: errors.New("publisher down")}
	head := validatedHeadWithProduct()

	repo.On("GetByID", ctx, head.ID()).Return(head, nil)
	repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).
		Return([]uuid.UUID{uuid.New(), uuid.New()}, nil)

	got, err := mbhead.NewUpdateHandler(repo, nil).WithLookupSyncTrigger(trigger).
		Handle(ctx, mbhead.UpdateCommand{ID: head.ID(), UpdatedBy: "editor"})
	require.NoError(t, err)
	require.NotNil(t, got)

	// Both spins are still attempted even though the first trigger failed.
	require.Eventually(t, func() bool { return len(trigger.calls()) == 2 }, time.Second, 5*time.Millisecond)
}

func TestUpdateHandler_Cascade_RepoErrorFailsUpdate_NoTrigger(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	trigger := &fakeLookupSyncTrigger{}
	head := validatedHeadWithProduct()
	boom := errors.New("freeze failed")

	repo.On("GetByID", ctx, head.ID()).Return(head, nil)
	repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).
		Return(nil, boom)

	_, err := mbhead.NewUpdateHandler(repo, nil).WithLookupSyncTrigger(trigger).
		Handle(ctx, mbhead.UpdateCommand{ID: head.ID(), UpdatedBy: "editor"})
	require.ErrorIs(t, err, boom)

	time.Sleep(20 * time.Millisecond)
	assert.Empty(t, trigger.calls())
}

func TestUpdateHandler_NoProduct_PlainUpdate_NoCascadeNoTrigger(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	params := new(MockParamRepository)
	trigger := &fakeLookupSyncTrigger{}
	head := newHeadWithCosting(t, "MB001") // never validated: CostProductID() == 0

	repo.On("GetByID", ctx, head.ID()).Return(head, nil)
	repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

	newMachine := uuid.New()
	_, err := mbhead.NewUpdateHandler(repo, nil).WithParamRepo(params).WithLookupSyncTrigger(trigger).
		Handle(ctx, mbhead.UpdateCommand{ID: head.ID(), MachineID: &newMachine, UpdatedBy: "editor"})
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	assert.Empty(t, trigger.calls())
	repo.AssertNotCalled(t, "UpdateWithCascade", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	params.AssertNotCalled(t, "ListActive", mock.Anything)
	repo.AssertExpectations(t)
}
