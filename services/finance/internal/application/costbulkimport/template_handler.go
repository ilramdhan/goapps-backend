package costbulkimport

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// TemplateHandler generates a downloadable Excel template for bulk product routing import.
type TemplateHandler struct{}

// NewTemplateHandler constructs a TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// Handle returns a 6-sheet Excel template with headers and one sample row.
func (h *TemplateHandler) Handle(_ context.Context) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheets := []struct {
		name    string
		headers []string
		sample  []string
	}{
		{
			name:    "product_master",
			headers: []string{"legacy_oracle_sys_id", "product_type_code", "product_name", "shade_code", "shade_name", "grade_code", "description", "erp_item_code", "flex_01", "flex_03", "is_active"},
			sample:  []string{"PROD-001", "FINISH", "Sample Product Name", "SH-001", "Shade Red", "A", "Sample description", "ERP-001", "", "", "true"},
		},
		{
			name:    "cpp",
			headers: []string{"legacy_oracle_sys_id", "param_code", "value_numeric", "value_text", "value_flag"},
			sample:  []string{"PROD-001", "PARAM_CODE", "100.5", "", ""},
		},
		{
			name:    "capp",
			headers: []string{"legacy_oracle_sys_id", "param_code", "is_required", "display_order"},
			sample:  []string{"PROD-001", "PARAM_CODE", "true", "1"},
		},
		{
			name:    "route_head",
			headers: []string{"legacy_oracle_sys_id", "notes"},
			sample:  []string{"PROD-001", "Main routing"},
		},
		{
			name:    "route_seq",
			headers: []string{"legacy_oracle_sys_id", "route_level", "route_seq", "route_name", "route_item_code", "position_x", "position_y", "cyl_type_id"},
			sample:  []string{"PROD-001", "1", "1", "Process 1", "SEQ-001", "0", "0", ""},
		},
		{
			name:    "route_rm",
			headers: []string{"legacy_oracle_sys_id", "route_level", "route_seq", "rm_type", "rm_product_legacy_id", "rm_item_code", "rm_group_code", "rm_name", "rm_item_code_ref", "ratio", "sub_type", "notes"},
			sample:  []string{"PROD-001", "1", "1", "PRODUCT", "RM-001", "", "", "RM Name", "", "1.0", "", ""},
		},
	}

	// Delete default Sheet1
	_ = f.DeleteSheet("Sheet1")

	for _, s := range sheets {
		if _, err := f.NewSheet(s.name); err != nil {
			return nil, fmt.Errorf("create sheet %s: %w", s.name, err)
		}
		for i, hdr := range s.headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1) //nolint:gosec // col index bounded by slice len
			_ = f.SetCellValue(s.name, cell, hdr)
		}
		for i, v := range s.sample {
			cell, _ := excelize.CoordinatesToCellName(i+1, 2) //nolint:gosec // col index bounded by slice len
			_ = f.SetCellValue(s.name, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write template to buffer: %w", err)
	}
	return buf.Bytes(), nil
}
