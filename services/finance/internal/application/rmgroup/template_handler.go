package rmgroup

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult is the template bytes + filename.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler produces the blank 2-sheet import template.
type TemplateHandler struct{}

// NewTemplateHandler builds a TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// Handle returns a blank Excel template.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close excel template")
			if err == nil {
				err = fmt.Errorf("close file: %w", cerr)
			}
		}
	}()

	if _, serr := f.NewSheet(sheetGroups); serr != nil {
		return nil, fmt.Errorf("new %s: %w", sheetGroups, serr)
	}
	if werr := writeHeaderRow(f, sheetGroups, groupsHeaders); werr != nil {
		return nil, werr
	}

	if _, serr := f.NewSheet(sheetItems); serr != nil {
		return nil, fmt.Errorf("new %s: %w", sheetItems, serr)
	}
	if werr := writeHeaderRow(f, sheetItems, itemsHeaders); werr != nil {
		return nil, werr
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, ierr := f.GetSheetIndex(sheetGroups); ierr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}

	return &TemplateResult{FileContent: buf.Bytes(), FileName: "rm_groups_template.xlsx"}, nil
}

// Column order and header labels for the per-group items template. Matches
// what ImportGroupItemsHandler reads (item_code required, rest optional).
var groupItemsTemplateHeaders = []string{"item_code", "grade_code", "sort_order"}

// GroupItemsTemplateResult mirrors TemplateResult — kept as its own type for
// clarity even though it has the same shape.
type GroupItemsTemplateResult = TemplateResult

// GroupItemsTemplateHandler produces a blank one-sheet import template
// dedicated to the per-group items upload.
type GroupItemsTemplateHandler struct{}

// NewGroupItemsTemplateHandler builds a GroupItemsTemplateHandler.
func NewGroupItemsTemplateHandler() *GroupItemsTemplateHandler {
	return &GroupItemsTemplateHandler{}
}

// Handle returns a blank template for per-group item import.
func (h *GroupItemsTemplateHandler) Handle() (result *GroupItemsTemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close group items template")
			if err == nil {
				err = fmt.Errorf("close file: %w", cerr)
			}
		}
	}()

	if _, serr := f.NewSheet(sheetItems); serr != nil {
		return nil, fmt.Errorf("new %s: %w", sheetItems, serr)
	}
	if werr := writeHeaderRow(f, sheetItems, groupItemsTemplateHeaders); werr != nil {
		return nil, werr
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, ierr := f.GetSheetIndex(sheetItems); ierr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return &GroupItemsTemplateResult{
		FileContent: buf.Bytes(),
		FileName:    "rm_group_items_template.xlsx",
	}, nil
}
