package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbheadbulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// --- test doubles ------------------------------------------------------------

// fakeBulkJobRepo is a minimal in-memory job.Repository, just enough to exercise
// RequestBulkTransitionHandler + the 5 Bulk MB Head Regenerate RPCs — mirrors
// fakeFrozenMBHeadRepo's "record calls, in-memory store" shape
// (mb_head_frozen_check_status_test.go) rather than a testify mock, since these RPC
// tests only need simple happy/sad-path stubbing, not call-matcher assertions.
type fakeBulkJobRepo struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]*job.Execution
	children map[uuid.UUID][]*job.Execution
}

func newFakeBulkJobRepo() *fakeBulkJobRepo {
	return &fakeBulkJobRepo{byID: map[uuid.UUID]*job.Execution{}, children: map[uuid.UUID][]*job.Execution{}}
}

func (f *fakeBulkJobRepo) Create(_ context.Context, e *job.Execution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[e.ID()] = e
	return nil
}

func (f *fakeBulkJobRepo) GetByID(_ context.Context, id uuid.UUID) (*job.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.byID[id]
	if !ok {
		return nil, job.ErrNotFound
	}
	return e, nil
}

func (f *fakeBulkJobRepo) GetByCode(_ context.Context, _ string) (*job.Execution, error) {
	return nil, job.ErrNotFound
}

func (f *fakeBulkJobRepo) List(_ context.Context, _ job.ListFilter) ([]*job.Execution, int64, error) {
	return nil, 0, nil
}

func (f *fakeBulkJobRepo) UpdateStatus(_ context.Context, e *job.Execution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[e.ID()] = e
	return nil
}

func (f *fakeBulkJobRepo) UpdateProgress(_ context.Context, _ uuid.UUID, _ int) error { return nil }

func (f *fakeBulkJobRepo) AddLog(_ context.Context, _ *job.ExecutionLog) error { return nil }

func (f *fakeBulkJobRepo) UpdateLog(_ context.Context, _ *job.ExecutionLog) error { return nil }

func (f *fakeBulkJobRepo) HasActiveJob(_ context.Context, _ job.Type, _ string) (bool, error) {
	return false, nil
}

func (f *fakeBulkJobRepo) GetNextSequence(_ context.Context, _ job.Type, _ string) (int, error) {
	return 1, nil
}

func (f *fakeBulkJobRepo) CreateChildren(_ context.Context, execs []*job.Execution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range execs {
		f.byID[e.ID()] = e
		if e.ParentJobID() != nil {
			f.children[*e.ParentJobID()] = append(f.children[*e.ParentJobID()], e)
		}
	}
	return nil
}

func (f *fakeBulkJobRepo) IncrementChildProgress(_ context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parent, ok := f.byID[parentJobID]
	if !ok {
		return false, job.ErrNotFound
	}
	if success {
		parent.IncrementCompletedChildren()
	} else {
		parent.IncrementFailedChildren()
	}
	return parent.IsBatchComplete(), nil
}

func (f *fakeBulkJobRepo) ListChildren(_ context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.children[parentJobID], nil
}

// fakeBulkPublisher is a testable mbheadbulk.BulkTransitionJobPublisher. failFor, when
// set, fails PublishMBBulkTransition for those specific mbhIDs — everything else
// succeeds.
type fakeBulkPublisher struct {
	failFor map[string]bool
}

func (f *fakeBulkPublisher) PublishMBBulkTransition(_ context.Context, _, mbhID, _, _, _ string) error {
	if f.failFor != nil && f.failFor[mbhID] {
		return errors.New("publish failed")
	}
	return nil
}

// newBulkTestHandler wires an MBHeadHandler with a real RequestBulkTransitionHandler
// backed by the fakes above, via WithBulkTransition — exactly the shape
// cmd/server/main.go wires in production, just with fakes standing in for the real
// job repository / RabbitMQ publisher / MB Head repository.
func newBulkTestHandler(t *testing.T, pub mbheadbulk.BulkTransitionJobPublisher) (*MBHeadHandler, *fakeBulkJobRepo, *fakeFrozenMBHeadRepo) {
	t.Helper()
	mbHeadRepo := &fakeFrozenMBHeadRepo{}
	h, err := NewMBHeadHandler(mbHeadRepo, nil, fakeMBMachineRepo{})
	require.NoError(t, err)

	jobRepo := newFakeBulkJobRepo()
	bulkHandler := mbheadbulk.NewRequestBulkTransitionHandler(jobRepo, pub)
	h = h.WithBulkTransition(bulkHandler, jobRepo, mbHeadRepo)
	return h, jobRepo, mbHeadRepo
}

// --- BulkForceUnvalidateMBHead / BulkSubmitMBHead / BulkValidateMBHead ------------

func TestBulkForceUnvalidateMBHead_Success_QueuesJobAndReturnsJobInfo(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkForceUnvalidateMBHead(context.Background(), &financev1.BulkForceUnvalidateMBHeadRequest{
		MbhIds: []string{uuid.NewString(), uuid.NewString()},
		Reason: "bulk regenerate",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, resp.Data)
	assert.NotEmpty(t, resp.Data.JobId)
	assert.Equal(t, int32(2), resp.Data.TotalChildren)
}

func TestBulkForceUnvalidateMBHead_EmptyMbhIds_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkForceUnvalidateMBHead(context.Background(), &financev1.BulkForceUnvalidateMBHeadRequest{
		MbhIds: nil,
	})
	require.NoError(t, err, "⛔ BaseResponse pattern: the error travels in the body, not the transport")
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Nil(t, resp.Data)
}

func TestBulkForceUnvalidateMBHead_QueueUnavailable_Returns503(t *testing.T) {
	// bulkTransitionHandler was never wired via WithBulkTransition.
	h, err := NewMBHeadHandler(&fakeFrozenMBHeadRepo{}, nil, fakeMBMachineRepo{})
	require.NoError(t, err)

	resp, callErr := h.BulkForceUnvalidateMBHead(context.Background(), &financev1.BulkForceUnvalidateMBHeadRequest{
		MbhIds: []string{uuid.NewString()},
	})
	require.NoError(t, callErr)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "503", resp.Base.StatusCode)
}

// TestBulkForceUnvalidateMBHead_PartialPublishFailure_StillReturnsSuccessWithJobInfo
// proves the bug fix at the delivery layer end-to-end: when some children fail
// to publish to RabbitMQ, the parent job.Execution and every child (including
// the ones that DID publish) are still persisted, and the RPC must report
// success (base.isSuccess=true) with the job_id and an accurate
// failed_children count — not fold the partial failure into base.isSuccess=false
// with no job_id, which would strand the caller with no way to discover the
// job actually exists and is partially in-flight.
func TestBulkForceUnvalidateMBHead_PartialPublishFailure_StillReturnsSuccessWithJobInfo(t *testing.T) {
	failingID := uuid.NewString()
	okID := uuid.NewString()
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{failFor: map[string]bool{failingID: true}})

	resp, err := h.BulkForceUnvalidateMBHead(context.Background(), &financev1.BulkForceUnvalidateMBHeadRequest{
		MbhIds: []string{okID, failingID},
		Reason: "bulk regenerate",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess, "a per-child publish failure must not fail the whole RPC")
	require.NotNil(t, resp.Data)
	assert.NotEmpty(t, resp.Data.JobId, "the job was created and must be discoverable by the caller")
	assert.Equal(t, int32(2), resp.Data.TotalChildren)
	assert.Equal(t, int32(1), resp.Data.FailedChildren)
}

func TestBulkSubmitMBHead_Success_QueuesJob(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkSubmitMBHead(context.Background(), &financev1.BulkSubmitMBHeadRequest{
		MbhIds: []string{uuid.NewString()},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, resp.Data)
	assert.Equal(t, int32(1), resp.Data.TotalChildren)
}

func TestBulkSubmitMBHead_EmptyMbhIds_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkSubmitMBHead(context.Background(), &financev1.BulkSubmitMBHeadRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
}

func TestBulkValidateMBHead_Success_QueuesJob(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkValidateMBHead(context.Background(), &financev1.BulkValidateMBHeadRequest{
		MbhIds: []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, resp.Data)
	assert.Equal(t, int32(3), resp.Data.TotalChildren)
}

func TestBulkValidateMBHead_EmptyMbhIds_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.BulkValidateMBHead(context.Background(), &financev1.BulkValidateMBHeadRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
}

// --- GetBulkMBHeadJobStatus -------------------------------------------------------

func TestGetBulkMBHeadJobStatus_Success_ReportsParentCounters(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	queued, err := h.BulkSubmitMBHead(context.Background(), &financev1.BulkSubmitMBHeadRequest{
		MbhIds: []string{uuid.NewString(), uuid.NewString()},
	})
	require.NoError(t, err)
	require.True(t, queued.Base.IsSuccess)
	jobID := queued.Data.JobId

	resp, err := h.GetBulkMBHeadJobStatus(context.Background(), &financev1.GetBulkMBHeadJobStatusRequest{JobId: jobID})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, jobID, resp.JobId)
	assert.Equal(t, int32(2), resp.TotalChildren)
}

func TestGetBulkMBHeadJobStatus_InvalidJobID_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.GetBulkMBHeadJobStatus(context.Background(), &financev1.GetBulkMBHeadJobStatusRequest{JobId: "not-a-uuid"})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "400", resp.Base.StatusCode)
}

func TestGetBulkMBHeadJobStatus_UnknownJobID_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.GetBulkMBHeadJobStatus(context.Background(), &financev1.GetBulkMBHeadJobStatusRequest{JobId: uuid.NewString()})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
}

// --- ListBulkMBHeadJobFailures -----------------------------------------------------

func TestListBulkMBHeadJobFailures_InvalidJobID_ReturnsFailureBase(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	resp, err := h.ListBulkMBHeadJobFailures(context.Background(), &financev1.ListBulkMBHeadJobFailuresRequest{JobId: "not-a-uuid"})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "400", resp.Base.StatusCode)
}

func TestListBulkMBHeadJobFailures_NoFailures_ReturnsEmptyList(t *testing.T) {
	h, _, _ := newBulkTestHandler(t, &fakeBulkPublisher{})

	queued, err := h.BulkSubmitMBHead(context.Background(), &financev1.BulkSubmitMBHeadRequest{
		MbhIds: []string{uuid.NewString()},
	})
	require.NoError(t, err)
	require.True(t, queued.Base.IsSuccess)

	resp, err := h.ListBulkMBHeadJobFailures(context.Background(), &financev1.ListBulkMBHeadJobFailuresRequest{JobId: queued.Data.JobId})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	assert.Empty(t, resp.Failures)
}
