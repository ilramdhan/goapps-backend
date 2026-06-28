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
// Uses the same cross-sheet FK checks as the actual import (preValidateAll) so
// the modal result is always consistent with what will happen when the user
// clicks Import. At most maxSampleErrors errors are shown per sheet in the
// modal response (the full set is in the error report on failure).
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

	// Run preValidateAll in a goroutine so the caller's context deadline is honoured.
	// preValidateAll is CPU-bound and does not accept a context; the goroutine may outlive
	// a ctx cancellation, but it is bounded and will finish on its own.
	type sheetResults struct{ sheets []SheetResult }
	ch := make(chan sheetResults, 1)
	go func() { ch <- sheetResults{preValidateAll(f, maps)} }()

	var allSheets []SheetResult
	select {
	case r := <-ch:
		allSheets = r.sheets
	case <-ctx.Done():
		return nil, fmt.Errorf("validation timed out — file may be too large; import directly to get the full error report")
	}

	// Cap errors per sheet for the modal response.
	result := &ValidateResult{IsValid: true}
	for _, s := range allSheets {
		capped := s
		if len(s.Errors) > maxSampleErrors {
			capped.Errors = s.Errors[:maxSampleErrors]
		}
		result.Sheets = append(result.Sheets, capped)
		if len(s.Errors) > 0 {
			result.IsValid = false
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

// appendIfUnderLimit appends a SheetError only if len(errs) < limit.
// Retained here for use by package tests.
//
//nolint:unparam // limit parameterized for testability
func appendIfUnderLimit(errs []SheetError, e SheetError, limit int) []SheetError {
	if len(errs) < limit {
		return append(errs, e)
	}
	return errs
}
