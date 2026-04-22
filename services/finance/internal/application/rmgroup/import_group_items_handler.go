package rmgroup

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ImportGroupItemsCommand bulk-assigns items to a SINGLE existing group from a
// one-sheet Excel. Distinct from ImportHandler which owns the full list-page
// 2-sheet import.
type ImportGroupItemsCommand struct {
	HeadID      string
	FileContent []byte
	FileName    string
	CreatedBy   string
}

// ImportGroupItemsResult summarizes the outcome.
type ImportGroupItemsResult struct {
	ItemsAdded   int32
	ItemsSkipped int32
	FailedCount  int32
	Errors       []ImportError
	Added        []*rmgroup.Detail
	Skipped      []SkippedItem
}

// ImportGroupItemsHandler parses a one-sheet Excel and delegates to
// AddItemsHandler so validation (one item / one active group, metadata
// backfill, sync-feed lookup) stays identical to the interactive add flow.
type ImportGroupItemsHandler struct {
	addItems *AddItemsHandler
	lookup   ImportItemLookup
}

// NewImportGroupItemsHandler builds an ImportGroupItemsHandler.
func NewImportGroupItemsHandler(addItems *AddItemsHandler, lookup ImportItemLookup) *ImportGroupItemsHandler {
	return &ImportGroupItemsHandler{addItems: addItems, lookup: lookup}
}

// Handle parses the "Items" sheet, enriches rows from the sync feed, and
// invokes AddItemsHandler. Rows with an unparseable item_code are recorded as
// errors; the AddItems handler's own Skipped output is propagated.
func (h *ImportGroupItemsHandler) Handle(ctx context.Context, cmd ImportGroupItemsCommand) (*ImportGroupItemsResult, error) {
	if cmd.CreatedBy == "" {
		return nil, rmgroup.ErrEmptyCreatedBy
	}
	if _, err := uuid.Parse(cmd.HeadID); err != nil {
		return nil, rmgroup.ErrNotFound
	}

	f, err := excelize.OpenReader(bytes.NewReader(cmd.FileContent))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheet, err := resolveItemsSheet(f)
	if err != nil {
		return nil, err
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sheet, err)
	}

	result := &ImportGroupItemsResult{}
	inputs := h.parseRows(ctx, rows, result)

	if len(inputs) == 0 {
		return result, nil
	}

	addRes, err := h.addItems.Handle(ctx, AddItemsCommand{
		HeadID:    cmd.HeadID,
		CreatedBy: cmd.CreatedBy,
		Items:     inputs,
	})
	if err != nil {
		return nil, fmt.Errorf("add items: %w", err)
	}

	result.Added = addRes.Added
	result.Skipped = addRes.Skipped
	result.ItemsAdded = safeLen32(len(addRes.Added))
	result.ItemsSkipped = safeLen32(len(addRes.Skipped))
	return result, nil
}

// parseRows walks the items sheet (1 header row + data rows) and returns the
// AddItemInput list. Rows that fail to resolve item_code are recorded in
// result.Errors with FailedCount++.
func (h *ImportGroupItemsHandler) parseRows(ctx context.Context, rows [][]string, result *ImportGroupItemsResult) []AddItemInput {
	if len(rows) < 2 {
		return nil
	}
	inputs := make([]AddItemInput, 0, len(rows)-1)
	for i, row := range rows[1:] {
		rowNum := int32(i + 2) //nolint:gosec // row index bounded by Excel size
		if isBlankRow(row) {
			continue
		}
		in, err := h.buildInput(ctx, row)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "item_code",
				Message:   err.Error(),
			})
			continue
		}
		inputs = append(inputs, in)
	}
	return inputs
}

func (h *ImportGroupItemsHandler) buildInput(ctx context.Context, row []string) (AddItemInput, error) {
	itemCodeStr := colStr(row, 0)
	if itemCodeStr == "" {
		return AddItemInput{}, errors.New("item_code is required")
	}
	in := AddItemInput{ItemCode: itemCodeStr}

	if v := colStr(row, 1); v != "" {
		in.GradeCode = v
	}
	if s := colStr(row, 2); s != "" {
		// Re-use import_handler's column parsing via colOptBool for is_dummy,
		// but sort_order parsing lives inline.
		if n, ok := parseInt32(s); ok {
			in.SortOrder = n
		}
	}

	// Enrich metadata from sync feed when available. Missing rows are still
	// handed to AddItemsHandler so it can produce the correct "not in sync
	// feed" skip reason — we do not reject here.
	if h.lookup != nil {
		if syncItem, err := h.lookup.GetItemByCode(ctx, itemCodeStr); err == nil && syncItem != nil {
			if in.ItemName == "" {
				in.ItemName = syncItem.ItemName
			}
			if in.GradeCode == "" {
				in.GradeCode = syncItem.GradeCode
			}
			in.ItemGrade = syncItem.GradeName
			in.UOMCode = syncItem.UOM
		}
	}
	return in, nil
}

func resolveItemsSheet(f *excelize.File) (string, error) {
	sheets := f.GetSheetList()
	if containsSheet(sheets, sheetItems) {
		return sheetItems, nil
	}
	if len(sheets) == 0 {
		return "", errors.New("workbook has no sheets")
	}
	// Fallback to the first sheet so operators don't need the exact tab name.
	return sheets[0], nil
}

func parseInt32(s string) (int32, bool) {
	var n int32
	var neg bool
	i := 0
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		i = 1
	}
	if i == len(s) {
		return 0, false
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int32(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

func safeLen32(n int) int32 {
	if n < 0 {
		return 0
	}
	if n > 2_147_483_647 {
		return 2_147_483_647
	}
	return int32(n) //nolint:gosec // bounds checked above
}
