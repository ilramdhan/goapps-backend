// Package prdrequest_test contains application-layer handler tests for the ProductRequest aggregate.
package prdrequest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/prdrequest"
	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// =============================================================================
// Mock: prdrequest.Repository
// =============================================================================

// MockRepository is a mock implementation of domainprdrequest.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, r *domainprdrequest.Request) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainprdrequest.Request, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainprdrequest.Request), args.Error(1)
}

func (m *MockRepository) GetByTicketNo(ctx context.Context, ticketNo string) (*domainprdrequest.Request, error) {
	args := m.Called(ctx, ticketNo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainprdrequest.Request), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, f domainprdrequest.ListFilter) ([]*domainprdrequest.Request, int, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*domainprdrequest.Request), args.Int(1), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, r *domainprdrequest.Request) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

// =============================================================================
// Mock: prdrequest.TicketNoGenerator
// =============================================================================

// MockTicketNoGenerator is a mock implementation of domainprdrequest.TicketNoGenerator.
type MockTicketNoGenerator struct {
	mock.Mock
}

func (m *MockTicketNoGenerator) Next(ctx context.Context, period string) (domainprdrequest.TicketNo, error) {
	args := m.Called(ctx, period)
	return args.Get(0).(domainprdrequest.TicketNo), args.Error(1)
}

// =============================================================================
// Mock: product.Repository (cross-aggregate read)
// =============================================================================

// MockProductRepository is a mock implementation of domainproduct.Repository.
type MockProductRepository struct {
	mock.Mock
}

func (m *MockProductRepository) Create(ctx context.Context, p *domainproduct.Product) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProductRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainproduct.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainproduct.Product), args.Error(1)
}

func (m *MockProductRepository) GetByCode(ctx context.Context, code string) (*domainproduct.Product, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainproduct.Product), args.Error(1)
}

func (m *MockProductRepository) List(ctx context.Context, f domainproduct.ListFilter) ([]*domainproduct.Product, int, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*domainproduct.Product), args.Int(1), args.Error(2)
}

func (m *MockProductRepository) Update(ctx context.Context, p *domainproduct.Product) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProductRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockProductRepository) SearchByText(ctx context.Context, opts domainproduct.SearchOptions) ([]*domainproduct.Product, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).([]*domainproduct.Product), args.Error(1)
}

func (m *MockProductRepository) ListByRequestID(ctx context.Context, requestID uuid.UUID, page, pageSize int) ([]*domainproduct.Product, int, error) {
	args := m.Called(ctx, requestID, page, pageSize)
	return args.Get(0).([]*domainproduct.Product), args.Int(1), args.Error(2)
}

// =============================================================================
// Helpers
// =============================================================================

// newValidTicketNo returns a TicketNo value object for test use.
func newValidTicketNo(t *testing.T) domainprdrequest.TicketNo {
	t.Helper()
	tn, err := domainprdrequest.NewTicketNo("PR-202504-001")
	require.NoError(t, err)
	return tn
}

// newOpenRequest builds a valid OPEN product request for use in tests.
func newOpenRequest(t *testing.T) *domainprdrequest.Request {
	t.Helper()
	tn := newValidTicketNo(t)
	r, err := domainprdrequest.NewRequest(
		tn,
		uuid.New(),
		"requester.user",
		uuid.New(),
		"DEPT-A",
		"A valid request title",
		"Some description of the request.",
		"",
		nil,
		"admin",
	)
	require.NoError(t, err)
	return r
}

// newResolvedRequest builds a RESOLVED product request via ReconstructRequest.
func newResolvedRequest(t *testing.T) *domainprdrequest.Request {
	t.Helper()
	now := time.Now().UTC()
	return domainprdrequest.ReconstructRequest(
		uuid.New(),
		"PR-202504-001",
		uuid.New(), "requester.user",
		uuid.New(), "DEPT-A",
		"A valid request title", "Some description.", "",
		"RESOLVED",
		uuid.New(),
		"Linked to existing product.",
		"",
		uuid.New(),
		nil,
		time.Now().UTC(), "admin",
		&now, "admin",
		nil, "",
	)
}

// newRejectedRequest builds a REJECTED product request via ReconstructRequest.
func newRejectedRequest(t *testing.T) *domainprdrequest.Request {
	t.Helper()
	now := time.Now().UTC()
	return domainprdrequest.ReconstructRequest(
		uuid.New(),
		"PR-202504-002",
		uuid.New(), "requester.user",
		uuid.New(), "DEPT-A",
		"A valid request title", "Some description.", "",
		"REJECTED",
		uuid.Nil,
		"",
		"Not feasible at this time.",
		uuid.Nil,
		nil,
		time.Now().UTC(), "admin",
		&now, "admin",
		nil, "",
	)
}

// newValidProduct builds a valid Product for use in search tests.
func newValidProduct(t *testing.T) *domainproduct.Product {
	t.Helper()
	p, err := domainproduct.NewProduct(
		"PROD-001", "Test Product", "ITEM-001",
		"SC01", "Shade One",
		uuid.New(), "DEPT-A",
		"COMMERCIAL",
		uuid.Nil,
		"admin",
	)
	require.NoError(t, err)
	return p
}

// =============================================================================
// CreateHandler tests
// =============================================================================

func TestCreateHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGen := new(MockTicketNoGenerator)
	handler := prdrequest.NewCreateHandler(mockRepo, mockGen)
	ctx := context.Background()

	tn := newValidTicketNo(t)
	mockGen.On("Next", ctx, mock.AnythingOfType("string")).Return(tn, nil)
	mockRepo.On("Create", ctx, mock.AnythingOfType("*prdrequest.Request")).Return(nil)

	cmd := prdrequest.CreateCommand{
		RequesterID:       uuid.New(),
		RequesterUsername: "requester.user",
		RequesterDeptID:   uuid.New(),
		RequesterDeptCode: "DEPT-A",
		Title:             "A valid request title",
		Description:       "Some description of the request.",
		TargetSpecsJSON:   "",
		CreatedBy:         "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "PR-202504-001", result.TicketNo().String())
	assert.NotEqual(t, uuid.Nil, result.ID())
	mockGen.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestCreateHandler_Handle_GeneratorError(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGen := new(MockTicketNoGenerator)
	handler := prdrequest.NewCreateHandler(mockRepo, mockGen)
	ctx := context.Background()

	genErr := errors.New("sequence table locked")
	mockGen.On("Next", ctx, mock.AnythingOfType("string")).Return(domainprdrequest.TicketNo{}, genErr)

	cmd := prdrequest.CreateCommand{
		RequesterID:       uuid.New(),
		RequesterUsername: "requester.user",
		RequesterDeptID:   uuid.New(),
		RequesterDeptCode: "DEPT-A",
		Title:             "A valid request title",
		Description:       "",
		CreatedBy:         "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, genErr)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreateHandler_Handle_DomainValidationError(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGen := new(MockTicketNoGenerator)
	handler := prdrequest.NewCreateHandler(mockRepo, mockGen)
	ctx := context.Background()

	tn := newValidTicketNo(t)
	mockGen.On("Next", ctx, mock.AnythingOfType("string")).Return(tn, nil)

	cmd := prdrequest.CreateCommand{
		RequesterID:       uuid.New(),
		RequesterUsername: "requester.user",
		RequesterDeptID:   uuid.New(),
		RequesterDeptCode: "DEPT-A",
		Title:             "ab", // too short — min 3 trimmed chars
		Description:       "",
		CreatedBy:         "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrInvalidTitle)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// =============================================================================
// GetHandler tests
// =============================================================================

func TestGetHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewGetHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)

	result, err := handler.Handle(ctx, prdrequest.GetCommand{ID: r.ID()})

	require.NoError(t, err)
	assert.Equal(t, r.ID(), result.ID())
	mockRepo.AssertExpectations(t)
}

func TestGetHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewGetHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("GetByID", ctx, id).Return(nil, domainprdrequest.ErrNotFound)

	result, err := handler.Handle(ctx, prdrequest.GetCommand{ID: id})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrNotFound)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// ListHandler tests
// =============================================================================

func TestListHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewListHandler(mockRepo)
	ctx := context.Background()

	r1 := newOpenRequest(t)
	r2 := newOpenRequest(t)

	mockRepo.On("List", ctx, mock.AnythingOfType("prdrequest.ListFilter")).
		Return([]*domainprdrequest.Request{r1, r2}, 2, nil)

	q := prdrequest.ListQuery{Page: 1, PageSize: 10}
	result, err := handler.Handle(ctx, q)

	require.NoError(t, err)
	assert.Len(t, result.Requests, 2)
	assert.Equal(t, 2, result.TotalItems)
	assert.Equal(t, int32(1), result.TotalPages)
	assert.Equal(t, int32(1), result.CurrentPage)
	assert.Equal(t, int32(10), result.PageSize)
	mockRepo.AssertExpectations(t)
}

func TestListHandler_Handle_RepoError(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewListHandler(mockRepo)
	ctx := context.Background()

	repoErr := errors.New("database error")
	mockRepo.On("List", ctx, mock.AnythingOfType("prdrequest.ListFilter")).
		Return([]*domainprdrequest.Request{}, 0, repoErr)

	result, err := handler.Handle(ctx, prdrequest.ListQuery{Page: 1, PageSize: 10})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// UpdateHandler tests
// =============================================================================

func TestUpdateHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*prdrequest.Request")).Return(nil)

	cmd := prdrequest.UpdateCommand{
		ID:          r.ID(),
		Title:       "Updated request title",
		Description: "Updated description of the request.",
		UpdatedBy:   "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Updated request title", result.Title().String())
	mockRepo.AssertExpectations(t)
}

func TestUpdateHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("GetByID", ctx, id).Return(nil, domainprdrequest.ErrNotFound)

	result, err := handler.Handle(ctx, prdrequest.UpdateCommand{
		ID:        id,
		Title:     "Some title here",
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrNotFound)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestUpdateHandler_Handle_TerminalStatusReturnsError(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	resolved := newResolvedRequest(t)
	mockRepo.On("GetByID", ctx, resolved.ID()).Return(resolved, nil)

	cmd := prdrequest.UpdateCommand{
		ID:        resolved.ID(),
		Title:     "Attempt to update resolved",
		UpdatedBy: "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrInvalidTransition)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// =============================================================================
// DeleteHandler tests
// =============================================================================

func TestDeleteHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewDeleteHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("Delete", ctx, id, "admin").Return(nil)

	err := handler.Handle(ctx, prdrequest.DeleteCommand{ID: id, DeletedBy: "admin"})

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestDeleteHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewDeleteHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("Delete", ctx, id, "admin").Return(domainprdrequest.ErrNotFound)

	err := handler.Handle(ctx, prdrequest.DeleteCommand{ID: id, DeletedBy: "admin"})

	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrNotFound)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// AssignHandler tests
// =============================================================================

func TestAssignHandler_Handle_Success_OpenToInReview(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewAssignHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	assigneeID := uuid.New()

	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*prdrequest.Request")).Return(nil)

	result, err := handler.Handle(ctx, prdrequest.AssignCommand{
		ID:         r.ID(),
		AssigneeID: assigneeID,
		UpdatedBy:  "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, assigneeID, result.AssignedTo())
	assert.Equal(t, domainprdrequest.StatusInReview, result.Status())
	mockRepo.AssertExpectations(t)
}

func TestAssignHandler_Handle_TerminalStatusReturnsErrCannotAssign(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewAssignHandler(mockRepo)
	ctx := context.Background()

	resolved := newResolvedRequest(t)
	mockRepo.On("GetByID", ctx, resolved.ID()).Return(resolved, nil)

	result, err := handler.Handle(ctx, prdrequest.AssignCommand{
		ID:         resolved.ID(),
		AssigneeID: uuid.New(),
		UpdatedBy:  "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrCannotAssign)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestAssignHandler_Handle_ZeroAssigneeReturnsErrInvalidAssignee(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewAssignHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)

	result, err := handler.Handle(ctx, prdrequest.AssignCommand{
		ID:         r.ID(),
		AssigneeID: uuid.Nil,
		UpdatedBy:  "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrInvalidAssignee)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// =============================================================================
// ResolveHandler tests
// =============================================================================

func TestResolveHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewResolveHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	productID := uuid.New()

	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*prdrequest.Request")).Return(nil)

	result, err := handler.Handle(ctx, prdrequest.ResolveCommand{
		ID:             r.ID(),
		ProductID:      productID,
		ResolutionNote: "Matched existing product.",
		UpdatedBy:      "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domainprdrequest.StatusResolved, result.Status())
	assert.Equal(t, productID, result.ResolvedProductID())
	mockRepo.AssertExpectations(t)
}

func TestResolveHandler_Handle_AlreadyResolvedFails(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewResolveHandler(mockRepo)
	ctx := context.Background()

	resolved := newResolvedRequest(t)
	mockRepo.On("GetByID", ctx, resolved.ID()).Return(resolved, nil)

	result, err := handler.Handle(ctx, prdrequest.ResolveCommand{
		ID:        resolved.ID(),
		ProductID: uuid.New(),
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrAlreadyResolved)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestResolveHandler_Handle_OnRejectedFails(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewResolveHandler(mockRepo)
	ctx := context.Background()

	rejected := newRejectedRequest(t)
	mockRepo.On("GetByID", ctx, rejected.ID()).Return(rejected, nil)

	result, err := handler.Handle(ctx, prdrequest.ResolveCommand{
		ID:        rejected.ID(),
		ProductID: uuid.New(),
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrAlreadyRejected)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestResolveHandler_Handle_ZeroProductIDFails(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewResolveHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)

	result, err := handler.Handle(ctx, prdrequest.ResolveCommand{
		ID:        r.ID(),
		ProductID: uuid.Nil,
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrInvalidProductLink)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// =============================================================================
// RejectHandler tests
// =============================================================================

func TestRejectHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewRejectHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*prdrequest.Request")).Return(nil)

	result, err := handler.Handle(ctx, prdrequest.RejectCommand{
		ID:        r.ID(),
		Reason:    "Not feasible at this time due to capacity constraints.",
		UpdatedBy: "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domainprdrequest.StatusRejected, result.Status())
	mockRepo.AssertExpectations(t)
}

func TestRejectHandler_Handle_ShortReasonFails(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewRejectHandler(mockRepo)
	ctx := context.Background()

	r := newOpenRequest(t)
	mockRepo.On("GetByID", ctx, r.ID()).Return(r, nil)

	result, err := handler.Handle(ctx, prdrequest.RejectCommand{
		ID:        r.ID(),
		Reason:    "No",  // too short — min 5 trimmed chars
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrInvalidRejectReason)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestRejectHandler_Handle_AlreadyRejectedFails(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := prdrequest.NewRejectHandler(mockRepo)
	ctx := context.Background()

	rejected := newRejectedRequest(t)
	mockRepo.On("GetByID", ctx, rejected.ID()).Return(rejected, nil)

	result, err := handler.Handle(ctx, prdrequest.RejectCommand{
		ID:        rejected.ID(),
		Reason:    "Rejecting an already-rejected ticket.",
		UpdatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainprdrequest.ErrAlreadyRejected)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// =============================================================================
// SearchExistingHandler tests
// =============================================================================

func TestSearchExistingHandler_Handle_Success(t *testing.T) {
	mockProducts := new(MockProductRepository)
	handler := prdrequest.NewSearchExistingHandler(mockProducts)
	ctx := context.Background()

	p := newValidProduct(t)
	opts := domainproduct.SearchOptions{
		Query:     "blue shade",
		ShadeCode: "SC01",
		Limit:     10,
	}

	mockProducts.On("SearchByText", ctx, opts).Return([]*domainproduct.Product{p}, nil)

	results, err := handler.Handle(ctx, prdrequest.SearchExistingCommand{
		Query:     "blue shade",
		ShadeCode: "SC01",
		Limit:     10,
	})

	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, p.ID(), results[0].ID())
	mockProducts.AssertExpectations(t)
}

func TestSearchExistingHandler_Handle_RepoError(t *testing.T) {
	mockProducts := new(MockProductRepository)
	handler := prdrequest.NewSearchExistingHandler(mockProducts)
	ctx := context.Background()

	repoErr := errors.New("fts index unavailable")
	mockProducts.On("SearchByText", ctx, mock.AnythingOfType("product.SearchOptions")).
		Return(([]*domainproduct.Product)(nil), repoErr)

	results, err := handler.Handle(ctx, prdrequest.SearchExistingCommand{
		Query: "test",
		Limit: 5,
	})

	assert.Nil(t, results)
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr)
	mockProducts.AssertExpectations(t)
}
