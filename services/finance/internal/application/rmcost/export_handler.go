package rmcost

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// ExportQuery is the query for exporting RM cost rows.
type ExportQuery struct {
	Period      string
	RMType      rmcost.RMType
	GroupHeadID *uuid.UUID
	Search      string
}

// ExportResult is the export bytes + filename.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler produces a single-sheet Excel of RM cost rows.
type ExportHandler struct {
	repo rmcost.Repository
}

// NewExportHandler builds an ExportHandler.
func NewExportHandler(repo rmcost.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

const costSheetName = "RMCosts"

var costExportHeaders = []string{
	"period", "rm_code", "rm_name", "rm_type", "uom_code",
	"cons_rate", "stores_rate", "dept_rate", "po_rate_1", "po_rate_2", "po_rate_3",
	"cost_valuation", "cost_marketing", "cost_simulation",
	"flag_valuation", "flag_marketing", "flag_simulation",
	"flag_valuation_used", "flag_marketing_used", "flag_simulation_used",
	"calculated_at", "calculated_by",
}

// Handle executes the export.
func (h *ExportHandler) Handle(ctx context.Context, q ExportQuery) (result *ExportResult, err error) {
	costs, err := h.repo.ListAll(ctx, rmcost.ExportFilter{
		Period:      q.Period,
		RMType:      q.RMType,
		GroupHeadID: q.GroupHeadID,
		Search:      q.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list all costs: %w", err)
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

	if _, err := f.NewSheet(costSheetName); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	if err := writeHeaderRow(f, costSheetName, costExportHeaders); err != nil {
		return nil, err
	}

	var errs []error
	for i, c := range costs {
		row := i + 2
		rates := c.Rates()
		vals := []any{
			c.Period(),
			c.RMCode(),
			c.RMName(),
			c.RMType().String(),
			c.UOMCode(),
			rates.Cons, rates.Stores, rates.Dept, rates.PO1, rates.PO2, rates.PO3,
			nilableFloatCost(c.CostValuation()),
			nilableFloatCost(c.CostMarketing()),
			nilableFloatCost(c.CostSimulation()),
			c.FlagValuation().String(), c.FlagMarketing().String(), c.FlagSimulation().String(),
			c.FlagValuationUsed().String(), c.FlagMarketingUsed().String(), c.FlagSimulationUsed().String(),
			formatTimePtr(c.CalculatedAt()),
			derefStringCost(c.CalculatedBy()),
		}
		if werr := writeRow(f, costSheetName, row, vals); werr != nil {
			errs = append(errs, werr)
		}
	}
	if len(errs) > 0 {
		log.Warn().Err(errors.Join(errs...)).Msg("rm cost export partial row errors")
	}

	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("delete default sheet")
	}
	if idx, idxErr := f.GetSheetIndex(costSheetName); idxErr == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return &ExportResult{FileContent: buf.Bytes(), FileName: "rm_costs_export.xlsx"}, nil
}

// writeHeaderRow writes a styled header row to the given sheet.
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

func nilableFloatCost(v *float64) any {
	if v == nil {
		return ""
	}
	return *v
}

func derefStringCost(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
