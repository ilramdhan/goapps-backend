package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// fakeAuditEmitter records every event; err simulates a failing sink.
type fakeAuditEmitter struct {
	events []AuditEvent
	err    error
}

func (f *fakeAuditEmitter) Emit(_ context.Context, e AuditEvent) error {
	f.events = append(f.events, e)
	return f.err
}

// stubResultRepo implements only the transitions; other methods panic via the
// nil embedded interface if ever called.
type stubResultRepo struct {
	costcalcdom.ResultRepository
	verified, approved []int64
	err                error
}

func (s *stubResultRepo) MarkVerified(_ context.Context, id int64, _ string) error {
	s.verified = append(s.verified, id)
	return s.err
}

func (s *stubResultRepo) MarkApproved(_ context.Context, id int64, _ string) error {
	s.approved = append(s.approved, id)
	return s.err
}

func newAuditTestService(repo costcalcdom.ResultRepository, em AuditEmitter) *Service {
	return NewService(nil, nil, nil, repo, nil, nil, nil, em, nil)
}

func TestVerifyCostHandler_EmitsOneAuditEvent(t *testing.T) {
	t.Parallel()
	em := &fakeAuditEmitter{}
	repo := &stubResultRepo{}
	h := NewVerifyCostHandler(newAuditTestService(repo, em))
	require.NoError(t, h.Handle(context.Background(), VerifyCostCommand{CostID: 11, Actor: "bob"}))
	require.Len(t, em.events, 1)
	assert.Equal(t, "COST_RESULT_VERIFIED", em.events[0].EventType)
	assert.Equal(t, "COST_RESULT", em.events[0].EntityKind)
	assert.Equal(t, "11", em.events[0].EntityID)
	assert.Equal(t, "bob", em.events[0].Actor)
	assert.Equal(t, []int64{11}, repo.verified)
}

func TestApproveCostHandler_EmitsOneAuditEvent(t *testing.T) {
	t.Parallel()
	em := &fakeAuditEmitter{}
	repo := &stubResultRepo{}
	h := NewApproveCostHandler(newAuditTestService(repo, em))
	require.NoError(t, h.Handle(context.Background(), ApproveCostCommand{CostID: 12, Actor: "carol"}))
	require.Len(t, em.events, 1)
	assert.Equal(t, "COST_RESULT_APPROVED", em.events[0].EventType)
	assert.Equal(t, "12", em.events[0].EntityID)
	assert.Equal(t, "carol", em.events[0].Actor)
	assert.Equal(t, []int64{12}, repo.approved)
}

func TestApproveCostHandler_EmitterErrorIsBestEffort(t *testing.T) {
	t.Parallel()
	em := &fakeAuditEmitter{err: errors.New("audit down")}
	h := NewApproveCostHandler(newAuditTestService(&stubResultRepo{}, em))
	require.NoError(t, h.Handle(context.Background(), ApproveCostCommand{CostID: 1, Actor: "x"}),
		"a failing audit sink must not fail the approval")
	assert.Len(t, em.events, 1)
}

func TestApproveCostHandler_RepoErrorEmitsNothing(t *testing.T) {
	t.Parallel()
	em := &fakeAuditEmitter{}
	h := NewApproveCostHandler(newAuditTestService(&stubResultRepo{err: costcalcdom.ErrCostInvalidStatus}, em))
	require.Error(t, h.Handle(context.Background(), ApproveCostCommand{CostID: 1, Actor: "x"}))
	assert.Empty(t, em.events)
}
