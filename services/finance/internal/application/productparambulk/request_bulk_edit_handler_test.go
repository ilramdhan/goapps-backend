package productparambulk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/productparambulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// jobRepoMock is a small testify mock for job.Repository — only the methods
// exercised by RequestBulkEditHandler are stubbed, mirroring
// mbheadbulk.jobRepoMock (see request_bulk_transition_handler_test.go).
type jobRepoMock struct{ mock.Mock }

func (m *jobRepoMock) Create(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *jobRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *jobRepoMock) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	args := m.Called(ctx, code)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *jobRepoMock) List(ctx context.Context, f job.ListFilter) ([]*job.Execution, int64, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*job.Execution), args.Get(1).(int64), args.Error(2)
}

func (m *jobRepoMock) UpdateStatus(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *jobRepoMock) UpdateProgress(ctx context.Context, id uuid.UUID, p int) error {
	return m.Called(ctx, id, p).Error(0)
}

func (m *jobRepoMock) AddLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *jobRepoMock) UpdateLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *jobRepoMock) HasActiveJob(ctx context.Context, t job.Type, p string) (bool, error) {
	args := m.Called(ctx, t, p)
	return args.Bool(0), args.Error(1)
}

func (m *jobRepoMock) GetNextSequence(ctx context.Context, t job.Type, p string) (int, error) {
	args := m.Called(ctx, t, p)
	return args.Int(0), args.Error(1)
}

func (m *jobRepoMock) CreateChildren(ctx context.Context, execs []*job.Execution) error {
	return m.Called(ctx, execs).Error(0)
}

func (m *jobRepoMock) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	args := m.Called(ctx, parentJobID, success)
	return args.Bool(0), args.Error(1)
}

func (m *jobRepoMock) ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	args := m.Called(ctx, parentJobID)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Error(1)
}

// publisherMock is a testify mock for productparambulk.BulkEditJobPublisher.
// failFor, when non-nil, marks a subset of productSysIDs whose publish call
// must fail — everything else succeeds.
type publisherMock struct {
	mock.Mock
	failFor map[int64]bool
}

func (m *publisherMock) PublishProductParamBulk(
	ctx context.Context, jobID string, productSysID int64, operationsJSON string, skipMissingApplicable bool, createdBy string,
) error {
	m.Called(ctx, jobID, productSysID, operationsJSON, skipMissingApplicable, createdBy)
	if m.failFor != nil && m.failFor[productSysID] {
		return errors.New("publish failed for product")
	}
	return nil
}

// productCheckerStub is a minimal stand-in for productparambulk.ProductChecker.
type productCheckerStub struct {
	missing map[int64]bool
	err     error
}

func (s *productCheckerStub) ProductExists(_ context.Context, productSysID int64) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if s.missing != nil && s.missing[productSysID] {
		return false, nil
	}
	return true, nil
}

func validOp() productparambulk.OperationDTO {
	return productparambulk.OperationDTO{
		Kind:    productparambulk.OpAddApplicable,
		ParamID: uuid.NewString(),
	}
}

func TestRequestBulkEditHandler_Validate(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}

	t.Run("publisher nil", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, nil, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1}, Operations: []productparambulk.OperationDTO{validOp()}, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrPublisherUnavailable)
	})

	t.Run("empty product_sys_ids", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			Operations: []productparambulk.OperationDTO{validOp()}, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrNoProducts)
	})

	t.Run("too many products", func(t *testing.T) {
		ids := make([]int64, 501)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: ids, Operations: []productparambulk.OperationDTO{validOp()}, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrTooManyProducts)
	})

	t.Run("empty operations", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1}, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrNoOperations)
	})

	t.Run("missing created by", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1}, Operations: []productparambulk.OperationDTO{validOp()},
		})
		require.Error(t, err)
	})

	t.Run("invalid param_id", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1},
			Operations:    []productparambulk.OperationDTO{{Kind: productparambulk.OpAddApplicable, ParamID: "not-a-uuid"}},
			CreatedBy:     "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrInvalidOperation)
	})

	t.Run("upsert_value wrong shape (zero fields set)", func(t *testing.T) {
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1},
			Operations:    []productparambulk.OperationDTO{{Kind: productparambulk.OpUpsertValue, ParamID: uuid.NewString()}},
			CreatedBy:     "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrInvalidOperation)
	})

	t.Run("upsert_value wrong shape (two fields set)", func(t *testing.T) {
		numeric := "1.5"
		text := "x"
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1},
			Operations: []productparambulk.OperationDTO{{
				Kind: productparambulk.OpUpsertValue, ParamID: uuid.NewString(), ValueNumeric: &numeric, ValueText: &text,
			}},
			CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrInvalidOperation)
	})

	t.Run("unknown product rejects whole batch", func(t *testing.T) {
		products := &productCheckerStub{missing: map[int64]bool{2: true}}
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, products)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1, 2}, Operations: []productparambulk.OperationDTO{validOp()}, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, productparambulk.ErrProductNotFound)
	})

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestRequestBulkEditHandler_OneChildPerProduct_NoChunking proves the parent
// job's total_children matches len(ProductSysIDs) exactly and exactly one
// child is created per product_sys_id.
func TestRequestBulkEditHandler_OneChildPerProduct_NoChunking(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	productSysIDs := []int64{101, 102, 103}
	ops := []productparambulk.OperationDTO{validOp()}

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == len(productSysIDs)
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		return len(execs) == len(productSysIDs)
	})).Return(nil).Once()
	for _, id := range productSysIDs {
		pub.On("PublishProductParamBulk", mock.Anything, mock.Anything, id, mock.Anything, false, "admin").Return(nil).Once()
	}

	h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
	res, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
		ProductSysIDs: productSysIDs,
		Operations:    ops,
		CreatedBy:     "admin",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Execution.IsParent())
	assert.Equal(t, len(productSysIDs), res.Execution.TotalChildren())

	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkEditHandler_PerChildPublishFailure_DoesNotAbortBatch proves a
// publish failure on a SUBSET of children is recorded on those children only
// (failJob + IncrementChildProgress(success=false)) and does not fail Handle
// itself nor stop the remaining children from being published — mirroring
// mbheadbulk's precedent test exactly (this is the "partial failure" scenario
// from design.md §4.5).
func TestRequestBulkEditHandler_PerChildPublishFailure_DoesNotAbortBatch(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	productSysIDs := []int64{1, 2, 3, 4}
	pub := &publisherMock{failFor: map[int64]bool{2: true, 4: true}}
	ops := []productparambulk.OperationDTO{validOp()}

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == len(productSysIDs)
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		return len(execs) == len(productSysIDs)
	})).Return(nil).Once()
	for _, id := range productSysIDs {
		pub.On("PublishProductParamBulk", mock.Anything, mock.Anything, id, mock.Anything, false, "admin").Return(nil).Once()
	}
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Twice()
	repo.On("IncrementChildProgress", mock.Anything, mock.Anything, false).Return(false, nil).Twice()

	refreshedParent, err := job.NewParentExecution(job.TypeProductParamBulk, "", "", "admin", 5, nil, len(productSysIDs))
	require.NoError(t, err)
	refreshedParent.IncrementFailedChildren()
	refreshedParent.IncrementFailedChildren()
	repo.On("GetByID", mock.Anything, mock.Anything).Return(refreshedParent, nil).Once()

	h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
	res, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
		ProductSysIDs: productSysIDs,
		Operations:    ops,
		CreatedBy:     "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Execution)
	assert.Equal(t, 2, res.Execution.FailedChildren())
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkEditHandler_SkipMissingApplicable_PropagatedToPublisher
// proves cmd.SkipMissingApplicable is forwarded verbatim to every child's
// publish call, so the worker can honor "skip + report" per design.md §4.5.
func TestRequestBulkEditHandler_SkipMissingApplicable_PropagatedToPublisher(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	productSysIDs := []int64{1, 2}
	ops := []productparambulk.OperationDTO{validOp()}

	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.Anything).Return(nil).Once()
	for _, id := range productSysIDs {
		pub.On("PublishProductParamBulk", mock.Anything, mock.Anything, id, mock.Anything, true, "admin").Return(nil).Once()
	}

	h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil)
	_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
		ProductSysIDs:         productSysIDs,
		Operations:            ops,
		SkipMissingApplicable: true,
		CreatedBy:             "admin",
	})
	require.NoError(t, err)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}
