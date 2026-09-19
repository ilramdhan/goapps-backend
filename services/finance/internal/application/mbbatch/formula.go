package mbbatch

// The 7 F_MB_* formula codes and their result param codes, per migration
// 000452_seed_mst_formula_mb.up.sql (the definitive source — note F_MB_FIXED_COST's result
// param code is MB_FIXED_TOTAL, not MB_FIXED_COST; a deliberate divergence in the seed data).
//
// All 7 formulas are computed independently, from scratch, for EVERY calc type
// (ACTUAL/SELLING/FORECAST) — none of them is copied from another calc type's pass. This
// used to not be true: F_MB_NET_PROD, F_MB_WASTE_VAL, F_MB_FIXED_COST, F_MB_COST_OTHERS and
// F_MB_CONV_COST were computed once on the ACTUAL pass and copied into SELLING/FORECAST via
// CAPP pre-seeding (see the removed sharedFormulaCodes/partitionFormulas/mergeCAPP/
// sharedOutputs mechanism). That was a bug: F_MB_WASTE_VAL and F_MB_COST_OTHERS both consume
// MB_RM_COST, which legitimately differs per calc type (actual=cost_val, forecast=cost_mark,
// selling=cost_sim — see costcalc/loader.go LoadRMCosts), so their outputs — and F_MB_CONV_COST,
// which transitively depends on them — must differ per calc type too, not be forced equal.
const (
	FormulaCodeRMCost    = "F_MB_RM_COST"
	FormulaCodeWasteVal  = "F_MB_WASTE_VAL"
	FormulaCodeNetProd   = "F_MB_NET_PROD"
	FormulaCodeFixedCost = "F_MB_FIXED_COST"
	FormulaCodeOthers    = "F_MB_COST_OTHERS"
	FormulaCodeConvCost  = "F_MB_CONV_COST"
	FormulaCodeFinalCost = "F_MB_FINAL_COST"

	ResultParamRMCost    = "MB_RM_COST"
	ResultParamWasteVal  = "MB_WASTE_VAL"
	ResultParamNetProd   = "MB_NET_PROD"
	ResultParamFixedCost = "MB_FIXED_TOTAL"
	ResultParamOthers    = "MB_COST_OTHERS"
	ResultParamConvCost  = "MB_CONV_COST"
	ResultParamFinalCost = "MB_FINAL_COST"
)
