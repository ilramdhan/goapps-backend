package worker

// costsheet_export_layouts.go holds the two single-purpose workbook layouts
// of the product cost sheet export. The original combined layout ("all data"
// plus one vertical sheet per product) lives in buildCombinedWorkbook.

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// buildVerticalWorkbook renders only the per-product vertical sheets — no
// "all data" sheet — and opens on the first of them. Sheet naming matches the
// combined layout ("parameter check" for a single product, FG labels
// otherwise).
//
// excelize can only append sheets, so products are appended after the default
// sheet, which is then deleted. Its name is reserved in stats.taken so a
// product whose FG label collides with it is suffixed rather than deleted
// together with the placeholder.
func (h *CostSheetExportHandler) buildVerticalWorkbook(
	ctx context.Context, msg rabbitmq.JobMessage, calcType costcalcdom.CalculationType,
) (*excelize.File, *exportStats, error) {
	book := excelize.NewFile()
	stats := &exportStats{taken: map[string]bool{}}
	placeholder := book.GetSheetName(0)
	stats.taken[placeholder] = true

	singleProduct := len(msg.ProductSysIDs) == 1
	for _, productSysID := range msg.ProductSysIDs {
		if err := h.addProductSheet(ctx, book, stats, productSysID, msg.Period, calcType, singleProduct); err != nil {
			closeWorkbook(h.logger, book)
			return nil, nil, err
		}
	}
	if stats.written == 0 {
		closeWorkbook(h.logger, book)
		return nil, nil, fmt.Errorf("no product produced a cost sheet (all %d products had no route stages)", len(msg.ProductSysIDs))
	}

	if err := book.DeleteSheet(placeholder); err != nil {
		closeWorkbook(h.logger, book)
		return nil, nil, fmt.Errorf("remove placeholder sheet %q: %w", placeholder, err)
	}
	delete(stats.taken, placeholder)
	book.SetActiveSheet(0)
	return book, stats, nil
}

// buildHorizontalWorkbook renders only the flat "all data" sheet. Rows are
// streamed (excelize StreamWriter) one product at a time, so neither the
// per-product vertical sheets nor the full stage list are ever held in
// memory — this layout is not chunked at maxExportProducts.
//
// Should the stages ever outgrow one sheet (Excel's 1,048,576-row limit), the
// rows continue on "all data (2)", "all data (3)", … each with its own header,
// and the rollover is logged and recorded as a warning. The request handler
// bounds products per file so this is a last-resort guard.
func (h *CostSheetExportHandler) buildHorizontalWorkbook(
	ctx context.Context, msg rabbitmq.JobMessage, calcType costcalcdom.CalculationType,
) (*excelize.File, *exportStats, error) {
	book := excelize.NewFile()
	stats := &exportStats{taken: map[string]bool{}}
	if err := h.writeHorizontalSheets(ctx, book, stats, msg, calcType); err != nil {
		closeWorkbook(h.logger, book)
		return nil, nil, err
	}
	if stats.written == 0 {
		closeWorkbook(h.logger, book)
		return nil, nil, fmt.Errorf("no product produced a cost sheet (all %d products had no route stages)", len(msg.ProductSysIDs))
	}
	book.SetActiveSheet(0)
	return book, stats, nil
}

// writeHorizontalSheets streams every product's stages into the flat sheet
// (plus continuation sheets on overflow). The caller owns closing book.
func (h *CostSheetExportHandler) writeHorizontalSheets(
	ctx context.Context, book *excelize.File, stats *exportStats,
	msg rabbitmq.JobMessage, calcType costcalcdom.CalculationType,
) error {
	if err := book.SetSheetName(book.GetSheetName(0), allDataSheetName); err != nil {
		return fmt.Errorf("reserve %q sheet: %w", allDataSheetName, err)
	}
	stats.taken[allDataSheetName] = true

	sw, err := newAllDataStream(book, allDataSheetName)
	if err != nil {
		return err
	}
	no := 0
	for _, productSysID := range msg.ProductSysIDs {
		stages, ok, loadErr := h.loadProductStages(ctx, stats, productSysID, msg.Period, calcType)
		if loadErr != nil {
			return loadErr
		}
		if !ok {
			continue
		}
		for _, stage := range stages {
			if sw.full() {
				if sw, err = h.rolloverAllDataStream(book, sw, stats, msg.JobID); err != nil {
					return err
				}
			}
			no++
			if err := sw.writeStage(no, stage); err != nil {
				return err
			}
		}
		stats.written++
		stats.fgLabels = append(stats.fgLabels, finishedGoodLabel(stages))
	}
	return sw.flush()
}

// rolloverAllDataStream closes the full sheet and opens the next continuation
// sheet, logging the overflow.
func (h *CostSheetExportHandler) rolloverAllDataStream(
	book *excelize.File, current *allDataStream, stats *exportStats, jobID string,
) (*allDataStream, error) {
	if err := current.flush(); err != nil {
		return nil, err
	}
	name := sanitizeSheetName(allDataSheetName, stats.taken)
	if _, err := book.NewSheet(name); err != nil {
		return nil, fmt.Errorf("create continuation sheet %q: %w", name, err)
	}
	msg := fmt.Sprintf("%q reached Excel's %d-row limit; remaining rows continue on %q",
		current.sheet, excelize.TotalRows, name)
	stats.warnings = append(stats.warnings, msg)
	h.logger.Warn().Str("job_id", jobID).Str("sheet", name).Msg("cost sheet export: " + msg)
	return newAllDataStream(book, name)
}
