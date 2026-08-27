package mbrelock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// fakeRelockRepo is a hand-written double for mbhead.RelockRepository. It records every
// ApplyRelock call and can be told to fail for specific mb_costing values.
type fakeRelockRepo struct {
	candidates []mbhead.RelockCandidate
	listErr    error
	failFor    map[string]error
	applied    []appliedCall
}

type appliedCall struct {
	MBCosting string
	ToState   string
}

func (f *fakeRelockRepo) ListExpiredUnlocks(_ context.Context) ([]mbhead.RelockCandidate, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.candidates, nil
}

func (f *fakeRelockRepo) ApplyRelock(_ context.Context, c mbhead.RelockCandidate, toState string) error {
	if err, ok := f.failFor[c.MBCosting]; ok {
		return err
	}
	f.applied = append(f.applied, appliedCall{MBCosting: c.MBCosting, ToState: toState})
	return nil
}

var _ mbhead.RelockRepository = (*fakeRelockRepo)(nil)

// GrantedAt is stamped an hour ago: the job forwards it to ApplyRelock, which re-checks
// the untouched-since-grant condition inside its own transaction. ⛔ The JOB itself does
// not evaluate it — that check is SQL, so no test in this package can exercise it.
func candidate(costing, preUnlock string) mbhead.RelockCandidate {
	return mbhead.RelockCandidate{
		ID: uuid.New(), MBCosting: costing, PreUnlockStatus: preUnlock, CurrentVersion: 3,
		GrantedAt: time.Now().Add(-time.Hour),
	}
}

// TestRun_RelocksEachCandidateToItsOrigin pins K-57 option (a): every candidate goes back
// to the state it was unlocked FROM, ⛔ not to a single hardcoded status.
func TestRun_RelocksEachCandidateToItsOrigin(t *testing.T) {
	repo := &fakeRelockRepo{candidates: []mbhead.RelockCandidate{
		candidate("MB-A", mbhead.StatusApproved),
		candidate("MB-B", mbhead.StatusValidated),
	}}
	NewJob(repo).Run()

	require.Len(t, repo.applied, 2)
	assert.Equal(t, appliedCall{"MB-A", mbhead.StatusApproved}, repo.applied[0])
	assert.Equal(t, appliedCall{"MB-B", mbhead.StatusValidated}, repo.applied[1])
}

// TestRun_SkipsCandidateWithUnknownOrigin is the ⛔ never-guess rule: with no pre-unlock
// status the job must NOT invent APPROVED or VALIDATED, and must leave the head alone.
func TestRun_SkipsCandidateWithUnknownOrigin(t *testing.T) {
	repo := &fakeRelockRepo{candidates: []mbhead.RelockCandidate{
		candidate("MB-UNKNOWN", ""),
		candidate("MB-OK", mbhead.StatusValidated),
	}}
	NewJob(repo).Run()

	require.Len(t, repo.applied, 1, "the origin-less candidate must not be relocked at all")
	assert.Equal(t, "MB-OK", repo.applied[0].MBCosting)
}

// TestRun_OneFailureDoesNotStopTheRest — a single bad candidate must not shield the others.
func TestRun_OneFailureDoesNotStopTheRest(t *testing.T) {
	boom := errors.New("deadlock detected")
	repo := &fakeRelockRepo{
		candidates: []mbhead.RelockCandidate{
			candidate("MB-1", mbhead.StatusApproved),
			candidate("MB-BOOM", mbhead.StatusApproved),
			candidate("MB-3", mbhead.StatusValidated),
		},
		failFor: map[string]error{"MB-BOOM": boom},
	}
	NewJob(repo).Run()

	require.Len(t, repo.applied, 2)
	assert.Equal(t, "MB-1", repo.applied[0].MBCosting)
	assert.Equal(t, "MB-3", repo.applied[1].MBCosting)
}

// TestRun_ListFailureIsLoggedNotPanicked — a cron goroutine that panics kills the service.
func TestRun_ListFailureIsLoggedNotPanicked(t *testing.T) {
	repo := &fakeRelockRepo{listErr: errors.New("connection refused")}
	assert.NotPanics(t, NewJob(repo).Run)
	assert.Empty(t, repo.applied)
}

func TestRun_NoCandidatesIsANoOp(t *testing.T) {
	repo := &fakeRelockRepo{}
	assert.NotPanics(t, NewJob(repo).Run)
	assert.Empty(t, repo.applied)
}
