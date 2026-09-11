// Package worker hosts the RabbitMQ-driven background jobs (RM cost export,
// product cost sheet export, and the calc-engine bridge).
package worker

// costsheet_rows.go is the single source of the product cost sheet's row
// layout. It intentionally does NOT derive rows from mst_parameter.display_order
// at runtime.
//
// The CSV template (<repo-root>/docs/export-product-cost/template_export_product_cost.csv)
// is a report layout, not a param list: two rows are both printed "33.", three
// rows are all-dash section separators (report rows 36, 68, 78), and one row is
// a bare section label with dashed filler cells and no printed number at all
// ("For sale of AX Grade only", report row 80) — four kindSeparator entries in
// total, three dashed plus one labeled. Deriving that from display_order would
// silently drift the moment someone adds or reorders a param. Pinning the
// layout here keeps display_order as the source of truth for the master-data
// screens while this file stays the source of truth for the export's fixed
// report shape. See design doc
// <repo-root>/docs/superpowers/specs/2026-08-04-cost-results-enhancements-design.md §2.2.
//
// The CSV carries 96 lines: one leading report-title line ("Report : Find
// Color by Product") plus 95 data rows. This manifest reproduces ALL 95 of
// them and adds ONE synthetic divider of its own, so len(costSheetRows) is 96,
// not 95. The layout splits into two sections:
//
//   - Manifest entries 1..84 — the printed sheet. Verified 2026-09-11 by
//     unzipping the reference workbook
//     (<repo-root>/data-examples/export-product-cost/example-export-param.xlsx):
//     its "parameter check" sheet ends at row 84 exactly, so this prefix is
//     what a user comparing against the reference expects to see on paper.
//
//   - Manifest entries 85.. — the "others" section, opened by the labeled
//     separator othersSeparatorLabel and carrying CSV rows 85-95. These rows
//     are present in the worksheet but are deliberately left OUTSIDE the
//     worksheet's print area, which costsheet_export_excel.go pins to the last
//     row before that separator (see applyPrintArea / printableRowCount).
//     Printing the sheet therefore still reproduces the reference's 84-row
//     shape while the extra rows stay available on screen.
//
// Nine of the others rows are also carried by the "all data" sheet (see
// costsheet_alldata.go), where they appear under printed header numbers
// 132-140 — sheet columns 141-149, EK..ES:
//
//	85.R-AX..              → 132.R-AX.            (R_AX)
//	86.R-AE./A9/A.         → 133.R-AE./A9/A       (R_AE_A9_A)
//	87.R-BC.               → 134.R-BC (%)         (R_BC)
//	88.R NS SP.            → 135.R NS SP          (R_NON_STD_SP)
//	89.R NS difference.    → 136.R NS difference   (R_NON_STD_DIFF)
//	90.B/C SP.             → 137.B/C SP           (BC_SP)
//	91.R NS loss.          → 138.R NS loss        (R_NON_STD_LOSS)
//	92.R BC loss.          → 139.R BC loss        (R_BC_LOSS)
//	94.Addl Val Loss.      → 140.Addl Val Loss    (ADDITIONAL_VAL_LOSS)
//
// Those nine carry no per-sheet param code of their own here and stay
// kindMissing — no mst_parameter code was proven for them, so they print "-".
// The remaining two others rows DO have params, verified 2026-09-11 by
// grepping <repo-root>/goapps-backend/services/finance/migrations/postgres/:
//
//   - "93.Std loss as above." → QLTY_LOSS_DELIVERY_COST, seeded active in
//     000407_seed_oracle_142_params.up.sql:109 and produced by formula
//     F_YARN_QLOSS_DEL (000408_seed_oracle_formulas.up.sql:47, is_active TRUE).
//   - "95.Domestic cost with uneven packing." → DOMESTIC_COST_UNEVEN_PACK,
//     seeded in 000469_seed_derived_cost_params.up.sql:49 with formula
//     F_YARN_DOMESTIC_COST_UNEVEN (same file, line 71 — inserted with
//     is_active TRUE by the PART 2 INSERT, currently a pass-through of
//     DOMESTIC_COST until costing supplies the uneven-packing delta).
//
// No later migration soft-deletes or deactivates either param or formula
// (checked every "UPDATE mst_formula" in migrations/postgres/).

// sheetRowKind classifies where a row's value comes from.
type sheetRowKind int

const (
	kindSnapshot  sheetRowKind = iota // numeric value from cpc_param_snapshot[ParamCode]
	kindText                          // snapshot value rendered as a string (codes/names)
	kindStage                         // from the route seq / product master, not the snapshot
	kindSeparator                     // dashed filler row, no value
	kindMissing                       // Group A/B param with no formula — always "-"
)

// Excel number formats used across the sheet. Rates, costs, percentages, and
// final costs all share the same three-decimal format in the template; counts,
// production, and speed rows share the integer format. Exactly THREE rows carry
// numFmtDecimal4 — 13 (MB Rate.), 79 (Final Conversion excl MB.) and 81 (Cost
// lessQL,CO,Frwd.); the justification and the reference-workbook sweep behind
// that set are documented at the row-13 entry below.
const (
	numFmtDecimal  = "#,##0.000"
	numFmtDecimal4 = "#,##0.0000"
	numFmtInt      = "#,##0"
	numFmtText     = "@"
)

// sheetRow is one of the 96 fixed entries of the product cost sheet: the
// CSV template's 95 data rows plus the synthetic "others" divider.
type sheetRow struct {
	Num       string // the row number as printed in column A ("1.", "33.", "" for separators)
	Label     string // the printed label
	ParamCode string // mst_parameter.param_code; empty for separator/stage rows
	Kind      sheetRowKind
	NumFmt    string // Excel number format; "" for non-numeric rows
}

// othersSeparatorLabel opens the "others" section that carries CSV rows
// 85-95. It is a labeled kindSeparator — the same mechanism as the
// "For sale of AX Grade only" divider — and it doubles as the print-area
// boundary marker read by printableRowCount.
const othersSeparatorLabel = "Others (CSV rows 85-95) - outside print area"

// printableRowCount reports how many manifest rows fall inside the worksheet's
// print area: every row up to, but not including, the othersSeparatorLabel
// divider. Falls back to the whole manifest if the divider is ever removed, so
// a missing marker widens the print area rather than emptying it.
func printableRowCount() int {
	for i := range costSheetRows {
		if costSheetRows[i].Kind == kindSeparator && costSheetRows[i].Label == othersSeparatorLabel {
			return i
		}
	}
	return len(costSheetRows)
}

// costSheetRows is the ordered, fixed layout of the product cost sheet,
// transcribed from template_export_product_cost.csv in CSV order. Duplicate
// printed numbers (two "33."s) and the dashed section separators (rows 36,
// 68, 78, and the unnumbered "For sale of AX Grade only" divider) are
// reproduced faithfully because the manifest is a report layout, not a param
// list. Column A is rendered as Num+Label, so the leading space on the indented
// sub-item labels (rows 50-54 and 70-77) is deliberate — it reproduces the
// template's indentation and must not be trimmed.
var costSheetRows = []sheetRow{
	{Num: "1.", Label: "Particulars.", Kind: kindStage},
	{Num: "2.", Label: "Product Name.", Kind: kindStage},
	{Num: "3.", Label: "Item Code.", Kind: kindStage},
	{Num: "4.", Label: "Item Name.", Kind: kindStage},
	{Num: "5.", Label: "Raw Material.", Kind: kindStage},
	{Num: "6.", Label: "RM Rate.", ParamCode: "RM_RATE", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "7.", Label: "RM Landed Cost.", ParamCode: "RM_LANDED_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "8.", Label: "Total End.", ParamCode: "NO_OF_END", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "9.", Label: "Total Fixed Cost.", ParamCode: "TOTAL_FIXED_COST", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "10.", Label: "Shade Code / Name.", Kind: kindStage},
	{Num: "11.", Label: "MB / SP Dye Name.", ParamCode: "MB_SP_DYE", Kind: kindText, NumFmt: numFmtText},
	{Num: "12.", Label: "Dozing %.", ParamCode: "MB_SP_DOZING", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	// 4 decimals, not 3. A sweep of the reference workbook
	// (<repo-root>/data-examples/export-product-cost/example-export-param.xlsx, sheet
	// "parameter check") found exactly THREE cells carrying more than 3 dp:
	// this row (6.6858), row 79 (0.7927) and row 81 (2.0141). The other two
	// already declare numFmtDecimal4, so the 4-dp set is a deliberate pattern
	// in the template rather than one stray sample; 3 dp here would round
	// 6.6858 to 6.686 and silently lose a digit the template shows.
	{Num: "13.", Label: "MB Rate.", ParamCode: "MB_RATE_MKT", Kind: kindSnapshot, NumFmt: numFmtDecimal4},
	{Num: "14.", Label: "Machine Name.", ParamCode: "MC_NAME", Kind: kindText, NumFmt: numFmtText},
	{Num: "15.", Label: "Net Prdn.", ParamCode: "NET_PRODUCTION", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "16.", Label: "MC Speed.", ParamCode: "MC_SPEED", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "17.", Label: "TPM.", ParamCode: "TPM", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "18.", Label: "Denier.", ParamCode: "DENIER", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "19.", Label: "Actual Denier.", ParamCode: "ACT_DENIER", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "20.", Label: "No of Ply.", ParamCode: "NO_OF_PLY", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "21.", Label: "No of Filaments.", ParamCode: "NO_OF_FILAMENTS", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "22.", Label: "MC Efficiency.", ParamCode: "MC_EFFICIENCY", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "23.", Label: "Waste %.", ParamCode: "WASTE_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "24.", Label: "OPU % / Rate.", ParamCode: "OPU", Kind: kindText, NumFmt: numFmtText},
	{Num: "25.", Label: "AX.", ParamCode: "AX_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "26.", Label: "AE.", ParamCode: "AE_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "27.", Label: "A9.", ParamCode: "A9_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "28.", Label: "A.", ParamCode: "A_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "29.", Label: "B.", ParamCode: "B_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "30.", Label: "C.", ParamCode: "C_PERC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "31.", Label: "Captpack Name.", ParamCode: "CAPTIVE_PACK_CODE", Kind: kindText, NumFmt: numFmtText},
	// CSV prints "33." twice (Delpack Name, then AX-wt.) — no "32." exists in the source template.
	{Num: "33.", Label: "Delpack Name.", ParamCode: "DELIVERY_PACK_CODE", Kind: kindText, NumFmt: numFmtText},
	{Num: "33.", Label: "AX-wt.", ParamCode: "AX_WT", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "34.", Label: "No of Bobbins.", ParamCode: "CAPTIVE_NO_OF_BOB", Kind: kindSnapshot, NumFmt: numFmtInt},
	{Num: "35.", Label: "RM Norm.", ParamCode: "RM_NORMS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Kind: kindSeparator},
	{Num: "37.", Label: "Duty,Inward,Waste.", ParamCode: "DUTY_INWARD_WASTE", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "38.", Label: "Oil Cost.", ParamCode: "OIL_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "39.", Label: "Oil Gain.", ParamCode: "OIL_GAIN", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "40.", Label: "MB Cost.", ParamCode: "MB_COST_MKT", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "41.", Label: "Intermingling.", ParamCode: "INTERMINGLING", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "42.", Label: "Heatset Cost per Kg.", ParamCode: "HEATSET_COST_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "43.", Label: "Spl Cost flag.", ParamCode: "SPECIAL_COST_FLAG", Kind: kindText, NumFmt: numFmtText},
	{Num: "44.", Label: "Spl Cost 2.", ParamCode: "SPECIAL_COST_2", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "45.", Label: "Spl Cost 1.", ParamCode: "SPECIAL_COST_1", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "46.", Label: "Steam Cost (CNG).", ParamCode: "STEAM_COST_CNG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "47.", Label: "Softner Cost.", ParamCode: "SOFTNER_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "48.", Label: "Washing Cost.", ParamCode: "WASHING_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	// "Fixed Cost." prints "-" for every product in the template; treated as a
	// Group A/B section label with no formula rather than a real snapshot value.
	{Num: "49.", Label: "Fixed Cost.", Kind: kindMissing},
	{Num: "50.", Label: " Pwr/kg.", ParamCode: "POWER_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "51.", Label: " MP/kg.", ParamCode: "MANPOWER_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "52.", Label: " OH/kg.", ParamCode: "OVERHEAD_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "53.", Label: " CS/kg.", ParamCode: "SPARESCOST_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "54.", Label: " Total Fixed Cost.", ParamCode: "TOTAL_FIXEDCOST_PER_KG", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "55.", Label: "Cap-Pack cost.", ParamCode: "CAPTIVE_PACK_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "56.", Label: "Del-Pack cost.", ParamCode: "DELIVERY_PACK_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "57.", Label: "Quality Loss. Cap Cost.", ParamCode: "QLTY_LOSS_CAPTIVE_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "58.", Label: "Quality Loss. Delv Cost.", ParamCode: "QLTY_LOSS_DELIVERY_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "59.", Label: "Total Conv. Cost Cap Cost.", ParamCode: "ONLY_CONV_CAP_PACK_EXCL_MB", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "60.", Label: "Total Conv. Cost Del Cost.", ParamCode: "ONLY_CONV_DEL_PACK_EXCL_MB", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "61.", Label: "CapCost with Q.Loss.", ParamCode: "CAPTIVE_COST_QLTY_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "62.", Label: "DelCost with Q.Loss.", ParamCode: "DELIVERY_COST_QLTY_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "63.", Label: "Change Over Loss (4).", ParamCode: "CHANGE_OVER_QLTY_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	// "Extra Yarn if any %." and "Cost of extra Yarn." print "-" for every
	// product in the template; no corresponding formula param exists.
	{Num: "64.", Label: "Extra Yarn if any %.", Kind: kindMissing},
	{Num: "65.", Label: "Cost of extra Yarn.", Kind: kindMissing},
	{Num: "66.", Label: "Forwarding Cost.", ParamCode: "FORWARDING_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "67.", Label: "Domestic Cost. (AX~AM).", ParamCode: "DOMESTIC_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Kind: kindSeparator},
	// "Standard Quality Loss Calc.." prints "-" for every product in the
	// template; treated as a section label rather than a snapshot value.
	{Num: "69.", Label: "Standard Quality Loss Calc..", Kind: kindMissing},
	{Num: "70.", Label: " NS Loss Type.", ParamCode: "NS_LOSS_TYPE", Kind: kindText, NumFmt: numFmtText},
	{Num: "71.", Label: " BC Loss Type.", ParamCode: "BC_LOSS_TYPE", Kind: kindText, NumFmt: numFmtText},
	{Num: "72.", Label: " NS Loss.", ParamCode: "NS_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "73.", Label: " STD SP AX.", ParamCode: "STD_SP_AX", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "74.", Label: " STD SP BC.", ParamCode: "STD_SP_BC", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "75.", Label: " NS V.loss.", ParamCode: "NON_STD_VALUE_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "76.", Label: " BC V Loss (Cap).", ParamCode: "BC_VAL_LOSS_CAPTIVE", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "77.", Label: " BC V Loss (Del).", ParamCode: "BC_VAL_LOSS_DELIVERY", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Kind: kindSeparator},
	{Num: "79.", Label: "Final Conversion excl MB.", ParamCode: "DELIVERY_CONVERSION", Kind: kindSnapshot, NumFmt: numFmtDecimal4},
	// Unnumbered section divider in the source template — printed as a label
	// with dashed filler cells, not a data row.
	{Label: "For sale of AX Grade only", Kind: kindSeparator},
	// "Cost lessQL,CO,Frwd." has NO mst_parameter code — this was settled by
	// query, not assumed. <repo-root>/docs/export-product-cost/verify-param-rows-81-84.sql
	// (run 2026-09-11) swept mst_parameter by name for "less"/"frwd"/"forward"/
	// "domestic"/"AX grade" and found only R_AX, AX_PERC, AX_WT and
	// WASTE_LESS_MB_OPU — all pre-existing rows in unrelated display groups.
	// Migrations 000234/000240/000381/000391/000407/000469 were swept too.
	// The param was never seeded, so it has no formula and never reaches
	// cpc_param_snapshot: wiring a ParamCode here would still print "-".
	// Seeding it is a costing-team decision, not an export fix.
	// NumFmt is carried anyway so the row prints with the template's four
	// decimals the day a param does exist.
	{Num: "81.", Label: "Cost lessQL,CO,Frwd.", Kind: kindMissing, NumFmt: numFmtDecimal4},
	{Num: "82.", Label: "NSBC SP.", ParamCode: "NON_STD_BC_SP", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "83.", Label: "Addl. NSBC Loss.", ParamCode: "ADD_NON_STD_BC_LOSS", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	// "Domestic Cost AX grd only." has NO mst_parameter code either — same
	// sweep, same verdict as row 81 above. ⚠ And it is NOT derivable from its
	// neighbors: the reference shows 84 = 2.306 while 81 + 83 = 2.0141 + 0.231
	// = 2.2451, so the obvious sum is wrong. Do not infer a formula from the
	// template's numbers.
	{Num: "84.", Label: "Domestic Cost AX grd only.", Kind: kindMissing},
	// ── "Others" section ──────────────────────────────────────────────────────
	// Everything below this labeled separator is CSV rows 85-95. It sits
	// OUTSIDE the worksheet print area (see applyPrintArea in
	// costsheet_export_excel.go), so printing still reproduces the reference
	// workbook's 84-row sheet while the rows stay visible on screen.
	{Label: othersSeparatorLabel, Kind: kindSeparator},
	// Labels below are transcribed verbatim from
	// <repo-root>/docs/export-product-cost/template_export_product_cost.csv
	// lines 86-96 (CSV data rows 85-95), including the doubled full stop on
	// "R-AX..". Rows 85-92 and 94 have no proven mst_parameter code and stay
	// kindMissing; only 93 and 95 were verified against the seed migrations.
	{Num: "85.", Label: "R-AX..", Kind: kindMissing},
	{Num: "86.", Label: "R-AE./A9/A.", Kind: kindMissing},
	{Num: "87.", Label: "R-BC.", Kind: kindMissing},
	{Num: "88.", Label: "R NS SP.", Kind: kindMissing},
	{Num: "89.", Label: "R NS difference.", Kind: kindMissing},
	{Num: "90.", Label: "B/C SP.", Kind: kindMissing},
	{Num: "91.", Label: "R NS loss.", Kind: kindMissing},
	{Num: "92.", Label: "R BC loss.", Kind: kindMissing},
	{Num: "93.", Label: "Std loss as above.", ParamCode: "QLTY_LOSS_DELIVERY_COST", Kind: kindSnapshot, NumFmt: numFmtDecimal},
	{Num: "94.", Label: "Addl Val Loss.", Kind: kindMissing},
	{Num: "95.", Label: "Domestic cost with uneven packing.", ParamCode: "DOMESTIC_COST_UNEVEN_PACK", Kind: kindSnapshot, NumFmt: numFmtDecimal},
}
