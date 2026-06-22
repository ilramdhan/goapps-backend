package costbulkimport

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// processCPP parses Sheet 2 ("product_parameters"), upserts CPP value rows.
// Exactly one of value_numeric, value_text, value_flag must be non-empty per row.
// Returns counts of rows inserted and updated, plus any per-row errors.
func processCPP(
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costproductparameter.Repository,
	actor string,
	now time.Time,
) (inserted, updated int, errs []SheetError, err error) {
	const sheetName = "product_parameters"
	requiredHeaders := []string{"legacy_oracle_sys_id", "param_code", "data_type"}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, 0, nil, parseErr
	}

	inputs := make([]costproductparameter.CPPUpsertInput, 0, len(rows))
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"})
			continue
		}
		productSysID, ok := maps.ProductMap[legacyID]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "product not found in ProductMap: " + legacyID})
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

		inp, parseValErr := parseCPPValue(rowNum, row)
		if parseValErr != nil {
			errs = append(errs, *parseValErr)
			continue
		}
		inp.ProductSysID = productSysID
		inp.ParamID = paramID
		inp.FilledAt = now
		inp.FilledBy = actor
		inputs = append(inputs, inp)
	}

	if len(inputs) == 0 {
		return 0, 0, errs, nil
	}

	ins, upd, repoErr := repo.BulkUpsertValues(ctx, inputs, actor)
	if repoErr != nil {
		return 0, 0, errs, repoErr
	}
	return ins, upd, errs, nil
}

// parseCPPValue validates that exactly one value column is set and returns
// the partially-filled CPPUpsertInput. Returns a SheetError on validation failure.
func parseCPPValue(rowNum int32, row map[string]string) (costproductparameter.CPPUpsertInput, *SheetError) {
	var inp costproductparameter.CPPUpsertInput
	numStr := strings.TrimSpace(row["value_numeric"])
	txtStr := strings.TrimSpace(row["value_text"])
	flagStr := strings.TrimSpace(row["value_flag"])
	filled := 0
	if numStr != "" {
		filled++
	}
	if txtStr != "" {
		filled++
	}
	if flagStr != "" {
		filled++
	}
	if filled == 0 {
		return inp, &SheetError{RowNumber: rowNum, Field: "value_numeric/value_text/value_flag", Message: "at least one value column must be set"}
	}
	if filled > 1 {
		return inp, &SheetError{RowNumber: rowNum, Field: "value_numeric/value_text/value_flag", Message: "exactly one value column must be set"}
	}
	if numStr != "" {
		v, parseErr := strconv.ParseFloat(numStr, 64)
		if parseErr != nil {
			return inp, &SheetError{RowNumber: rowNum, Field: "value_numeric", Message: "invalid number: " + numStr}
		}
		inp.ValueNumeric = &v
	}
	if txtStr != "" {
		s := txtStr
		inp.ValueText = &s
	}
	if flagStr != "" {
		lower := strings.ToLower(flagStr)
		b := lower == boolTrueStr || lower == "1" || lower == "yes"
		inp.ValueFlag = &b
	}
	return inp, nil
}
