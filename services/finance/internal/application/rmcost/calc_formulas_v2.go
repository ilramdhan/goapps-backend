// Package rmcost — V2 RM Cost engine pure formula functions.
// Faithfully mirrors the Excel reference (RM_COST_V2.xlsx) row by row.
//
// Conventions:
//   - All percentage inputs (header AND per-detail) are stored as *decimal*
//     (0.10 = 10%). The UI form layer converts the user-typed whole percent
//     to/from decimal so users still see "5" for 5%. This matches the Excel
//     reference where P5/Q5/F22/G22 all carry decimal values.
//   - Per-detail CL/SL EXCLUDE anti-dumping (Excel V10 = O+U+S, AI10 = AB+AH+AF).
//   - Group-aggregate CL/SL INCLUDE anti-dumping (Excel V13 = O+Q+U+S,
//     AI13 = AB+AD+AH+AF). Both group totals use rate_based (with freight).
//   - FL group total = MAX(detail FL), not SUM (per Excel AS13).
package rmcost

import "math"

// SourceQty is one item's source quantities/values for one period.
// All fields zero when no sync row was found for the (item, grade, period).
type SourceQty struct {
	ConsVal  float64
	ConsQty  float64
	StockVal float64
	StockQty float64
	POVal    float64
	POQty    float64
}

// DetailInputs is the per-(item, grade) bag of valuation inputs.
type DetailInputs struct {
	FreightRate           float64
	AntiDumpingPct        float64 // decimal
	DutyPct               float64 // decimal
	TransportRate         float64
	ValuationDefaultValue float64 // 0 = unset (no FL contribution)
}

// HeaderInputsV2 is the bag of header marketing inputs. The two pct fields
// are stored as *decimal* (0.05 = 5%) — the UI form converts to/from whole
// percent for human-friendly entry. simulationRate carries the per-row
// simulation override.
type HeaderInputsV2 struct {
	MarketingFreightRate    float64
	MarketingAntiDumpingPct float64 // decimal (0.05 = 5%)
	MarketingDutyPct        float64 // decimal (0.05 = 5%)
	MarketingTransportRate  float64
	MarketingDefaultValue   float64
	SimulationRate          float64
	ValuationFlag           string // "AUTO"/"CR"/"SR"/"PR"/"CL"/"SL"/"FL"
	MarketingFlag           string // "AUTO"/"SP"/"PP"/"FP"
}

// DetailOutput is one detail row's full per-stage computation.
type DetailOutput struct {
	// Inputs (snapshot back).
	Inputs DetailInputs
	Source SourceQty

	// Consumption stage.
	ConsRate            float64
	ConsFreightVal      float64
	ConsValBased        float64
	ConsRateBased       float64
	ConsAntiDumpingVal  float64
	ConsAntiDumpingRate float64
	ConsDutyVal         float64
	ConsDutyRate        float64
	ConsTransportVal    float64
	ConsTransportRate   float64
	ConsLandedCost      float64

	// Stock stage.
	StockRate            float64
	StockFreightVal      float64
	StockValBased        float64
	StockRateBased       float64
	StockAntiDumpingVal  float64
	StockAntiDumpingRate float64
	StockDutyVal         float64
	StockDutyRate        float64
	StockTransportVal    float64
	StockTransportRate   float64
	StockLandedCost      float64

	// PO stage (rate only — no per-detail landed cost).
	PORate float64

	// Fix stage (driven by ValuationDefaultValue).
	FixRate            float64
	FixFreightRate     float64
	FixRateBased       float64
	FixAntiDumpingRate float64
	FixDutyRate        float64
	FixTransportRate   float64
	FixLandedCost      float64
}

// ComputeDetail runs the per-detail per-stage chain. Pure function — used by
// the engine and the unit tests.
func ComputeDetail(in DetailInputs, src SourceQty) DetailOutput {
	out := DetailOutput{Inputs: in, Source: src}

	// Consumption.
	if src.ConsQty > 0 {
		out.ConsRate = src.ConsVal / src.ConsQty
	}
	out.ConsFreightVal = src.ConsQty * in.FreightRate
	out.ConsValBased = src.ConsVal + out.ConsFreightVal
	if src.ConsQty > 0 {
		out.ConsRateBased = out.ConsValBased / src.ConsQty
	}
	out.ConsAntiDumpingVal = out.ConsValBased * in.AntiDumpingPct
	if src.ConsQty > 0 {
		out.ConsAntiDumpingRate = out.ConsAntiDumpingVal / src.ConsQty
	}
	out.ConsDutyVal = out.ConsValBased * in.DutyPct
	if src.ConsQty > 0 {
		out.ConsDutyRate = out.ConsDutyVal / src.ConsQty
	}
	out.ConsTransportVal = in.TransportRate * src.ConsQty
	if src.ConsQty > 0 {
		out.ConsTransportRate = out.ConsTransportVal / src.ConsQty
	}
	// CL = rate_based + transport + duty (NO anti-dumping per Excel).
	out.ConsLandedCost = out.ConsRateBased + out.ConsTransportRate + out.ConsDutyRate

	// Stock.
	if src.StockQty > 0 {
		out.StockRate = src.StockVal / src.StockQty
	}
	out.StockFreightVal = in.FreightRate * src.StockQty
	out.StockValBased = src.StockVal + out.StockFreightVal
	if src.StockQty > 0 {
		out.StockRateBased = out.StockValBased / src.StockQty
	}
	out.StockAntiDumpingVal = out.StockValBased * in.AntiDumpingPct
	if src.StockQty > 0 {
		out.StockAntiDumpingRate = out.StockAntiDumpingVal / src.StockQty
	}
	out.StockDutyVal = out.StockValBased * in.DutyPct
	if src.StockQty > 0 {
		out.StockDutyRate = out.StockDutyVal / src.StockQty
	}
	out.StockTransportVal = in.TransportRate * src.StockQty
	if src.StockQty > 0 {
		out.StockTransportRate = out.StockTransportVal / src.StockQty
	}
	// SL = rate_based + transport + duty (NO anti-dumping per Excel).
	out.StockLandedCost = out.StockRateBased + out.StockTransportRate + out.StockDutyRate

	// PO.
	if src.POQty > 0 {
		out.PORate = src.POVal / src.POQty
	}

	// Fix. ValuationDefaultValue == 0 means "unset" (Excel uses empty-cell test).
	if in.ValuationDefaultValue > 0 {
		out.FixRate = in.ValuationDefaultValue
		out.FixFreightRate = in.FreightRate
		out.FixRateBased = out.FixRate + out.FixFreightRate
		out.FixAntiDumpingRate = out.FixRateBased * in.AntiDumpingPct
		out.FixDutyRate = out.FixRateBased * in.DutyPct
		out.FixTransportRate = in.TransportRate
		// FL INCLUDES anti-dumping (per Excel AR formula).
		out.FixLandedCost = out.FixRateBased + out.FixAntiDumpingRate + out.FixDutyRate + out.FixTransportRate
	}
	return out
}

// GroupTotals is the aggregate output across all details for one group/period.
type GroupTotals struct {
	CR float64 // cons_val_total / cons_qty_total
	SR float64 // stock_val_total / stock_qty_total
	PR float64 // po_val_total / po_qty_total
	CL float64 // cons_rate_based + cons_anti_dumping_rate + cons_transport_rate + cons_duty_rate
	SL float64 // stock_rate_based + stock_anti_dumping_rate + stock_transport_rate + stock_duty_rate
	FL float64 // MAX(detail FL)
}

// AggregateGroupTotals computes group-level CR/SR/PR/CL/SL/FL from a slice of
// already-computed DetailOutputs. Mirrors Excel row 12.
func AggregateGroupTotals(outs []DetailOutput) GroupTotals {
	var (
		consQtyTotal, consValTotal, consValBasedTotal               float64
		consAntiValTotal, consDutyValTotal, consTransValTotal       float64
		stockQtyTotal, stockValTotal, stockValBasedTotal            float64
		stockAntiValTotal, stockDutyValTotal, stockTransValTotal    float64
		poQtyTotal, poValTotal                                      float64
		flMax                                                       float64
	)
	for _, o := range outs {
		consQtyTotal += o.Source.ConsQty
		consValTotal += o.Source.ConsVal
		consValBasedTotal += o.ConsValBased
		consAntiValTotal += o.ConsAntiDumpingVal
		consDutyValTotal += o.ConsDutyVal
		consTransValTotal += o.ConsTransportVal

		stockQtyTotal += o.Source.StockQty
		stockValTotal += o.Source.StockVal
		stockValBasedTotal += o.StockValBased
		stockAntiValTotal += o.StockAntiDumpingVal
		stockDutyValTotal += o.StockDutyVal
		stockTransValTotal += o.StockTransportVal

		poQtyTotal += o.Source.POQty
		poValTotal += o.Source.POVal

		flMax = math.Max(flMax, o.FixLandedCost)
	}
	tot := GroupTotals{}
	if consQtyTotal > 0 {
		tot.CR = consValTotal / consQtyTotal
		// Excel V13 = O13 + Q13 + U13 + S13
		//          = rate_based + anti_dumping_rate + transport_rate + duty_rate.
		consRateBasedTotal := consValBasedTotal / consQtyTotal
		consAntiRateTotal := consAntiValTotal / consQtyTotal
		consDutyRateTotal := consDutyValTotal / consQtyTotal
		consTransRateTotal := consTransValTotal / consQtyTotal
		tot.CL = consRateBasedTotal + consAntiRateTotal + consTransRateTotal + consDutyRateTotal
	}
	if stockQtyTotal > 0 {
		tot.SR = stockValTotal / stockQtyTotal
		// Excel AI13 = AB13 + AD13 + AH13 + AF13
		//           = stock_rate_based + anti_dumping_rate + transport_rate + duty_rate.
		stockRateBasedTotal := stockValBasedTotal / stockQtyTotal
		stockAntiRateTotal := stockAntiValTotal / stockQtyTotal
		stockDutyRateTotal := stockDutyValTotal / stockQtyTotal
		stockTransRateTotal := stockTransValTotal / stockQtyTotal
		tot.SL = stockRateBasedTotal + stockAntiRateTotal + stockTransRateTotal + stockDutyRateTotal
	}
	if poQtyTotal > 0 {
		tot.PR = poValTotal / poQtyTotal
	}
	tot.FL = flMax
	return tot
}

// MarketingProjections is the SP/PP/FP triple computed from group totals + header inputs.
//
// All four projections (SP, PP, FP, cost_sim) use the same multiplier
// 1 + duty + anti — added as a single % uplift over the chosen base rate
// (SR / PR / defaultVal / sim). duty/anti are decimal (0.04 = 4%).
//
// Excel reference (Testing_RM_Cost.xlsx row 5) writes `Q5% + P5%` for SP and
// the simulation cell — that is a known spreadsheet typo (the `%` operator
// divides decimal-stored values by 100 a second time, producing ≈1.0006).
// PP and FP in the same sheet correctly omit the `%` operator. Spec: all
// four projections behave identically.
type MarketingProjections struct {
	SP float64 // (SR + freight) * (1 + duty + anti) + transport
	PP float64 // (PR + freight) * (1 + duty + anti) + transport
	FP float64 // (defaultVal + freight) * (1 + duty + anti) + transport
}

// ComputeMarketingProjections produces SP/PP/FP from group totals and header
// marketing inputs. duty / anti are decimal in `h` (0.04 = 4%).
func ComputeMarketingProjections(tot GroupTotals, h HeaderInputsV2) MarketingProjections {
	multiplier := 1.0 + h.MarketingDutyPct + h.MarketingAntiDumpingPct
	out := MarketingProjections{}
	if tot.SR > 0 {
		out.SP = (tot.SR+h.MarketingFreightRate)*multiplier + h.MarketingTransportRate
	}
	if tot.PR > 0 {
		out.PP = (tot.PR+h.MarketingFreightRate)*multiplier + h.MarketingTransportRate
	}
	if h.MarketingDefaultValue > 0 {
		out.FP = (h.MarketingDefaultValue+h.MarketingFreightRate)*multiplier + h.MarketingTransportRate
	}
	return out
}

// ComputeSimulation produces cost_sim from sim_rate + marketing header inputs.
// Returns 0 when sim_rate is 0/unset. Same multiplier as SP/PP/FP.
func ComputeSimulation(simRate float64, h HeaderInputsV2) float64 {
	if simRate <= 0 {
		return 0
	}
	return (simRate+h.MarketingFreightRate)*(1.0+h.MarketingDutyPct+h.MarketingAntiDumpingPct) + h.MarketingTransportRate
}

// SelectValuation picks cost_val based on flag + group totals. AUTO triggers
// the CL→SL→FL fallback (first non-zero).
func SelectValuation(tot GroupTotals, flag string) float64 {
	switch flag {
	case "CR":
		return tot.CR
	case "SR":
		return tot.SR
	case "PR":
		return tot.PR
	case "CL":
		return tot.CL
	case "SL":
		return tot.SL
	case "FL":
		return tot.FL
	}
	// AUTO / "" / unknown → cascade.
	return firstNonZero(tot.CL, tot.SL, tot.FL)
}

// SelectMarketing picks cost_mark based on flag + projections. AUTO triggers
// the SP→PP→FP fallback.
func SelectMarketing(p MarketingProjections, flag string) float64 {
	switch flag {
	case "SP":
		return p.SP
	case "PP":
		return p.PP
	case "FP":
		return p.FP
	}
	return firstNonZero(p.SP, p.PP, p.FP)
}

// firstNonZero returns the first argument that is strictly > 0, else 0.
func firstNonZero(vs ...float64) float64 {
	for _, v := range vs {
		if v > 0 {
			return v
		}
	}
	return 0
}
