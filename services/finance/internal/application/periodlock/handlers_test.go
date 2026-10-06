package periodlock

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// fakeRepo is an in-memory cst_period_lock.
type fakeRepo struct {
	rows          map[string]*domain.PeriodLock
	batchLocked   bool
	repushFlagged int64
	getErr        error
	lockErr       error
	unlockErr     error
	batchErr      error
	lockCalls     int
	unlockCalls   int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]*domain.PeriodLock{}} }

func key(p, c string) string { return p + "/" + c }

func (f *fakeRepo) Get(_ context.Context, p, c string) (*domain.PeriodLock, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	l, ok := f.rows[key(p, c)]
	if !ok {
		return nil, domain.ErrLockNotFound
	}
	cp := *l
	return &cp, nil
}

func (f *fakeRepo) IsLocked(_ context.Context, p, c string) (bool, error) {
	l, ok := f.rows[key(p, c)]
	return ok && l.IsLocked(), nil
}

func (f *fakeRepo) HasLockedErpBatch(context.Context, string) (bool, error) {
	return f.batchLocked, f.batchErr
}

func (f *fakeRepo) Lock(_ context.Context, l *domain.PeriodLock) error {
	f.lockCalls++
	if f.lockErr != nil {
		return f.lockErr
	}
	if cur, ok := f.rows[key(l.Period(), l.CalcType())]; ok && cur.IsLocked() {
		return domain.ErrAlreadyLocked
	}
	cp := *l
	f.rows[key(l.Period(), l.CalcType())] = &cp
	return nil
}

func (f *fakeRepo) Unlock(_ context.Context, l *domain.PeriodLock) (int64, error) {
	f.unlockCalls++
	if f.unlockErr != nil {
		return 0, f.unlockErr
	}
	cp := *l
	f.rows[key(l.Period(), l.CalcType())] = &cp
	return f.repushFlagged, nil
}

type spyAudit struct {
	events []auditdomain.NewInput
	err    error
}

func (s *spyAudit) Emit(_ context.Context, in auditdomain.NewInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	s.events = append(s.events, in)
	return s.err
}

type probeFunc func(context.Context, string) (bool, error)

func (p probeFunc) IsAdjPosted(ctx context.Context, period string) (bool, error) {
	return p(ctx, period)
}

var fixedNow = time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)

func newLock(repo domain.Repository, a AuditSink) *LockHandler {
	h := NewLockHandler(repo, a)
	h.now = func() time.Time { return fixedNow }
	return h
}

func newUnlock(repo domain.Repository, p domain.AdjPostedProbe, a AuditSink) *UnlockHandler {
	h := NewUnlockHandler(repo, p, a)
	h.now = func() time.Time { return fixedNow.Add(time.Hour) }
	return h
}

func TestLockThenIsLocked(t *testing.T) {
	ctx := context.Background()
	repo, audit := newFakeRepo(), &spyAudit{}
	l, err := newLock(repo, audit).Handle(ctx, LockCommand{Period: "202608", User: "alice", Reason: "month close"})
	require.NoError(t, err)
	assert.True(t, l.IsLocked())
	assert.Equal(t, domain.CalcTypeActual, l.CalcType())

	locked, err := repo.IsLocked(ctx, "202608", domain.CalcTypeActual)
	require.NoError(t, err)
	assert.True(t, locked)

	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpErpPeriodLock, ev.Operation)
	assert.Equal(t, AuditEntityPeriodLock, ev.EntityType)
	assert.Equal(t, int64(202608), ev.EntityID)
	assert.Equal(t, "alice", ev.UserID)
	assert.Empty(t, ev.BeforeData)
	var after lockSnapshot
	require.NoError(t, json.Unmarshal([]byte(ev.AfterData), &after))
	assert.True(t, after.Locked)
	assert.Equal(t, "month close", after.Reason)
}

func TestLockAlreadyLocked(t *testing.T) {
	ctx := context.Background()
	repo, audit := newFakeRepo(), &spyAudit{}
	h := newLock(repo, audit)
	_, err := h.Handle(ctx, LockCommand{Period: "202608", User: "alice", Reason: "r"})
	require.NoError(t, err)
	_, err = h.Handle(ctx, LockCommand{Period: "202608", User: "bob", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrAlreadyLocked)
	assert.Equal(t, 1, repo.lockCalls)
	assert.Len(t, audit.events, 1)
}

func TestLockValidation(t *testing.T) {
	ctx := context.Background()
	repo, audit := newFakeRepo(), &spyAudit{}
	h := newLock(repo, audit)
	cases := []struct {
		cmd  LockCommand
		want error
	}{
		{LockCommand{Period: "2026", User: "u", Reason: "r"}, domain.ErrInvalidPeriod},
		{LockCommand{Period: "202608", CalcType: "FORECAST", User: "u", Reason: "r"}, domain.ErrInvalidCalcType},
		{LockCommand{Period: "202608", CalcType: "SELLING", User: "u", Reason: "r"}, domain.ErrInvalidCalcType},
		{LockCommand{Period: "202608", Reason: "r"}, domain.ErrUserRequired},
		{LockCommand{Period: "202608", User: "u"}, domain.ErrReasonRequired},
	}
	for _, tc := range cases {
		_, err := h.Handle(ctx, tc.cmd)
		assert.ErrorIs(t, err, tc.want)
	}
	assert.Zero(t, repo.lockCalls)
	assert.Empty(t, audit.events)
}

func TestLockRepoErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db down")

	repo := newFakeRepo()
	repo.getErr = boom
	_, err := newLock(repo, nil).Handle(ctx, LockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom)

	repo = newFakeRepo()
	repo.lockErr = boom
	audit := &spyAudit{}
	_, err = newLock(repo, audit).Handle(ctx, LockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom)
	assert.Empty(t, audit.events, "no audit when the write fails")
}

func TestLockAuditFailureIsBestEffort(t *testing.T) {
	repo, audit := newFakeRepo(), &spyAudit{err: errors.New("audit down")}
	l, err := newLock(repo, audit).Handle(context.Background(), LockCommand{Period: "202608", User: "u", Reason: "r"})
	require.NoError(t, err)
	assert.True(t, l.IsLocked())
}

func TestLockNilAudit(t *testing.T) {
	_, err := newLock(newFakeRepo(), nil).Handle(context.Background(), LockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.NoError(t, err)
}

func lockedRepo(t *testing.T) *fakeRepo {
	t.Helper()
	repo := newFakeRepo()
	_, err := newLock(repo, nil).Handle(context.Background(), LockCommand{Period: "202608", User: "alice", Reason: "close"})
	require.NoError(t, err)
	return repo
}

func TestUnlockSuccess(t *testing.T) {
	ctx := context.Background()
	repo, audit := lockedRepo(t), &spyAudit{}
	repo.repushFlagged = 2
	res, err := newUnlock(repo, StaticAdjPostedProbe{}, audit).Handle(ctx,
		UnlockCommand{Period: "202608", User: "bob", Reason: "reopen"})
	require.NoError(t, err)
	assert.False(t, res.Lock.IsLocked())
	assert.Equal(t, int64(2), res.RepushFlagged)

	locked, err := repo.IsLocked(ctx, "202608", domain.CalcTypeActual)
	require.NoError(t, err)
	assert.False(t, locked)

	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpErpPeriodUnlock, ev.Operation)
	assert.Equal(t, "bob", ev.UserID)
	var before, after lockSnapshot
	require.NoError(t, json.Unmarshal([]byte(ev.BeforeData), &before))
	require.NoError(t, json.Unmarshal([]byte(ev.AfterData), &after))
	assert.True(t, before.Locked)
	assert.Equal(t, "alice", before.LockedBy)
	assert.False(t, after.Locked)
	assert.Equal(t, "reopen", after.UnlockReason)
	require.NotNil(t, after.RepushFlagged)
	assert.Equal(t, int64(2), *after.RepushFlagged)
}

func TestUnlockPostedRefused(t *testing.T) {
	ctx := context.Background()
	repo, audit := lockedRepo(t), &spyAudit{}
	var probed string
	probe := probeFunc(func(_ context.Context, p string) (bool, error) { probed = p; return true, nil })
	_, err := newUnlock(repo, probe, audit).Handle(ctx, UnlockCommand{Period: "202608", User: "bob", Reason: "reopen"})
	assert.ErrorIs(t, err, domain.ErrUnlockPostedPeriod)
	assert.Equal(t, "202608", probed)
	assert.Zero(t, repo.unlockCalls)
	assert.Empty(t, audit.events)
	locked, _ := repo.IsLocked(ctx, "202608", domain.CalcTypeActual)
	assert.True(t, locked)
}

func TestUnlockStaticPostedProbeRefuses(t *testing.T) {
	repo := lockedRepo(t)
	_, err := newUnlock(repo, StaticAdjPostedProbe{Posted: true}, nil).Handle(context.Background(),
		UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrUnlockPostedPeriod)
}

func TestUnlockBatchLockedRefused(t *testing.T) {
	repo, audit := lockedRepo(t), &spyAudit{}
	repo.batchLocked = true
	probeCalled := false
	probe := probeFunc(func(context.Context, string) (bool, error) { probeCalled = true; return false, nil })
	_, err := newUnlock(repo, probe, audit).Handle(context.Background(),
		UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrUnlockBatchLocked)
	assert.False(t, probeCalled)
	assert.Zero(t, repo.unlockCalls)
	assert.Empty(t, audit.events)
}

func TestUnlockNotLocked(t *testing.T) {
	ctx := context.Background()
	_, err := newUnlock(newFakeRepo(), StaticAdjPostedProbe{}, nil).Handle(ctx,
		UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrNotLocked)

	repo := lockedRepo(t)
	h := newUnlock(repo, StaticAdjPostedProbe{}, nil)
	_, err = h.Handle(ctx, UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	require.NoError(t, err)
	_, err = h.Handle(ctx, UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrNotLocked)
}

func TestUnlockValidationAndErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	_, err := newUnlock(newFakeRepo(), StaticAdjPostedProbe{}, nil).Handle(ctx, UnlockCommand{Period: "x"})
	assert.ErrorIs(t, err, domain.ErrInvalidPeriod)
	_, err = newUnlock(newFakeRepo(), StaticAdjPostedProbe{}, nil).Handle(ctx, UnlockCommand{Period: "202608", CalcType: "FORECAST"})
	assert.ErrorIs(t, err, domain.ErrInvalidCalcType)

	repo := lockedRepo(t)
	_, err = newUnlock(repo, StaticAdjPostedProbe{}, nil).Handle(ctx, UnlockCommand{Period: "202608", User: "", Reason: "r"})
	assert.ErrorIs(t, err, domain.ErrUserRequired)

	repo = lockedRepo(t)
	repo.getErr = boom
	_, err = newUnlock(repo, StaticAdjPostedProbe{}, nil).Handle(ctx, UnlockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom)

	repo = lockedRepo(t)
	repo.batchErr = boom
	_, err = newUnlock(repo, StaticAdjPostedProbe{}, nil).Handle(ctx, UnlockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom)

	repo = lockedRepo(t)
	probe := probeFunc(func(context.Context, string) (bool, error) { return false, boom })
	_, err = newUnlock(repo, probe, nil).Handle(ctx, UnlockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom, "probe error fails closed")
	assert.Zero(t, repo.unlockCalls)

	repo = lockedRepo(t)
	_, err = newUnlock(repo, nil, nil).Handle(ctx, UnlockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, ErrAdjProbeNotConfigured)
	assert.Zero(t, repo.unlockCalls)

	repo = lockedRepo(t)
	repo.unlockErr = boom
	audit := &spyAudit{}
	_, err = newUnlock(repo, StaticAdjPostedProbe{}, audit).Handle(ctx, UnlockCommand{Period: "202608", User: "u", Reason: "r"})
	assert.ErrorIs(t, err, boom)
	assert.Empty(t, audit.events)
}

func TestRelockAfterUnlock(t *testing.T) {
	ctx := context.Background()
	repo, audit := lockedRepo(t), &spyAudit{}
	_, err := newUnlock(repo, StaticAdjPostedProbe{}, audit).Handle(ctx, UnlockCommand{Period: "202608", User: "bob", Reason: "r"})
	require.NoError(t, err)
	_, err = newLock(repo, audit).Handle(ctx, LockCommand{Period: "202608", User: "carol", Reason: "again"})
	require.NoError(t, err)
	require.Len(t, audit.events, 2)
	assert.Equal(t, auditdomain.OpErpPeriodLock, audit.events[1].Operation)
	var before lockSnapshot
	require.NoError(t, json.Unmarshal([]byte(audit.events[1].BeforeData), &before))
	assert.False(t, before.Locked, "re-lock carries the unlocked before-snapshot")
}

func TestGetHandler(t *testing.T) {
	ctx := context.Background()
	h := NewGetHandler(lockedRepo(t))
	l, err := h.Handle(ctx, GetQuery{Period: "202608"})
	require.NoError(t, err)
	assert.True(t, l.IsLocked())

	_, err = h.Handle(ctx, GetQuery{Period: "202607"})
	assert.ErrorIs(t, err, domain.ErrLockNotFound)
	_, err = h.Handle(ctx, GetQuery{Period: "bad"})
	assert.ErrorIs(t, err, domain.ErrInvalidPeriod)
	_, err = h.Handle(ctx, GetQuery{Period: "202608", CalcType: "SELLING"})
	assert.ErrorIs(t, err, domain.ErrInvalidCalcType)
}

func TestEmitAuditBadPeriodSkipped(t *testing.T) {
	audit := &spyAudit{}
	emitAudit(context.Background(), audit, auditdomain.OpErpPeriodLock, "notanumber", "u", nil, nil)
	assert.Empty(t, audit.events)
	assert.Empty(t, marshalSnapshot(nil))
	assert.Nil(t, snapshotOf(nil))
}
