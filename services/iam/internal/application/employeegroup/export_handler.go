// Package employeegroup provides application layer handlers for Employee Group operations.
package employeegroup

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/shared/excel"
)

// ExportQuery represents the export query parameters.
type ExportQuery struct {
	IsActive *bool
}

// ExportResult holds the generated Excel file.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the export employee groups query.
type ExportHandler struct {
	repo employeegroup.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo employeegroup.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

var exportColumns = []excel.Column{
	{Header: "No", Width: 5},
	{Header: "Code", Width: 15},
	{Header: "Name", Width: 30},
	{Header: "Active", Width: 8},
	{Header: "Created At", Width: 20},
	{Header: "Created By", Width: 15},
}

// Handle executes the export query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (*ExportResult, error) {
	items, err := h.repo.ListAll(ctx, employeegroup.ExportFilter{
		IsActive: query.IsActive,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get employee groups for export: %w", err)
	}

	rows := make([]excel.ExportRow, len(items))
	for i, eg := range items {
		rows[i] = excel.ExportRow{
			i + 1,
			eg.Code().String(),
			eg.Name(),
			eg.IsActive(),
			eg.Audit().CreatedAt.Format("2006-01-02 15:04:05"),
			eg.Audit().CreatedBy,
		}
	}

	data, err := excel.Export("Employee Groups", exportColumns, rows)
	if err != nil {
		return nil, fmt.Errorf("failed to generate excel: %w", err)
	}

	return &ExportResult{
		FileContent: data,
		FileName:    "employee_group_export.xlsx",
	}, nil
}
