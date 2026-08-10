package rmcost_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
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

func (m *mockJobRepo) CreateChildren(ctx context.Context, execs []*job.Execution) error {
	return m.Called(ctx, execs).Error(0)
}

func (m *mockJobRepo) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	args := m.Called(ctx, parentJobID, success)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobRepo) ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	args := m.Called(ctx, parentJobID)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Error(1)
}

// mockPublisher implements appcost.JobPublisher.
type mockPublisher struct{ mock.Mock }

func (m *mockPublisher) PublishRMCostCalculation(ctx context.Context, jobID, period string, groupHeadID *uuid.UUID, reason, createdBy string) error {
	return m.Called(ctx, jobID, period, groupHeadID, reason, createdBy).Error(0)
}
