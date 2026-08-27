package mbhead_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// fakeNotifier records every event it is handed and can be told to fail, so tests can
// pin BOTH the recipient rules of each event AND the guarantee that a failing notifier
// never turns into a handler error.
type fakeNotifier struct {
	events []mbhead.Event
	err    error
}

func (f *fakeNotifier) NotifyEvent(_ context.Context, e mbhead.Event) error {
	f.events = append(f.events, e)
	return f.err
}

// only returns the single event the notifier received, failing the test otherwise.
func (f *fakeNotifier) only(t *testing.T) mbhead.Event {
	t.Helper()
	require.Len(t, f.events, 1, "expected exactly one notification")
	return f.events[0]
}

// assertSingleRule pins that an event carries exactly one recipient rule of the given
// type and value. This is the assertion that actually protects the feature: a wrong
// permission code silently notifies the wrong people.
func assertSingleRule(t *testing.T, e mbhead.Event, wantType, wantValue string) {
	t.Helper()
	require.Len(t, e.Rules, 1)
	assert.Equal(t, wantType, e.Rules[0].RuleType)
	assert.Equal(t, wantValue, e.Rules[0].Value)
}

// ---------------------------------------------------------------------------
// E1 — DRAFT → SUBMITTED notifies approvers
// ---------------------------------------------------------------------------

func TestSubmitHandler_Notifies_ApprovePermissionHolders(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewSubmitHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusDraft, mbheaddomain.StatusSubmitted,
		int32(1), "", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "drafter-1"})
	require.NoError(t, err)

	e := notifier.only(t)
	assert.Equal(t, mbhead.EventSubmitted, e.EventType)
	assert.Equal(t, entity.ID(), e.MbhID)
	assert.Equal(t, mbheaddomain.StatusDraft, e.FromState)
	assert.Equal(t, mbheaddomain.StatusSubmitted, e.ToState)
	// 🔴 finance.mb.head.approve — there is NO finance.mb.recipe.approve permission.
	assertSingleRule(t, e, mbhead.RuleByPermission, "finance.mb.head.approve")

	mockRepo.AssertExpectations(t)
}

// A failing notifier must NOT fail the submit — the transition is already committed.
func TestSubmitHandler_NotifierError_DoesNotFailTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{err: errors.New("iam unreachable")}
	handler := mbhead.NewSubmitHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusDraft, mbheaddomain.StatusSubmitted,
		int32(1), "", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "drafter-1"})
	require.NoError(t, err, "notification failure must not surface as a handler error")
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusSubmitted, got.EntryStatus())
}

// A nil notifier is the default construction and must stay a silent no-op.
func TestSubmitHandler_NilNotifier_IsNoOp(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := mbhead.NewSubmitHandler(mockRepo)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusDraft, mbheaddomain.StatusSubmitted,
		int32(1), "", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "drafter-1"})
	require.NoError(t, err)
}

// A failed transition must notify NOBODY — the event never happened.
func TestSubmitHandler_TransitionError_EmitsNothing(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewSubmitHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusDraft, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusDraft, mbheaddomain.StatusSubmitted,
		int32(1), "", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(errors.New("db down"))

	_, err := handler.Handle(ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "drafter-1"})
	require.Error(t, err)
	assert.Empty(t, notifier.events, "no notification for a transition that never committed")
}

// ---------------------------------------------------------------------------
// E2 — REJECTED → DRAFT notifies submitters
// ---------------------------------------------------------------------------

func TestReturnToDraftHandler_Notifies_SubmitPermissionHolders(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewReturnToDraftHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusRejected, "composition does not add up")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusRejected, mbheaddomain.StatusDraft,
		int32(1), "composition does not add up", "approver-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{MbhID: entity.ID(), ActorUserID: "approver-1"})
	require.NoError(t, err)

	e := notifier.only(t)
	assert.Equal(t, mbhead.EventReturnedToDraft, e.EventType)
	// 🔴 fromState pins this to the REJECTED road specifically — the grant-unlock and
	// create-new roads into DRAFT deliberately do not emit this event.
	assert.Equal(t, mbheaddomain.StatusRejected, e.FromState)
	assert.Equal(t, mbheaddomain.StatusDraft, e.ToState)
	assertSingleRule(t, e, mbhead.RuleByPermission, "finance.mb.head.submit")

	mockRepo.AssertExpectations(t)
}

func TestReturnToDraftHandler_NotifierError_DoesNotFailTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{err: errors.New("iam unreachable")}
	handler := mbhead.NewReturnToDraftHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := headInState(mbheaddomain.StatusRejected, "bad")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusRejected, mbheaddomain.StatusDraft,
		int32(1), "bad", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.ReturnToDraftCommand{MbhID: entity.ID(), ActorUserID: "approver-1"})
	require.NoError(t, err)
	assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus())
}

// ---------------------------------------------------------------------------
// E3 — APPROVED/VALIDATED → UNLOCK_REQUESTED notifies unlock deciders
// ---------------------------------------------------------------------------

func TestRequestUnlockHandler_Notifies_UnlockPermissionHolders(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewRequestUnlockHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
		int32(1), "shade is wrong", "drafter-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "shade is wrong", ActorUserID: "drafter-1",
	})
	require.NoError(t, err)

	e := notifier.only(t)
	assert.Equal(t, mbhead.EventUnlockRequested, e.EventType)
	assert.Equal(t, mbheaddomain.StatusValidated, e.FromState)
	assert.Equal(t, mbheaddomain.StatusUnlockRequested, e.ToState)
	assertSingleRule(t, e, mbhead.RuleByPermission, "finance.mb.recipe.unlock")

	mockRepo.AssertExpectations(t)
}

func TestRequestUnlockHandler_NotifierError_DoesNotFailTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{err: errors.New("iam unreachable")}
	handler := mbhead.NewRequestUnlockHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
		int32(1), "shade is wrong", "drafter-1",
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "shade is wrong", ActorUserID: "drafter-1",
	})
	require.NoError(t, err)
	assert.Equal(t, mbheaddomain.StatusUnlockRequested, got.EntryStatus())
}

// ---------------------------------------------------------------------------
// E4/E5 guard — the requester identity is destroyed before persist
// ---------------------------------------------------------------------------

// TestGrantUnlock_ClearsRequesterBeforePersist pins the domain behavior that dictates the
// SHAPE of E4/E5: by the time the transition is persisted, the entity no longer knows who
// asked for the unlock. Entity.GrantUnlock nils unlockRequestedBy (lock.go GrantUnlock),
// and RejectUnlock does the same — both BEFORE repo.Transition runs.
//
// 🔴 This is why GrantUnlockHandler/RejectUnlockHandler read the requester BEFORE calling
// the domain method. ⛔ Do not "simplify" them by moving that read after the transition.
// Note also that the value nil'd here is a USERNAME (mbh_unlock_requested_by is
// VARCHAR(20), migration 000485) and is ⛔ NOT usable for IAM's BY_USER_ID rule, which
// does uuid.Parse — the UUID travels separately via mbhl_meta.
//
// ⛔ The nil-ing behavior itself must not change; only the reading order around it.
func TestGrantUnlock_ClearsRequesterBeforePersist(t *testing.T) {
	requestedAt := time.Now()
	entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &requestedAt, mbheaddomain.StatusValidated)
	entity.HydrateExtras(mbheaddomain.PersistedExtras{
		IsLocked:          true,
		UnlockRequestedAt: &requestedAt,
		UnlockRequestedBy: strPtr("drafter-1"),
		PreUnlockStatus:   mbheaddomain.StatusValidated,
	})
	require.NotNil(t, entity.UnlockRequestedBy(), "precondition: requester is known")

	require.NoError(t, entity.GrantUnlock("approver-1"))

	assert.Nil(t, entity.UnlockRequestedBy(),
		"GrantUnlock destroys the requester identity — capture it BEFORE calling this")
}

// TestRejectUnlock_ClearsRequesterBeforePersist is the RejectUnlock twin of the above.
func TestRejectUnlock_ClearsRequesterBeforePersist(t *testing.T) {
	requestedAt := time.Now()
	entity := lockedHead(mbheaddomain.StatusUnlockRequested, true, &requestedAt, mbheaddomain.StatusValidated)
	entity.HydrateExtras(mbheaddomain.PersistedExtras{
		IsLocked:          true,
		UnlockRequestedAt: &requestedAt,
		UnlockRequestedBy: strPtr("drafter-1"),
		PreUnlockStatus:   mbheaddomain.StatusValidated,
	})
	require.NotNil(t, entity.UnlockRequestedBy())

	require.NoError(t, entity.RejectUnlock("approver-1", "not now"))

	assert.Nil(t, entity.UnlockRequestedBy(),
		"RejectUnlock destroys the requester identity — capture it BEFORE calling this")
}

// ---------------------------------------------------------------------------
// E4 — UNLOCK_REQUESTED → DRAFT notifies the original REQUESTER
// E5 — UNLOCK_REQUESTED → APPROVED/VALIDATED notifies the original REQUESTER
// ---------------------------------------------------------------------------

// fakeUnlockActors is an in-memory UnlockActorStore. recorded holds what
// RequestUnlockHandler stamped; lookup is what the grant/reject side reads back.
type fakeUnlockActors struct {
	recorded  map[uuid.UUID]string
	lookup    string
	lookupErr error
	readCalls int
}

func newFakeUnlockActors(lookup string) *fakeUnlockActors {
	return &fakeUnlockActors{recorded: map[uuid.UUID]string{}, lookup: lookup}
}

func (f *fakeUnlockActors) RecordUnlockRequestActor(_ context.Context, mbhID uuid.UUID, actorUUID string) error {
	f.recorded[mbhID] = actorUUID
	return nil
}

func (f *fakeUnlockActors) LatestUnlockRequestActor(_ context.Context, _ uuid.UUID) (string, error) {
	f.readCalls++
	return f.lookup, f.lookupErr
}

// pendingHead builds a head parked in UNLOCK_REQUESTED with a live pending marker.
func pendingHead(preUnlock string) *mbheaddomain.Entity {
	pending := time.Now()
	return lockedHead(mbheaddomain.StatusUnlockRequested, true, &pending, preUnlock)
}

// requesterUUID is a fixed, valid UUID standing in for the IAM user id of whoever asked
// for the unlock. 🔴 It must be parseable — that is the whole point of the field.
const requesterUUID = "3f1d0f7a-6b2e-4c4a-9a1b-2d5e8c7f0a11"

func TestGrantUnlockHandler_Notifies_OriginalRequesterByUserID(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	actors := newFakeUnlockActors(requesterUUID)
	handler := mbhead.NewGrantUnlockHandler(mockRepo).WithNotifier(notifier).WithUnlockActors(actors)
	ctx := context.Background()

	entity := pendingHead(mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusDraft,
		int32(1), "", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.NoError(t, err)

	e := notifier.only(t)
	assert.Equal(t, mbhead.EventUnlockGranted, e.EventType)
	assert.Equal(t, entity.ID(), e.MbhID)
	assert.Equal(t, mbheaddomain.StatusUnlockRequested, e.FromState)
	assert.Equal(t, mbheaddomain.StatusDraft, e.ToState)
	// 🔴 The recipient is the REQUESTER's UUID, ⛔ not the approver standing here.
	assertSingleRule(t, e, mbhead.RuleByUserID, requesterUUID)
	assert.NotEqual(t, "approver-1", e.Rules[0].Value)

	mockRepo.AssertExpectations(t)
}

func TestRejectUnlockHandler_Notifies_OriginalRequesterByUserID(t *testing.T) {
	for _, origin := range []string{mbheaddomain.StatusApproved, mbheaddomain.StatusValidated} {
		t.Run(origin, func(t *testing.T) {
			mockRepo := new(MockRepository)
			notifier := &fakeNotifier{}
			actors := newFakeUnlockActors(requesterUUID)
			handler := mbhead.NewRejectUnlockHandler(mockRepo).WithNotifier(notifier).WithUnlockActors(actors)
			ctx := context.Background()

			entity := pendingHead(origin)
			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
			mockRepo.On("Transition", ctx, entity.ID(),
				mbheaddomain.StatusUnlockRequested, origin,
				int32(1), "still in production", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
			).Return(nil)

			_, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
				MbhID: entity.ID(), Reason: "still in production", ActorUserID: "approver-1",
			})
			require.NoError(t, err)

			e := notifier.only(t)
			assert.Equal(t, mbhead.EventUnlockRejected, e.EventType)
			assert.Equal(t, mbheaddomain.StatusUnlockRequested, e.FromState)
			assert.Equal(t, origin, e.ToState)
			assertSingleRule(t, e, mbhead.RuleByUserID, requesterUUID)

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestGrantUnlockHandler_ReadsRequesterBeforeDomainClearsIt pins the ORDER that makes
// E4 possible at all: the store must be consulted before entity.GrantUnlock runs, since
// that method destroys the pending markers. Reading afterwards would still "work" against
// this fake, so the assertion is on the entity's post-state plus a delivered notification
// — together they can only both hold if the read happened first in a real flow.
func TestGrantUnlockHandler_ReadsRequesterBeforeDomainClearsIt(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	actors := newFakeUnlockActors(requesterUUID)
	handler := mbhead.NewGrantUnlockHandler(mockRepo).WithNotifier(notifier).WithUnlockActors(actors)
	ctx := context.Background()

	entity := pendingHead(mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		int32(1), mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		(*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.NoError(t, err)
	assert.Nil(t, got.UnlockRequestedAt(), "the domain still clears the pending marker")
	assert.Equal(t, 1, actors.readCalls, "the requester is looked up exactly once")
	require.Len(t, notifier.events, 1, "and the notification still found a recipient")
}

// TestUnlockDecision_NoRequesterUUID_EmitsNothing is the ⚠ WAJIB TANGGUH case, and it is
// a PASS/FAIL condition for the feature, not a nicety.
//
// 🔴 IAM resolves BY_USER_ID by calling uuid.Parse on the rule value and ABANDONS THE
// WHOLE FAN-OUT when it fails. So when the requester's UUID is missing (every lock-log row
// written before this feature existed carries mbhl_meta = NULL) or unparseable (a
// system/service-raised request), the handler must send ⛔ NO RULE AT ALL — not a blank
// one, not a username — and simply stay silent.
func TestUnlockDecision_NoRequesterUUID_EmitsNothing(t *testing.T) {
	cases := map[string]*fakeUnlockActors{
		"legacy row — mbhl_meta is NULL":  newFakeUnlockActors(""),
		"system-raised — not a UUID":      newFakeUnlockActors("system"),
		"username leaked in — not a UUID": newFakeUnlockActors("drafter-1"),
		"store unavailable — read fails":  {recorded: map[uuid.UUID]string{}, lookupErr: errors.New("db down")},
		"malformed UUID — nearly right":   newFakeUnlockActors("3f1d0f7a-6b2e-4c4a-9a1b"),
	}
	for name, actors := range cases {
		t.Run("grant/"+name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			notifier := &fakeNotifier{}
			handler := mbhead.NewGrantUnlockHandler(mockRepo).WithNotifier(notifier).WithUnlockActors(actors)
			ctx := context.Background()

			entity := pendingHead(mbheaddomain.StatusValidated)
			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
			mockRepo.On("Transition", ctx, entity.ID(),
				mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusDraft,
				int32(1), "", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
			).Return(nil)

			got, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
				MbhID: entity.ID(), ActorUserID: "approver-1",
			})
			require.NoError(t, err, "a missing requester must NEVER fail the grant")
			require.NotNil(t, got)
			assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus(), "the unlock still happened")
			assert.Empty(t, notifier.events,
				"⛔ no BY_USER_ID rule may be sent without a parseable UUID — it would kill IAM's whole fan-out")

			mockRepo.AssertExpectations(t)
		})
		t.Run("reject/"+name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			notifier := &fakeNotifier{}
			handler := mbhead.NewRejectUnlockHandler(mockRepo).WithNotifier(notifier).WithUnlockActors(actors)
			ctx := context.Background()

			entity := pendingHead(mbheaddomain.StatusApproved)
			mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
			mockRepo.On("Transition", ctx, entity.ID(),
				mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusApproved,
				int32(1), "no", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
			).Return(nil)

			got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
				MbhID: entity.ID(), Reason: "no", ActorUserID: "approver-1",
			})
			require.NoError(t, err, "a missing requester must NEVER fail the refusal")
			require.NotNil(t, got)
			assert.Equal(t, mbheaddomain.StatusApproved, got.EntryStatus(), "the refusal still happened")
			assert.Empty(t, notifier.events)

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestUnlockDecision_NilActorStore_IsSilentNoOp — the store is optional wiring, exactly
// like the notifier. Its absence must leave the transition working untouched.
func TestUnlockDecision_NilActorStore_IsSilentNoOp(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewGrantUnlockHandler(mockRepo).WithNotifier(notifier)
	ctx := context.Background()

	entity := pendingHead(mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusDraft,
		int32(1), "", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.NoError(t, err)
	assert.Equal(t, mbheaddomain.StatusDraft, got.EntryStatus())
	assert.Empty(t, notifier.events)
}

// TestUnlockDecision_NotifierError_DoesNotFailTransition — best-effort, same rule as E1/E2.
func TestUnlockDecision_NotifierError_DoesNotFailTransition(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{err: errors.New("iam unreachable")}
	handler := mbhead.NewRejectUnlockHandler(mockRepo).
		WithNotifier(notifier).WithUnlockActors(newFakeUnlockActors(requesterUUID))
	ctx := context.Background()

	entity := pendingHead(mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusValidated,
		int32(1), "no", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	got, err := handler.Handle(ctx, mbhead.RejectUnlockCommand{
		MbhID: entity.ID(), Reason: "no", ActorUserID: "approver-1",
	})
	require.NoError(t, err, "notification failure must not surface as a handler error")
	assert.Equal(t, mbheaddomain.StatusValidated, got.EntryStatus())
}

// TestUnlockDecision_TransitionError_EmitsNothing — a refusal that never committed
// notifies nobody, even though the requester WAS resolvable.
func TestUnlockDecision_TransitionError_EmitsNothing(t *testing.T) {
	mockRepo := new(MockRepository)
	notifier := &fakeNotifier{}
	handler := mbhead.NewGrantUnlockHandler(mockRepo).
		WithNotifier(notifier).WithUnlockActors(newFakeUnlockActors(requesterUUID))
	ctx := context.Background()

	entity := pendingHead(mbheaddomain.StatusValidated)
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusUnlockRequested, mbheaddomain.StatusDraft,
		int32(1), "", "approver-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(errors.New("db down"))

	_, err := handler.Handle(ctx, mbhead.GrantUnlockCommand{
		MbhID: entity.ID(), ActorUserID: "approver-1",
	})
	require.Error(t, err)
	assert.Empty(t, notifier.events, "no notification for a transition that never committed")
}

// TestRequestUnlockHandler_RecordsRequesterUUID — the write half of E4/E5. The UUID must
// be stamped for the head, and it must be the UUID, ⛔ never the username on ActorUserID.
func TestRequestUnlockHandler_RecordsRequesterUUID(t *testing.T) {
	mockRepo := new(MockRepository)
	actors := newFakeUnlockActors("")
	handler := mbhead.NewRequestUnlockHandler(mockRepo).WithUnlockActors(actors)
	ctx := context.Background()

	entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockRepo.On("Transition", ctx, entity.ID(),
		mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
		int32(1), "shade is wrong", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
	).Return(nil)

	_, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
		MbhID: entity.ID(), Reason: "shade is wrong",
		ActorUserID: "drafter-1", ActorUserUUID: requesterUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, requesterUUID, actors.recorded[entity.ID()])
	assert.NotEqual(t, "drafter-1", actors.recorded[entity.ID()],
		"⛔ the username must never be stored where a UUID is expected")
}

// TestRequestUnlockHandler_NonUUIDActor_IsNotRecorded — a caller with no UUID (system or
// service token) leaves nothing behind, so the later decision stays silent rather than
// storing a value IAM cannot parse.
func TestRequestUnlockHandler_NonUUIDActor_IsNotRecorded(t *testing.T) {
	for _, actorUUID := range []string{"", "system", "drafter-1"} {
		mockRepo := new(MockRepository)
		actors := newFakeUnlockActors("")
		handler := mbhead.NewRequestUnlockHandler(mockRepo).WithUnlockActors(actors)
		ctx := context.Background()

		entity := lockedHead(mbheaddomain.StatusValidated, true, nil, "")
		mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
		mockRepo.On("Transition", ctx, entity.ID(),
			mbheaddomain.StatusValidated, mbheaddomain.StatusUnlockRequested,
			int32(1), "r", "drafter-1", (*mbheaddomain.ParamSnapshot)(nil),
		).Return(nil)

		_, err := handler.Handle(ctx, mbhead.RequestUnlockCommand{
			MbhID: entity.ID(), Reason: "r",
			ActorUserID: "drafter-1", ActorUserUUID: actorUUID,
		})
		require.NoError(t, err, "an unrecordable identity must never fail the request")
		assert.Empty(t, actors.recorded, "nothing unparseable may be stored (actor=%q)", actorUUID)
	}
}
