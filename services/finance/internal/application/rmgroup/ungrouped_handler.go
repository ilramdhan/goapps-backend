// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// GroupingScope toggles the monitor view between items NOT in any active
// group (Ungrouped) and items WITH an active group assignment (Grouped).
// Status is evaluated cross-period — group membership is master data and
// does not depend on which sync period the item appeared in.
type GroupingScope int

// Grouping scope values.
const (
	// GroupingScopeUngrouped is the default — items with no active detail.
	GroupingScopeUngrouped GroupingScope = 0
	// GroupingScopeGrouped lists items currently assigned to an active group.
	GroupingScopeGrouped GroupingScope = 1
)

// GroupingMonitorItem is the slim per-(item_code, grade_code) row returned
// by the grouping monitor. Group fields are populated only when the item
// is currently assigned to an active group (scope = Grouped).
type GroupingMonitorItem struct {
	ItemCode     string
	ItemName     string
	ItemTypeCode string
	GradeCode    string
	GradeName    string
	UOM          string

	// Populated when scope = Grouped.
	GroupHeadID string
	GroupCode   string
	GroupName   string
	SortOrder   int32
	AssignedAt  string // RFC3339 — empty when ungrouped
}

// UngroupedItemsReader exposes the cross-period grouping-monitor lookup.
// The repository joins `cst_item_cons_stk_po` (DISTINCT by item_code +
// grade_code) against `cst_rm_group_detail` (active rows only) and filters
// by the requested scope. The interface lives in the rm_group package
// rather than syncdata because the join references rm_group tables.
type UngroupedItemsReader interface {
	ListGroupingMonitor(ctx context.Context, filter UngroupedItemsFilter) ([]*GroupingMonitorItem, int64, error)
}

// UngroupedItemsFilter scopes the grouping monitor.
type UngroupedItemsFilter struct {
	// Search matches against item_code / item_name / grade_code / grade_name
	// and (when scope = Grouped) group_code / group_name.
	Search   string
	Scope    GroupingScope
	Page     int
	PageSize int
	// SortBy is the column key. Empty defaults to item_code. Allowed:
	// item_code, item_name, grade_code, item_grade, uom_code,
	// group_code, group_name, sort_order, assigned_at (last 4 only when
	// scope = Grouped). Unknown values are ignored by the repository.
	SortBy string
	// SortOrder is "asc" or "desc". Empty defaults to asc.
	SortOrder string
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

// UngroupedQuery is the input for the grouping-monitor report.
type UngroupedQuery struct {
	Search    string
	Scope     GroupingScope
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// UngroupedResult is the paginated result.
type UngroupedResult struct {
	Items       []*GroupingMonitorItem
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// UngroupedHandler reports raw-material items in the grouping monitor view.
// Two scopes are supported: Ungrouped (no active group assignment) and
// Grouped (currently assigned). The report is cross-period — period is no
// longer a filter dimension because group membership is master data.
type UngroupedHandler struct {
	reader UngroupedItemsReader
}

// NewUngroupedHandler builds an UngroupedHandler.
func NewUngroupedHandler(reader UngroupedItemsReader) *UngroupedHandler {
	return &UngroupedHandler{reader: reader}
}

// Handle executes the grouping-monitor query.
func (h *UngroupedHandler) Handle(ctx context.Context, query UngroupedQuery) (*UngroupedResult, error) {
	// UngroupedQuery and UngroupedItemsFilter have identical field
	// shapes — staticcheck S1016 requires direct conversion over a
	// field-by-field struct literal.
	filter := UngroupedItemsFilter(query)
	filter.Validate()

	items, total, err := h.reader.ListGroupingMonitor(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list grouping monitor: %w", err)
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
