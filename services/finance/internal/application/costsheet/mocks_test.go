package costsheet_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// jobRepoMock is a small testify mock for job.Repository — only the methods
// exercised by the costsheet handlers are stubbed; calls to other methods
// will panic loudly if a future change adds them.
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

// resultRepoMock is a testify mock covering costcalcdom.ResultRepository —
// only ListResults is exercised by RequestExportHandler.
type resultRepoMock struct{ mock.Mock }

func (m *resultRepoMock) UpsertWithSupersede(ctx context.Context, r *costcalcdom.Result) (int64, int, float64, int64, error) {
	args := m.Called(ctx, r)
	return args.Get(0).(int64), args.Get(1).(int), args.Get(2).(float64), args.Get(3).(int64), args.Error(4)
}

func (m *resultRepoMock) GetActive(ctx context.Context, productSysID int64, period string, calcType costcalcdom.CalculationType) (*costcalcdom.Result, error) {
	args := m.Called(ctx, productSysID, period, calcType)
	if v := args.Get(0); v != nil {
		return v.(*costcalcdom.Result), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *resultRepoMock) GetByID(ctx context.Context, id int64) (*costcalcdom.Result, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*costcalcdom.Result), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *resultRepoMock) ListHistory(ctx context.Context, productSysID int64, calcType costcalcdom.CalculationType, page, pageSize int) ([]*costcalcdom.Result, int, error) {
	args := m.Called(ctx, productSysID, calcType, page, pageSize)
	var out []*costcalcdom.Result
	if v := args.Get(0); v != nil {
		out = v.([]*costcalcdom.Result)
	}
	return out, args.Int(1), args.Error(2)
}

func (m *resultRepoMock) ListResults(ctx context.Context, f costcalcdom.ResultListFilter) ([]*costcalcdom.ResultSummary, int, string, error) {
	args := m.Called(ctx, f)
	var out []*costcalcdom.ResultSummary
	if v := args.Get(0); v != nil {
		out = v.([]*costcalcdom.ResultSummary)
	}
	return out, args.Int(1), args.String(2), args.Error(3)
}

func (m *resultRepoMock) ListByProductIDsPeriodType(ctx context.Context, productSysIDs []int64, period string, calcType costcalcdom.CalculationType) (map[int64]*costcalcdom.Result, error) {
	args := m.Called(ctx, productSysIDs, period, calcType)
	var out map[int64]*costcalcdom.Result
	if v := args.Get(0); v != nil {
		out = v.(map[int64]*costcalcdom.Result)
	}
	return out, args.Error(1)
}

func (m *resultRepoMock) MarkVerified(ctx context.Context, costID int64, by string) error {
	return m.Called(ctx, costID, by).Error(0)
}

func (m *resultRepoMock) MarkApproved(ctx context.Context, costID int64, by string) error {
	return m.Called(ctx, costID, by).Error(0)
}

func (m *resultRepoMock) ListDistinctPeriods(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

// exportPubMock is a testify mock for ExportJobPublisher.
type exportPubMock struct{ mock.Mock }

func (m *exportPubMock) PublishProductCostSheetExport(ctx context.Context, jobID, period, requestingUserID, createdBy string, productSysIDs []int64) error {
	return m.Called(ctx, jobID, period, requestingUserID, createdBy, productSysIDs).Error(0)
}

// presignMock is a testify mock for PresignedURLProvider.
type presignMock struct{ mock.Mock }

func (m *presignMock) PresignedGetURL(ctx context.Context, key string, validity time.Duration, name string) (string, error) {
	args := m.Called(ctx, key, validity, name)
	return args.String(0), args.Error(1)
}
