package costbulkimport

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// TestCountErrors verifies that countErrors sums row error counts across sheets.
func TestCountErrors(t *testing.T) {
	tests := []struct {
		name     string
		results  []SheetResult
		expected int
	}{
		{
			name:     "no results",
			results:  nil,
			expected: 0,
		},
		{
			name: "no errors",
			results: []SheetResult{
				{SheetName: "product_master", Inserted: 5},
				{SheetName: "route_head", Inserted: 3},
			},
			expected: 0,
		},
		{
			name: "errors across multiple sheets",
			results: []SheetResult{
				{SheetName: "product_master", Errors: []SheetError{{RowNumber: 2, Field: "name", Message: "required"}}},
				{SheetName: "product_parameters", Errors: []SheetError{
					{RowNumber: 3, Field: "param_code", Message: "required"},
					{RowNumber: 4, Field: "param_code", Message: "unknown"},
				}},
				{SheetName: "route_head"},
			},
			expected: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countErrors(tt.results))
		})
	}
}

// TestAppendIfUnderLimit verifies the capping behavior.
func TestAppendIfUnderLimit(t *testing.T) {
	var errs []SheetError
	for i := range 25 {
		errs = appendIfUnderLimit(errs, SheetError{RowNumber: int32(i + 2), Field: "f", Message: "m"}, maxSampleErrors) //nolint:gosec // test row count fits int32
	}
	assert.Len(t, errs, maxSampleErrors, "should be capped at maxSampleErrors")

	// Appending to already-at-limit slice must not grow it.
	errs = appendIfUnderLimit(errs, SheetError{RowNumber: 99, Field: "x", Message: "y"}, maxSampleErrors)
	assert.Len(t, errs, maxSampleErrors)
}

// TestGenerateErrorReport verifies that GenerateErrorReport produces valid Excel bytes
// with the expected summary sheet and per-sheet error tabs.
func TestGenerateErrorReport(t *testing.T) {
	results := []SheetResult{
		{
			SheetName: "product_master",
			TotalRows: 10,
			Inserted:  8,
			Updated:   1,
			Errors: []SheetError{
				{RowNumber: 5, Field: "product_type_code", Message: "unknown"},
			},
		},
		{
			SheetName: "product_parameters",
			TotalRows: 5,
			Inserted:  5,
		},
		{
			SheetName: "route_head",
			TotalRows: 3,
			Skipped:   1,
			Errors: []SheetError{
				{RowNumber: 2, Field: "routing_status", Message: "route is LOCKED — skipped"},
			},
		},
	}

	reportBytes, err := GenerateErrorReport(results)
	require.NoError(t, err)
	require.NotEmpty(t, reportBytes)

	// Verify the bytes are valid Excel.
	f, openErr := excelize.OpenReader(bytes.NewReader(reportBytes))
	require.NoError(t, openErr)
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	assert.Contains(t, sheets, "summary")
	// Sheets with errors should have an _errors tab.
	assert.Contains(t, sheets, "product_master_errors")
	assert.Contains(t, sheets, "route_head_errors")
	// Sheet without errors should NOT have an _errors tab.
	assert.NotContains(t, sheets, "product_parameters_errors")

	// Verify summary sheet has header + 3 data rows.
	summaryRows, rowsErr := f.GetRows("summary")
	require.NoError(t, rowsErr)
	assert.Len(t, summaryRows, 4) // 1 header + 3 data rows

	// Verify error counts in summary row for product_master (row index 1).
	assert.Equal(t, "product_master", summaryRows[1][0])
	assert.Equal(t, "10", summaryRows[1][1]) // total_rows
	assert.Equal(t, "1", summaryRows[1][5])  // errors
}

// TestGenerateErrorReport_NoErrors verifies that no error sheets are created when there
// are no row-level errors.
func TestGenerateErrorReport_NoErrors(t *testing.T) {
	results := []SheetResult{
		{SheetName: "product_master", TotalRows: 5, Inserted: 5},
	}
	reportBytes, err := GenerateErrorReport(results)
	require.NoError(t, err)
	require.NotEmpty(t, reportBytes)

	f, openErr := excelize.OpenReader(bytes.NewReader(reportBytes))
	require.NoError(t, openErr)
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	assert.Contains(t, sheets, "summary")
	assert.NotContains(t, sheets, "product_master_errors")
}
