package costbulkimport

import (
	"context"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// processRouteHead parses Sheet 4 ("route_head"), upserts route heads, and
// populates maps.RouteHeadMap with legacySysId → crh_head_id.
// Skipped rows (LOCKED routes) are counted and returned in errs as warnings.
// Returns counts of rows inserted, updated, and skipped.
func processRouteHead(
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costroute.Repository,
	actor string,
	_ time.Time,
) (inserted, updated, skipped int, errs []SheetError, err error) {
	const sheetName = "route_head"
	requiredHeaders := []string{legacyOracleSysIDField}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, 0, 0, nil, parseErr
	}

	inputs := make([]costroute.HeadUpsertInput, 0, len(rows))
	rowNums := make([]int32, 0, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row[legacyOracleSysIDField]
		if legacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: legacyOracleSysIDField, Message: "required"})
			continue
		}
		productSysID, ok := maps.ProductMap[legacyID]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: legacyOracleSysIDField, Message: "product not found in ProductMap: " + legacyID})
			continue
		}
		routingStatus := row["routing_status"]
		if routingStatus == "" {
			routingStatus = costroute.StatusDraft
		}
		inputs = append(inputs, costroute.HeadUpsertInput{
			LegacySysID:   legacyID,
			ProductSysID:  productSysID,
			RoutingStatus: routingStatus,
			Notes:         row["notes"],
		})
		rowNums = append(rowNums, rowNum)
	}

	if len(inputs) == 0 {
		return 0, 0, 0, errs, nil
	}

	results, repoErr := repo.BulkUpsertHeads(ctx, inputs, actor)
	if repoErr != nil {
		return 0, 0, 0, errs, repoErr
	}

	for j, r := range results {
		if r.Skipped {
			skipped++
			errs = append(errs, SheetError{
				RowNumber: rowNums[j],
				Field:     "routing_status",
				Message:   "route is LOCKED — skipped: " + r.LegacySysID,
			})
			continue
		}
		maps.RouteHeadMap[r.LegacySysID] = r.HeadID
		if r.WasInserted {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, skipped, errs, nil
}
