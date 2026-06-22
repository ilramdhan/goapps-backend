package costbulkimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// rmBatch groups RMInput items by their crs_seq_id for batch replacement.
type rmBatch struct {
	seqID int64
	rms   []costroute.RMInput
}

// processRouteRM parses Sheet 6 ("route_rms") and performs a full replace
// (DELETE + re-INSERT) of RMs for each unique (head, level, seq) combination.
// Returns the count of seq nodes that had their RMs replaced, plus per-row errors.
func processRouteRM(
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costroute.Repository,
	actor string,
	_ time.Time,
) (seqsReplaced int, errs []SheetError, err error) {
	const sheetName = "route_rms"
	requiredHeaders := []string{
		"route_head_legacy_product_id", "route_level", "route_seq", "rm_type", "ratio",
	}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, nil, parseErr
	}

	// Group RM rows by seq_id using composite key to look up RouteSeqMap.
	batchMap := make(map[string]*rmBatch) // key = "headLegacyID:level:seq"
	batchOrder := make([]string, 0)       // preserve insertion order

	for i, row := range rows {
		rowNum := int32(i+2) //nolint:gosec // row count fits int32
		headLegacyID := row["route_head_legacy_product_id"]
		if headLegacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "route_head_legacy_product_id", Message: "required"})
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
		seqID, ok := maps.RouteSeqMap[compositeKey]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "route_head_legacy_product_id", Message: "seq node not found (key: " + compositeKey + ")"})
			continue
		}

		rmType := strings.ToUpper(strings.TrimSpace(row["rm_type"]))
		if rmType != costroute.RmTypeProduct && rmType != costroute.RmTypeItem && rmType != costroute.RmTypeGroup {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "rm_type", Message: "must be PRODUCT, ITEM, or GROUP"})
			continue
		}

		ratioStr := row["ratio"]
		ratio, ratioErr := strconv.ParseFloat(ratioStr, 64)
		if ratioErr != nil || ratio <= 0 {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "ratio", Message: "must be a positive number"})
			continue
		}

		rmInput, rmErr := buildRMInput(rowNum, rmType, ratio, row, maps)
		if rmErr != nil {
			errs = append(errs, *rmErr)
			continue
		}

		if _, exists := batchMap[compositeKey]; !exists {
			batchMap[compositeKey] = &rmBatch{seqID: seqID}
			batchOrder = append(batchOrder, compositeKey)
		}
		batchMap[compositeKey].rms = append(batchMap[compositeKey].rms, rmInput)
	}

	// Replace RMs for each seq node.
	for _, key := range batchOrder {
		batch := batchMap[key]
		if replaceErr := repo.BulkReplaceRMs(ctx, batch.seqID, batch.rms, actor); replaceErr != nil {
			return seqsReplaced, errs, fmt.Errorf("replace RMs for seq %d: %w", batch.seqID, replaceErr)
		}
		seqsReplaced++
	}
	return seqsReplaced, errs, nil
}

// buildRMInput validates RM type-specific reference fields and returns a populated RMInput.
func buildRMInput(rowNum int32, rmType string, ratio float64, row map[string]string, maps *ImportMaps) (costroute.RMInput, *SheetError) {
	inp := costroute.RMInput{
		RmType:      rmType,
		Ratio:       ratio,
		RmName:      row["rm_name"],
		RmShadeCode: row["rm_shade_code"],
		RmShadeName: row["rm_shade_name"],
		SubType:     row["sub_type"],
		Notes:       row["notes"],
	}

	switch rmType {
	case costroute.RmTypeProduct:
		rmLegacyID := row["rm_product_legacy_id"]
		if rmLegacyID == "" {
			return inp, &SheetError{RowNumber: rowNum, Field: "rm_product_legacy_id", Message: "required when rm_type=PRODUCT"}
		}
		productSysID, ok := maps.ProductMap[rmLegacyID]
		if !ok {
			return inp, &SheetError{RowNumber: rowNum, Field: "rm_product_legacy_id", Message: "product not found: " + rmLegacyID}
		}
		inp.RmProductSysID = productSysID
	case costroute.RmTypeItem:
		inp.RmItemCode = row["rm_item_code"]
		if inp.RmItemCode == "" {
			return inp, &SheetError{RowNumber: rowNum, Field: "rm_item_code", Message: "required when rm_type=ITEM"}
		}
	case costroute.RmTypeGroup:
		inp.RmGroupCode = row["rm_group_code"]
		if inp.RmGroupCode == "" {
			return inp, &SheetError{RowNumber: rowNum, Field: "rm_group_code", Message: "required when rm_type=GROUP"}
		}
	}
	return inp, nil
}
