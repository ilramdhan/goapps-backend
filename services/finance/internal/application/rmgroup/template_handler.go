// Package rmgroup — V2 import template handler.
//
// Produces a 3-sheet workbook (Groups + Items + Notes) with V2 columns,
// matching the layout produced by ExportHandler so the round-trip is clean.
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

// TemplateHandler produces the blank import workbook.
type TemplateHandler struct{}

// NewTemplateHandler builds a TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// Handle generates the template.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close excel")
			if err == nil {
				err = fmt.Errorf("close: %w", cerr)
			}
		}
	}()

	if _, e := f.NewSheet(sheetGroups); e != nil {
		return nil, fmt.Errorf("new groups sheet: %w", e)
	}
	if e := writeHeaderRow(f, sheetGroups, groupsHeaders); e != nil {
		return nil, e
	}
	if e := writeRow(f, sheetGroups, 2, exampleGroupsRow()); e != nil {
		log.Debug().Err(e).Msg("template example group row")
	}

	if _, e := f.NewSheet(sheetItems); e != nil {
		return nil, fmt.Errorf("new items sheet: %w", e)
	}
	if e := writeHeaderRow(f, sheetItems, itemsHeaders); e != nil {
		return nil, e
	}
	if e := writeRow(f, sheetItems, 2, exampleItemsRow()); e != nil {
		log.Debug().Err(e).Msg("template example item row")
	}

	if e := buildNotesSheet(f); e != nil {
		log.Debug().Err(e).Msg("template notes sheet")
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, idxErr := f.GetSheetIndex(sheetGroups); idxErr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return &TemplateResult{FileContent: buf.Bytes(), FileName: "rm_groups_template.xlsx"}, nil
}

// exampleGroupsRow returns one populated example showing the expected types.
func exampleGroupsRow() []any {
	return []any{
		"GROUP-EXAMPLE",      // group_code
		"Example Group",      // group_name
		"Sample description", // description
		"",                   // colourant
		"",                   // ci_name
		4,                    // duty_pct (whole percent: 4 = 4%)
		0.0813,               // transport_rate
		0.5,                  // mkt_freight
		2,                    // mkt_anti_pct (whole percent: 2 = 2%)
		15,                   // mkt_default_value
		"AUTO",               // valuation_flag
		"AUTO",               // marketing_flag
		"TRUE",               // is_active
	}
}

// exampleItemsRow returns one populated example linking to the example group.
// grade_code is filled in even though item_name / uom_code can autofill from
// the sync feed — multi-variant items REQUIRE grade_code so the example
// shows the safer pattern up front.
func exampleItemsRow() []any {
	return []any{
		"GROUP-EXAMPLE", // group_code
		"CHP0000033",    // item_code
		"",              // item_name (autofills from sync feed via grade_code)
		"",              // item_type_code (snapshot only, leave empty)
		"NA",            // grade_code (REQUIRED for multi-variant items)
		"",              // item_grade (autofills from sync feed)
		"",              // uom_code (autofills from sync feed)
		1,               // sort_order
		0.06,            // val_freight
		4,               // val_anti_pct (whole %)
		4,               // val_duty_pct (whole %)
		0.08125,         // val_transport
		0.10,            // val_default_value
		"TRUE",          // is_active
	}
}

// =============================================================================
// Per-group items-only template (used by the AddItems Excel upload, not the
// 2-sheet bulk export). Kept as a separate handler with its own header set
// so the 2-sheet schema can evolve independently of the per-group flow.
// =============================================================================

// GroupItemsTemplateResult is an alias kept for backwards compatibility with
// the existing handler wiring.
type GroupItemsTemplateResult = TemplateResult

// GroupItemsTemplateHandler produces a blank one-sheet template for the
// per-group items uploader.
type GroupItemsTemplateHandler struct{}

// NewGroupItemsTemplateHandler builds a GroupItemsTemplateHandler.
func NewGroupItemsTemplateHandler() *GroupItemsTemplateHandler {
	return &GroupItemsTemplateHandler{}
}

var groupItemsTemplateHeaders = []string{"item_code", "grade_code", "sort_order"}

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
