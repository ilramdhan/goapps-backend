// Package worker hosts asynchronous job handlers consumed from RabbitMQ.
package worker

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// BuildRMCostExcel renders headers + details into a 2-sheet xlsx workbook.
//
// Sheet 1 "Header" mirrors cst_rm_cost columns. Sheet 2 "Detail" mirrors
// cst_rm_cost_detail columns. Column order follows the database schema
// (natural order from migrations) — audit columns trail at the right.
//
// The detail rows are joined to header rows externally; this function only
// formats whatever is passed. Callers pass empty slices for empty exports.
//
//nolint:gocognit,gocyclo // Wide DTO mappers are linear and trivial in cognitive load.
func BuildRMCostExcel(headers []*rmcostdomain.Cost, details []*rmcostdomain.CostDetail) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if cErr := f.Close(); cErr != nil {
			_ = cErr // best-effort: caller already received bytes.
		}
	}()

	// Excelize creates "Sheet1" by default — rename to "Header" so the workbook
	// contains exactly two sheets.
	if err := f.SetSheetName("Sheet1", "Header"); err != nil {
		return nil, fmt.Errorf("rename default sheet: %w", err)
	}
	if _, err := f.NewSheet("Detail"); err != nil {
		return nil, fmt.Errorf("create detail sheet: %w", err)
	}

	if err := writeHeaderSheet(f, headers); err != nil {
		return nil, err
	}
	if err := writeDetailSheet(f, details); err != nil {
		return nil, err
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

// =============================================================================
// Header sheet
// =============================================================================

var headerColumns = []string{
	"rm_cost_id", "period", "rm_code", "rm_type", "group_head_id", "item_code", "rm_name", "uom_code",
	"cons_rate", "stores_rate", "dept_rate", "po1_rate", "po2_rate", "po3_rate",
	"flag_valuation", "flag_marketing", "flag_simulation",
	"flag_valuation_used", "flag_marketing_used", "flag_simulation_used",
	"valuation_flag", "marketing_flag", "valuation_flag_used", "marketing_flag_used",
	"cost_valuation", "cost_marketing", "cost_simulation",
	"calculated_at", "calculated_by",
	"created_at", "created_by", "updated_at", "updated_by",
}

func writeHeaderSheet(f *excelize.File, rows []*rmcostdomain.Cost) error {
	const sheet = "Header"
	for i, col := range headerColumns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return fmt.Errorf("header column %d: %w", i, err)
		}
		if err := f.SetCellValue(sheet, cell, col); err != nil {
			return fmt.Errorf("set header label %s: %w", col, err)
		}
	}

	for ri, c := range rows {
		row := ri + 2 // header is row 1
		values := headerRowValues(c)
		for ci, v := range values {
			cell, err := excelize.CoordinatesToCellName(ci+1, row)
			if err != nil {
				return fmt.Errorf("header coord r=%d c=%d: %w", row, ci, err)
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return fmt.Errorf("write header r=%d c=%d: %w", row, ci, err)
			}
		}
	}

	// Freeze top row + autofilter for usability — best-effort, ignored on error.
	if pErr := f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); pErr != nil {
		_ = pErr
	}
	if lastCell, err := excelize.CoordinatesToCellName(len(headerColumns), 1); err == nil {
		if afErr := f.AutoFilter(sheet, "A1:"+lastCell, nil); afErr != nil {
			_ = afErr
		}
	}
	return nil
}

//nolint:gocyclo,gocognit // Linear field mapping — each branch is one line.
func headerRowValues(c *rmcostdomain.Cost) []any {
	rates := c.Rates()
	groupHeadID := ""
	if g := c.GroupHeadID(); g != nil {
		groupHeadID = g.String()
	}
	itemCode := ""
	if it := c.ItemCode(); it != nil {
		itemCode = *it
	}
	v2In := c.V2Inputs()
	v2ValuationFlag, v2MarketingFlag := "", ""
	if v2In != nil {
		v2ValuationFlag = v2In.ValuationFlag
		v2MarketingFlag = v2In.MarketingFlag
	}

	return []any{
		c.ID().String(),
		c.Period(),
		c.RMCode(),
		string(c.RMType()),
		groupHeadID,
		itemCode,
		c.RMName(),
		c.UOMCode(),
		rates.Cons, rates.Stores, rates.Dept, rates.PO1, rates.PO2, rates.PO3,
		c.FlagValuation().String(), c.FlagMarketing().String(), c.FlagSimulation().String(),
		c.FlagValuationUsed().String(), c.FlagMarketingUsed().String(), c.FlagSimulationUsed().String(),
		v2ValuationFlag, v2MarketingFlag, "", "", // valuation_flag_used / marketing_flag_used resolved by FE, leave blank in export
		floatPtrCell(c.CostValuation()),
		floatPtrCell(c.CostMarketing()),
		floatPtrCell(c.CostSimulation()),
		timePtrCell(c.CalculatedAt()),
		stringPtrCell(c.CalculatedBy()),
		c.CreatedAt().UTC().Format("2006-01-02 15:04:05"),
		c.CreatedBy(),
		timePtrCell(c.UpdatedAt()),
		stringPtrCell(c.UpdatedBy()),
	}
}

// =============================================================================
// Detail sheet
// =============================================================================

var detailColumns = []string{
	"cost_detail_id", "rm_cost_id", "period", "group_head_id", "group_detail_id", "item_code", "item_name", "grade_code",
	"freight_rate", "anti_dumping_pct", "duty_pct", "transport_rate", "valuation_default_value",
	"cons_val", "cons_qty", "cons_rate", "cons_freight_val", "cons_val_based", "cons_rate_based",
	"cons_anti_dumping_val", "cons_anti_dumping_rate", "cons_duty_val", "cons_duty_rate",
	"cons_transport_val", "cons_transport_rate", "cons_landed_cost",
	"stock_val", "stock_qty", "stock_rate", "stock_freight_val", "stock_val_based", "stock_rate_based",
	"stock_anti_dumping_val", "stock_anti_dumping_rate", "stock_duty_val", "stock_duty_rate",
	"stock_transport_val", "stock_transport_rate", "stock_landed_cost",
	"po_val", "po_qty", "po_rate",
	"fix_rate", "fix_freight_rate", "fix_rate_based", "fix_anti_dumping_rate", "fix_duty_rate",
	"fix_transport_rate", "fix_landed_cost",
	"created_at", "created_by", "updated_at", "updated_by",
}

func writeDetailSheet(f *excelize.File, rows []*rmcostdomain.CostDetail) error {
	const sheet = "Detail"
	for i, col := range detailColumns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return fmt.Errorf("detail column %d: %w", i, err)
		}
		if err := f.SetCellValue(sheet, cell, col); err != nil {
			return fmt.Errorf("set detail label %s: %w", col, err)
		}
	}

	for ri, d := range rows {
		row := ri + 2
		values := detailRowValues(d)
		for ci, v := range values {
			cell, err := excelize.CoordinatesToCellName(ci+1, row)
			if err != nil {
				return fmt.Errorf("detail coord r=%d c=%d: %w", row, ci, err)
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return fmt.Errorf("write detail r=%d c=%d: %w", row, ci, err)
			}
		}
	}

	if pErr := f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); pErr != nil {
		_ = pErr
	}
	if lastCell, err := excelize.CoordinatesToCellName(len(detailColumns), 1); err == nil {
		if afErr := f.AutoFilter(sheet, "A1:"+lastCell, nil); afErr != nil {
			_ = afErr
		}
	}
	return nil
}

//nolint:gocyclo,gocognit // Linear field mapping.
func detailRowValues(d *rmcostdomain.CostDetail) []any {
	snap := d.Snapshot()
	groupDetailID := ""
	if g := d.GroupDetailID(); g != nil {
		groupDetailID = g.String()
	}
	return []any{
		d.ID().String(),
		d.CostID().String(),
		d.Period(),
		d.GroupHeadID().String(),
		groupDetailID,
		d.ItemCode(),
		d.ItemName(),
		d.GradeCode(),
		floatPtrCell(snap.FreightRate),
		floatPtrCell(snap.AntiDumpingPct),
		floatPtrCell(snap.DutyPct),
		floatPtrCell(snap.TransportRate),
		floatPtrCell(snap.ValuationDefaultValue),
		floatPtrCell(snap.ConsVal),
		floatPtrCell(snap.ConsQty),
		floatPtrCell(snap.ConsRate),
		floatPtrCell(snap.ConsFreightVal),
		floatPtrCell(snap.ConsValBased),
		floatPtrCell(snap.ConsRateBased),
		floatPtrCell(snap.ConsAntiDumpingVal),
		floatPtrCell(snap.ConsAntiDumpingRate),
		floatPtrCell(snap.ConsDutyVal),
		floatPtrCell(snap.ConsDutyRate),
		floatPtrCell(snap.ConsTransportVal),
		floatPtrCell(snap.ConsTransportRate),
		floatPtrCell(snap.ConsLandedCost),
		floatPtrCell(snap.StockVal),
		floatPtrCell(snap.StockQty),
		floatPtrCell(snap.StockRate),
		floatPtrCell(snap.StockFreightVal),
		floatPtrCell(snap.StockValBased),
		floatPtrCell(snap.StockRateBased),
		floatPtrCell(snap.StockAntiDumpingVal),
		floatPtrCell(snap.StockAntiDumpingRate),
		floatPtrCell(snap.StockDutyVal),
		floatPtrCell(snap.StockDutyRate),
		floatPtrCell(snap.StockTransportVal),
		floatPtrCell(snap.StockTransportRate),
		floatPtrCell(snap.StockLandedCost),
		floatPtrCell(snap.POVal),
		floatPtrCell(snap.POQty),
		floatPtrCell(snap.PORate),
		floatPtrCell(snap.FixRate),
		floatPtrCell(snap.FixFreightRate),
		floatPtrCell(snap.FixRateBased),
		floatPtrCell(snap.FixAntiDumpingRate),
		floatPtrCell(snap.FixDutyRate),
		floatPtrCell(snap.FixTransportRate),
		floatPtrCell(snap.FixLandedCost),
		d.CreatedAt().UTC().Format("2006-01-02 15:04:05"),
		d.CreatedBy(),
		timePtrCell(d.UpdatedAt()),
		stringPtrCell(d.UpdatedBy()),
	}
}

// =============================================================================
// Cell helpers — keep nil values empty rather than serializing as zero.
// =============================================================================

func floatPtrCell(p *float64) any {
	if p == nil {
		return ""
	}
	return *p
}

func stringPtrCell(p *string) any {
	if p == nil {
		return ""
	}
	return *p
}

func timePtrCell(p *time.Time) any {
	if p == nil {
		return ""
	}
	return p.UTC().Format("2006-01-02 15:04:05")
}
