package costbulkimport

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// preValidateAll parses all six sheets and validates every row without writing
// to the database. Cross-sheet references are checked using in-memory sets built
// from the file itself, so orphaned rows are caught before the write phase.
//
// No error limit is applied — all errors are returned so the caller can build a
// complete error report (including the "missing_param_codes" summary sheet).
func preValidateAll(f *excelize.File, maps *ImportMaps) []SheetResult { //nolint:gocognit,gocyclo // sequential validation pipeline
	// Sheet 1 — product_master.
	s1, inProducts := preflightProductMaster(f, maps)

	// Sheets 2+3 — param sheets (need inProducts for cross-ref).
	s2 := preflightParamSheet(f, maps, inProducts, "product_parameters", []string{"legacy_oracle_sys_id", "param_code", "data_type"})
	s3 := preflightParamSheet(f, maps, inProducts, "product_applicable_params", []string{"legacy_oracle_sys_id", "param_code", "is_required"})

	// Sheet 4 — route_head (cross-ref to inProducts); build inHeads set.
	s4, inHeads := preflightRouteHead(f, inProducts)

	// Sheet 5 — route_sequences (cross-ref to inHeads + inProducts); build inSeqs set.
	s5, inSeqs := preflightRouteSeq(f, inHeads, inProducts)

	// Sheet 6 — route_rms (cross-ref to inSeqs).
	s6 := preflightRouteRM(f, inSeqs, inProducts)

	return []SheetResult{s1, s2, s3, s4, s5, s6}
}

// preflightProductMaster validates product_master rows and returns the set of
// all valid legacy_oracle_sys_id values found in the sheet.
func preflightProductMaster(f *excelize.File, maps *ImportMaps) (SheetResult, map[string]struct{}) {
	const sheetName = "product_master"
	rows, parseErr := ParseSheet(f, sheetName, []string{"legacy_oracle_sys_id", "product_type_code", "product_name"})
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr), nil
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	inProducts := make(map[string]struct{}, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, "legacy_oracle_sys_id", "required"})
			continue
		}
		typeCode := row["product_type_code"]
		if typeCode == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, "product_type_code", "required"})
			continue
		}
		if _, ok := maps.ProductTypeMap[typeCode]; !ok {
			result.Errors = append(result.Errors, SheetError{rowNum, "product_type_code", "unknown product type: " + typeCode})
			continue
		}
		if row["product_name"] == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, "product_name", "required"})
			continue
		}
		inProducts[legacyID] = struct{}{}
	}
	return result, inProducts
}

// preflightParamSheet validates a param sheet (CPP or CAPP).
// Checks legacy_oracle_sys_id against inProducts and param_code against ParamMap.
func preflightParamSheet(
	f *excelize.File,
	maps *ImportMaps,
	inProducts map[string]struct{},
	sheetName string,
	requiredHeaders []string,
) SheetResult {
	rows, parseErr := ParseSheetOptional(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr)
	}
	// Sheet absent or empty — skip silently (param sheets are optional).
	if len(rows) == 0 {
		return SheetResult{SheetName: sheetName}
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, "legacy_oracle_sys_id", "required"})
			continue
		}
		if inProducts != nil {
			if _, ok := inProducts[legacyID]; !ok {
				result.Errors = append(result.Errors, SheetError{rowNum, "legacy_oracle_sys_id", "product not found in product_master sheet: " + legacyID})
				continue
			}
		}
		paramCode := row["param_code"]
		if paramCode == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, "param_code", "required"})
			continue
		}
		if _, ok := maps.ParamMap[paramCode]; !ok {
			result.Errors = append(result.Errors, SheetError{rowNum, "param_code", unknownParamPrefix + paramCode})
		}
	}
	return result
}

// preflightRouteHead validates route_head rows (cross-ref to inProducts) and
// returns the set of all valid legacy_oracle_sys_id values in the sheet.
func preflightRouteHead(f *excelize.File, inProducts map[string]struct{}) (SheetResult, map[string]struct{}) {
	const sheetName = "route_head"
	rows, parseErr := ParseSheet(f, sheetName, []string{legacyOracleSysIDField})
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr), nil
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	inHeads := make(map[string]struct{}, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row[legacyOracleSysIDField]
		if legacyID == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, legacyOracleSysIDField, "required"})
			continue
		}
		if inProducts != nil {
			if _, ok := inProducts[legacyID]; !ok {
				result.Errors = append(result.Errors, SheetError{rowNum, legacyOracleSysIDField, "product not found in product_master sheet: " + legacyID})
				continue
			}
		}
		inHeads[legacyID] = struct{}{}
	}
	return result, inHeads
}

// preflightRouteSeq validates route_sequences rows and returns the set of all
// valid composite keys (headLegacyID:level:seq) for use in route_rms validation.
func preflightRouteSeq(
	f *excelize.File,
	inHeads map[string]struct{},
	inProducts map[string]struct{},
) (SheetResult, map[string]struct{}) {
	const sheetName = "route_sequences"
	rows, parseErr := ParseSheet(f, sheetName, []string{routeHeadLegacyIDField, nodeProductLegacyIDField, "route_level", "route_seq"})
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr), nil
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	inSeqs := make(map[string]struct{}, len(rows))

	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		headID := row[routeHeadLegacyIDField]
		if headID == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, routeHeadLegacyIDField, "required"})
			continue
		}
		if inHeads != nil {
			if _, ok := inHeads[headID]; !ok {
				result.Errors = append(result.Errors, SheetError{rowNum, routeHeadLegacyIDField, "route head not found in route_head sheet: " + headID})
				continue
			}
		}
		nodeID := row[nodeProductLegacyIDField]
		if nodeID == "" {
			result.Errors = append(result.Errors, SheetError{rowNum, nodeProductLegacyIDField, "required"})
			continue
		}
		if inProducts != nil {
			if _, ok := inProducts[nodeID]; !ok {
				result.Errors = append(result.Errors, SheetError{rowNum, nodeProductLegacyIDField, "product not found in product_master sheet: " + nodeID})
				continue
			}
		}
		level, levelErr := strconv.Atoi(row["route_level"])
		if levelErr != nil || level < 1 {
			result.Errors = append(result.Errors, SheetError{rowNum, "route_level", "must be a positive integer"})
			continue
		}
		seq, seqErr := strconv.Atoi(row["route_seq"])
		if seqErr != nil || seq < 1 {
			result.Errors = append(result.Errors, SheetError{rowNum, "route_seq", "must be a positive integer"})
			continue
		}
		inSeqs[fmt.Sprintf("%s:%d:%d", headID, level, seq)] = struct{}{}
	}
	return result, inSeqs
}

// preflightRouteRM validates route_rms rows against inSeqs and inProducts.
func preflightRouteRM(f *excelize.File, inSeqs map[string]struct{}, inProducts map[string]struct{}) SheetResult {
	const sheetName = "route_rms"
	rows, parseErr := ParseSheet(f, sheetName, []string{routeHeadLegacyIDField, "route_level", "route_seq", "rm_type", "ratio"})
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr)
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		if e := preflightRMRow(rowNum, row, inSeqs, inProducts); e != nil {
			result.Errors = append(result.Errors, *e)
		}
	}
	return result
}

// preflightRMRow validates a single route_rms row. Returns the first error found.
func preflightRMRow(rowNum int32, row map[string]string, inSeqs map[string]struct{}, inProducts map[string]struct{}) *SheetError {
	headID := row[routeHeadLegacyIDField]
	if headID == "" {
		return &SheetError{rowNum, routeHeadLegacyIDField, "required"}
	}
	level, levelErr := strconv.Atoi(row["route_level"])
	if levelErr != nil || level < 1 {
		return &SheetError{rowNum, "route_level", "must be a positive integer"}
	}
	seq, seqErr := strconv.Atoi(row["route_seq"])
	if seqErr != nil || seq < 1 {
		return &SheetError{rowNum, "route_seq", "must be a positive integer"}
	}
	if inSeqs != nil {
		key := fmt.Sprintf("%s:%d:%d", headID, level, seq)
		if _, ok := inSeqs[key]; !ok {
			return &SheetError{rowNum, routeHeadLegacyIDField, "sequence not found in route_sequences sheet: " + key}
		}
	}
	rmType := row["rm_type"]
	if rmType == "" {
		return &SheetError{rowNum, "rm_type", "required"}
	}
	if row["ratio"] == "" {
		return &SheetError{rowNum, "ratio", "required"}
	}
	if rmType == "PRODUCT" {
		rmProductID := row["rm_product_legacy_id"]
		if rmProductID == "" {
			return &SheetError{rowNum, "rm_product_legacy_id", "required when rm_type=PRODUCT"}
		}
		if inProducts != nil {
			if _, ok := inProducts[rmProductID]; !ok {
				return &SheetError{rowNum, "rm_product_legacy_id", "product not found in product_master sheet: " + rmProductID}
			}
		}
	}
	return nil
}

// sheetErrResult creates a SheetResult for a parse-level failure (missing sheet / header).
func sheetErrResult(sheetName string, parseErr error) SheetResult {
	return SheetResult{
		SheetName: sheetName,
		Errors:    []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}},
	}
}

