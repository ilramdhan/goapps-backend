// Package costbulkimport contains the application-layer logic for the cost calculation engine.
package costbulkimport

import (
	"bytes"
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

const (
	maxSampleErrors = 20

	// validateMaxFileBytes is the maximum file size accepted by the preview validation
	// endpoint. excelize.OpenReader loads the entire xlsx into memory (ZIP extraction +
	// XML parsing), which can use 10-50× the file size. A 5 MB limit keeps peak
	// excelize memory (~150-350 MB) safely within the finance-service pod limits
	// (1 Gi production, 1.5 Gi staging). Files larger than this should be imported
	// directly — the async import handler validates all rows and produces a
	// downloadable error report.
	validateMaxFileBytes = 5 * 1024 * 1024 // 5 MB
)

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
// Files larger than validateMaxFileBytes are rejected immediately.
// Both excelize.OpenReader and preValidateAll run inside a goroutine so the
// caller's context deadline is honored without a race against the gRPC transport
// layer's RST_STREAM on context cancellation.
func (h *ValidateHandler) Validate(ctx context.Context, fileContent []byte) (*ValidateResult, error) {
	if len(fileContent) > validateMaxFileBytes {
		return nil, fmt.Errorf(
			"file too large for preview validation (%.1f MB > 5 MB limit); use Import directly — it validates all rows and generates a downloadable error report",
			float64(len(fileContent))/(1024*1024), //nolint:mnd // 1024*1024 = bytes-to-MB conversion
		)
	}

	maps, mapsErr := h.loadMaps(ctx)
	if mapsErr != nil {
		return nil, mapsErr
	}

	// Both OpenReader and preValidateAll run in a goroutine so that:
	//   1. ctx.Done() is reached before the gRPC transport sends RST_STREAM,
	//      avoiding the "Connection dropped" UNAVAILABLE race.
	//   2. f.Close() is deferred inside the goroutine, preventing use-after-close
	//      when ctx is cancelled while preValidateAll is still running.
	// The goroutine is bounded — it will finish on its own even if ctx is cancelled.
	type asyncResult struct {
		sheets []SheetResult
		err    error
	}
	ch := make(chan asyncResult, 1)
	go func() {
		f, openErr := excelize.OpenReader(bytes.NewReader(fileContent))
		if openErr != nil {
			ch <- asyncResult{err: fmt.Errorf("open file: %w", openErr)}
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				_ = closeErr
			}
		}()
		ch <- asyncResult{sheets: preValidatePreview(f, maps)}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return buildValidateResult(r.sheets), nil
	case <-ctx.Done():
		return nil, fmt.Errorf("validation timed out — file may be too large; import directly to get the full error report")
	}
}

// buildValidateResult caps per-sheet errors for the modal response.
func buildValidateResult(allSheets []SheetResult) *ValidateResult {
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
	return result
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
