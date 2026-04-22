package rmcost_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// mockCostRepo implements rmcost.Repository.
type mockCostRepo struct{ mock.Mock }

func (m *mockCostRepo) Upsert(ctx context.Context, cost *rmcost.Cost, hist rmcost.History) error {
	return m.Called(ctx, cost, hist).Error(0)
}

func (m *mockCostRepo) GetByID(ctx context.Context, id uuid.UUID) (*rmcost.Cost, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmcost.Cost), args.Error(1)
}

func (m *mockCostRepo) GetByPeriodAndCode(ctx context.Context, period, rmCode string) (*rmcost.Cost, error) {
	args := m.Called(ctx, period, rmCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmcost.Cost), args.Error(1)
}

func (m *mockCostRepo) List(ctx context.Context, filter rmcost.ListFilter) ([]*rmcost.Cost, int64, error) {
	args := m.Called(ctx, filter)
	var out []*rmcost.Cost
	if v := args.Get(0); v != nil {
		out = v.([]*rmcost.Cost)
	}
	return out, args.Get(1).(int64), args.Error(2)
}

func (m *mockCostRepo) ListAll(ctx context.Context, filter rmcost.ExportFilter) ([]*rmcost.Cost, error) {
	args := m.Called(ctx, filter)
	var out []*rmcost.Cost
	if v := args.Get(0); v != nil {
		out = v.([]*rmcost.Cost)
	}
	return out, args.Error(1)
}

func (m *mockCostRepo) ExistsForGroupHead(ctx context.Context, groupHeadID uuid.UUID) (bool, error) {
	args := m.Called(ctx, groupHeadID)
	return args.Bool(0), args.Error(1)
}

func (m *mockCostRepo) ListDistinctPeriods(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	var out []string
	if v := args.Get(0); v != nil {
		out = v.([]string)
	}
	return out, args.Error(1)
}

func (m *mockCostRepo) ListHistory(ctx context.Context, filter rmcost.HistoryFilter) ([]rmcost.History, int64, error) {
	args := m.Called(ctx, filter)
	var out []rmcost.History
	if v := args.Get(0); v != nil {
		out = v.([]rmcost.History)
	}
	return out, args.Get(1).(int64), args.Error(2)
}

// mockJobRepo implements job.Repository.
type mockJobRepo struct{ mock.Mock }

func (m *mockJobRepo) Create(ctx context.Context, exec *job.Execution) error {
	return m.Called(ctx, exec).Error(0)
}

func (m *mockJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*job.Execution), args.Error(1)
}

func (m *mockJobRepo) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*job.Execution), args.Error(1)
}

func (m *mockJobRepo) List(ctx context.Context, filter job.ListFilter) ([]*job.Execution, int64, error) {
	args := m.Called(ctx, filter)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Get(1).(int64), args.Error(2)
}

func (m *mockJobRepo) UpdateStatus(ctx context.Context, exec *job.Execution) error {
	return m.Called(ctx, exec).Error(0)
}

func (m *mockJobRepo) UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error {
	return m.Called(ctx, id, progress).Error(0)
}

func (m *mockJobRepo) AddLog(ctx context.Context, log *job.ExecutionLog) error {
	return m.Called(ctx, log).Error(0)
}

func (m *mockJobRepo) UpdateLog(ctx context.Context, log *job.ExecutionLog) error {
	return m.Called(ctx, log).Error(0)
}

func (m *mockJobRepo) HasActiveJob(ctx context.Context, jobType job.Type, period string) (bool, error) {
	args := m.Called(ctx, jobType, period)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobRepo) GetNextSequence(ctx context.Context, jobType job.Type, period string) (int, error) {
	args := m.Called(ctx, jobType, period)
	return args.Int(0), args.Error(1)
}

// mockPublisher implements appcost.JobPublisher.
type mockPublisher struct{ mock.Mock }

func (m *mockPublisher) PublishRMCostCalculation(ctx context.Context, jobID, period string, groupHeadID *uuid.UUID, reason, createdBy string) error {
	return m.Called(ctx, jobID, period, groupHeadID, reason, createdBy).Error(0)
}

// mockGroupRepo implements rmgroup.Repository (subset used by CalculateHandler).
type mockGroupRepo struct{ mock.Mock }

func (m *mockGroupRepo) CreateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockGroupRepo) GetHeadByID(ctx context.Context, id uuid.UUID) (*rmgroup.Head, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockGroupRepo) GetHeadByCode(ctx context.Context, code rmgroup.Code) (*rmgroup.Head, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockGroupRepo) ListHeads(ctx context.Context, filter rmgroup.ListFilter) ([]*rmgroup.Head, int64, error) {
	args := m.Called(ctx, filter)
	var out []*rmgroup.Head
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Head)
	}
	return out, args.Get(1).(int64), args.Error(2)
}

func (m *mockGroupRepo) UpdateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockGroupRepo) ListAllHeads(_ context.Context, _ *bool) ([]*rmgroup.Head, error) {
	return nil, nil
}

func (m *mockGroupRepo) SoftDeleteHead(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

func (m *mockGroupRepo) ExistsHeadByCode(ctx context.Context, code rmgroup.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) ExistsHeadByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockGroupRepo) AddDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockGroupRepo) UpdateDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockGroupRepo) GetDetailByID(ctx context.Context, id uuid.UUID) (*rmgroup.Detail, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockGroupRepo) GetActiveDetailByItemCodeGrade(ctx context.Context, itemCode rmgroup.ItemCode, gradeCode string) (*rmgroup.Detail, error) {
	args := m.Called(ctx, itemCode, gradeCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockGroupRepo) ListDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockGroupRepo) ListActiveDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockGroupRepo) SoftDeleteDetail(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

// mockSourceReader implements appcost.SourceDataReader.
type mockSourceReader struct{ mock.Mock }

func (m *mockSourceReader) FetchRateInputs(ctx context.Context, period string, itemCodes []string) ([]rmcost.RateInputs, int, error) {
	args := m.Called(ctx, period, itemCodes)
	var out []rmcost.RateInputs
	if v := args.Get(0); v != nil {
		out = v.([]rmcost.RateInputs)
	}
	return out, args.Int(1), args.Error(2)
}

func (m *mockSourceReader) FetchItemUOMs(ctx context.Context, period string, itemCodes []string) (map[string]string, error) {
	args := m.Called(ctx, period, itemCodes)
	var out map[string]string
	if v := args.Get(0); v != nil {
		out = v.(map[string]string)
	}
	return out, args.Error(1)
}
