// Package employeegroup provides application layer handlers for Employee Group operations.
package employeegroup

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
	{Header: "Name", Width: 30},
}

var sampleData = []excel.SampleRow{
	{"ASM", "Assembly"},
	{"DYM", "Dynamometer"},
	{"DRV", "Driver"},
	{"MGR", "Manager"},
}

var templateInstructions = []excel.Instruction{
	{Cell: "A1", Text: "Employee Group Import Instructions"},
	{Cell: "A3", Text: "1. Code: Uppercase letters and digits only (e.g., ASM, DYM, MGR). Max 20 chars."},
	{Cell: "A4", Text: "2. Name: Display name (required, max 100 chars)."},
	{Cell: "A6", Text: "Notes:"},
	{Cell: "A7", Text: "- Delete sample data rows before importing."},
	{Cell: "A8", Text: "- Save file as .xlsx format."},
}

// Handle generates the import template.
func (h *TemplateHandler) Handle() (*TemplateResult, error) {
	data, err := excel.Template("Employee Group Template", templateColumns, sampleData, templateInstructions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate template: %w", err)
	}

	return &TemplateResult{
		FileContent: data,
		FileName:    "employee_group_import_template.xlsx",
	}, nil
}
