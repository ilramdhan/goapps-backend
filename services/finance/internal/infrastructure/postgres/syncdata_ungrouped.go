package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
)

// Verify interface compliance at compile time.
var _ appgroup.UngroupedItemsReader = (*SyncDataRepository)(nil)

// ListGroupingMonitor returns one row per distinct (item_code, grade_code)
// pair. The pool is built cross-period from `cst_item_cons_stk_po` so an
// item that ever appeared in any sync period is considered. Status is
// determined by the LEFT JOIN against `cst_rm_group_detail` (active,
// non-deleted only):
//   - scope = Ungrouped → rows where no active detail exists.
//   - scope = Grouped   → rows that currently belong to an active group;
//     group_head_id / group_code / group_name / sort_order / assigned_at
//     are populated from the matched detail.
//
// Period and per-period qty/rate values are no longer returned because the
// monitor is cross-period and those numbers are not meaningful here.
func (r *SyncDataRepository) ListGroupingMonitor(
	ctx context.Context,
	filter appgroup.UngroupedItemsFilter,
) ([]*appgroup.GroupingMonitorItem, int64, error) {
	filter.Validate()

	args := make([]any, 0, 4)
	idx := 1

	// Build the search predicate. In Grouped mode, search also matches
	// against group_code / group_name.
	var searchPred string
	if filter.Search != "" {
		needle := "%" + filter.Search + "%"
		args = append(args, needle)
		switch filter.Scope {
		case appgroup.GroupingScopeGrouped:
			searchPred = fmt.Sprintf(
				"(s.item_code ILIKE $%d OR s.item_name ILIKE $%d OR s.grade_code ILIKE $%d OR s.grade_name ILIKE $%d OR h.group_code ILIKE $%d OR h.group_name ILIKE $%d)",
				idx, idx, idx, idx, idx, idx,
			)
		case appgroup.GroupingScopeUngrouped:
			searchPred = fmt.Sprintf(
				"(s.item_code ILIKE $%d OR s.item_name ILIKE $%d OR s.grade_code ILIKE $%d OR s.grade_name ILIKE $%d)",
				idx, idx, idx, idx,
			)
		}
		idx++
	}

	// scopePred drives the LEFT JOIN result filter.
	var scopePred string
	switch filter.Scope {
	case appgroup.GroupingScopeGrouped:
		scopePred = "d.group_detail_id IS NOT NULL"
	case appgroup.GroupingScopeUngrouped:
		scopePred = "d.group_detail_id IS NULL"
	}

	conds := []string{scopePred}
	if searchPred != "" {
		conds = append(conds, searchPred)
	}
	where := "WHERE " + strings.Join(conds, " AND ")

	// Distinct (item_code, grade_code) per item — sync feed has many rows
	// per pair across periods. Pick the latest row for metadata.
	const distinctCTE = `
		WITH s AS (
			SELECT DISTINCT ON (item_code, COALESCE(grade_code,''))
			       item_code,
			       grade_code,
			       grade_name,
			       item_name,
			       uom
			FROM cst_item_cons_stk_po
			ORDER BY item_code, COALESCE(grade_code,''), period DESC
		)
	`

	const joinClause = `
		FROM s
		LEFT JOIN cst_rm_group_detail d
		  ON d.item_code = s.item_code
		 AND COALESCE(d.grade_code, '') = COALESCE(s.grade_code, '')
		 AND d.is_active = true
		 AND d.deleted_at IS NULL
		LEFT JOIN cst_rm_group_head h
		  ON h.group_head_id = d.group_head_id
		 AND h.deleted_at IS NULL
	`

	countSQL := distinctCTE + "SELECT COUNT(*) " + joinClause + " " + where

	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count grouping monitor: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	orderBy := buildGroupingMonitorOrderBy(filter.SortBy, filter.SortOrder, filter.Scope)
	listSQL := distinctCTE + `
		SELECT s.item_code, s.grade_code, s.grade_name, s.item_name, s.uom,
		       d.group_head_id, h.group_code, h.group_name,
		       d.sort_order, d.created_at
	` + joinClause + " " + where +
		fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, idx, idx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list grouping monitor: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var items []*appgroup.GroupingMonitorItem
	for rows.Next() {
		item, scanErr := scanGroupingMonitorRow(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan grouping monitor row: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate grouping monitor rows: %w", err)
	}

	return items, total, nil
}

// buildGroupingMonitorOrderBy maps the application-layer sort key to a safe
// SQL fragment. Unknown keys fall back to item_code asc — never interpolate
// user input directly into SQL. Group-related keys are accepted only in
// Grouped scope; in Ungrouped scope they fall back to the default because
// d.* columns are NULL for those rows.
func buildGroupingMonitorOrderBy(sortBy, sortOrder string, scope appgroup.GroupingScope) string {
	dir := sortASC
	if strings.EqualFold(sortOrder, "desc") {
		dir = sortDESC
	}

	type colMap struct {
		expr   string
		needed appgroup.GroupingScope // 0 = always available
	}
	cols := map[string]colMap{
		"item_code":   {"s.item_code", appgroup.GroupingScopeUngrouped},
		"item_name":   {"s.item_name", appgroup.GroupingScopeUngrouped},
		"grade_code":  {"COALESCE(s.grade_code,'')", appgroup.GroupingScopeUngrouped},
		"item_grade":  {"s.grade_name", appgroup.GroupingScopeUngrouped},
		"uom_code":    {"s.uom", appgroup.GroupingScopeUngrouped},
		"group_code":  {"h.group_code", appgroup.GroupingScopeGrouped},
		"group_name":  {"h.group_name", appgroup.GroupingScopeGrouped},
		"sort_order":  {"d.sort_order", appgroup.GroupingScopeGrouped},
		"assigned_at": {"d.created_at", appgroup.GroupingScopeGrouped},
	}

	col, ok := cols[sortBy]
	requiresGrouped := ok && col.needed == appgroup.GroupingScopeGrouped
	if !ok || (requiresGrouped && scope != appgroup.GroupingScopeGrouped) {
		// Default: item_code, then grade_code as deterministic tiebreaker.
		return fmt.Sprintf("s.item_code %s, COALESCE(s.grade_code,'') %s", dir, dir)
	}

	// Always tiebreak by item_code+grade_code so the order is stable across
	// rows that share the sort key.
	if sortBy == "item_code" {
		return fmt.Sprintf("%s %s, COALESCE(s.grade_code,'') %s", col.expr, dir, dir)
	}
	return fmt.Sprintf("%s %s, s.item_code ASC, COALESCE(s.grade_code,'') ASC", col.expr, dir)
}

func scanGroupingMonitorRow(rows *sql.Rows) (*appgroup.GroupingMonitorItem, error) {
	var (
		itemCode    string
		gradeCode   sql.NullString
		gradeName   sql.NullString
		itemName    sql.NullString
		uom         sql.NullString
		groupHeadID sql.NullString
		groupCode   sql.NullString
		groupName   sql.NullString
		sortOrder   sql.NullInt32
		assignedAt  sql.NullTime
	)
	if err := rows.Scan(
		&itemCode, &gradeCode, &gradeName, &itemName, &uom,
		&groupHeadID, &groupCode, &groupName, &sortOrder, &assignedAt,
	); err != nil {
		return nil, err
	}
	out := &appgroup.GroupingMonitorItem{
		ItemCode:    itemCode,
		ItemName:    itemName.String,
		GradeCode:   gradeCode.String,
		GradeName:   gradeName.String,
		UOM:         uom.String,
		GroupHeadID: groupHeadID.String,
		GroupCode:   groupCode.String,
		GroupName:   groupName.String,
	}
	if sortOrder.Valid {
		out.SortOrder = sortOrder.Int32
	}
	if assignedAt.Valid {
		out.AssignedAt = assignedAt.Time.UTC().Format(time.RFC3339)
	}
	return out, nil
}
