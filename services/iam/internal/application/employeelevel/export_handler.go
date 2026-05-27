// Package employeelevel provides application layer handlers for Employee Level operations.
package employeelevel

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/shared/excel"
)

// ExportQuery represents the export query parameters.
type ExportQuery struct {
	IsActive *bool
	Type     *employeelevel.Type
	Workflow *employeelevel.Workflow
}

// ExportResult holds the generated Excel file.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the export employee levels query.
type ExportHandler struct {
	repo employeelevel.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo employeelevel.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

var exportColumns = []excel.Column{
	{Header: "No", Width: 5},
	{Header: "Code", Width: 15},
	{Header: "Name", Width: 25},
	{Header: "Grade", Width: 8},
	{Header: "Type", Width: 15},
	{Header: "Sequence", Width: 10},
	{Header: "Workflow", Width: 15},
	{Header: "Active", Width: 8},
	{Header: "Created At", Width: 20},
	{Header: "Created By", Width: 15},
}

// Handle executes the export query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (*ExportResult, error) {
	items, err := h.repo.ListAll(ctx, employeelevel.ExportFilter{
		IsActive: query.IsActive,
		Type:     query.Type,
		Workflow: query.Workflow,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get employee levels for export: %w", err)
	}

	rows := make([]excel.ExportRow, len(items))
	for i, el := range items {
		rows[i] = excel.ExportRow{
			i + 1,
			el.Code().String(),
			el.Name(),
			el.Grade(),
			el.Type().String(),
			el.Sequence(),
			el.Workflow().String(),
			el.IsActive(),
			el.Audit().CreatedAt.Format("2006-01-02 15:04:05"),
			el.Audit().CreatedBy,
		}
	}

	data, err := excel.Export("Employee Levels", exportColumns, rows)
	if err != nil {
		return nil, fmt.Errorf("failed to generate excel: %w", err)
	}

	return &ExportResult{
		FileContent: data,
		FileName:    "employee_level_export.xlsx",
	}, nil
}
