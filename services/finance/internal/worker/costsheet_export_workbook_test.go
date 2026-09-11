package worker

// Internal test package: buildWorkbook/addProductSheet are unexported and only
// need a RouteCostSheetProvider, so they are exercised directly here without
// storage, notifications, or the job repository.

import (
	"context"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	appcostcalc "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// stubRouteCostSheets returns a canned two-stage route per product sys id,
// naming the finished good (route level 1) after the id so each product gets a
// distinct FG label.
type stubRouteCostSheets struct{ itemCodes map[int64]string }

func (s stubRouteCostSheets) Handle(
	_ context.Context, q appcostcalc.GetRouteCostSheetQuery,
) ([]appcostcalc.RouteCostSheetStage, error) {
	code, ok := s.itemCodes[q.ProductSysID]
	if !ok {
		return nil, fmt.Errorf("unexpected product sys id %d", q.ProductSysID)
	}
	// The query returns stages downstream-first; toStages reverses them.
	return []appcostcalc.RouteCostSheetStage{
		{
			RouteLevel: 1, RouteSeq: 1, ProductSysID: q.ProductSysID,
			ItemCode: code, ProductName: "FG " + code, HasCost: true,
			ParamSnapshot: map[string]string{"MC_NAME": "A1-8-S"},
		},
		{
			RouteLevel: 2, RouteSeq: 1, ProductSysID: q.ProductSysID + 1,
			ItemCode: "POY" + code, HasCost: true,
			ParamSnapshot: map[string]string{"MC_NAME": "BT-S"},
		},
	}, nil
}

// newWorkbookTestHandler builds a handler wired with only what buildWorkbook
// touches.
func newWorkbookTestHandler(itemCodes map[int64]string) *CostSheetExportHandler {
	return &CostSheetExportHandler{
		sheets:   stubRouteCostSheets{itemCodes: itemCodes},
		logger:   zerolog.Nop(),
		calcType: costcalcdom.CalcTypeActual,
	}
}

// TestBuildWorkbook_SheetNames pins the product-sheet naming rule: a
// single-product export takes the reference workbook's fixed "parameter check",
// a multi-product export keeps one FG-code-named sheet per product.
func TestBuildWorkbook_SheetNames(t *testing.T) {
	t.Parallel()

	itemCodes := map[int64]string{100: "PTY0000090", 200: "PTY0000091"}

	tests := []struct {
		name          string
		productSysIDs []int64
		wantSheets    []string
	}{
		{
			name:          "single product uses the reference workbook's sheet name",
			productSysIDs: []int64{100},
			wantSheets:    []string{allDataSheetName, parameterCheckSheetName},
		},
		{
			name:          "two products keep their FG codes",
			productSysIDs: []int64{100, 200},
			wantSheets:    []string{allDataSheetName, "PTY0000090", "PTY0000091"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWorkbookTestHandler(itemCodes)
			book, stats, err := h.buildWorkbook(
				context.Background(),
				rabbitmq.JobMessage{Period: "202604", ProductSysIDs: tc.productSysIDs},
				costcalcdom.CalcTypeActual,
			)
			require.NoError(t, err)
			defer func() { require.NoError(t, book.Close()) }()

			assert.Equal(t, tc.wantSheets, book.GetSheetList())
			// The flat sheet must stay at position 0 under its exact name.
			assert.Equal(t, allDataSheetName, book.GetSheetName(0))
			assert.Equal(t, len(tc.productSysIDs), stats.written)

			// Every written sheet name is recorded as taken, "all data" included.
			for _, name := range tc.wantSheets {
				assert.True(t, stats.taken[name], "%q must be recorded as taken", name)
			}

			if len(tc.productSysIDs) > 1 {
				// A bulk export must never fall back to the reference name, nor to
				// its collision-suffixed variants.
				for _, name := range book.GetSheetList() {
					assert.NotEqual(t, parameterCheckSheetName, name)
					assert.NotEqual(t, parameterCheckSheetName+" (2)", name)
				}
			}
		})
	}
}

// TestBuildWorkbook_SingleProductSheetNameIsExact guards the literal spelling
// of the reference sheet name (lowercase, single space) against drift.
func TestBuildWorkbook_SingleProductSheetNameIsExact(t *testing.T) {
	t.Parallel()

	h := newWorkbookTestHandler(map[int64]string{100: "PTY0000090"})
	book, _, err := h.buildWorkbook(
		context.Background(),
		rabbitmq.JobMessage{Period: "202604", ProductSysIDs: []int64{100}},
		costcalcdom.CalcTypeActual,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, book.Close()) }()

	require.Equal(t, []string{"all data", "parameter check"}, book.GetSheetList())
}

// TestBuildWorkbook_FreezePanes pins the two different freezes the workbook
// uses. They are not interchangeable: the per-product sheet is transposed (row
// labels down column A, one stage per column) so only the label COLUMN can be
// frozen — it has no header row at all, headerRowCount is 0. The flat "all
// data" sheet is the opposite shape, so it freezes the header ROW instead.
//
// Verified that excelize v2.8.1 round-trips SetPanes through GetPanes for both
// shapes before relying on it here.
func TestBuildWorkbook_FreezePanes(t *testing.T) {
	t.Parallel()

	h := newWorkbookTestHandler(map[int64]string{100: "PTY0000090"})
	book, _, err := h.buildWorkbook(
		context.Background(),
		rabbitmq.JobMessage{Period: "202604", ProductSysIDs: []int64{100}},
		costcalcdom.CalcTypeActual,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, book.Close()) }()

	tests := []struct {
		name   string
		sheet  string
		want   excelize.Panes
		reason string
	}{
		{
			name:  "per-product sheet freezes the label column",
			sheet: parameterCheckSheetName,
			want: excelize.Panes{
				Freeze: true, XSplit: 1, YSplit: 0,
				TopLeftCell: "B1", ActivePane: "topRight",
			},
			reason: "the sheet is transposed and has no header row to freeze",
		},
		{
			name:  "all-data sheet freezes the header row",
			sheet: allDataSheetName,
			want: excelize.Panes{
				Freeze: true, XSplit: 0, YSplit: 1,
				TopLeftCell: "A2", ActivePane: "bottomLeft",
			},
			reason: "the sheet is a wide flat extract scrolled vertically",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			panes, err := book.GetPanes(tc.sheet)
			require.NoError(t, err)

			assert.True(t, panes.Freeze, "%s: %s", tc.sheet, tc.reason)
			assert.False(t, panes.Split, "%s must freeze, not split", tc.sheet)
			assert.Equal(t, tc.want.XSplit, panes.XSplit, "%s: %s", tc.sheet, tc.reason)
			assert.Equal(t, tc.want.YSplit, panes.YSplit, "%s: %s", tc.sheet, tc.reason)
			assert.Equal(t, tc.want.TopLeftCell, panes.TopLeftCell, "%s", tc.sheet)
			assert.Equal(t, tc.want.ActivePane, panes.ActivePane, "%s", tc.sheet)
		})
	}
}

// TestBuildWorkbook_NoProductProducedASheet pins the stats.written == 0 guard.
// A product whose route resolves to zero stages is a SKIP, not a failure — but
// when every product skips, the workbook would otherwise be shipped holding
// nothing but an empty "all data" sheet. buildWorkbook must fail the job
// instead, and must close the part-built workbook on the way out rather than
// handing back a file the caller has no error-free way to release.
func TestBuildWorkbook_NoProductProducedASheet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		productSysIDs []int64
	}{
		{name: "single product with no route stages", productSysIDs: []int64{100}},
		{name: "every product in a bulk export skips", productSysIDs: []int64{100, 200}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// An empty stage slice per product is the skip path: the stub answers
			// successfully, it just has no stages to render.
			h := newWorkbookTestHandler(nil)
			h.sheets = emptyRouteCostSheets{}

			book, stats, err := h.buildWorkbook(
				context.Background(),
				rabbitmq.JobMessage{Period: "202604", ProductSysIDs: tc.productSysIDs},
				costcalcdom.CalcTypeActual,
			)

			require.Error(t, err, "an export with nothing written must not succeed")
			assert.Nil(t, book, "the part-built workbook must not escape on the error path")
			assert.Nil(t, stats)
			assert.Contains(t, err.Error(), "no product produced a cost sheet")
			// The message carries the product count so the operator can tell a
			// one-product miss from a whole-batch miss in the job log.
			assert.Contains(t, err.Error(), fmt.Sprintf("all %d products", len(tc.productSysIDs)))
		})
	}
}

// TestBuildWorkbook_PartialSkipStillSucceeds is the other side of the guard
// above: as long as ONE product renders, the skips are recorded and the job
// completes. Without this, tightening the zero-written check into "any skip
// fails" would look correct.
func TestBuildWorkbook_PartialSkipStillSucceeds(t *testing.T) {
	t.Parallel()

	h := newWorkbookTestHandler(map[int64]string{100: "PTY0000090"})
	h.sheets = partialRouteCostSheets{
		inner:   stubRouteCostSheets{itemCodes: map[int64]string{100: "PTY0000090"}},
		empties: map[int64]bool{200: true},
	}

	book, stats, err := h.buildWorkbook(
		context.Background(),
		rabbitmq.JobMessage{Period: "202604", ProductSysIDs: []int64{100, 200}},
		costcalcdom.CalcTypeActual,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, book.Close()) }()

	assert.Equal(t, 1, stats.written)
	assert.Equal(t, []int64{200}, stats.skipped)
	assert.Equal(t, []string{allDataSheetName, "PTY0000090"}, book.GetSheetList())
}

// emptyRouteCostSheets resolves every product to zero stages — the skip path.
type emptyRouteCostSheets struct{}

func (emptyRouteCostSheets) Handle(
	_ context.Context, _ appcostcalc.GetRouteCostSheetQuery,
) ([]appcostcalc.RouteCostSheetStage, error) {
	return nil, nil
}

// partialRouteCostSheets skips the products named in empties and delegates the
// rest, so one export can mix a written product with a skipped one.
type partialRouteCostSheets struct {
	inner   stubRouteCostSheets
	empties map[int64]bool
}

func (p partialRouteCostSheets) Handle(
	ctx context.Context, q appcostcalc.GetRouteCostSheetQuery,
) ([]appcostcalc.RouteCostSheetStage, error) {
	if p.empties[q.ProductSysID] {
		return nil, nil
	}
	return p.inner.Handle(ctx, q)
}
