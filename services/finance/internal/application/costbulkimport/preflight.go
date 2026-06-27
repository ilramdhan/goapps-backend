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

	// Sheet 6 — route_rms (cross-ref to inSeqs; validates rm_group_code against RmGroupMap).
	s6 := preflightRouteRM(f, inSeqs, inProducts, maps)

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
		if e := validateParamRow(rowNum, row, inProducts, maps); e != nil {
			result.Errors = append(result.Errors, *e)
		}
	}
	return result
}

// validateParamRow validates a single param sheet row.
// Returns the first error found, or nil when the row is valid.
func validateParamRow(rowNum int32, row map[string]string, inProducts map[string]struct{}, maps *ImportMaps) *SheetError {
	legacyID := row["legacy_oracle_sys_id"]
	if legacyID == "" {
		return &SheetError{rowNum, "legacy_oracle_sys_id", "required"}
	}
	if inProducts != nil {
		if _, ok := inProducts[legacyID]; !ok {
			return &SheetError{rowNum, "legacy_oracle_sys_id", "product not found in product_master sheet: " + legacyID}
		}
	}
	paramCode := row["param_code"]
	if paramCode == "" {
		return &SheetError{rowNum, "param_code", "required"}
	}
	if _, ok := maps.ParamMap[paramCode]; !ok {
		return &SheetError{rowNum, "param_code", unknownParamPrefix + paramCode}
	}
	// Validate MASTER_LOOKUP values against the referenced master table.
	if errMsg := validateMasterLookupValue(row, paramCode, maps); errMsg != "" {
		return &SheetError{rowNum, "value_text", errMsg}
	}
	return nil
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
		key, e := validateRouteSeqRow(rowNum, row, inHeads, inProducts)
		if e != nil {
			result.Errors = append(result.Errors, *e)
			continue
		}
		inSeqs[key] = struct{}{}
	}
	return result, inSeqs
}

// validateRouteSeqRow validates a single route_sequences row.
// On success it returns the composite key "headID:level:seq" and nil.
// On failure it returns "" and the first error found.
func validateRouteSeqRow(rowNum int32, row map[string]string, inHeads, inProducts map[string]struct{}) (string, *SheetError) {
	headID := row[routeHeadLegacyIDField]
	if headID == "" {
		return "", &SheetError{rowNum, routeHeadLegacyIDField, "required"}
	}
	if inHeads != nil {
		if _, ok := inHeads[headID]; !ok {
			return "", &SheetError{rowNum, routeHeadLegacyIDField, "route head not found in route_head sheet: " + headID}
		}
	}
	nodeID := row[nodeProductLegacyIDField]
	if nodeID == "" {
		return "", &SheetError{rowNum, nodeProductLegacyIDField, "required"}
	}
	if inProducts != nil {
		if _, ok := inProducts[nodeID]; !ok {
			return "", &SheetError{rowNum, nodeProductLegacyIDField, "product not found in product_master sheet: " + nodeID}
		}
	}
	level, levelErr := strconv.Atoi(row["route_level"])
	if levelErr != nil || level < 1 {
		return "", &SheetError{rowNum, "route_level", "must be a positive integer"}
	}
	seq, seqErr := strconv.Atoi(row["route_seq"])
	if seqErr != nil || seq < 1 {
		return "", &SheetError{rowNum, "route_seq", "must be a positive integer"}
	}
	return fmt.Sprintf("%s:%d:%d", headID, level, seq), nil
}

// preflightRouteRM validates route_rms rows against inSeqs, inProducts, and maps.RmGroupMap.
func preflightRouteRM(f *excelize.File, inSeqs map[string]struct{}, inProducts map[string]struct{}, maps *ImportMaps) SheetResult {
	const sheetName = "route_rms"
	rows, parseErr := ParseSheet(f, sheetName, []string{routeHeadLegacyIDField, "route_level", "route_seq", "rm_type", "ratio"})
	if parseErr != nil {
		return sheetErrResult(sheetName, parseErr)
	}

	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		if e := preflightRMRow(rowNum, row, inSeqs, inProducts, maps); e != nil {
			result.Errors = append(result.Errors, *e)
		}
	}
	return result
}

// preflightRMRow validates a single route_rms row. Returns the first error found.
func preflightRMRow(rowNum int32, row map[string]string, inSeqs map[string]struct{}, inProducts map[string]struct{}, maps *ImportMaps) *SheetError { //nolint:gocognit,gocyclo // sequential single-row validation — splitting would obscure the linear field-check flow
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
			return &SheetError{rowNum, "route_level:route_seq", "sequence not found in route_sequences sheet for key " + key + " — add row (route_head=" + headID + " level=" + row["route_level"] + " seq=" + row["route_seq"] + ") to route_sequences"}
		}
	}
	rmType := row["rm_type"]
	if rmType == "" {
		return &SheetError{rowNum, "rm_type", "required"}
	}
	if row["ratio"] == "" {
		return &SheetError{rowNum, "ratio", "required"}
	}
	switch rmType {
	case "PRODUCT":
		rmProductID := row["rm_product_legacy_id"]
		if rmProductID == "" {
			return &SheetError{rowNum, "rm_product_legacy_id", "required when rm_type=PRODUCT"}
		}
		if inProducts != nil {
			if _, ok := inProducts[rmProductID]; !ok {
				return &SheetError{rowNum, "rm_product_legacy_id", "product not found in product_master sheet: " + rmProductID}
			}
		}
	case "GROUP":
		groupCode := row["rm_group_code"]
		if groupCode == "" {
			// Empty GROUP rows (no group_code, no item_code) are Oracle placeholder rows.
			// They are skipped silently during import — not a hard error.
			return nil
		}
		if maps != nil && len(maps.RmGroupMap) > 0 {
			if !maps.RmGroupMap[groupCode] {
				return &SheetError{rowNum, "rm_group_code", "unknown rm group code: " + groupCode + " — create it in Finance > Master > RM Groups first"}
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

// validateMasterLookupValue checks that a MASTER_LOOKUP param's value exists
// in the referenced master table option set. Returns an error sentinel string
// or "" if the value is valid / no master validation applies.
func validateMasterLookupValue(row map[string]string, paramCode string, maps *ImportMaps) string {
	masterCode, isMasterLookup := maps.ParamLookupMap[paramCode]
	if !isMasterLookup {
		return ""
	}
	optSet, loaded := maps.MasterLookupValues[masterCode]
	if !loaded || len(optSet) == 0 {
		return ""
	}
	value := row["value_text"]
	if value == "" {
		value = row["value_numeric"]
	}
	if value != "" && !optSet[value] {
		return unknownMasterValuePrefix + masterCode + ":" + value
	}
	return ""
}
