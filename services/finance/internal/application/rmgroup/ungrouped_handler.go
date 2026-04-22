// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// UngroupedItemsReader exposes the LEFT JOIN lookup used by the Ungrouped Items
// report: `cst_item_cons_stk_po LEFT JOIN cst_rm_group_detail` filtered to rows
// where no active (non-deleted) detail claims the item_code. The interface is
// defined here rather than on syncdata.PostgresTargetRepository because the join
// references a table from a different bounded context (rm grouping).
type UngroupedItemsReader interface {
	ListUngroupedItems(ctx context.Context, filter UngroupedItemsFilter) ([]*syncdata.ItemConsStockPO, int64, error)
}

// UngroupedItemsFilter scopes the ungrouped report.
type UngroupedItemsFilter struct {
	// Period is optional; empty matches all periods.
	Period string
	// Search matches against item_code / item_name / grade_name.
	Search   string
	Page     int
	PageSize int
}

// Validate normalizes the filter with pagination defaults.
func (f *UngroupedItemsFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
}

// UngroupedQuery is the input for the ungrouped-items report.
type UngroupedQuery struct {
	Period   string
	Search   string
	Page     int
	PageSize int
}

// UngroupedResult is the paginated result.
type UngroupedResult struct {
	Items       []*syncdata.ItemConsStockPO
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// UngroupedHandler reports raw-material items that have been synced from Oracle
// but are not yet assigned to any active RM group — the seed list for operators
// deciding what to group next.
type UngroupedHandler struct {
	reader UngroupedItemsReader
}

// NewUngroupedHandler builds an UngroupedHandler.
func NewUngroupedHandler(reader UngroupedItemsReader) *UngroupedHandler {
	return &UngroupedHandler{reader: reader}
}

// Handle executes the ungrouped-items query.
func (h *UngroupedHandler) Handle(ctx context.Context, query UngroupedQuery) (*UngroupedResult, error) {
	filter := UngroupedItemsFilter(query)
	filter.Validate()

	items, total, err := h.reader.ListUngroupedItems(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list ungrouped items: %w", err)
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &UngroupedResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
