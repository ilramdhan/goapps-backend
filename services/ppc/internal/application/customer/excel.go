// Package customer provides application-layer use cases for the PPC customer master.
package customer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	"github.com/mutugading/goapps-backend/services/shared/excel"
)

// Sheet and file names for the customer workbook.
const (
	exportSheetName   = "Customers"
	exportFileName    = "customer_export.xlsx"
	templateSheetName = "Customer Import Template"
	templateFileName  = "customer_import_template.xlsx"
)

// Import field labels, reported back on a failed row. Extracted as constants
// because each appears on several error paths.
const (
	fieldCode   = "code"
	fieldName   = "name"
	fieldImport = "import"
)

// Duplicate-handling actions accepted by Import.
const (
	// DuplicateSkip leaves an existing customer untouched.
	DuplicateSkip = "skip"
	// DuplicateUpdate refreshes an existing customer from the row.
	DuplicateUpdate = "update"
	// DuplicateError reports an existing customer as a row failure.
	DuplicateError = "error"
)

// exportColumns defines the customer export sheet layout.
var exportColumns = []excel.Column{
	{Header: "Code", Width: 16},
	{Header: "Name", Width: 45},
	{Header: "Short Name", Width: 22},
	{Header: "Tax No", Width: 24},
	{Header: "Parent Code", Width: 16},
	{Header: "Active", Width: 10},
	{Header: "Source", Width: 10},
	{Header: "Synced At", Width: 20},
}

// importColumns defines the import template layout. It is the export layout minus
// the columns the sync owns — a planner cannot author Source or Synced At.
var importColumns = []excel.Column{
	{Header: "Code", Width: 16},
	{Header: "Name", Width: 45},
	{Header: "Short Name", Width: 22},
	{Header: "Tax No", Width: 24},
	{Header: "Parent Code", Width: 16},
}

// ExportResult carries a generated workbook plus its suggested file name.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// Export builds an .xlsx workbook of every customer matching the filter. The
// workbook always carries a header row, so it is never empty.
func (s *Service) Export(ctx context.Context, query ListQuery) (*ExportResult, error) {
	filter := customerdomain.ListFilter{
		Search:   query.Search,
		IsActive: query.IsActive,
		Source:   query.Source,
	}
	items, err := s.repo.ListAll(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("read customers for export: %w", err)
	}

	rows := make([]excel.ExportRow, 0, len(items))
	for _, c := range items {
		rows = append(rows, excel.ExportRow{
			c.Code(),
			c.Name(),
			derefString(c.ShortName()),
			derefString(c.TaxNo()),
			derefString(c.ParentCode()),
			c.IsActive(),
			c.Source(),
			formatTime(c.SyncedAt()),
		})
	}

	content, err := excel.Export(exportSheetName, exportColumns, rows)
	if err != nil {
		return nil, err
	}
	return &ExportResult{FileContent: content, FileName: exportFileName}, nil
}

// Template builds the customer import template with sample rows and instructions.
func (s *Service) Template() (*ExportResult, error) {
	samples := []excel.SampleRow{
		{"DC00594", "PT. BESTOW ATRI PARIS TEKSTIL", "BESTOW", "01.234.567.8-901.000", ""},
		{"DC00100", "PT. BEHAESTEX", "BEHAESTEX", "", "DC00099"},
	}
	instructions := []excel.Instruction{
		{Cell: "A1", Text: "Customer Import Instructions"},
		{Cell: "A3", Text: "1. Code: required, unique, upper-cased automatically (matches Orion CUST_CODE)."},
		{Cell: "A4", Text: "2. Name: required."},
		{Cell: "A5", Text: "3. Short Name / Tax No / Parent Code: optional."},
		{Cell: "A7", Text: "Notes:"},
		{Cell: "A8", Text: "- Delete the sample rows before importing."},
		{Cell: "A9", Text: "- Imported rows are marked MANUAL and are never overwritten by the Oracle sync."},
		{Cell: "A10", Text: "- Save the file as .xlsx."},
	}

	content, err := excel.Template(templateSheetName, importColumns, samples, instructions)
	if err != nil {
		return nil, err
	}
	return &ExportResult{FileContent: content, FileName: templateFileName}, nil
}

// ImportCommand carries an uploaded workbook plus how to treat existing codes.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string
	CreatedBy       string
}

// ImportResult summarizes an import run.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	UpdatedCount int32
	FailedCount  int32
	Errors       []excel.ImportError
}

// Import creates or updates customers from an uploaded workbook. Rows it creates
// are marked MANUAL, so a later Oracle sync will not clobber a planner's entry.
// A bad row never aborts the run — it is counted and reported.
func (s *Service) Import(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
	rows, err := excel.ParseFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	result := &ImportResult{Errors: []excel.ImportError{}}
	for _, row := range rows {
		s.importRow(ctx, row, cmd, result)
	}
	return result, nil
}

// importRow processes a single parsed row into the master.
func (s *Service) importRow(ctx context.Context, row excel.ParsedRow, cmd ImportCommand, result *ImportResult) {
	code := customerdomain.NormalizeCode(row.Cell(0))
	name := strings.TrimSpace(row.Cell(1))
	if code == "" && name == "" {
		return // Blank trailing row from Excel; not a failure.
	}

	existing, err := s.repo.GetByCode(ctx, code)
	switch {
	case err == nil:
		s.importDuplicate(ctx, existing, row, cmd, result)
		return
	case !isNotFound(err):
		addImportError(result, row.RowNumber, fieldCode, fmt.Sprintf("lookup failed: %v", err))
		return
	}

	entity, err := customerdomain.New(customerdomain.NewParams{
		Code:       code,
		Name:       name,
		ShortName:  optionalText(row.Cell(2)),
		TaxNo:      optionalText(row.Cell(3)),
		ParentCode: optionalText(row.Cell(4)),
		CreatedBy:  cmd.CreatedBy,
	})
	if err != nil {
		addImportError(result, row.RowNumber, fieldName, err.Error())
		return
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		addImportError(result, row.RowNumber, fieldImport, fmt.Sprintf("create failed: %v", err))
		return
	}
	result.SuccessCount++
}

// importDuplicate applies the requested duplicate policy to an existing customer.
func (s *Service) importDuplicate(
	ctx context.Context,
	existing *customerdomain.Customer,
	row excel.ParsedRow,
	cmd ImportCommand,
	result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case DuplicateUpdate:
		s.importUpdate(ctx, existing, row, cmd.CreatedBy, result)
	case DuplicateError:
		addImportError(result, row.RowNumber, fieldCode, "customer code already exists")
	default:
		// skip, and anything unrecognized, are treated as skip.
		result.SkippedCount++
	}
}

// importUpdate refreshes an existing customer from a workbook row.
func (s *Service) importUpdate(
	ctx context.Context,
	existing *customerdomain.Customer,
	row excel.ParsedRow,
	updatedBy string,
	result *ImportResult,
) {
	name := strings.TrimSpace(row.Cell(1))
	params := customerdomain.UpdateParams{
		ShortName:  optionalText(row.Cell(2)),
		TaxNo:      optionalText(row.Cell(3)),
		ParentCode: optionalText(row.Cell(4)),
		UpdatedBy:  updatedBy,
	}
	if name != "" {
		params.Name = &name
	}
	if err := existing.Update(params); err != nil {
		addImportError(result, row.RowNumber, fieldName, err.Error())
		return
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		addImportError(result, row.RowNumber, fieldImport, fmt.Sprintf("update failed: %v", err))
		return
	}
	result.UpdatedCount++
}

// addImportError records one failed row.
func addImportError(result *ImportResult, rowNumber int32, field, message string) {
	result.FailedCount++
	result.Errors = append(result.Errors, excel.ImportError{
		RowNumber: rowNumber,
		Field:     field,
		Message:   message,
	})
}

// isNotFound reports whether an error is the customer-not-found sentinel.
func isNotFound(err error) bool {
	return errors.Is(err, customerdomain.ErrNotFound)
}

// derefString renders an optional string for a spreadsheet cell.
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// formatTime renders an optional timestamp for a spreadsheet cell.
func formatTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format(time.RFC3339)
}
