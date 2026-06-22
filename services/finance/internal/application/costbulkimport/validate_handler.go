package costbulkimport

import (
	"bytes"
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

const maxSampleErrors = 20

// ValidateHandler performs a synchronous dry-run validation of a bulk import file.
// It parses all 6 sheets and returns per-sheet validation results without writing to the database.
type ValidateHandler struct {
	cppRepo  costproductparameter.Repository
	typeRepo costproducttype.Repository
}

// NewValidateHandler creates a new ValidateHandler.
func NewValidateHandler(
	cppRepo costproductparameter.Repository,
	typeRepo costproducttype.Repository,
) *ValidateHandler {
	return &ValidateHandler{cppRepo: cppRepo, typeRepo: typeRepo}
}

// ValidateResult holds per-sheet dry-run validation results.
type ValidateResult struct {
	IsValid bool
	Sheets  []SheetResult
}

// Validate parses and validates the file without writing to the database.
// At most 20 sample errors are returned per sheet.
func (h *ValidateHandler) Validate(ctx context.Context, fileContent []byte) (*ValidateResult, error) {
	f, openErr := excelize.OpenReader(bytes.NewReader(fileContent))
	if openErr != nil {
		return nil, fmt.Errorf("open file: %w", openErr)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	maps, mapsErr := h.loadMaps(ctx)
	if mapsErr != nil {
		return nil, mapsErr
	}

	result := &ValidateResult{IsValid: true}
	result.Sheets = append(result.Sheets, h.validateProductMaster(f, maps))
	result.Sheets = append(result.Sheets, h.validateCPP(f, maps))
	result.Sheets = append(result.Sheets, h.validateCAP(f, maps))
	result.Sheets = append(result.Sheets, h.validateRouteHead(f, maps))
	result.Sheets = append(result.Sheets, h.validateRouteSeq(f, maps))
	result.Sheets = append(result.Sheets, h.validateRouteRM(f, maps))

	for _, s := range result.Sheets {
		if len(s.Errors) > 0 {
			result.IsValid = false
			break
		}
	}
	return result, nil
}

// loadMaps preloads ParamMap and ProductTypeMap for validation lookups.
func (h *ValidateHandler) loadMaps(ctx context.Context) (*ImportMaps, error) {
	maps := NewImportMaps()
	params, err := h.cppRepo.ListAllParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("load param map: %w", err)
	}
	for _, p := range params {
		maps.ParamMap[p.ParamCode] = p.ParamID
	}
	types, err := h.typeRepo.ListAllActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("load type map: %w", err)
	}
	for _, t := range types {
		maps.ProductTypeMap[t.TypeCode()] = t.TypeID()
	}
	return maps, nil
}

// validateProductMaster checks required headers and required fields in product_master sheet.
func (h *ValidateHandler) validateProductMaster(f *excelize.File, maps *ImportMaps) SheetResult {
	const sheetName = "product_master"
	rows, parseErr := ParseSheet(f, sheetName, []string{"legacy_oracle_sys_id", "product_type_code", "product_name"})
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		legacyID := row["legacy_oracle_sys_id"]
		if legacyID == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"}, maxSampleErrors)
			continue
		}
		typeCode := row["product_type_code"]
		if typeCode == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "product_type_code", Message: "required"}, maxSampleErrors)
			continue
		}
		if _, ok := maps.ProductTypeMap[typeCode]; !ok {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "product_type_code", Message: "unknown: " + typeCode}, maxSampleErrors)
		}
		if row["product_name"] == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "product_name", Message: "required"}, maxSampleErrors)
		}
	}
	return result
}

// validateCPP checks required fields in product_parameters sheet.
func (h *ValidateHandler) validateCPP(f *excelize.File, maps *ImportMaps) SheetResult {
	const sheetName = "product_parameters"
	rows, parseErr := ParseSheet(f, sheetName, []string{"legacy_oracle_sys_id", "param_code", "data_type"})
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		if row["legacy_oracle_sys_id"] == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"}, maxSampleErrors)
			continue
		}
		paramCode := row["param_code"]
		if paramCode == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "param_code", Message: "required"}, maxSampleErrors)
			continue
		}
		if _, ok := maps.ParamMap[paramCode]; !ok {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "param_code", Message: "unknown: " + paramCode}, maxSampleErrors)
		}
	}
	return result
}

// validateCAP checks required fields in product_applicable_params sheet.
func (h *ValidateHandler) validateCAP(f *excelize.File, maps *ImportMaps) SheetResult {
	const sheetName = "product_applicable_params"
	rows, parseErr := ParseSheet(f, sheetName, []string{"legacy_oracle_sys_id", "param_code", "is_required"})
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		if row["legacy_oracle_sys_id"] == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "legacy_oracle_sys_id", Message: "required"}, maxSampleErrors)
			continue
		}
		paramCode := row["param_code"]
		if paramCode == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "param_code", Message: "required"}, maxSampleErrors)
			continue
		}
		if _, ok := maps.ParamMap[paramCode]; !ok {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: "param_code", Message: "unknown: " + paramCode}, maxSampleErrors)
		}
	}
	return result
}

// validateRouteHead checks required fields in route_head sheet.
func (h *ValidateHandler) validateRouteHead(f *excelize.File, _ *ImportMaps) SheetResult {
	const sheetName = "route_head"
	rows, parseErr := ParseSheet(f, sheetName, []string{legacyOracleSysIDField})
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		if row[legacyOracleSysIDField] == "" {
			result.Errors = appendIfUnderLimit(result.Errors, SheetError{RowNumber: rowNum, Field: legacyOracleSysIDField, Message: "required"}, maxSampleErrors)
		}
	}
	return result
}

// validateRouteSeq checks required fields in route_sequences sheet.
func (h *ValidateHandler) validateRouteSeq(f *excelize.File, _ *ImportMaps) SheetResult {
	const sheetName = "route_sequences"
	requiredHeaders := []string{routeHeadLegacyIDField, nodeProductLegacyIDField, "route_level", "route_seq"}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		result.Errors = validateRouteSeqRow(rowNum, row, result.Errors)
	}
	return result
}

// validateRouteSeqRow validates a single route_sequences row and appends errors.
func validateRouteSeqRow(rowNum int32, row map[string]string, errs []SheetError) []SheetError {
	if row[routeHeadLegacyIDField] == "" {
		return appendIfUnderLimit(errs, SheetError{RowNumber: rowNum, Field: routeHeadLegacyIDField, Message: "required"}, maxSampleErrors)
	}
	if row[nodeProductLegacyIDField] == "" {
		return appendIfUnderLimit(errs, SheetError{RowNumber: rowNum, Field: nodeProductLegacyIDField, Message: "required"}, maxSampleErrors)
	}
	if _, levelErr := parseInt32(rowNum, "route_level", row["route_level"]); levelErr != nil {
		return appendIfUnderLimit(errs, *levelErr, maxSampleErrors)
	}
	if _, seqErr := parseInt32(rowNum, "route_seq", row["route_seq"]); seqErr != nil {
		return appendIfUnderLimit(errs, *seqErr, maxSampleErrors)
	}
	return errs
}

// validateRouteRM checks required fields in route_rms sheet.
func (h *ValidateHandler) validateRouteRM(f *excelize.File, _ *ImportMaps) SheetResult {
	const sheetName = "route_rms"
	requiredHeaders := []string{routeHeadLegacyIDField, "route_level", "route_seq", "rm_type", "ratio"}
	rows, parseErr := ParseSheet(f, sheetName, requiredHeaders)
	if parseErr != nil {
		return SheetResult{SheetName: sheetName, Errors: []SheetError{{RowNumber: 1, Field: "sheet", Message: parseErr.Error()}}}
	}
	result := SheetResult{SheetName: sheetName, TotalRows: len(rows)}
	for i, row := range rows {
		rowNum := int32(i + 2) //nolint:gosec // row count fits int32
		result.Errors = validateRouteRMRow(rowNum, row, result.Errors)
	}
	return result
}

// validateRouteRMRow validates a single route_rms row and appends errors.
func validateRouteRMRow(rowNum int32, row map[string]string, errs []SheetError) []SheetError {
	if row[routeHeadLegacyIDField] == "" {
		return appendIfUnderLimit(errs, SheetError{RowNumber: rowNum, Field: routeHeadLegacyIDField, Message: "required"}, maxSampleErrors)
	}
	if _, levelErr := parseInt32(rowNum, "route_level", row["route_level"]); levelErr != nil {
		return appendIfUnderLimit(errs, *levelErr, maxSampleErrors)
	}
	if _, seqErr := parseInt32(rowNum, "route_seq", row["route_seq"]); seqErr != nil {
		return appendIfUnderLimit(errs, *seqErr, maxSampleErrors)
	}
	if row["rm_type"] == "" {
		return appendIfUnderLimit(errs, SheetError{RowNumber: rowNum, Field: "rm_type", Message: "required"}, maxSampleErrors)
	}
	if row["ratio"] == "" {
		return appendIfUnderLimit(errs, SheetError{RowNumber: rowNum, Field: "ratio", Message: "required"}, maxSampleErrors)
	}
	return errs
}

// appendIfUnderLimit appends a SheetError to errs only if len(errs) < limit.
//
//nolint:unparam // limit parameterized for testability
func appendIfUnderLimit(errs []SheetError, e SheetError, limit int) []SheetError {
	if len(errs) < limit {
		return append(errs, e)
	}
	return errs
}
