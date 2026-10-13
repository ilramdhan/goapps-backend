package worker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	appcostsheet "github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// reopen round-trips a workbook through its serialized form, so assertions
// see exactly what a user downloads (stream-written sheets included).
func reopen(t *testing.T, book *excelize.File) *excelize.File {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, book.Write(&buf))
	require.NoError(t, book.Close())
	out, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	return out
}

func TestBuildWorkbook_Layouts(t *testing.T) {
	t.Parallel()

	itemCodes := map[int64]string{100: "PTY0000090", 200: "PTY0000091"}
	tests := []struct {
		name       string
		layout     string
		ids        []int64
		wantSheets []string
		wantActive string
	}{
		{"unset is combined", "", []int64{100, 200}, []string{allDataSheetName, "PTY0000090", "PTY0000091"}, "PTY0000090"},
		{"combined", "combined", []int64{100, 200}, []string{allDataSheetName, "PTY0000090", "PTY0000091"}, "PTY0000090"},
		{"horizontal", "horizontal", []int64{100, 200}, []string{allDataSheetName}, allDataSheetName},
		{"vertical", "vertical", []int64{100, 200}, []string{"PTY0000090", "PTY0000091"}, "PTY0000090"},
		{"vertical single product", "vertical", []int64{100}, []string{parameterCheckSheetName}, parameterCheckSheetName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWorkbookTestHandler(itemCodes)
			book, stats, err := h.buildWorkbook(context.Background(),
				rabbitmq.JobMessage{Period: "202604", ProductSysIDs: tc.ids, Layout: tc.layout},
				costcalcdom.CalcTypeActual)
			require.NoError(t, err)
			assert.Equal(t, len(tc.ids), stats.written)

			got := reopen(t, book)
			defer func() { require.NoError(t, got.Close()) }()
			assert.Equal(t, tc.wantSheets, got.GetSheetList())
			assert.Equal(t, tc.wantActive, got.GetSheetName(got.GetActiveSheetIndex()))
		})
	}
}

// TestBuildWorkbook_HorizontalMatchesCombinedAllData pins that the streamed
// flat sheet carries exactly the same cells and freeze as the combined
// layout's "all data" sheet.
func TestBuildWorkbook_HorizontalMatchesCombinedAllData(t *testing.T) {
	t.Parallel()

	itemCodes := map[int64]string{100: "PTY0000090", 200: "PTY0000091"}
	build := func(layout string) *excelize.File {
		h := newWorkbookTestHandler(itemCodes)
		book, _, err := h.buildWorkbook(context.Background(),
			rabbitmq.JobMessage{Period: "202604", ProductSysIDs: []int64{100, 200}, Layout: layout},
			costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		return reopen(t, book)
	}
	combined := build("")
	defer func() { require.NoError(t, combined.Close()) }()
	horizontal := build("horizontal")
	defer func() { require.NoError(t, horizontal.Close()) }()

	opts := excelize.Options{RawCellValue: true}
	wantRows, err := combined.GetRows(allDataSheetName, opts)
	require.NoError(t, err)
	gotRows, err := horizontal.GetRows(allDataSheetName, opts)
	require.NoError(t, err)
	require.Len(t, gotRows, 5) // header + 2 products x 2 stages
	assert.Equal(t, wantRows, gotRows)

	panes, err := horizontal.GetPanes(allDataSheetName)
	require.NoError(t, err)
	assert.True(t, panes.Freeze)
	assert.Equal(t, 1, panes.YSplit)
	assert.Equal(t, "A2", panes.TopLeftCell)
}

func TestBuildWorkbook_HorizontalAndVerticalAllSkippedFail(t *testing.T) {
	t.Parallel()

	for _, layout := range []string{"horizontal", "vertical"} {
		h := &CostSheetExportHandler{sheets: emptyRouteCostSheets{}, calcType: costcalcdom.CalcTypeActual}
		book, _, err := h.buildWorkbook(context.Background(),
			rabbitmq.JobMessage{Period: "202604", ProductSysIDs: []int64{1}, Layout: layout},
			costcalcdom.CalcTypeActual)
		require.Error(t, err, layout)
		assert.Nil(t, book, layout)
	}
}

func TestLayoutFileName(t *testing.T) {
	t.Parallel()

	const combined = "product-cost-sheet-202604-3products-20260401-000000.xlsx"
	assert.Equal(t, combined,
		layoutFileName(appcostsheet.LayoutCombined, combined, "202604", costcalcdom.CalcTypeActual))
	assert.Equal(t, "product-cost-sheet-202604-3products-20260401-000000-vertical.xlsx",
		layoutFileName(appcostsheet.LayoutVertical, combined, "202604", costcalcdom.CalcTypeActual))
	got := layoutFileName(appcostsheet.LayoutHorizontal, combined, "202604", costcalcdom.CalcTypeForecast)
	assert.Equal(t, "product-cost-all-data-202604-"+strings.ToLower(string(costcalcdom.CalcTypeForecast))+".xlsx", got)
}
