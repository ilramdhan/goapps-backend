package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// UngroupedExportQuery filters the ungrouped-items export.
type UngroupedExportQuery struct {
	Period string
	Search string
}

// UngroupedExportResult is the export bytes + filename.
type UngroupedExportResult struct {
	FileContent []byte
	FileName    string
}

// UngroupedExportHandler produces a single-sheet Excel of ungrouped items. It
// pages through the existing paginated reader to avoid introducing a new
// "list all" repo method.
type UngroupedExportHandler struct {
	reader UngroupedItemsReader
}

// NewUngroupedExportHandler builds an UngroupedExportHandler.
func NewUngroupedExportHandler(reader UngroupedItemsReader) *UngroupedExportHandler {
	return &UngroupedExportHandler{reader: reader}
}

const (
	ungroupedSheetName     = "UngroupedItems"
	ungroupedExportPageMax = 100
)

var ungroupedExportHeaders = []string{
	"period", "item_code", "grade_code", "grade_name", "item_name", "uom",
	"cons_qty", "cons_val", "cons_rate",
	"stores_qty", "stores_val", "stores_rate",
	"dept_qty", "dept_val", "dept_rate",
	"last_po_qty1", "last_po_val1", "last_po_rate1",
	"last_po_qty2", "last_po_val2", "last_po_rate2",
	"last_po_qty3", "last_po_val3", "last_po_rate3",
}

// Handle executes the ungrouped-items export.
func (h *UngroupedExportHandler) Handle(ctx context.Context, q UngroupedExportQuery) (result *UngroupedExportResult, err error) {
	items, err := h.collectAll(ctx, q)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close excel")
			if err == nil {
				err = fmt.Errorf("close file: %w", cerr)
			}
		}
	}()

	if _, err := f.NewSheet(ungroupedSheetName); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	if err := writeHeaderRow(f, ungroupedSheetName, ungroupedExportHeaders); err != nil {
		return nil, err
	}

	var errs []error
	for i, it := range items {
		row := i + 2
		vals := []any{
			it.Period, it.ItemCode, it.GradeCode, it.GradeName, it.ItemName, it.UOM,
			it.ConsQty, it.ConsVal, it.ConsRate,
			it.StoresQty, it.StoresVal, it.StoresRate,
			it.DeptQty, it.DeptVal, it.DeptRate,
			it.LastPOQty1, it.LastPOVal1, it.LastPORate1,
			it.LastPOQty2, it.LastPOVal2, it.LastPORate2,
			it.LastPOQty3, it.LastPOVal3, it.LastPORate3,
		}
		if werr := writeRow(f, ungroupedSheetName, row, vals); werr != nil {
			errs = append(errs, werr)
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("ungrouped export partial row errors")
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, idxErr := f.GetSheetIndex(ungroupedSheetName); idxErr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return &UngroupedExportResult{FileContent: buf.Bytes(), FileName: "ungrouped_items_export.xlsx"}, nil
}

// collectAll walks the paginated reader until no more rows are returned.
// ListUngroupedItems caps page_size at 100, so this is how "list all" is
// expressed without adding a new repo method.
func (h *UngroupedExportHandler) collectAll(ctx context.Context, q UngroupedExportQuery) ([]*syncdata.ItemConsStockPO, error) {
	var all []*syncdata.ItemConsStockPO
	page := 1
	for {
		filter := UngroupedItemsFilter{
			Period:   q.Period,
			Search:   q.Search,
			Page:     page,
			PageSize: ungroupedExportPageMax,
		}
		items, total, err := h.reader.ListUngroupedItems(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("list ungrouped page %d: %w", page, err)
		}
		all = append(all, items...)
		if len(items) == 0 || int64(len(all)) >= total {
			break
		}
		page++
	}
	return all, nil
}
