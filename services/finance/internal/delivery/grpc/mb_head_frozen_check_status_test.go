package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// --- test doubles ------------------------------------------------------------

// fakeFrozenMBHeadRepo records whether a write ever reached persistence. The seal
// under test (plan §11 item 106) must reject BEFORE any repository call.
type fakeFrozenMBHeadRepo struct {
	stored     *mbhead.Entity
	createCall int
	updateCall int
}

func (f *fakeFrozenMBHeadRepo) Create(_ context.Context, e *mbhead.Entity) error {
	f.createCall++
	f.stored = e
	return nil
}

func (f *fakeFrozenMBHeadRepo) GetByID(_ context.Context, _ uuid.UUID) (*mbhead.Entity, error) {
	if f.stored == nil {
		return nil, mbhead.ErrNotFound
	}
	return f.stored, nil
}

func (f *fakeFrozenMBHeadRepo) GetByMBCosting(_ context.Context, _ string) (*mbhead.Entity, error) {
	return nil, mbhead.ErrNotFound
}

func (f *fakeFrozenMBHeadRepo) List(_ context.Context, _ mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	return nil, 0, nil
}

func (f *fakeFrozenMBHeadRepo) Update(_ context.Context, e *mbhead.Entity) error {
	f.updateCall++
	f.stored = e
	return nil
}

func (f *fakeFrozenMBHeadRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error { return nil }

func (f *fakeFrozenMBHeadRepo) ExistsByMBCosting(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *fakeFrozenMBHeadRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.stored != nil, nil
}

func (f *fakeFrozenMBHeadRepo) ListAll(_ context.Context, _ mbhead.ExportFilter) ([]*mbhead.Entity, error) {
	return nil, nil
}

func (f *fakeFrozenMBHeadRepo) Transition(_ context.Context, _ uuid.UUID, _, _ string, _ int32, _, _ string, _ *mbhead.ParamSnapshot) error {
	return nil
}

func (f *fakeFrozenMBHeadRepo) TransitionWithAutoGen(_ context.Context, _ uuid.UUID, _, _ string, _ int32, _, _ string, _ *mbhead.ParamSnapshot, _ *mbhead.Entity) error {
	return nil
}

func (f *fakeFrozenMBHeadRepo) ListShades(_ context.Context, _ uuid.UUID) ([]mbhead.Shade, error) {
	return nil, nil
}

func (f *fakeFrozenMBHeadRepo) ReplaceShades(_ context.Context, _ uuid.UUID, _ []mbhead.Shade, _ string) error {
	return nil
}

func (f *fakeFrozenMBHeadRepo) ExistsByVSNumber(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeFrozenMBHeadRepo) ExistsByDevCode(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeFrozenMBHeadRepo) RefreezeCostParams(_ context.Context, _ uuid.UUID, _ *mbhead.Entity, _ *mbhead.ParamSnapshot) error {
	return nil
}

// fakeMBMachineRepo resolves the "MB" machine that B5 pins every create to.
type fakeMBMachineRepo struct{}

func (fakeMBMachineRepo) Create(_ context.Context, _ *machine.Entity) error { return nil }

func (fakeMBMachineRepo) GetByID(_ context.Context, _ uuid.UUID) (*machine.Entity, error) {
	return nil, machine.ErrNotFound
}

func (fakeMBMachineRepo) GetByCode(_ context.Context, _ string) (*machine.Entity, error) {
	return machine.New(
		"MB", "Melange Batch", "MB", "PLANT",
		1, 1, 1, nil, 1, nil,
		nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		"", "admin",
	)
}

func (fakeMBMachineRepo) List(_ context.Context, _ machine.ListFilter) ([]*machine.Entity, int64, error) {
	return nil, 0, nil
}

func (fakeMBMachineRepo) Update(_ context.Context, _ *machine.Entity) error { return nil }

func (fakeMBMachineRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error { return nil }

func (fakeMBMachineRepo) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (fakeMBMachineRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) {
	return true, nil
}

func newFrozenSealHandler(t *testing.T) (*MBHeadHandler, *fakeFrozenMBHeadRepo) {
	t.Helper()
	repo := &fakeFrozenMBHeadRepo{}
	h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
	require.NoError(t, err)
	return h, repo
}

func strPtr(s string) *string { return &s }

// --- the seal ----------------------------------------------------------------

// TestCreateMBHead_RejectsFrozenCheckStatus fails the moment the §11 item 106 seal
// is lifted from the create path. User decision: option 2 — REJECT LOUDLY.
func TestCreateMBHead_RejectsFrozenCheckStatus(t *testing.T) {
	// Presence, not emptiness, is the trigger. "" is included on purpose: an
	// explicitly-sent empty string is the value that would ERASE an Oracle trace,
	// so it is the LAST case that may be waved through.
	for name, sent := range map[string]string{
		"non-empty value": "Current",
		"explicit empty":  "",
	} {
		t.Run(name, func(t *testing.T) {
			h, repo := newFrozenSealHandler(t)

			resp, err := h.CreateMBHead(context.Background(), &financev1.CreateMBHeadRequest{
				MbhMbCosting:   "MB-SEAL-1",
				MbhCheckStatus: strPtr(sent),
			})

			require.NoError(t, err)
			require.NotNil(t, resp.Base)
			assert.False(t, resp.Base.IsSuccess)
			assert.Equal(t, "400", resp.Base.StatusCode)
			assert.Contains(t, resp.Base.Message, "mbh_check_status is frozen",
				"the message must teach the rule, not just say 'invalid'")
			assert.Contains(t, resp.Base.Message, "mbh_check_status_calc",
				"the message must point at the replacement column")
			require.Len(t, resp.Base.ValidationErrors, 1)
			assert.Equal(t, "mbh_check_status", resp.Base.ValidationErrors[0].Field)

			assert.Nil(t, resp.Data)
			assert.Zero(t, repo.createCall, "the seal must reject before persistence is touched")
		})
	}
}

// TestUpdateMBHead_RejectsFrozenCheckStatus is the higher-stakes half: the row may
// already carry an Oracle trace, and an accepted write would destroy it.
func TestUpdateMBHead_RejectsFrozenCheckStatus(t *testing.T) {
	for name, sent := range map[string]string{
		"non-empty value": "Old",
		"explicit empty":  "",
	} {
		t.Run(name, func(t *testing.T) {
			h, repo := newFrozenSealHandler(t)
			seedFrozenHead(t, repo, "Current")

			resp, err := h.UpdateMBHead(context.Background(), &financev1.UpdateMBHeadRequest{
				MbhId:          repo.stored.ID().String(),
				MbhCheckStatus: strPtr(sent),
			})

			require.NoError(t, err)
			require.NotNil(t, resp.Base)
			assert.False(t, resp.Base.IsSuccess)
			assert.Equal(t, "400", resp.Base.StatusCode)
			assert.Contains(t, resp.Base.Message, "mbh_check_status is frozen")
			assert.Zero(t, repo.updateCall, "the seal must reject before persistence is touched")

			// The whole point of the seal: the Oracle trace survives untouched.
			require.NotNil(t, repo.stored.MBHCheckStatus())
			assert.Equal(t, "Current", *repo.stored.MBHCheckStatus())
		})
	}
}

// TestUpdateMBHead_WithoutCheckStatusSucceedsAndPreservesTrace is the regression
// guard the seal is most likely to break: an ORDINARY update, carrying no
// check-status field at all, must still succeed — and must leave the stored Oracle
// trace byte-for-byte intact (the repository writes it back from the loaded entity).
func TestUpdateMBHead_WithoutCheckStatusSucceedsAndPreservesTrace(t *testing.T) {
	h, repo := newFrozenSealHandler(t)
	seedFrozenHead(t, repo, "Current")

	resp, err := h.UpdateMBHead(context.Background(), &financev1.UpdateMBHeadRequest{
		MbhId:      repo.stored.ID().String(),
		MbhMgtName: strPtr("renamed"),
		// ⛔ MbhCheckStatus intentionally absent (nil).
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess, "an update without the frozen field must NOT be rejected")
	assert.Equal(t, 1, repo.updateCall)

	require.NotNil(t, repo.stored.MBHCheckStatus())
	assert.Equal(t, "Current", *repo.stored.MBHCheckStatus(),
		"the seal must PRESERVE the Oracle trace, never blank it")
	require.NotNil(t, repo.stored.MgtName())
	assert.Equal(t, "renamed", *repo.stored.MgtName())

	// The frozen column is still READ back to the client — sealing the write path
	// must not hide the value.
	require.NotNil(t, resp.Data)
	require.NotNil(t, resp.Data.MbhCheckStatus)
	assert.Equal(t, "Current", *resp.Data.MbhCheckStatus)
}

// TestCreateMBHead_WithoutCheckStatusSucceeds is the create-side regression guard.
func TestCreateMBHead_WithoutCheckStatusSucceeds(t *testing.T) {
	h, repo := newFrozenSealHandler(t)

	resp, err := h.CreateMBHead(context.Background(), &financev1.CreateMBHeadRequest{
		MbhMbCosting: "MB-SEAL-OK",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, 1, repo.createCall)

	// A newly created head never has an Oracle import trace — that column is written
	// only by the historical Oracle import.
	assert.Nil(t, repo.stored.MBHCheckStatus())
}

// seedFrozenHead stores a head carrying an Oracle import trace, reproducing the
// production shape the seal exists to protect.
//
// Reconstruct is the only door: mbhead.New can no longer accept a check status
// (§11 item 106), which is exactly the property under test — the trace can arrive
// from PERSISTENCE and nowhere else.
func seedFrozenHead(t *testing.T, repo *fakeFrozenMBHeadRepo, trace string) {
	t.Helper()
	repo.stored = mbhead.Reconstruct(
		uuid.New(), nil, "MB-SEAL-2", nil, nil,
		nil, nil, &trace, nil, nil, nil,
		nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbhead.StatusDraft, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "", "",
		nil,
	)
}
