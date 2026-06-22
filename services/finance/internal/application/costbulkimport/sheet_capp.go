package costbulkimport

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// processCAP parses Sheet 3 ("product_applicable_params"), upserts CAPP rows.
// Returns counts of rows inserted and updated, plus any per-row errors.
func processCAP(
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costproductparameter.Repository,
	actor string,
	_ time.Time,
) (inserted, updated int, errs []SheetError, err error) {
	const sheetName = "product_applicable_params"
	requiredHeaders := []string{"legacy_oracle_sys_id", "param_code", "is_required"}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, 0, nil, parseErr
	}

	inputs := make([]costproductparameter.CAPPUpsertInput, 0, len(rows))
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"})
			continue
		}
		productSysID, ok := maps.ProductMap[legacyID]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "product not found: " + legacyID})
			continue
		}
		paramCode := row["param_code"]
		if paramCode == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "param_code", Message: "required"})
			continue
		}
		paramID, ok2 := maps.ParamMap[paramCode]
		if !ok2 {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "param_code", Message: "unknown param code: " + paramCode})
			continue
		}
		isRequired := strings.ToLower(row["is_required"]) == "true" || row["is_required"] == "1"

		displayOrder, doErr := parseCAPPDisplayOrder(rowNum, row["display_order"])
		if doErr != nil {
			errs = append(errs, *doErr)
			continue
		}

		inputs = append(inputs, costproductparameter.CAPPUpsertInput{
			ProductSysID: productSysID,
			ParamID:      paramID,
			IsRequired:   isRequired,
			DisplayOrder: displayOrder,
		})
	}

	if len(inputs) == 0 {
		return 0, 0, errs, nil
	}

	ins, upd, repoErr := repo.BulkUpsertApplicable(ctx, inputs, actor)
	if repoErr != nil {
		return 0, 0, errs, repoErr
	}
	return ins, upd, errs, nil
}

// parseCAPPDisplayOrder parses an optional display_order string into *int32.
// Returns nil when doStr is empty. Returns a SheetError on invalid or out-of-range input.
func parseCAPPDisplayOrder(rowNum int32, doStr string) (*int32, *SheetError) {
	if doStr == "" {
		return nil, nil
	}
	doVal, parseErr := strconv.ParseInt(doStr, 10, 64)
	if parseErr != nil {
		return nil, &SheetError{RowNumber: rowNum, Field: "display_order", Message: "invalid integer: " + doStr}
	}
	if doVal < math.MinInt32 || doVal > math.MaxInt32 {
		return nil, &SheetError{RowNumber: rowNum, Field: "display_order", Message: "value out of range"}
	}
	v := int32(doVal) //nolint:gosec // bounds checked above
	return &v, nil
}
