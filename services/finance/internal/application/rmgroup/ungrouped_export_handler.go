package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// UngroupedExportQuery filters the grouping-monitor export.
type UngroupedExportQuery struct {
	Search    string
	Scope     GroupingScope
	SortBy    string
	SortOrder string
}

// UngroupedExportResult is the export bytes + filename.
type UngroupedExportResult struct {
	FileContent []byte
	FileName    string
}

// UngroupedExportHandler produces a single-sheet Excel of the grouping
// monitor (ungrouped or grouped scope). It pages through the existing
// paginated reader to avoid introducing a new "list all" repo method.
type UngroupedExportHandler struct {
	reader UngroupedItemsReader
}

// NewUngroupedExportHandler builds an UngroupedExportHandler.
func NewUngroupedExportHandler(reader UngroupedItemsReader) *UngroupedExportHandler {
	return &UngroupedExportHandler{reader: reader}
}

const (
	ungroupedSheetName     = "GroupingMonitor"
	ungroupedExportPageMax = 100
)

// Headers vary by scope — Grouped includes group_code / group_name /
// sort_order / assigned_at, Ungrouped has only the item identity columns.
var ungroupedExportHeadersBase = []string{
	"item_code", "grade_code", "grade_name", "item_name", "uom",
}

var ungroupedExportHeadersGrouped = append(
	append([]string{}, ungroupedExportHeadersBase...),
	"group_code", "group_name", "sort_order", "assigned_at",
)

// Handle executes the grouping-monitor export.
func (h *UngroupedExportHandler) Handle(ctx context.Context, q UngroupedExportQuery) (result *UngroupedExportResult, err error) {
	items, err := h.collectAll(ctx, q)
	if err != nil {
		return nil, err
	}

	headers := ungroupedExportHeadersBase
	fileNameSuffix := "ungrouped"
	if q.Scope == GroupingScopeGrouped {
		headers = ungroupedExportHeadersGrouped
		fileNameSuffix = "grouped"
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
	if err := writeHeaderRow(f, ungroupedSheetName, headers); err != nil {
		return nil, err
	}

	var errs []error
	for i, it := range items {
		row := i + 2
		vals := []any{
			it.ItemCode, it.GradeCode, it.GradeName, it.ItemName, it.UOM,
		}
		if q.Scope == GroupingScopeGrouped {
			vals = append(vals, it.GroupCode, it.GroupName, it.SortOrder, it.AssignedAt)
		}
		if werr := writeRow(f, ungroupedSheetName, row, vals); werr != nil {
			errs = append(errs, werr)
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("grouping monitor export partial row errors")
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
	return &UngroupedExportResult{
		FileContent: buf.Bytes(),
		FileName:    fmt.Sprintf("rm_grouping_%s_export.xlsx", fileNameSuffix),
	}, nil
}

// collectAll walks the paginated reader until no more rows are returned.
// ListGroupingMonitor caps page_size at 100, so this is how "list all" is
// expressed without adding a new repo method.
func (h *UngroupedExportHandler) collectAll(ctx context.Context, q UngroupedExportQuery) ([]*GroupingMonitorItem, error) {
	var all []*GroupingMonitorItem
	page := 1
	for {
		filter := UngroupedItemsFilter{
			Search:    q.Search,
			Scope:     q.Scope,
			Page:      page,
			PageSize:  ungroupedExportPageMax,
			SortBy:    q.SortBy,
			SortOrder: q.SortOrder,
		}
		items, total, err := h.reader.ListGroupingMonitor(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("list grouping monitor page %d: %w", page, err)
		}
		all = append(all, items...)
		if len(items) == 0 || int64(len(all)) >= total {
			break
		}
		page++
	}
	return all, nil
}
