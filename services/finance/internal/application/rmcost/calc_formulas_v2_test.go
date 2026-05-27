package rmcost

import (
	"math"
	"testing"
)

// floatNear asserts |got-want| <= eps, with a helpful message.
func floatNear(t *testing.T, label string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s: got %.10f, want %.10f (diff %.2e)", label, got, want, math.Abs(got-want))
	}
}

// TestComputeDetail_ExcelRow10 locks the Excel reference for the CGC variant.
// Inputs: cons_val=1391.41, cons_qty=100, stock_val=2566.1, stock_qty=184.425,
// po_val=3000, po_qty=150, freight=0.06, anti=0.10 (decimal), duty=0.04, transport=0.08125.
// Expected from Excel:
//
//	K10 cons_rate                = 13.9141
//	M10 cons_val_based           = 1397.41
//	N10 cons_rate_based          = 13.9741
//	U10 cons_landed_cost (CL)    = 14.614314
//	X10 stock_rate               = 13.9140572048258
//	Z10 stock_val_based          = 2577.1655
//	AA10 stock_rate_based        = 13.9740572048258
//	AH10 stock_landed_cost (SL)  = 14.6142694930188
//	AK10 po_rate                 = 20
//	AR10 fix_landed_cost (FL)    = 0  (no fix_rate)
func TestComputeDetail_ExcelRow10(t *testing.T) {
	in := DetailInputs{FreightRate: 0.06, AntiDumpingPct: 0.10, DutyPct: 0.04, TransportRate: 0.08125}
	src := SourceQty{ConsVal: 1391.41, ConsQty: 100, StockVal: 2566.1, StockQty: 184.425, POVal: 3000, POQty: 150}
	o := ComputeDetail(in, src)

	const eps = 1e-6
	floatNear(t, "cons_rate", o.ConsRate, 13.9141, eps)
	floatNear(t, "cons_val_based", o.ConsValBased, 1397.41, eps)
	floatNear(t, "cons_rate_based", o.ConsRateBased, 13.9741, eps)
	floatNear(t, "cons_anti_dumping_val", o.ConsAntiDumpingVal, 139.741, eps)
	floatNear(t, "cons_anti_dumping_rate", o.ConsAntiDumpingRate, 1.39741, eps)
	floatNear(t, "cons_duty_val", o.ConsDutyVal, 55.8964, eps)
	floatNear(t, "cons_duty_rate", o.ConsDutyRate, 0.558964, eps)
	floatNear(t, "cons_transport_rate", o.ConsTransportRate, 0.08125, eps)
	floatNear(t, "CL", o.ConsLandedCost, 14.614314, eps)

	floatNear(t, "stock_rate", o.StockRate, 13.9140572048258, eps)
	floatNear(t, "stock_val_based", o.StockValBased, 2577.1655, eps)
	floatNear(t, "stock_rate_based", o.StockRateBased, 13.9740572048258, eps)
	floatNear(t, "SL", o.StockLandedCost, 14.6142694930188, eps)

	floatNear(t, "po_rate", o.PORate, 20, eps)
	floatNear(t, "FL (no fix_rate)", o.FixLandedCost, 0, eps)
}

// TestComputeDetail_FixRateProvided locks AR formula when fix_rate is set.
// fix_rate=15, freight=0.06, anti=0.10, duty=0.04, transport=0.08125
// Expected:
//
//	AN fix_rate_based       = 15.06
//	AO fix_anti_dumping_rate = 1.506
//	AP fix_duty_rate         = 0.6024
//	AQ fix_transport_rate    = 0.08125
//	AR FL                    = 17.24965
func TestComputeDetail_FixRateProvided(t *testing.T) {
	in := DetailInputs{FreightRate: 0.06, AntiDumpingPct: 0.10, DutyPct: 0.04, TransportRate: 0.08125, ValuationDefaultValue: 15}
	src := SourceQty{ConsVal: 0, ConsQty: 0, StockVal: 0, StockQty: 0}
	o := ComputeDetail(in, src)
	const eps = 1e-6
	floatNear(t, "fix_rate_based", o.FixRateBased, 15.06, eps)
	floatNear(t, "fix_anti_dumping_rate", o.FixAntiDumpingRate, 1.506, eps)
	floatNear(t, "fix_duty_rate", o.FixDutyRate, 0.6024, eps)
	floatNear(t, "fix_transport_rate", o.FixTransportRate, 0.08125, eps)
	floatNear(t, "FL", o.FixLandedCost, 17.24965, eps)
}

// TestAggregateGroupTotals_ExcelRow13 locks the group-total row from the
// Excel reference (rows 10+11 → row 12).
// Group totals INCLUDE anti-dumping at aggregate level (Excel V13/AI13).
// CL = rate_based + anti + transport + duty,
// SL = stock_rate_based + anti + transport + duty.
// FL = MAX(detail FL).
func TestAggregateGroupTotals_ExcelRow13(t *testing.T) {
	d10 := ComputeDetail(
		DetailInputs{FreightRate: 0.06, AntiDumpingPct: 0.10, DutyPct: 0.04, TransportRate: 0.08125},
		SourceQty{ConsVal: 1391.41, ConsQty: 100, StockVal: 2566.1, StockQty: 184.425, POVal: 3000, POQty: 150},
	)
	d11 := ComputeDetail(
		DetailInputs{FreightRate: 0.06, AntiDumpingPct: 0.0, DutyPct: 0.04, TransportRate: 0.0815},
		SourceQty{ConsVal: 222, ConsQty: 10, StockVal: 172.05, StockQty: 7.75, POVal: 200, POQty: 9},
	)
	tot := AggregateGroupTotals([]DetailOutput{d10, d11})
	const eps = 1e-6
	floatNear(t, "CR", tot.CR, 14.6673636363636, eps)
	floatNear(t, "SR", tot.SR, 14.2482112657734, eps)
	floatNear(t, "PR", tot.PR, 20.125786163522, eps)
	floatNear(t, "CL", tot.CL, 16.6681036363636, eps)
	floatNear(t, "SL", tot.SL, 16.3028511838376, eps)
	floatNear(t, "FL", tot.FL, 0, eps)
}

// TestComputeMarketingProjections_ExcelRow5 locks SP/PP/FP from the
// Excel reference. Inputs are decimal (0.05 = 5%) per the unified Option-C
// convention.
//
// Header: freight=0, anti=0, duty=0.05, transport=0.89, market_default=15.
// Group totals: SR=14.2482112657734, PR=20.125786163522.
// Expected (multiplier = 1.05):
//
//	SP = (14.2482112... + 0) * 1.05 + 0.89 = 15.8506218290621
//	PP = (20.1257861... + 0) * 1.05 + 0.89 = 22.0220754716981
//	FP = (15           + 0) * 1.05 + 0.89 = 16.64
func TestComputeMarketingProjections_ExcelRow5(t *testing.T) {
	tot := GroupTotals{
		SR: 14.2482112657734,
		PR: 20.125786163522,
	}
	h := HeaderInputsV2{
		MarketingFreightRate:    0,
		MarketingAntiDumpingPct: 0,
		MarketingDutyPct:        0.05,
		MarketingTransportRate:  0.89,
		MarketingDefaultValue:   15,
	}
	p := ComputeMarketingProjections(tot, h)
	const eps = 1e-6
	floatNear(t, "SP", p.SP, 15.8506218290621, eps)
	floatNear(t, "PP", p.PP, 22.0220754716981, eps)
	floatNear(t, "FP", p.FP, 16.64, eps)
}

// TestComputeSimulation_ExcelW5 locks W5 with decimal inputs: sim=2,
// duty=0.05 (decimal), transport=0.89 → 2.99.
func TestComputeSimulation_ExcelW5(t *testing.T) {
	h := HeaderInputsV2{
		MarketingFreightRate:    0,
		MarketingAntiDumpingPct: 0,
		MarketingDutyPct:        0.05,
		MarketingTransportRate:  0.89,
	}
	got := ComputeSimulation(2, h)
	floatNear(t, "cost_sim", got, 2.99, 1e-6)
}

func TestComputeSimulation_Zero(t *testing.T) {
	got := ComputeSimulation(0, HeaderInputsV2{MarketingDutyPct: 0.05, MarketingTransportRate: 0.89})
	floatNear(t, "zero sim", got, 0, 1e-9)
}

// TestSelectValuation_FlagSL locks "flag=SL → cost_val = SL group total".
func TestSelectValuation_FlagSL(t *testing.T) {
	tot := GroupTotals{CR: 14.6673636363636, SR: 14.2482112657734, PR: 20.125786163522,
		CL: 16.6681036363636, SL: 16.3028511838376, FL: 0}
	got := SelectValuation(tot, "SL")
	floatNear(t, "cost_val SL", got, 16.3028511838376, 1e-9)
}

// TestAggregateGroupTotals_TestingRMCost locks the user-provided Excel scenario
// (Testing_RM_Cost.xlsx, group GROUP-1 / Group-One, period 202604).
// Three details: CHP0000033/IRS, CHP0000033/NA, CHP0000034/NA.
// Expected from Excel row 13:
//
//	L13 CR = 0.83604572  (J13/K13)
//	Y13 SR = 0.81085784  (W13/X13)
//	AL13 PR = 1.07538082 (AJ13/AK13)
//	V13 CL = 1.04897898  (O13 + Q13 + U13 + S13)
//	AI13 SL = 1.02178214 (AB13 + AD13 + AH13 + AF13)
//	AS13 FL = 0.25405    (MAX(AS10:AS12))
func TestAggregateGroupTotals_TestingRMCost(t *testing.T) {
	// All three details share the same valuation inputs.
	in := DetailInputs{
		FreightRate:           0.06,
		AntiDumpingPct:        0.04,
		DutyPct:               0.04,
		TransportRate:         0.08125,
		ValuationDefaultValue: 0, // overridden per-row below
	}
	in1 := in
	in1.ValuationDefaultValue = 0.10 // CHP0000033/IRS
	d1 := ComputeDetail(in1, SourceQty{
		ConsVal: 615484.40, ConsQty: 711220.70,
		StockVal: 93013.94, StockQty: 96000.00,
		POVal: 81408.24, POQty: 72000.00,
	})
	in2 := in
	in2.ValuationDefaultValue = 0.09 // CHP0000033/NA
	d2 := ComputeDetail(in2, SourceQty{
		ConsVal: 225602.00, ConsQty: 287140.00,
		StockVal: 1196101.31, StockQty: 1500000.00,
		POVal: 856000.00, POQty: 800000.00,
	})
	in3 := in
	in3.ValuationDefaultValue = 0.09 // CHP0000034/NA
	d3 := ComputeDetail(in3, SourceQty{
		ConsVal: 135776.00, ConsQty: 170071.10,
		StockVal: 282327.25, StockQty: 342000.00,
		POVal: 215400.00, POQty: 200000.00,
	})
	tot := AggregateGroupTotals([]DetailOutput{d1, d2, d3})
	const eps = 5e-5
	floatNear(t, "CR", tot.CR, 0.83604572, eps)
	floatNear(t, "SR", tot.SR, 0.81085784, eps)
	floatNear(t, "PR", tot.PR, 1.07538082, eps)
	floatNear(t, "CL", tot.CL, 1.04897898, eps)
	floatNear(t, "SL", tot.SL, 1.02178214, eps)
	floatNear(t, "FL", tot.FL, 0.25405, eps)
}

// TestSelectValuation_AutoCascadeFL prefers CL when set, falls back to SL then FL.
func TestSelectValuation_AutoCascadeFL(t *testing.T) {
	tests := []struct {
		name string
		tot  GroupTotals
		want float64
	}{
		{"CL set", GroupTotals{CL: 1.5, SL: 2.5, FL: 3.5}, 1.5},
		{"CL=0, SL set", GroupTotals{CL: 0, SL: 2.5, FL: 3.5}, 2.5},
		{"CL=0, SL=0, FL set", GroupTotals{CL: 0, SL: 0, FL: 3.5}, 3.5},
		{"all zero", GroupTotals{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectValuation(tc.tot, "AUTO")
			floatNear(t, tc.name, got, tc.want, 1e-9)
		})
	}
}

// TestSelectMarketing_AutoCascade locks SP→PP→FP fallback.
func TestSelectMarketing_AutoCascade(t *testing.T) {
	got := SelectMarketing(MarketingProjections{SP: 0, PP: 0, FP: 7.5}, "AUTO")
	floatNear(t, "FP fallback", got, 7.5, 1e-9)
	got = SelectMarketing(MarketingProjections{SP: 1.1, PP: 2.2, FP: 7.5}, "AUTO")
	floatNear(t, "SP first", got, 1.1, 1e-9)
	got = SelectMarketing(MarketingProjections{SP: 1.1, PP: 2.2, FP: 7.5}, "PP")
	floatNear(t, "PP explicit", got, 2.2, 1e-9)
}

// TestFirstNonZero edge cases.
func TestFirstNonZero(t *testing.T) {
	if got := firstNonZero(0, 0, 0); got != 0 {
		t.Errorf("all zero: got %v", got)
	}
	if got := firstNonZero(0, 0.0001, 0.5); got != 0.0001 {
		t.Errorf("got %v", got)
	}
}
