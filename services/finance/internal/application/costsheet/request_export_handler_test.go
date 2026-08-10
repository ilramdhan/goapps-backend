package costsheet_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appcostsheet "github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

func TestRequestExportHandler_ValidationErrors(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}
	h := appcostsheet.NewRequestExportHandler(repo, results, pub)

	t.Run("publisher nil", func(t *testing.T) {
		hh := appcostsheet.NewRequestExportHandler(repo, results, nil)
		_, err := hh.Handle(context.Background(), appcostsheet.RequestExportCommand{Period: "202604", RequestingUserID: "u"})
		require.Error(t, err)
	})

	t.Run("missing period", func(t *testing.T) {
		_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{RequestingUserID: "u"})
		require.Error(t, err)
	})

	t.Run("missing requesting user id", func(t *testing.T) {
		_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{Period: "202604"})
		require.Error(t, err)
	})
}

func TestRequestExportHandler_ExplicitProductSysIDsBypassesFilter(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-7", "user-7", []int64{10, 20, 30}).Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		ProductSysIDs:    []int64{10, 20, 30},
		Search:           "ignored-when-explicit",
		RequestingUserID: "user-7",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, job.TypeProductCostSheetExport, res.Execution.JobType())

	// ListResults must never be called when ProductSysIDs is explicit.
	results.AssertNotCalled(t, "ListResults", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_FilterResolvedUnderCap(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	items := makeResultSummaries(50)
	results.On("ListResults", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Period == "202604" && f.Search == "widget"
	})).Return(items, 50, "202604", nil).Once()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1", mock.MatchedBy(func(ids []int64) bool {
		return len(ids) == 50
	})).Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		Search:           "widget",
		RequestingUserID: "user-1",
	})
	require.NoError(t, err)

	var params map[string]any
	require.NoError(t, json.Unmarshal(res.Execution.Params(), &params))
	assert.NotContains(t, params, "truncated")
	assert.NotContains(t, params, "requested")

	results.AssertExpectations(t)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_ExactlyCapDoesNotTruncate(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	items := makeResultSummaries(200)
	results.On("ListResults", mock.Anything, mock.Anything).Return(items, 200, "202604", nil).Once()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1", mock.MatchedBy(func(ids []int64) bool {
		return len(ids) == 200
	})).Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.NoError(t, err)

	var params map[string]any
	require.NoError(t, json.Unmarshal(res.Execution.Params(), &params))
	assert.NotContains(t, params, "truncated")
}

func TestRequestExportHandler_OverCapFansOutIntoParentAndChildren(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	// Filter resolves to 450 products across two pages of the 500-sized
	// resolvePageSize — more than maxExportProducts(200), so Handle must fan
	// out into a parent + ceil(450/200)=3 children (200, 200, 50).
	page1 := makeResultSummaries(450)
	results.On("ListResults", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Page == 1 && f.PageSize == 500
	})).Return(page1, 450, "202604", nil).Once()

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == 3
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		return len(execs) == 3
	})).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1", mock.MatchedBy(func(ids []int64) bool {
		return len(ids) == 200
	})).Return(nil).Twice()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1", mock.MatchedBy(func(ids []int64) bool {
		return len(ids) == 50
	})).Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Execution.IsParent())
	assert.Equal(t, 3, res.Execution.TotalChildren())

	results.AssertExpectations(t)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_ExplicitProductSysIDsOverCapIsRejected(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	ids := make([]int64, 201)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		ProductSysIDs:    ids,
		RequestingUserID: "user-1",
	})
	require.Error(t, err)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	results.AssertNotCalled(t, "ListResults", mock.Anything, mock.Anything)
}

func TestRequestExportHandler_EmptyResolutionIsError(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	results.On("ListResults", mock.Anything, mock.Anything).Return(nil, 0, "202604", nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.Error(t, err)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	pub.AssertNotCalled(t, "PublishProductCostSheetExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRequestExportHandler_PublishFailureMarksJobFailed(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("rabbitmq down")).Once()
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		ProductSysIDs:    []int64{1},
		RequestingUserID: "user-1",
	})
	require.Error(t, err)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func makeResultSummaries(n int) []*costcalcdom.ResultSummary {
	out := make([]*costcalcdom.ResultSummary, n)
	for i := range out {
		out[i] = &costcalcdom.ResultSummary{ProductSysID: int64(i + 1)}
	}
	return out
}
