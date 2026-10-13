package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

func TestProtoToExportLayout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, costsheet.LayoutCombined,
		protoToExportLayout(financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_UNSPECIFIED))
	assert.Equal(t, costsheet.LayoutCombined,
		protoToExportLayout(financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_COMBINED))
	assert.Equal(t, costsheet.LayoutHorizontal,
		protoToExportLayout(financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_HORIZONTAL))
	assert.Equal(t, costsheet.LayoutVertical,
		protoToExportLayout(financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_VERTICAL))
}

func TestExportJobInfo_LayoutFromParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params json.RawMessage
		want   financev1.CostSheetExportLayout
	}{
		{"legacy job without params", nil, financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_COMBINED},
		{"legacy params without layout", json.RawMessage(`{"period":"202604"}`),
			financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_COMBINED},
		{"horizontal", json.RawMessage(`{"layout":"horizontal"}`),
			financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_HORIZONTAL},
		{"vertical", json.RawMessage(`{"layout":"vertical"}`),
			financev1.CostSheetExportLayout_COST_SHEET_EXPORT_LAYOUT_VERTICAL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			exec, err := job.NewExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, tc.params)
			require.NoError(t, err)
			assert.Equal(t, tc.want, productCostSheetExportJobInfoFromExecution(exec).GetLayout())
			assert.Equal(t, tc.want, exportJobSummaryFromExecution(exec).GetLayout())
		})
	}
}
