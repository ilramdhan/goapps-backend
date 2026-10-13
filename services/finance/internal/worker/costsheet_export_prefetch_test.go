package worker

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	appcostcalc "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// batchRouteCostSheets wraps the per-product stub with a HandleBatch that
// records each batch size, so the worker's prefetch path can be compared
// against the per-product fallback.
type batchRouteCostSheets struct {
	partialRouteCostSheets
	batches *[]int
}

func (b batchRouteCostSheets) HandleBatch(
	ctx context.Context, q appcostcalc.GetRouteCostSheetBatchQuery,
) (map[int64][]appcostcalc.RouteCostSheetStage, error) {
	*b.batches = append(*b.batches, len(q.ProductSysIDs))
	out := map[int64][]appcostcalc.RouteCostSheetStage{}
	for _, id := range q.ProductSysIDs {
		stages, err := b.Handle(ctx, appcostcalc.GetRouteCostSheetQuery{ProductSysID: id, Period: q.Period, CalcType: q.CalcType})
		if err != nil {
			return nil, err
		}
		out[id] = stages
	}
	return out, nil
}

// TestBuildWorkbook_BatchPrefetchMatchesPerProduct pins that the prefetched
// (batched) stage loading renders exactly the workbook the per-product path
// renders, for every layout, in stagePrefetchSize-sized rounds.
func TestBuildWorkbook_BatchPrefetchMatchesPerProduct(t *testing.T) {
	t.Parallel()

	n := stagePrefetchSize + 7
	itemCodes := map[int64]string{}
	ids := make([]int64, 0, n)
	for i := range n {
		id := int64(10 * (i + 1))
		ids = append(ids, id)
		itemCodes[id] = "FG" + string(rune('A'+i%26)) + string(rune('A'+(i/26)%26))
	}
	empties := map[int64]bool{ids[3]: true, ids[n-1]: true}
	perProduct := partialRouteCostSheets{inner: stubRouteCostSheets{itemCodes: itemCodes}, empties: empties}

	for _, layout := range []string{"horizontal", "vertical", "combined"} {
		msgIDs := ids
		if layout != "horizontal" {
			msgIDs = ids[:40]
		}
		msg := rabbitmq.JobMessage{JobID: "j", Period: "202604", ProductSysIDs: msgIDs, Layout: layout}

		var batches []int
		hb := &CostSheetExportHandler{
			sheets:   batchRouteCostSheets{partialRouteCostSheets: perProduct, batches: &batches},
			logger:   zerolog.Nop(),
			calcType: costcalcdom.CalcTypeActual,
		}
		hp := &CostSheetExportHandler{sheets: perProduct, logger: zerolog.Nop(), calcType: costcalcdom.CalcTypeActual}

		b1, s1, err := hb.buildWorkbook(context.Background(), msg, costcalcdom.CalcTypeActual)
		require.NoError(t, err, layout)
		b2, s2, err := hp.buildWorkbook(context.Background(), msg, costcalcdom.CalcTypeActual)
		require.NoError(t, err, layout)

		assert.Equal(t, s2, s1, layout)
		require.Equal(t, b2.GetSheetList(), b1.GetSheetList(), layout)
		for _, sh := range b1.GetSheetList() {
			r1, err := b1.GetRows(sh, excelize.Options{RawCellValue: true})
			require.NoError(t, err)
			r2, err := b2.GetRows(sh, excelize.Options{RawCellValue: true})
			require.NoError(t, err)
			assert.Equal(t, r2, r1, "%s sheet %s", layout, sh)
		}
		if layout == "horizontal" {
			assert.Equal(t, []int{stagePrefetchSize, 7}, batches)
		} else {
			assert.Equal(t, []int{40}, batches)
		}
		require.NoError(t, b1.Close())
		require.NoError(t, b2.Close())
	}
}
