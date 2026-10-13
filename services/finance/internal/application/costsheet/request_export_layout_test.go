package costsheet_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appcostsheet "github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// layoutPubMock is a publisher that can also carry the layout on the message.
type layoutPubMock struct{ exportPubMock }

func (m *layoutPubMock) PublishProductCostSheetExportWithLayout(
	ctx context.Context, jobID, period, requestingUserID, createdBy string, productSysIDs []int64, layout string,
) error {
	return m.Called(ctx, jobID, period, requestingUserID, createdBy, productSysIDs, layout).Error(0)
}

func paramsLayout(t *testing.T, exec *job.Execution) string {
	t.Helper()
	var params map[string]any
	require.NoError(t, json.Unmarshal(exec.Params(), &params))
	v, _ := params["layout"].(string)
	return v
}

func expectFilterResolves(results *resultRepoMock, n int) {
	results.On("ListProductSysIDs", mock.Anything, mock.MatchedBy(func(f costcalcdom.ResultListFilter) bool {
		return f.Period == "202604"
	})).Return(makeProductIDs(n), nil).Once()
}

func TestRequestExportHandler_HorizontalOverCapIsOneJob(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &layoutPubMock{}
	expectFilterResolves(results, 450)

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return !e.IsParent() && !e.IsChild()
	})).Return(nil).Once()
	pub.On("PublishProductCostSheetExportWithLayout", mock.Anything, mock.Anything, "202604", "user-1", "user-1",
		mock.MatchedBy(func(ids []int64) bool { return len(ids) == 450 }), "horizontal").Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period: "202604", RequestingUserID: "user-1", Layout: appcostsheet.LayoutHorizontal,
	})
	require.NoError(t, err)
	assert.False(t, res.Execution.IsParent())
	assert.Equal(t, "horizontal", paramsLayout(t, res.Execution))
	repo.AssertNotCalled(t, "CreateChildren", mock.Anything, mock.Anything)
	pub.AssertNotCalled(t, "PublishProductCostSheetExport",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	pub.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestRequestExportHandler_VerticalOverCapIsBatch(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &layoutPubMock{}
	expectFilterResolves(results, 450)

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == 3
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		if len(execs) != 3 {
			return false
		}
		for _, e := range execs {
			var p map[string]any
			if json.Unmarshal(e.Params(), &p) != nil || p["layout"] != "vertical" {
				return false
			}
		}
		return true
	})).Return(nil).Once()
	pub.On("PublishProductCostSheetExportWithLayout", mock.Anything, mock.Anything, "202604", "user-1", "user-1",
		mock.MatchedBy(func(ids []int64) bool { return len(ids) == 200 }), "vertical").Return(nil).Twice()
	pub.On("PublishProductCostSheetExportWithLayout", mock.Anything, mock.Anything, "202604", "user-1", "user-1",
		mock.MatchedBy(func(ids []int64) bool { return len(ids) == 50 }), "vertical").Return(nil).Once()

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period: "202604", RequestingUserID: "user-1", Layout: appcostsheet.LayoutVertical,
	})
	require.NoError(t, err)
	assert.True(t, res.Execution.IsParent())
	assert.Equal(t, "vertical", paramsLayout(t, res.Execution))
	pub.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestRequestExportHandler_UnspecifiedLayoutKeepsCombinedMessage(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	results := &resultRepoMock{}
	pub := &layoutPubMock{}
	expectFilterResolves(results, 450)

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == 3
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.Anything).Return(nil).Once()
	pub.On("PublishProductCostSheetExport", mock.Anything, mock.Anything, "202604", "user-1", "user-1",
		mock.Anything).Return(nil).Times(3)

	h := appcostsheet.NewRequestExportHandler(repo, results, pub)
	res, err := h.Handle(context.Background(), appcostsheet.RequestExportCommand{
		Period: "202604", RequestingUserID: "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "combined", paramsLayout(t, res.Execution))
	pub.AssertNotCalled(t, "PublishProductCostSheetExportWithLayout",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	pub.AssertExpectations(t)
}

func TestLayoutFromParams_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want appcostsheet.ExportLayout
	}{
		{"no params", "", appcostsheet.LayoutCombined},
		{"legacy params without layout", `{"period":"202604"}`, appcostsheet.LayoutCombined},
		{"invalid json", `{`, appcostsheet.LayoutCombined},
		{"unknown value", `{"layout":"diagonal"}`, appcostsheet.LayoutCombined},
		{"combined", `{"layout":"combined"}`, appcostsheet.LayoutCombined},
		{"horizontal", `{"layout":"horizontal"}`, appcostsheet.LayoutHorizontal},
		{"vertical", `{"layout":"VERTICAL"}`, appcostsheet.LayoutVertical},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, appcostsheet.LayoutFromParams(json.RawMessage(tc.raw)))
		})
	}
}
