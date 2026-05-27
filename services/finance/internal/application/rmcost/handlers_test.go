package rmcost_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

func newGroupHead(t *testing.T) *rmgroup.Head {
	t.Helper()
	code, err := rmgroup.NewCode("GRP-TEST")
	require.NoError(t, err)
	head, err := rmgroup.NewHead(code, "Test Group", "", 1.0, 10.0, "user:test")
	require.NoError(t, err)
	return head
}

// --- TriggerHandler ---

func TestTriggerHandler_Success(t *testing.T) {
	ctx := context.Background()
	jobRepo := new(mockJobRepo)
	pub := new(mockPublisher)

	jobRepo.On("HasActiveJob", ctx, job.TypeRMCostCalculation, "202604").Return(false, nil)
	jobRepo.On("Create", ctx, mock.AnythingOfType("*job.Execution")).Return(nil)
	pub.On("PublishRMCostCalculation", ctx,
		mock.AnythingOfType("string"), "202604",
		(*uuid.UUID)(nil), "manual-ui", "user:x").Return(nil)

	h := appcost.NewTriggerHandler(jobRepo, pub)
	res, err := h.Handle(ctx, appcost.TriggerCommand{
		Period: "202604", CreatedBy: "user:x",
	})
	require.NoError(t, err)
	assert.NotNil(t, res.Execution)
}

func TestTriggerHandler_NilPublisher(t *testing.T) {
	jobRepo := new(mockJobRepo)
	h := appcost.NewTriggerHandler(jobRepo, nil)
	_, err := h.Handle(context.Background(), appcost.TriggerCommand{Period: "202604", CreatedBy: "u"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RabbitMQ not connected")
}

func TestTriggerHandler_DuplicateActive(t *testing.T) {
	ctx := context.Background()
	jobRepo := new(mockJobRepo)
	pub := new(mockPublisher)
	jobRepo.On("HasActiveJob", ctx, job.TypeRMCostCalculation, "202604").Return(true, nil)

	h := appcost.NewTriggerHandler(jobRepo, pub)
	_, err := h.Handle(ctx, appcost.TriggerCommand{Period: "202604", CreatedBy: "u"})
	assert.ErrorIs(t, err, job.ErrDuplicateActiveJob)
}

func TestTriggerHandler_PublishFailureMarksFailed(t *testing.T) {
	ctx := context.Background()
	jobRepo := new(mockJobRepo)
	pub := new(mockPublisher)
	jobRepo.On("HasActiveJob", ctx, job.TypeRMCostCalculation, "202604").Return(false, nil)
	jobRepo.On("Create", ctx, mock.AnythingOfType("*job.Execution")).Return(nil)
	pub.On("PublishRMCostCalculation", ctx,
		mock.AnythingOfType("string"), "202604",
		(*uuid.UUID)(nil), "manual-ui", "u").Return(errors.New("amqp closed"))
	jobRepo.On("UpdateStatus", ctx, mock.AnythingOfType("*job.Execution")).Return(nil)

	h := appcost.NewTriggerHandler(jobRepo, pub)
	_, err := h.Handle(ctx, appcost.TriggerCommand{Period: "202604", CreatedBy: "u"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish job")
	jobRepo.AssertCalled(t, "UpdateStatus", ctx, mock.AnythingOfType("*job.Execution"))
}

func TestTriggerHandler_EmptyCreatedBy(t *testing.T) {
	pub := new(mockPublisher)
	h := appcost.NewTriggerHandler(new(mockJobRepo), pub)
	_, err := h.Handle(context.Background(), appcost.TriggerCommand{Period: "202604"})
	assert.ErrorIs(t, err, rmcost.ErrEmptyCreatedBy)
}

// --- CalculateHandler ---

func TestCalculateHandler_SingleHead_EmptyDetails(t *testing.T) {
	ctx := context.Background()
	head := newGroupHead(t)
	id := head.ID()

	groupRepo := new(mockGroupRepo)
	costRepo := new(mockCostRepo)
	src := new(mockSourceReader)

	groupRepo.On("GetHeadByID", ctx, id).Return(head, nil)
	groupRepo.On("ListActiveDetailsByHeadID", ctx, id).Return([]*rmgroup.Detail{}, nil)
	costRepo.On("GetByPeriodAndCode", ctx, "202604", head.Code().String()).
		Return(nil, rmcost.ErrNotFound)
	costRepo.On("Upsert", ctx, mock.AnythingOfType("*rmcost.Cost"), mock.AnythingOfType("rmcost.History")).
		Return(nil)

	h := appcost.NewCalculateHandler(groupRepo, costRepo, src)
	res, err := h.Handle(ctx, appcost.CalculateCommand{
		Period: "202604", GroupHeadID: &id, CalculatedBy: "user:calc",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Processed)
	assert.Len(t, res.Costs, 1)
	// Empty details => no source fetch, all-zero rates => cost == costPerKg (10).
	assert.Equal(t, 10.0, *res.Costs[0].CostValuation())
	src.AssertNotCalled(t, "FetchRateInputs")
}

func TestCalculateHandler_InvalidPeriod(t *testing.T) {
	h := appcost.NewCalculateHandler(new(mockGroupRepo), new(mockCostRepo), new(mockSourceReader))
	_, err := h.Handle(context.Background(), appcost.CalculateCommand{
		Period: "bad", CalculatedBy: "u",
	})
	assert.ErrorIs(t, err, rmcost.ErrInvalidPeriod)
}

func TestCalculateHandler_EmptyCalculatedBy(t *testing.T) {
	h := appcost.NewCalculateHandler(new(mockGroupRepo), new(mockCostRepo), new(mockSourceReader))
	_, err := h.Handle(context.Background(), appcost.CalculateCommand{Period: "202604"})
	assert.ErrorIs(t, err, rmcost.ErrEmptyCalculatedBy)
}

// --- GetHandler ---

func TestGetHandler_ByID(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := new(mockCostRepo)
	repo.On("GetByID", ctx, id).Return((*rmcost.Cost)(nil), rmcost.ErrNotFound)

	h := appcost.NewGetHandler(repo)
	_, err := h.Handle(ctx, appcost.GetQuery{CostID: id.String()})
	assert.ErrorIs(t, err, rmcost.ErrNotFound)
}

func TestGetHandler_InvalidIDReturnsNotFound(t *testing.T) {
	h := appcost.NewGetHandler(new(mockCostRepo))
	_, err := h.Handle(context.Background(), appcost.GetQuery{CostID: "not-a-uuid"})
	assert.ErrorIs(t, err, rmcost.ErrNotFound)
}

func TestGetHandler_ByPeriodAndCode(t *testing.T) {
	ctx := context.Background()
	repo := new(mockCostRepo)
	repo.On("GetByPeriodAndCode", ctx, "202604", "GRP-1").Return((*rmcost.Cost)(nil), rmcost.ErrNotFound)

	h := appcost.NewGetHandler(repo)
	_, err := h.Handle(ctx, appcost.GetQuery{Period: "202604", RMCode: "GRP-1"})
	assert.ErrorIs(t, err, rmcost.ErrNotFound)
}

// --- ListHandler ---

func TestListHandler_Pagination(t *testing.T) {
	ctx := context.Background()
	repo := new(mockCostRepo)
	repo.On("List", ctx, mock.AnythingOfType("rmcost.ListFilter")).
		Return([]*rmcost.Cost{}, int64(23), nil)

	h := appcost.NewListHandler(repo)
	res, err := h.Handle(ctx, appcost.ListQuery{Page: 1, PageSize: 10, Period: "202604"})
	require.NoError(t, err)
	assert.Equal(t, int64(23), res.TotalItems)
	assert.Equal(t, int32(3), res.TotalPages)
}

func TestListHandler_InvalidRMType(t *testing.T) {
	h := appcost.NewListHandler(new(mockCostRepo))
	_, err := h.Handle(context.Background(), appcost.ListQuery{RMType: "BOGUS"})
	assert.ErrorIs(t, err, rmcost.ErrInvalidRMType)
}

func TestListHandler_InvalidGroupHeadID(t *testing.T) {
	h := appcost.NewListHandler(new(mockCostRepo))
	_, err := h.Handle(context.Background(), appcost.ListQuery{GroupHeadID: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid group head id")
}

// --- HistoryHandler ---

func TestHistoryHandler_Pagination(t *testing.T) {
	ctx := context.Background()
	repo := new(mockCostRepo)
	repo.On("ListHistory", ctx, mock.AnythingOfType("rmcost.HistoryFilter")).
		Return([]rmcost.History{{}}, int64(45), nil)

	h := appcost.NewHistoryHandler(repo)
	res, err := h.Handle(ctx, appcost.HistoryQuery{Page: 2, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(45), res.TotalItems)
	assert.Equal(t, int32(3), res.TotalPages)
	assert.Equal(t, int32(2), res.CurrentPage)
}

func TestHistoryHandler_InvalidGroupHeadID(t *testing.T) {
	h := appcost.NewHistoryHandler(new(mockCostRepo))
	_, err := h.Handle(context.Background(), appcost.HistoryQuery{GroupHeadID: "bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid group head id")
}

func TestHistoryHandler_InvalidJobID(t *testing.T) {
	h := appcost.NewHistoryHandler(new(mockCostRepo))
	_, err := h.Handle(context.Background(), appcost.HistoryQuery{JobID: "bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid job id")
}
