// Package employeelevel provides application layer handlers for Employee Level operations.
package employeelevel

import (
	"fmt"

	"github.com/mutugading/goapps-backend/services/shared/excel"
)

// TemplateResult holds the generated template file.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler generates an import template Excel file.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler { return &TemplateHandler{} }

var templateColumns = []excel.Column{
	{Header: "Code", Width: 15},
	{Header: "Name", Width: 25},
	{Header: "Grade", Width: 8},
	{Header: "Type", Width: 15},
	{Header: "Sequence", Width: 10},
	{Header: "Workflow", Width: 15},
}

var sampleData = []excel.SampleRow{
	{"SU", "Super User", "99", "EXECUTIVE", "10", "SUPER_USER"},
	{"D", "Director", "90", "EXECUTIVE", "20", "DRAFT"},
	{"GM", "General Manager", "80", "EXECUTIVE", "30", "DRAFT"},
	{"SP", "Supervisor", "50", "NON_EXECUTIVE", "40", "DRAFT"},
}

var templateInstructions = []excel.Instruction{
	{Cell: "A1", Text: "Employee Level Import Instructions"},
	{Cell: "A3", Text: "1. Code: Uppercase letters, digits, hyphens (e.g., SU, SS-22, P-9). Max 20 chars."},
	{Cell: "A4", Text: "2. Name: Display name (required, max 100 chars)."},
	{Cell: "A5", Text: "3. Grade: Integer 0-99."},
	{Cell: "A6", Text: "4. Type: EXECUTIVE, NON_EXECUTIVE, OPERATOR, or OTHER."},
	{Cell: "A7", Text: "5. Sequence: Sort order integer 0-999."},
	{Cell: "A8", Text: "6. Workflow: DRAFT, RELEASED, or SUPER_USER."},
	{Cell: "A10", Text: "Notes:"},
	{Cell: "A11", Text: "- Delete sample data rows before importing."},
	{Cell: "A12", Text: "- Save file as .xlsx format."},
}

// Handle generates the import template.
func (h *TemplateHandler) Handle() (*TemplateResult, error) {
	data, err := excel.Template("Employee Level Template", templateColumns, sampleData, templateInstructions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate template: %w", err)
	}

	return &TemplateResult{
		FileContent: data,
		FileName:    "employee_level_import_template.xlsx",
	}, nil
}
