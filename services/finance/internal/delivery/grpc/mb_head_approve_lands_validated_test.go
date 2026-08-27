package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// ---------------------------------------------------------------------------
// USER DECISION 2026-08-26 — "OPSI A".
//
// The MB Recipe workflow is DRAFT → SUBMITTED → APPROVED, and the Validate BUTTON was
// removed from the screen with MB product auto-generation moved onto Approve. Pressing
// Approve therefore has to land the recipe DIRECTLY in VALIDATED: the gRPC ApproveMBHead
// RPC drives ValidateHandler, not ApproveHandler.
//
// 🔴 Why the workflow could not simply stop at APPROVED, stated once here because it is
// the reason this whole file exists:
//
//	mb_head_repository.ListValidated() selects WHERE mbh_entry_status = 'VALIDATED', and
//	TWO engines consume it — MB Push to Head (application/mbpush preview + execute) and
//	Trigger MB Batch (application/mbbatch/dag.go). If nothing ever reached VALIDATED, MB
//	Push to Head would always be empty and MB Batch would always find zero candidates,
//	which kills MB costing outright.
//
// This file pins the landing status at the RPC boundary, ⛔ WITHOUT any database: the
// assertion is on the entity's own status string, which is precisely what ListValidated
// filters on.
// ---------------------------------------------------------------------------

// approveFakeRepo is a minimal mbhead.Repository that serves one in-memory head and
// records the transition the handler asked for. It deliberately does NOT re-implement any
// domain rule — the entity mutates itself, and this fake only observes.
type approveFakeRepo struct {
	head *mbhead.Entity

	autoGenCalls int
	lastFrom     string
	lastTo       string
	lastVersion  int32
	lastParams   *mbhead.ParamSnapshot

	// plainTransitionCalls counts calls to Transition (the NON-auto-gen path that
	// ApproveHandler used to take). Under Opsi A it must stay at zero: an approval that
	// skipped auto-gen would leave the recipe without its MB cost product.
	plainTransitionCalls int
}

func (f *approveFakeRepo) Create(_ context.Context, _ *mbhead.Entity) error { return nil }

func (f *approveFakeRepo) GetByID(_ context.Context, _ uuid.UUID) (*mbhead.Entity, error) {
	if f.head == nil {
		return nil, mbhead.ErrNotFound
	}
	return f.head, nil
}

func (f *approveFakeRepo) GetByMBCosting(_ context.Context, _ string) (*mbhead.Entity, error) {
	return nil, mbhead.ErrNotFound
}

func (f *approveFakeRepo) List(_ context.Context, _ mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	return nil, 0, nil
}

func (f *approveFakeRepo) Update(_ context.Context, _ *mbhead.Entity) error { return nil }

func (f *approveFakeRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error { return nil }

func (f *approveFakeRepo) ExistsByMBCosting(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *approveFakeRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.head != nil, nil
}

func (f *approveFakeRepo) ListAll(_ context.Context, _ mbhead.ExportFilter) ([]*mbhead.Entity, error) {
	return nil, nil
}

func (f *approveFakeRepo) Transition(_ context.Context, _ uuid.UUID, _, _ string, _ int32, _, _ string, _ *mbhead.ParamSnapshot) error {
	f.plainTransitionCalls++
	return nil
}

func (f *approveFakeRepo) TransitionWithAutoGen(
	_ context.Context, _ uuid.UUID, fromState, toState string, currentVersion int32,
	_, _ string, params *mbhead.ParamSnapshot, _ *mbhead.Entity,
) error {
	f.autoGenCalls++
	f.lastFrom, f.lastTo, f.lastVersion, f.lastParams = fromState, toState, currentVersion, params
	return nil
}

func (f *approveFakeRepo) ListShades(_ context.Context, _ uuid.UUID) ([]mbhead.Shade, error) {
	return nil, nil
}

func (f *approveFakeRepo) ReplaceShades(_ context.Context, _ uuid.UUID, _ []mbhead.Shade, _ string) error {
	return nil
}

func (f *approveFakeRepo) ExistsByVSNumber(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *approveFakeRepo) ExistsByDevCode(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *approveFakeRepo) RefreezeCostParams(_ context.Context, _ uuid.UUID, _ *mbhead.Entity, _ *mbhead.ParamSnapshot) error {
	return nil
}

// approveFakeParamRepo serves the 8 recipe params ValidateHandler freezes. Only
// ListActive is exercised; the rest satisfy the interface.
type approveFakeParamRepo struct{}

func (approveFakeParamRepo) Create(_ context.Context, _ *mbparam.Entity) error { return nil }
func (approveFakeParamRepo) Update(_ context.Context, _ *mbparam.Entity) error { return nil }
func (approveFakeParamRepo) Delete(_ context.Context, _ string) error          { return nil }

func (approveFakeParamRepo) GetByID(_ context.Context, _ string) (*mbparam.Entity, error) {
	return nil, mbparam.ErrParamNotFound
}

func (approveFakeParamRepo) List(_ context.Context, _ mbparam.ListFilter) ([]*mbparam.Entity, int64, error) {
	return nil, 0, nil
}

func (approveFakeParamRepo) ListActive(_ context.Context) ([]*mbparam.Entity, error) {
	specs := []struct{ code, typ, value, option string }{
		{"WASTE", mbparam.TypeScalar, "2", ""},
		{"QUALITY_LOSS", mbparam.TypeScalar, "0.6", ""},
		{"EFFICIENCY", mbparam.TypeScalar, "94", ""},
		{"DEV_EXPENSE", mbparam.TypeScalar, "1", ""},
		{"PACKING", mbparam.TypeScalar, "3", ""},
		{"MB_PROD_PER_DAY", mbparam.TypeScalar, "16", ""},
		{"THROUGHPUT_PER_HOUR", mbparam.TypePicklist, "", "B"},
		{"NO_OF_PROCESS", mbparam.TypePicklist, "", "D"},
	}
	out := make([]*mbparam.Entity, 0, len(specs))
	for i, s := range specs {
		e, err := mbparam.NewEntity(s.code, s.code, s.typ, "", s.value, s.option, "", int32(i), "seed") //nolint:gosec // fixed 8-element slice, index cannot overflow int32
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (approveFakeParamRepo) ListAll(_ context.Context, _ mbparam.ExportFilter) ([]*mbparam.Entity, error) {
	return nil, nil
}

func (approveFakeParamRepo) GetByCode(_ context.Context, _ string) (*mbparam.Entity, error) {
	return nil, mbparam.ErrParamNotFound
}

func (approveFakeParamRepo) CreateOption(_ context.Context, _ *mbparam.Option) error { return nil }
func (approveFakeParamRepo) UpdateOption(_ context.Context, _ *mbparam.Option) error { return nil }
func (approveFakeParamRepo) DeleteOption(_ context.Context, _ string) error          { return nil }

// headAt builds a persisted-shaped head sitting in the given status.
func headAt(id uuid.UUID, status string, boughtout bool) *mbhead.Entity {
	return mbhead.Reconstruct(
		id, nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		status, boughtout, 0, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "", "",
		nil,
	)
}

func newApproveHandler(t *testing.T, status string, boughtout bool) (*MBHeadHandler, *approveFakeRepo, uuid.UUID) {
	t.Helper()
	id := uuid.New()
	repo := &approveFakeRepo{head: headAt(id, status, boughtout)}
	h, err := NewMBHeadHandler(repo, approveFakeParamRepo{}, fakeMBMachineRepo{})
	require.NoError(t, err)
	return h, repo, id
}

// --- the seal ---------------------------------------------------------------

// TestApproveMBHead_LandsInValidated_OpsiA is the CORE assertion of the user's decision.
// Pressing Approve on a SUBMITTED recipe must leave it in VALIDATED — ⛔ not APPROVED.
func TestApproveMBHead_LandsInValidated_OpsiA(t *testing.T) {
	for _, boughtout := range []bool{false, true} {
		name := "own-production"
		if boughtout {
			name = "boughtout"
		}
		t.Run(name, func(t *testing.T) {
			h, repo, id := newApproveHandler(t, mbhead.StatusSubmitted, boughtout)

			resp, err := h.ApproveMBHead(context.Background(), &financev1.ApproveMBHeadRequest{MbhId: id.String()})

			require.NoError(t, err)
			require.NotNil(t, resp.Base)
			require.Truef(t, resp.Base.IsSuccess, "approve failed: %s", resp.Base.Message)
			require.NotNil(t, resp.Data)

			// 🔴 The whole decision in one line, asserted on the wire response.
			assert.Equal(t, mbhead.StatusValidated, resp.Data.EntryStatus,
				"Opsi A: Approve must land the recipe in VALIDATED, ⛔ never stop at APPROVED")

			// The exact literal ListValidated() filters on. Asserting the string rather
			// than the constant is deliberate: if the constant were ever renamed away
			// from the stored value, the two engines reading that column would go blank.
			assert.Equal(t, "VALIDATED", resp.Data.EntryStatus)

			// The transition actually asked of persistence.
			assert.Equal(t, 1, repo.autoGenCalls, "Approve must go through the auto-gen path")
			assert.Zero(t, repo.plainTransitionCalls,
				"⛔ the plain Transition path would skip MB product auto-gen")
			assert.Equal(t, mbhead.StatusSubmitted, repo.lastFrom)
			assert.Equal(t, mbhead.StatusValidated, repo.lastTo)

			// Validate's work really happened: version bumped and the 8 params frozen.
			assert.Equal(t, int32(1), repo.lastVersion, "validating bumps mbh_current_version")
			require.NotNil(t, repo.lastParams, "the 8 recipe params must be frozen on approve")
			assert.Equal(t, "B", repo.lastParams.ThroughputPerHour)
			assert.Equal(t, "D", repo.lastParams.NoOfProcess)
		})
	}
}

// TestApproveMBHead_ReportsLockInResponse pins the 2026-08-26 fix to the older defect:
// Approve()/Validate() moved the status but left the in-memory isLocked flag false, so
// the RPC response — built from that same entity — reported mbhIsLocked=false about a row
// the SQL layer had just locked. VALIDATED is a lockOnEnter state, so the response must
// now say so.
func TestApproveMBHead_ReportsLockInResponse(t *testing.T) {
	h, _, id := newApproveHandler(t, mbhead.StatusSubmitted, false)

	resp, err := h.ApproveMBHead(context.Background(), &financev1.ApproveMBHeadRequest{MbhId: id.String()})

	require.NoError(t, err)
	require.True(t, resp.Base.IsSuccess)
	require.NotNil(t, resp.Data.MbhIsLocked)
	assert.True(t, *resp.Data.MbhIsLocked,
		"entering VALIDATED locks the recipe; the response must not claim otherwise")
}

// TestApproveMBHead_RefusesFromIllegalOrigin keeps the widened path honest: Approve was
// opened up for SUBMITTED, ⛔ not for everything. A DRAFT own-production recipe must still
// be submitted first, and a REJECTED one must not jump forward.
func TestApproveMBHead_RefusesFromIllegalOrigin(t *testing.T) {
	for _, status := range []string{mbhead.StatusDraft, mbhead.StatusRejected, mbhead.StatusRevoked} {
		t.Run(status, func(t *testing.T) {
			h, repo, id := newApproveHandler(t, status, false)

			resp, err := h.ApproveMBHead(context.Background(), &financev1.ApproveMBHeadRequest{MbhId: id.String()})

			require.NoError(t, err, "⛔ BaseResponse pattern: the error travels in the body")
			require.NotNil(t, resp.Base)
			assert.False(t, resp.Base.IsSuccess, "approve from %s must be refused", status)
			assert.Zero(t, repo.autoGenCalls, "a refused approve must not reach persistence")
		})
	}
}

// TestValidateMBHead_LegacyApprovedRowStillWorks pins the half of the decision that is
// easy to lose: the ValidateMBHead RPC was ⛔ NOT removed and ⛔ NOT disabled — only its
// button left the screen. Production holds legacy rows parked in APPROVED, and this RPC
// is how they still reach VALIDATED.
func TestValidateMBHead_LegacyApprovedRowStillWorks(t *testing.T) {
	h, repo, id := newApproveHandler(t, mbhead.StatusApproved, false)

	resp, err := h.ValidateMBHead(context.Background(), &financev1.ValidateMBHeadRequest{MbhId: id.String()})

	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	require.Truef(t, resp.Base.IsSuccess, "validate failed: %s", resp.Base.Message)
	assert.Equal(t, mbhead.StatusValidated, resp.Data.EntryStatus)
	assert.Equal(t, mbhead.StatusApproved, repo.lastFrom, "the legacy origin must be recorded as APPROVED")
	assert.Equal(t, 1, repo.autoGenCalls)
}

// TestValidateMBHead_BoughtoutDraftShortcutSurvives pins the OTHER surviving path. A
// boughtout recipe used to reach VALIDATED only through the DRAFT shortcut behind the
// Validate button. That button is gone, and the normal route for such a recipe is now
// Submit → Approve (covered above) — but the shortcut itself was deliberately ⛔ NOT
// closed, so the surviving RPC can still perform it.
func TestValidateMBHead_BoughtoutDraftShortcutSurvives(t *testing.T) {
	h, repo, id := newApproveHandler(t, mbhead.StatusDraft, true)

	resp, err := h.ValidateMBHead(context.Background(), &financev1.ValidateMBHeadRequest{MbhId: id.String()})

	require.NoError(t, err)
	require.Truef(t, resp.Base.IsSuccess, "validate failed: %s", resp.Base.Message)
	assert.Equal(t, mbhead.StatusValidated, resp.Data.EntryStatus)
	assert.Equal(t, mbhead.StatusDraft, repo.lastFrom)
}
