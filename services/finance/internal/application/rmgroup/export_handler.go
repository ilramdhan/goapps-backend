package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ExportQuery is the query for exporting RM groups.
type ExportQuery struct {
	// IsActive filters by active flag when non-nil.
	IsActive *bool
}

// ExportResult is the export bytes + filename.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler produces a 2-sheet Excel (Groups + Items) for all RM groups.
type ExportHandler struct {
	repo rmgroup.Repository
}

// NewExportHandler builds an ExportHandler.
func NewExportHandler(repo rmgroup.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

const (
	sheetGroups = "Groups"
	sheetItems  = "Items"
)

var groupsHeaders = []string{
	"group_code", "group_name", "description", "colourant", "ci_name",
	"cost_percentage", "cost_per_kg",
	"flag_valuation", "flag_marketing", "flag_simulation",
	"init_val_valuation", "init_val_marketing", "init_val_simulation",
	"is_active",
}

var itemsHeaders = []string{
	"group_code", "item_code", "grade_code", "sort_order", "is_active", "is_dummy",
}

// Handle executes the export.
func (h *ExportHandler) Handle(ctx context.Context, q ExportQuery) (result *ExportResult, err error) {
	heads, err := h.repo.ListAllHeads(ctx, q.IsActive)
	if err != nil {
		return nil, fmt.Errorf("list heads: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close excel")
			if err == nil {
				err = fmt.Errorf("close file: %w", cerr)
			}
		}
	}()

	if err := buildGroupsSheet(f, heads); err != nil {
		return nil, err
	}
	if err := buildItemsSheet(ctx, f, h.repo, heads); err != nil {
		return nil, err
	}

	// Remove default sheet and set active to first real sheet.
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

	return &ExportResult{FileContent: buf.Bytes(), FileName: "rm_groups_export.xlsx"}, nil
}

func buildGroupsSheet(f *excelize.File, heads []*rmgroup.Head) error {
	if _, err := f.NewSheet(sheetGroups); err != nil {
		return fmt.Errorf("new sheet %s: %w", sheetGroups, err)
	}
	if err := writeHeaderRow(f, sheetGroups, groupsHeaders); err != nil {
		return err
	}
	var errs []error
	for i, head := range heads {
		row := i + 2
		vals := []any{
			head.Code().String(),
			head.Name(),
			head.Description(),
			head.Colourant(),
			head.CIName(),
			head.CostPercentage(),
			head.CostPerKg(),
			head.FlagValuation().String(),
			head.FlagMarketing().String(),
			head.FlagSimulation().String(),
			nilableFloat(head.InitValValuation()),
			nilableFloat(head.InitValMarketing()),
			nilableFloat(head.InitValSimulation()),
			head.IsActive(),
		}
		if err := writeRow(f, sheetGroups, row, vals); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("groups sheet partial errors")
	}
	return nil
}

func buildItemsSheet(
	ctx context.Context,
	f *excelize.File,
	repo rmgroup.Repository,
	heads []*rmgroup.Head,
) error {
	if _, err := f.NewSheet(sheetItems); err != nil {
		return fmt.Errorf("new sheet %s: %w", sheetItems, err)
	}
	if err := writeHeaderRow(f, sheetItems, itemsHeaders); err != nil {
		return err
	}
	rowIdx := 2
	var errs []error
	for _, head := range heads {
		details, err := repo.ListDetailsByHeadID(ctx, head.ID())
		if err != nil {
			return fmt.Errorf("list details for %s: %w", head.Code().String(), err)
		}
		for _, d := range details {
			if d.IsDeleted() {
				continue
			}
			vals := []any{
				head.Code().String(),
				d.ItemCode().String(),
				d.GradeCode(),
				d.SortOrder(),
				d.IsActive(),
				d.IsDummy(),
			}
			if werr := writeRow(f, sheetItems, rowIdx, vals); werr != nil {
				errs = append(errs, werr)
			}
			rowIdx++
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("items sheet partial errors")
	}
	return nil
}

func writeHeaderRow(f *excelize.File, sheet string, headers []string) error {
	for col, h := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("cell name: %w", err)
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("set header: %w", err)
		}
	}
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
	})
	if err == nil {
		lastCol, _ := excelize.CoordinatesToCellName(len(headers), 1)
		_ = f.SetCellStyle(sheet, "A1", lastCol, style)
	}
	return nil
}

func writeRow(f *excelize.File, sheet string, row int, values []any) error {
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return err
		}
	}
	return nil
}

func nilableFloat(v *float64) any {
	if v == nil {
		return ""
	}
	return *v
}
