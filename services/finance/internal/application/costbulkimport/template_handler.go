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
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	sheets := []struct {
		name    string
		headers []string
		sample  []string
	}{
		{
			name:    "product_master",
			headers: []string{"legacy_oracle_sys_id", "product_type_code", "product_name", "shade_code", "shade_name", "grade_code", "description", "erp_item_code", "legacy_erp_compound_key", "legacy_type_label", "is_active"},
			sample:  []string{"CPM_FG_EXAMPLE", "FINISH", "Sample Product Name", "SH-001", "Shade Red", "A", "Sample description", "ERP-001", "", "", boolTrueStr},
		},
		{
			name:    "product_parameters",
			headers: []string{"legacy_oracle_sys_id", "param_code", "data_type", "value_numeric", "value_text", "value_flag"},
			sample:  []string{"CPM_FG_EXAMPLE", "PARAM_CODE", "NUMERIC", "100.5", "", ""},
		},
		{
			name:    "product_applicable_params",
			headers: []string{"legacy_oracle_sys_id", "param_code", "is_required", "display_order"},
			sample:  []string{"CPM_FG_EXAMPLE", "PARAM_CODE", boolTrueStr, "1"},
		},
		{
			name:    "route_head",
			headers: []string{"legacy_oracle_sys_id", "routing_status", "notes"},
			sample:  []string{"CPM_FG_EXAMPLE", "DRAFT", "Main routing"},
		},
		{
			name:    "route_sequences",
			headers: []string{"route_head_legacy_product_id", "node_product_legacy_id", "route_level", "route_seq", "route_name", "route_item_code", "route_shade_code", "route_shade_name"},
			sample:  []string{"CPM_FG_EXAMPLE", "CPM_INT_EXAMPLE", "1", "1", "Process 1", "", "", ""},
		},
		{
			name:    "route_rms",
			headers: []string{"route_head_legacy_product_id", "route_level", "route_seq", "rm_type", "ratio", "rm_product_legacy_id", "rm_item_code", "rm_group_code", "rm_name", "rm_shade_code", "rm_shade_name", "sub_type", "notes"},
			sample:  []string{"CPM_FG_EXAMPLE", "1", "1", "PRODUCT", "1.0", "CPM_INT_EXAMPLE", "", "", "RM Name", "", "", "", ""},
		},
	}

	// Delete default Sheet1
	if err := f.DeleteSheet("Sheet1"); err != nil {
		_ = err
	}

	for _, s := range sheets {
		if err := populateTemplateSheet(f, s.name, s.headers, s.sample); err != nil {
			return nil, err
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write template to buffer: %w", err)
	}
	return buf.Bytes(), nil
}

// ParamsOnlyTemplateHandler generates a downloadable Excel template for the
// params-only bulk import (product_parameters + product_applicable_params only).
type ParamsOnlyTemplateHandler struct{}

// NewParamsOnlyTemplateHandler constructs a ParamsOnlyTemplateHandler.
func NewParamsOnlyTemplateHandler() *ParamsOnlyTemplateHandler {
	return &ParamsOnlyTemplateHandler{}
}

// Handle returns a 2-sheet Excel template with headers and one sample row.
func (h *ParamsOnlyTemplateHandler) Handle(_ context.Context) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	sheets := []struct {
		name    string
		headers []string
		sample  []string
	}{
		{
			name:    "product_parameters",
			headers: []string{"legacy_oracle_sys_id", "param_code", "data_type", "value_numeric", "value_text", "value_flag"},
			sample:  []string{"2512", "PARAM_CODE", "NUMERIC", "100.5", "", ""},
		},
		{
			name:    "product_applicable_params",
			headers: []string{"legacy_oracle_sys_id", "param_code", "is_required", "display_order"},
			sample:  []string{"2512", "PARAM_CODE", boolTrueStr, "1"},
		},
	}

	if err := f.DeleteSheet("Sheet1"); err != nil {
		_ = err
	}
	for _, s := range sheets {
		if err := populateTemplateSheet(f, s.name, s.headers, s.sample); err != nil {
			return nil, err
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write params-only template to buffer: %w", err)
	}
	return buf.Bytes(), nil
}

// populateTemplateSheet creates a named sheet and writes headers in row 1 and sample values in row 2.
func populateTemplateSheet(f *excelize.File, name string, headers []string, sample []string) error {
	if _, err := f.NewSheet(name); err != nil {
		return fmt.Errorf("create sheet %s: %w", name, err)
	}
	for i, hdr := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(i+1, 1)
		if cellErr != nil {
			return fmt.Errorf("template coord row 1 col %d: %w", i+1, cellErr)
		}
		if setErr := f.SetCellValue(name, cell, hdr); setErr != nil {
			return fmt.Errorf("template header %s: %w", cell, setErr)
		}
	}
	for i, v := range sample {
		cell, cellErr := excelize.CoordinatesToCellName(i+1, 2)
		if cellErr != nil {
			return fmt.Errorf("template coord row 2 col %d: %w", i+1, cellErr)
		}
		if setErr := f.SetCellValue(name, cell, v); setErr != nil {
			return fmt.Errorf("template sample %s: %w", cell, setErr)
		}
	}
	return nil
}
