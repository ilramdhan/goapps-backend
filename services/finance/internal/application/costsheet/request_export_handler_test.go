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

	// The filter must never be resolved when ProductSysIDs is explicit.
	results.AssertNotCalled(t, "ListProductSysIDs", mock.Anything, mock.Anything)
	results.AssertNotCalled(t, "ListResults", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_FilterResolvedUnderCap(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	results.On("ListProductSysIDs", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Period == "202604" && f.Search == "widget"
	})).Return(makeProductIDs(50), nil).Once()

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

	results.On("ListProductSysIDs", mock.Anything, mock.Anything).Return(makeProductIDs(200), nil).Once()

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

	// Filter resolves (in one lean query) to 450 products — more than
	// maxExportProducts(200), so Handle must fan out into a parent +
	// ceil(450/200)=3 children (200, 200, 50).
	results.On("ListProductSysIDs", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Period == "202604"
	})).Return(makeProductIDs(450), nil).Once()

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
	results.AssertNotCalled(t, "ListProductSysIDs", mock.Anything, mock.Anything)
}

func TestRequestExportHandler_EmptyResolutionIsError(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	results.On("ListProductSysIDs", mock.Anything, mock.Anything).Return([]int64{}, nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no products matched the given filter")
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	pub.AssertNotCalled(t, "PublishProductCostSheetExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRequestExportHandler_FilterResolutionPassesFullFilterOnce(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	// The whole filter goes to the lean id query exactly once — no paging.
	results.On("ListProductSysIDs", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Period == "202604" && f.CalcType == costcalcdom.CalcTypeForecast &&
			f.Status == "APPROVED" && f.Search == "poy" &&
			len(f.ProductTypeIDs) == 2 && f.ProductTypeIDs[1] == 7 &&
			len(f.ShadeCodes) == 1 && f.ShadeCodes[0] == "WHT" &&
			len(f.RMGroupCodes) == 1 && f.RMGroupCodes[0] == "BRT" &&
			f.Page == 0 && f.PageSize == 0
	})).Return([]int64{42, 7, 9}, nil).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1", []int64{42, 7, 9}).
		Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		CalcType:         costcalcdom.CalcTypeForecast,
		Status:           "APPROVED",
		Search:           "poy",
		ProductTypeIDs:   []int32{3, 7},
		ShadeCodes:       []string{"WHT"},
		RMGroupCodes:     []string{"BRT"},
		RequestingUserID: "user-1",
	})
	require.NoError(t, err)
	results.AssertExpectations(t)
	results.AssertNotCalled(t, "ListResults", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_FilterResolutionErrorIsWrapped(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &exportPubMock{}

	results.On("ListProductSysIDs", mock.Anything, mock.Anything).Return(nil, context.DeadlineExceeded).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	_, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "resolve product filter")
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
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

func makeProductIDs(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i + 1)
	}
	return out
}
