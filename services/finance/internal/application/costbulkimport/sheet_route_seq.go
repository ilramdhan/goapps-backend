package costbulkimport

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// processRouteSeq parses Sheet 5 ("route_sequences"), upserts route sequence rows,
// and populates maps.RouteSeqMap with composite key → crs_seq_id.
// Returns counts of rows inserted and updated, plus per-row errors.
func processRouteSeq( //nolint:gocognit // cohesive row-validation pipeline
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costroute.Repository,
	actor string,
	_ time.Time,
) (inserted, updated int, errs []SheetError, err error) {
	const sheetName = "route_sequences"
	requiredHeaders := []string{
		routeHeadLegacyIDField, nodeProductLegacyIDField,
		"route_level", "route_seq",
	}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, 0, nil, parseErr
	}

	inputs := make([]costroute.SeqUpsertInput, 0, len(rows))
	compositeKeys := make([]string, 0, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		headLegacyID := row[routeHeadLegacyIDField]
		if headLegacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: routeHeadLegacyIDField, Message: "required"})
			continue
		}
		headID, ok := maps.RouteHeadMap[headLegacyID]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: routeHeadLegacyIDField, Message: "head not found (route may be LOCKED or missing): " + headLegacyID})
			continue
		}
		nodeLegacyID := row[nodeProductLegacyIDField]
		if nodeLegacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: nodeProductLegacyIDField, Message: "required"})
			continue
		}
		nodeProductSysID, ok2 := maps.ProductMap[nodeLegacyID]
		if !ok2 {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: nodeProductLegacyIDField, Message: "product not found: " + nodeLegacyID})
			continue
		}

		routeLevel, levelErr := parseInt32(rowNum, "route_level", row["route_level"])
		if levelErr != nil {
			errs = append(errs, *levelErr)
			continue
		}
		routeSeq, seqErr := parseInt32(rowNum, "route_seq", row["route_seq"])
		if seqErr != nil {
			errs = append(errs, *seqErr)
			continue
		}

		compositeKey := fmt.Sprintf("%s:%d:%d", headLegacyID, routeLevel, routeSeq)
		inputs = append(inputs, costroute.SeqUpsertInput{
			HeadLegacySysID:  headLegacyID,
			HeadID:           headID,
			NodeProductSysID: nodeProductSysID,
			RouteLevel:       routeLevel,
			RouteSeq:         routeSeq,
			RouteName:        row["route_name"],
			RouteItemCode:    row["route_item_code"],
			RouteShadeCode:   row["route_shade_code"],
			RouteShadeName:   row["route_shade_name"],
		})
		compositeKeys = append(compositeKeys, compositeKey)
	}

	if len(inputs) == 0 {
		return 0, 0, errs, nil
	}

	// Snapshot which keys already exist before the upsert to distinguish inserts from updates.
	preExisting := make(map[string]bool, len(compositeKeys))
	for _, k := range compositeKeys {
		if _, exists := maps.RouteSeqMap[k]; exists {
			preExisting[k] = true
		}
	}

	results, repoErr := repo.BulkUpsertSeqs(ctx, inputs, actor)
	if repoErr != nil {
		return 0, 0, errs, repoErr
	}

	for j, r := range results {
		key := compositeKeys[j]
		maps.RouteSeqMap[key] = r.SeqID
		if preExisting[key] {
			updated++
		} else {
			inserted++
		}
	}
	return inserted, updated, errs, nil
}

// parseInt32 parses a decimal string as int32, returning a SheetError on failure.
// Returns an error when the string is empty, not a valid integer, or out of int32 range.
func parseInt32(rowNum int32, field, s string) (int32, *SheetError) {
	if s == "" {
		return 0, &SheetError{RowNumber: rowNum, Field: field, Message: "required"}
	}
	v, parseErr := strconv.ParseInt(s, 10, 64)
	if parseErr != nil {
		return 0, &SheetError{RowNumber: rowNum, Field: field, Message: "invalid integer: " + s}
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, &SheetError{RowNumber: rowNum, Field: field, Message: "value out of int32 range"}
	}
	return int32(v), nil //nolint:gosec // bounds checked above
}
