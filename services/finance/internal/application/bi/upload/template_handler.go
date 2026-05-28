// Package upload provides application-layer handlers for the BI Excel upload flow:
// template download, parse/validate into staging, commit (UPSERT to fact_metric),
// cancel, and session history listing.
package upload

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// uploadSheetName is the sheet the upload template/parse expects.
const uploadSheetName = "FACT_METRIC"

// uploadHeaders is the canonical column order of the upload template.
var uploadHeaders = []string{
	"TYPE", "GROUP_1", "GROUP_2", "GROUP_3",
	"GROUP_1_ORDER", "GROUP_2_ORDER", "GROUP_3_ORDER",
	"PERIODE_GRAIN", "PERIODE", "VALUE", "UOM", "SCENARIO",
}

// TemplateResult is the produced template file.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler builds a blank upload template workbook.
type TemplateHandler struct{}

// NewTemplateHandler constructs a TemplateHandler.
func NewTemplateHandler() *TemplateHandler { return &TemplateHandler{} }

// Handle generates the .xlsx template for the given target type.
func (h *TemplateHandler) Handle(targetType string) (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close BI upload template file")
			if err == nil {
				err = fmt.Errorf("close template file: %w", closeErr)
			}
		}
	}()

	index, err := f.NewSheet(uploadSheetName)
	if err != nil {
		return nil, fmt.Errorf("create sheet: %w", err)
	}
	f.SetActiveSheet(index)
	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("Could not delete default Sheet1")
	}

	if err := h.writeHeaders(f); err != nil {
		return nil, err
	}
	h.writeSampleRow(f, targetType)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write template buffer: %w", err)
	}
	name := targetType
	if name == "" {
		name = "GENERIC"
	}
	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    fmt.Sprintf("bi_upload_template_%s.xlsx", name),
	}, nil
}

func (h *TemplateHandler) writeHeaders(f *excelize.File) error {
	for col, header := range uploadHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("header cell name: %w", err)
		}
		if err := f.SetCellValue(uploadSheetName, cell, header); err != nil {
			return fmt.Errorf("set header %s: %w", header, err)
		}
	}
	return nil
}

// writeSampleRow writes one illustrative example row so users see the expected shape.
func (h *TemplateHandler) writeSampleRow(f *excelize.File, targetType string) {
	sampleType := targetType
	if sampleType == "" {
		sampleType = "MIS"
	}
	sample := []any{
		sampleType, "EBITDA", "INCOME", "", 1, 1, 1,
		"MONTHLY", "202604", 1000000, "IDR", "ACTUAL",
	}
	for col, v := range sample {
		cell, err := excelize.CoordinatesToCellName(col+1, 2)
		if err != nil {
			log.Debug().Err(err).Msg("sample cell name")
			continue
		}
		if err := f.SetCellValue(uploadSheetName, cell, v); err != nil {
			log.Debug().Err(err).Msg("set sample cell")
		}
	}
}
