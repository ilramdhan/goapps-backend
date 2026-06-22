package costbulkimport

import (
	"context"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// processProductMaster parses Sheet 1 ("product_master"), upserts products by
// legacy Oracle sys_id, and populates maps.ProductMap with the resulting IDs.
// Returns counts of rows inserted and updated, plus any per-row errors.
func processProductMaster(
	ctx context.Context,
	f *excelize.File,
	maps *ImportMaps,
	repo costproductmaster.Repository,
	actor string,
	_ time.Time,
) (inserted, updated int, errs []SheetError, err error) {
	const sheetName = "product_master"
	requiredHeaders := []string{
		"legacy_oracle_sys_id", "product_type_code", "product_name",
	}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return 0, 0, nil, parseErr
	}

	inputs := make([]costproductmaster.ProductUpsertInput, 0, len(rows))
	rowNums := make([]int32, 0, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"})
			continue
		}
		typeCode := row["product_type_code"]
		if typeCode == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "product_type_code", Message: "required"})
			continue
		}
		typeID, ok := maps.ProductTypeMap[typeCode]
		if !ok {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "product_type_code", Message: "unknown type code: " + typeCode})
			continue
		}
		productName := row["product_name"]
		if productName == "" {
			errs = append(errs, SheetError{RowNumber: rowNum, Field: "product_name", Message: "required"})
			continue
		}
		gradeCode := row["grade_code"]
		if gradeCode == "" {
			gradeCode = "AX"
		}
		isActive := true
		if v := strings.ToLower(row["is_active"]); v == "false" || v == "0" || v == "no" {
			isActive = false
		}
		inputs = append(inputs, costproductmaster.ProductUpsertInput{
			LegacySysID:   legacyID,
			ProductTypeID: typeID,
			ProductName:   productName,
			ShadeCode:     row["shade_code"],
			ShadeName:     row["shade_name"],
			GradeCode:     gradeCode,
			Description:   row["description"],
			ErpItemCode:   row["erp_item_code"],
			Flex01:        row["legacy_erp_compound_key"],
			Flex03:        row["legacy_type_label"],
			IsActive:      isActive,
		})
		rowNums = append(rowNums, rowNum)
	}

	if len(inputs) == 0 {
		return 0, 0, errs, nil
	}

	results, repoErr := repo.BulkUpsertByLegacyID(ctx, inputs, actor)
	if repoErr != nil {
		return 0, 0, errs, repoErr
	}

	for j, r := range results {
		maps.ProductMap[r.LegacySysID] = r.ProductSysID
		_ = rowNums[j] // index tracked for potential future per-row error mapping
		if r.WasInserted {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, errs, nil
}
