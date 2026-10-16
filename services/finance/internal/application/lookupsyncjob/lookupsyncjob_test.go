package lookupsyncjob

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// fakeJobRepo is an in-memory job.Repository covering what Enqueuer/Tracker use.
type fakeJobRepo struct {
	byID     map[uuid.UUID]*job.Execution
	children map[uuid.UUID][]*job.Execution
	failedN  int
}

func newFakeJobRepo() *fakeJobRepo {
	return &fakeJobRepo{byID: map[uuid.UUID]*job.Execution{}, children: map[uuid.UUID][]*job.Execution{}}
}

func (f *fakeJobRepo) Create(_ context.Context, e *job.Execution) error {
	e.SetCode(job.GenerateCode(e.JobType(), e.Period(), 1))
	f.byID[e.ID()] = e
	return nil
}
func (f *fakeJobRepo) GetByID(_ context.Context, id uuid.UUID) (*job.Execution, error) {
	if e, ok := f.byID[id]; ok {
		return e, nil
	}
	return nil, job.ErrNotFound
}
func (f *fakeJobRepo) GetByCode(context.Context, string) (*job.Execution, error) {
	return nil, job.ErrNotFound
}
func (f *fakeJobRepo) List(context.Context, job.ListFilter) ([]*job.Execution, int64, error) {
	return nil, 0, nil
}
func (f *fakeJobRepo) UpdateStatus(_ context.Context, e *job.Execution) error {
	f.byID[e.ID()] = e
	return nil
}
func (f *fakeJobRepo) UpdateProgress(context.Context, uuid.UUID, int) error { return nil }
func (f *fakeJobRepo) AddLog(context.Context, *job.ExecutionLog) error      { return nil }
func (f *fakeJobRepo) UpdateLog(context.Context, *job.ExecutionLog) error   { return nil }
func (f *fakeJobRepo) HasActiveJob(context.Context, job.Type, string) (bool, error) {
	return false, nil
}
func (f *fakeJobRepo) GetNextSequence(context.Context, job.Type, string) (int, error) { return 1, nil }
func (f *fakeJobRepo) CreateChildren(_ context.Context, execs []*job.Execution) error {
	for _, e := range execs {
		f.byID[e.ID()] = e
		f.children[*e.ParentJobID()] = append(f.children[*e.ParentJobID()], e)
	}
	return nil
}
func (f *fakeJobRepo) IncrementChildProgress(_ context.Context, _ uuid.UUID, success bool) (bool, error) {
	if !success {
		f.failedN++
	}
	return false, nil
}
func (f *fakeJobRepo) ListChildren(_ context.Context, id uuid.UUID) ([]*job.Execution, error) {
	return f.children[id], nil
}

type fakeWalker struct {
	items []lookupsync.Item
	err   error
}

func (w *fakeWalker) Walk(_ context.Context, _ lookupsync.Scope, visit func(lookupsync.Item) error) (lookupsync.Summary, error) {
	if w.err != nil {
		return lookupsync.Summary{}, w.err
	}
	for _, it := range w.items {
		if err := visit(it); err != nil {
			return lookupsync.Summary{}, err
		}
	}
	return lookupsync.Summary{}, nil
}

type published struct {
	jobID     string
	product   int64
	scopeJSON string
	fp        string
}

type fakePublisher struct {
	calls  []published
	failOn int64
}

func (p *fakePublisher) PublishProductLookupSync(_ context.Context, jobID string, pid int64, scopeJSON, fp, _ string) error {
	p.calls = append(p.calls, published{jobID, pid, scopeJSON, fp})
	if pid == p.failOn {
		return errors.New("broker down")
	}
	return nil
}

func change() []lookupsync.ValueChange { return []lookupsync.ValueChange{{Kind: lookupsync.ChangeSet}} }

func masterScope() lookupsync.Scope {
	return lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "MACHINE", MasterKey: "M1"}}
}

func TestEnqueue_OneChildPerProductWithChanges(t *testing.T) {
	repo := newFakeJobRepo()
	pub := &fakePublisher{}
	w := &fakeWalker{items: []lookupsync.Item{
		{ProductSysID: 1, Changes: change()},
		{ProductSysID: 1, Changes: change()}, // second trigger of same product → dedup
		{ProductSysID: 2, SkipReason: lookupsync.SkipLocked, Changes: change()},
		{ProductSysID: 3},
		{ProductSysID: 4, Changes: change()},
	}}
	parent, err := NewEnqueuer(repo, pub, w).Enqueue(context.Background(), masterScope(), "fp1", "alice")
	require.NoError(t, err)

	assert.Equal(t, job.TypeProductLookupSync, parent.JobType())
	assert.Equal(t, 2, parent.TotalChildren())
	require.Len(t, pub.calls, 2)
	assert.Equal(t, []int64{1, 4}, []int64{pub.calls[0].product, pub.calls[1].product})
	assert.Equal(t, "fp1", pub.calls[0].fp)
	scope, err := DecodeScope(pub.calls[0].scopeJSON)
	require.NoError(t, err)
	require.NotNil(t, scope.MasterRef)
	assert.Equal(t, "M1", scope.MasterRef.MasterKey)
	assert.Nil(t, scope.ProductSysIDs)

	kids := repo.children[parent.ID()]
	require.Len(t, kids, 2)
	var cp ChildParams
	require.NoError(t, json.Unmarshal(kids[1].Params(), &cp))
	assert.Equal(t, int64(4), cp.ProductSysID)
}

func TestEnqueue_Errors(t *testing.T) {
	ctx := context.Background()
	_, err := NewEnqueuer(newFakeJobRepo(), nil, &fakeWalker{}).Enqueue(ctx, masterScope(), "fp", "alice")
	require.ErrorIs(t, err, ErrPublisherUnavailable)

	_, err = NewEnqueuer(newFakeJobRepo(), &fakePublisher{}, &fakeWalker{items: []lookupsync.Item{{ProductSysID: 1}}}).
		Enqueue(ctx, masterScope(), "fp", "alice")
	require.ErrorIs(t, err, ErrNothingToApply)

	_, err = NewEnqueuer(newFakeJobRepo(), &fakePublisher{}, &fakeWalker{err: errors.New("db")}).
		Enqueue(ctx, masterScope(), "fp", "alice")
	require.Error(t, err)

	_, err = NewEnqueuer(newFakeJobRepo(), &fakePublisher{}, &fakeWalker{}).Enqueue(ctx, masterScope(), "fp", "")
	require.Error(t, err)
}

func TestEnqueue_PublishFailureFailsOnlyThatChild(t *testing.T) {
	repo := newFakeJobRepo()
	pub := &fakePublisher{failOn: 2}
	w := &fakeWalker{items: []lookupsync.Item{{ProductSysID: 1, Changes: change()}, {ProductSysID: 2, Changes: change()}}}
	parent, err := NewEnqueuer(repo, pub, w).Enqueue(context.Background(), masterScope(), "fp", "alice")
	require.NoError(t, err)
	assert.Equal(t, 1, repo.failedN)
	kids := repo.children[parent.ID()]
	assert.Equal(t, job.StatusQueued, kids[0].Status())
	assert.Equal(t, job.StatusFailed, kids[1].Status())
}

func TestTracker_StatusAndFailures(t *testing.T) {
	ctx := context.Background()
	repo := newFakeJobRepo()
	parent, err := job.NewParentExecution(job.TypeProductLookupSync, "", "", "alice", 5, nil, 3)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, parent))

	mk := func(pid int64) *job.Execution {
		cp, mErr := json.Marshal(ChildParams{ProductSysID: pid})
		require.NoError(t, mErr)
		c, cErr := job.NewChildExecution(job.TypeProductLookupSync, "", "", "alice", 5, cp, parent.ID())
		require.NoError(t, cErr)
		require.NoError(t, c.Start())
		return c
	}
	done, skipped, failed := mk(1), mk(2), mk(3)
	res := func(r ChildResult) json.RawMessage {
		b, mErr := json.Marshal(r)
		require.NoError(t, mErr)
		return b
	}
	require.NoError(t, done.Complete(res(ChildResult{ProductSysID: 1, ValuesWritten: 3, ValuesCleared: 1})))
	require.NoError(t, skipped.Complete(res(ChildResult{ProductSysID: 2, Skipped: true, SkipReason: lookupsync.SkipLocked})))
	require.NoError(t, failed.Fail("boom"))
	require.NoError(t, repo.CreateChildren(ctx, []*job.Execution{done, skipped, failed}))

	tr := NewTracker(repo, nil)
	st, err := tr.Status(ctx, parent.ID().String())
	require.NoError(t, err)
	assert.Equal(t, 1, st.SkippedChildren)
	assert.Equal(t, 4, st.ValuesWritten)
	assert.Equal(t, "QUEUED", StatusString(st.Execution))

	fails, err := tr.Failures(ctx, parent.ID().String())
	require.NoError(t, err)
	require.Len(t, fails, 1)
	assert.Equal(t, int64(3), fails[0].ProductSysID)
	assert.Equal(t, "boom", fails[0].ErrorMessage)

	_, err = tr.Status(ctx, "bad")
	require.ErrorIs(t, err, ErrInvalidJobID)
	_, err = tr.Failures(ctx, uuid.NewString())
	require.ErrorIs(t, err, job.ErrNotFound)
}
