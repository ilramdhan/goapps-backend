package worker

// costsheet_alldata.go holds the column manifest of the workbook's "all data"
// sheet — the flat, one-row-per-stage counterpart of the transposed per-product
// sheet rendered by costsheet_export_excel.go.
//
// CODE GENERATED from <repo-root>/docs/export-product-cost/all-data-column-map.tsv
// by <repo-root>/docs/export-product-cost/gen_alldata.py. Both paths are
// relative to the WORKSPACE root (the directory holding goapps-backend/), not
// to this service. Re-run that script rather than editing the table by hand.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// allDataColumnKind distinguishes where a column's value is read from.
type allDataColumnKind int

const (
	// allDataIdentity reads the value from the stage identity (item code, shade,
	// route position) rather than from the calculated parameter snapshot.
	allDataIdentity allDataColumnKind = iota
	// allDataParam reads the value from the stage's ParamSnapshot under ParamCode.
	allDataParam
)

// Identity column selectors. These name the nine leading columns whose value
// comes from the stage record instead of the parameter snapshot.
const (
	identityNo = iota
	identityLeftSysID
	identityLeftNo
	identityName
	identityYarnType
	identityItemCode
	identityShadeCode
	identityShadeName
	identityMachineCode
)

// allDataColumn is one column of the "all data" sheet.
type allDataColumn struct {
	// Header is the exact text written into row 1.
	Header string
	// Kind selects the value source.
	Kind allDataColumnKind
	// ParamCode is the mst_parameter code read from ParamSnapshot. Set only
	// when Kind is allDataParam.
	ParamCode string
	// Identity selects which stage identity field to read. Set only when Kind
	// is allDataIdentity.
	Identity int
}

// allDataColumns is the full 149-column manifest, in sheet order (A..ES).
var allDataColumns = []allDataColumn{
	{Header: "No", Kind: allDataIdentity, Identity: identityNo},
	{Header: "Left Sys ID", Kind: allDataIdentity, Identity: identityLeftSysID},
	{Header: "Left No", Kind: allDataIdentity, Identity: identityLeftNo},
	{Header: "Name", Kind: allDataIdentity, Identity: identityName},
	{Header: "Yarn Type", Kind: allDataIdentity, Identity: identityYarnType},
	{Header: "Item Code", Kind: allDataIdentity, Identity: identityItemCode},
	{Header: "Shade Code", Kind: allDataIdentity, Identity: identityShadeCode},
	{Header: "Shade Name", Kind: allDataIdentity, Identity: identityShadeName},
	{Header: "Machine Code", Kind: allDataIdentity, Identity: identityMachineCode},
	{Header: "1.Item Code", Kind: allDataParam, ParamCode: "ORION_ITEM"},
	{Header: "2.Marketing Costing Link", Kind: allDataParam, ParamCode: "COSTING_LINK"},
	{Header: "3.Orion Link", Kind: allDataParam, ParamCode: "ORION_LINK"},
	{Header: "4.Item Name", Kind: allDataParam, ParamCode: "ITEM_NAME"},
	{Header: "5.Shade Code", Kind: allDataParam, ParamCode: "SHADE_CODE"},
	{Header: "6.Shade Name", Kind: allDataParam, ParamCode: "SHADE_NAME"},
	{Header: "7.Machine Name", Kind: allDataParam, ParamCode: "MC_NAME"},
	{Header: "8.No of Position", Kind: allDataParam, ParamCode: "NO_OF_POSITION"},
	{Header: "9.No of End", Kind: allDataParam, ParamCode: "NO_OF_END"},
	{Header: "10.MC Efficiency", Kind: allDataParam, ParamCode: "MC_EFFICIENCY"},
	{Header: "11.MC Speed", Kind: allDataParam, ParamCode: "MC_SPEED"},
	{Header: "12.TPM", Kind: allDataParam, ParamCode: "TPM"},
	{Header: "13.Denier", Kind: allDataParam, ParamCode: "DENIER"},
	{Header: "14.Actual Denier", Kind: allDataParam, ParamCode: "ACT_DENIER"},
	{Header: "15.No of Ply", Kind: allDataParam, ParamCode: "NO_OF_PLY"},
	{Header: "16.No of Filaments", Kind: allDataParam, ParamCode: "NO_OF_FILAMENTS"},
	{Header: "17.Cross Section", Kind: allDataParam, ParamCode: "CROSS_SECTION"},
	{Header: "18.Inter Migle", Kind: allDataParam, ParamCode: "INTERMINGLE"},
	{Header: "19.RM Table Type", Kind: allDataParam, ParamCode: "RM_TYPE"},
	{Header: "20.Raw Material", Kind: allDataParam, ParamCode: paramRawMaterial},
	{Header: "21.Waste %", Kind: allDataParam, ParamCode: "WASTE_PERC"},
	{Header: "22.OPU", Kind: allDataParam, ParamCode: "OPU"},
	{Header: "23.AX", Kind: allDataParam, ParamCode: "AX_PERC"},
	{Header: "24.AE", Kind: allDataParam, ParamCode: "AE_PERC"},
	{Header: "25.A9", Kind: allDataParam, ParamCode: "A9_PERC"},
	{Header: "26.A", Kind: allDataParam, ParamCode: "A_PERC"},
	{Header: "27.B", Kind: allDataParam, ParamCode: "B_PERC"},
	{Header: "28.C", Kind: allDataParam, ParamCode: "C_PERC"},
	{Header: "29.Y-Type", Kind: allDataParam, ParamCode: "Y_TYPE"},
	{Header: "30.AX-wt", Kind: allDataParam, ParamCode: "AX_WT"},
	{Header: "31.AE-wt", Kind: allDataParam, ParamCode: "AE_WT"},
	{Header: "32.A9-wt", Kind: allDataParam, ParamCode: "A9_WT"},
	{Header: "33.A-wt", Kind: allDataParam, ParamCode: "A_WT"},
	{Header: "34.B-wt", Kind: allDataParam, ParamCode: "B_WT"},
	{Header: "35.C-wt", Kind: allDataParam, ParamCode: "C_WT"},
	{Header: "36.Net Bbn Wt", Kind: allDataParam, ParamCode: "NET_BOB_WT"},
	{Header: "37.Captpack name", Kind: allDataParam, ParamCode: "CAPTIVE_PACK_CODE"},
	{Header: "38.No of Bobbins", Kind: allDataParam, ParamCode: "CAPTIVE_NO_OF_BOB"},
	{Header: "39.Box weight", Kind: allDataParam, ParamCode: "CAPTIVE_BOX_WT"},
	{Header: "40.Bobbin Rate", Kind: allDataParam, ParamCode: "CAPTIVE_BOB_RATE"},
	{Header: "41.Box Rate", Kind: allDataParam, ParamCode: "CAPTIVE_BOX_RATE"},
	{Header: "42.Cap-Pack cost", Kind: allDataParam, ParamCode: "CAPTIVE_PACK_COST"},
	{Header: "43.Delpack name", Kind: allDataParam, ParamCode: "DELIVERY_PACK_CODE"},
	{Header: "44.No of Bobbins", Kind: allDataParam, ParamCode: "DELIVERY_NO_OF_BOB"},
	{Header: "45.Box weight", Kind: allDataParam, ParamCode: "DELIVERY_BOX_WT"},
	{Header: "46.Bobbin Rate", Kind: allDataParam, ParamCode: "DELIVERY_BOB_RATE"},
	{Header: "47.Box Rate", Kind: allDataParam, ParamCode: "DELIVERY_BOX_RATE"},
	{Header: "48.Del-Pack cost", Kind: allDataParam, ParamCode: "DELIVERY_PACK_COST"},
	{Header: "49.Heatset flag", Kind: allDataParam, ParamCode: "HEATSET_CODE"},
	{Header: "50.No of trollies", Kind: allDataParam, ParamCode: "NO_OF_TROLLIES"},
	{Header: "51.No Bbns per trolley", Kind: allDataParam, ParamCode: "NO_BOB_PER_TROLLIES"},
	{Header: "52.Batch weight", Kind: allDataParam, ParamCode: "BATCH_WEIGHT"},
	{Header: "53.Heatset Cost per Batch", Kind: allDataParam, ParamCode: "HEATSET_COST_PER_BATCH"},
	{Header: "54.Heatset Cost per Kg", Kind: allDataParam, ParamCode: "HEATSET_COST_PER_KG"},
	{Header: "55.RM Rate", Kind: allDataParam, ParamCode: "RM_RATE"},
	{Header: "56.RM Landed cost", Kind: allDataParam, ParamCode: "RM_LANDED_COST"},
	{Header: "57.RM Norm", Kind: allDataParam, ParamCode: "RM_NORMS"},
	{Header: "58.Waste LESS Mb doz, opu", Kind: allDataParam, ParamCode: "WASTE_LESS_MB_OPU"},
	{Header: "59.Oil Name", Kind: allDataParam, ParamCode: "OIL_NAME"},
	{Header: "60.Oil Rate", Kind: allDataParam, ParamCode: "OIL_RATE"},
	{Header: "61.Oil Cost", Kind: allDataParam, ParamCode: "OIL_COST"},
	{Header: "62.mb Flag", Kind: allDataParam, ParamCode: "MB_FLAG"},
	{Header: "63.MB/SP Code", Kind: allDataParam, ParamCode: "MB_SP_CODE"},
	{Header: "64.MB/ SP Dye Name", Kind: allDataParam, ParamCode: "MB_SP_DYE"},
	{Header: "65.SP-Den", Kind: allDataParam, ParamCode: "MB_SP_DENIER"},
	{Header: "66.SP-Fil", Kind: allDataParam, ParamCode: "MB_SP_FILAMENT"},
	{Header: "67.SP-CC", Kind: allDataParam, ParamCode: "MB_SP_CC"},
	{Header: "68.SP-Dozing", Kind: allDataParam, ParamCode: "MB_SP_DOZING"},
	{Header: "69.Con-factor", Kind: allDataParam, ParamCode: "CONV_FACTOR"},
	{Header: "70.RP-CC", Kind: allDataParam, ParamCode: "RP_CC"},
	{Header: "71.RP-Doz", Kind: allDataParam, ParamCode: "RP_DOZING"},
	{Header: "72.MB Rate Marketing", Kind: allDataParam, ParamCode: "MB_RATE_MKT"},
	{Header: "73.MB Cost Marketing", Kind: allDataParam, ParamCode: "MB_COST_MKT"},
	{Header: "74.Intermingling", Kind: allDataParam, ParamCode: "INTERMINGLING"},
	{Header: "75.Spl Cost flag", Kind: allDataParam, ParamCode: "SPECIAL_COST_FLAG"},
	{Header: "76.Spl Cost1", Kind: allDataParam, ParamCode: "SPECIAL_COST_1"},
	{Header: "77.Spl Cost2", Kind: allDataParam, ParamCode: "SPECIAL_COST_2"},
	{Header: "78.Steam Cost (CNG)", Kind: allDataParam, ParamCode: "STEAM_COST_CNG"},
	{Header: "79.Softner Cost", Kind: allDataParam, ParamCode: "SOFTNER_COST"},
	{Header: "80.Washing Cost", Kind: allDataParam, ParamCode: "WASHING_COST"},
	{Header: "81.Production Index", Kind: allDataParam, ParamCode: "PRODUCT_INDEX"},
	{Header: "82.Net Prdn", Kind: allDataParam, ParamCode: "NET_PRODUCTION"},
	{Header: "83.Pwr/day", Kind: allDataParam, ParamCode: "POWER_PER_DAY"},
	{Header: "84.MP/day", Kind: allDataParam, ParamCode: "MANPOWER_PER_DAY"},
	{Header: "85.OH/day", Kind: allDataParam, ParamCode: "OVERHEAD_PER_HEAD"},
	{Header: "86.CS/day", Kind: allDataParam, ParamCode: "SPARESCOST_PER_DAY"},
	{Header: "87.Pwr/kg", Kind: allDataParam, ParamCode: "POWER_PER_KG"},
	{Header: "88.MP/kg", Kind: allDataParam, ParamCode: "MANPOWER_PER_KG"},
	{Header: "89.OH/kg", Kind: allDataParam, ParamCode: "OVERHEAD_PER_KG"},
	{Header: "90.CS/kg", Kind: allDataParam, ParamCode: "SPARESCOST_PER_KG"},
	{Header: "91.Total", Kind: allDataParam, ParamCode: "TOTAL_FIXEDCOST_PER_KG"},
	{Header: "92.Only Conversion Cap. Packing ex MB", Kind: allDataParam, ParamCode: "ONLY_CONV_CAP_PACK_EXCL_MB"},
	{Header: "93.Only Conversion Del. Packing ex MB", Kind: allDataParam, ParamCode: "ONLY_CONV_DEL_PACK_EXCL_MB"},
	{Header: "94.CapCost before Q.Loss", Kind: allDataParam, ParamCode: "CAPTIVE_COST_BEFORE_QLOSS"},
	{Header: "95.DelCost before Q.Loss", Kind: allDataParam, ParamCode: "DELIVERY_COST_BEFORE_QLOSS"},
	{Header: "96.Std Val. Loss", Kind: allDataParam, ParamCode: "NS_LOSS_TYPE"},
	{Header: "97.Value loss", Kind: allDataParam, ParamCode: "BC_LOSS_TYPE"},
	{Header: "98.NS SP", Kind: allDataParam, ParamCode: "NON_STD_BC_SP"},
	{Header: "99.BC SP", Kind: allDataParam, ParamCode: "BC_SP"},
	{Header: "100.NS V.loss", Kind: allDataParam, ParamCode: "NON_STD_VALUE_LOSS"},
	{Header: "101.BC V Loss (Cap)", Kind: allDataParam, ParamCode: "BC_VAL_LOSS_CAPTIVE"},
	{Header: "102.BC V Loss (Del)", Kind: allDataParam, ParamCode: "BC_VAL_LOSS_DELIVERY"},
	{Header: "103.Quality Loss. Cap Cost", Kind: allDataParam, ParamCode: "QLTY_LOSS_CAPTIVE_COST"},
	{Header: "104.Quality Loss. Delv Cost", Kind: allDataParam, ParamCode: "QLTY_LOSS_DELIVERY_COST"},
	{Header: "105.CapCost with Q.Loss", Kind: allDataParam, ParamCode: "CAPTIVE_COST_QLTY_LOSS"},
	{Header: "106.DelCost with Q.Loss", Kind: allDataParam, ParamCode: "DELIVERY_COST_QLTY_LOSS"},
	{Header: "107.Change over qty loss (info)", Kind: allDataParam, ParamCode: "CHANGE_OVER_QLTY_LOSS"},
	{Header: "108.VB1-Q", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_1_QTY"},
	{Header: "109.VB2-Q", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_2_QTY"},
	{Header: "110.VB3-Q", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_3_QTY"},
	{Header: "111.VB4-Q", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_4_QTY"},
	{Header: "112.VB5-Q", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_5_QTY"},
	{Header: "113.VB1-L", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_1_LOSS"},
	{Header: "114.VB2-L", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_2_LOSS"},
	{Header: "115.VB3-L", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_3_LOSS"},
	{Header: "116.VB4-L", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_4_LOSS"},
	{Header: "117.VB5-L", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_5_LOSS"},
	{Header: "118.VB1-Del Cost", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_1_DEL_COST"},
	{Header: "119.VB2-Del Cost", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_2_DEL_COST"},
	{Header: "120.VB3-Del Cost", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_3_DEL_COST"},
	{Header: "121.VB4-Del Cost", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_4_DEL_COST"},
	{Header: "122.VB5-Del Cost", Kind: allDataParam, ParamCode: "VOLUME_BUCKET_5_DEL_COST"},
	{Header: "123.CUSTOMER", Kind: allDataParam, ParamCode: "CUSTOMER"},
	{Header: "124.VALUATION", Kind: allDataParam, ParamCode: "VALUATION"},
	{Header: "125.OIL GAIN", Kind: allDataParam, ParamCode: "OIL_GAIN"},
	{Header: "126.DOZING ADJUST", Kind: allDataParam, ParamCode: "DOZING_ADJUST"},
	{Header: "127.% Add Top 95", Kind: allDataParam, ParamCode: "DELIVERY_COST_BEFORE_QLOSS_ADD_PER"},
	{Header: "128.Value Top 95 before Process", Kind: allDataParam, ParamCode: "DELIVERY_COST_BEFORE_QLOSS_BEFORE_PROCESS"},
	{Header: "129.Top 95 X % Add", Kind: allDataParam, ParamCode: "DELIVERY_COST_BEFORE_QLOSS_ADDITION"},
	{Header: "130.NSBC SP.", Kind: allDataParam, ParamCode: "NON_STD_BC_SP"},
	{Header: "131.Addl. NSBC Loss.", Kind: allDataParam, ParamCode: "ADD_NON_STD_BC_LOSS"},
	{Header: "132.R-AX.", Kind: allDataParam, ParamCode: "R_AX"},
	{Header: "133.R-AE./A9/A", Kind: allDataParam, ParamCode: "R_AE_A9_A"},
	{Header: "134.R-BC (%)", Kind: allDataParam, ParamCode: "R_BC"},
	{Header: "135.R NS SP", Kind: allDataParam, ParamCode: "R_NON_STD_SP"},
	{Header: "136.R NS difference", Kind: allDataParam, ParamCode: "R_NON_STD_DIFF"},
	{Header: "137.B/C SP", Kind: allDataParam, ParamCode: "BC_SP"},
	{Header: "138.R NS loss", Kind: allDataParam, ParamCode: "R_NON_STD_LOSS"},
	{Header: "139.R BC loss", Kind: allDataParam, ParamCode: "R_BC_LOSS"},
	{Header: "140.Addl Val Loss", Kind: allDataParam, ParamCode: "ADDITIONAL_VAL_LOSS"},
}

// =============================================================================
// "all data" sheet rendering
// =============================================================================

// allDataSheetName is the exact worksheet name of the flat sheet. It is the
// first sheet of the exported workbook, ahead of the per-product transposed
// sheets.
const allDataSheetName = "all data"

// parameterCheckSheetName is the worksheet name the reference workbook
// (data-examples/export-product-cost/example-export-param.xlsx) uses for the
// per-product transposed sheet, alongside "all data". It applies only to a
// single-product export: the reference is itself a single-product export, so
// that is the only mode it can specify. A bulk export cannot use a fixed name —
// N products would all collide on it and sanitizeSheetName would emit
// "parameter check (2)", "(3)", … which matches no reference and is less useful
// than the FG code, which at least tells the reader which product a sheet is.
const parameterCheckSheetName = "parameter check"

// Param codes the identity columns fall back to when the stage record itself
// carries no value. MC_NAME is a text param loaded from master data (see
// ProductLoader.LoadCAPPText), so it is the only source of the machine code.
const (
	paramMachineName = "MC_NAME"
	paramCostingLink = "COSTING_LINK"
	paramOrionLink   = "ORION_LINK"
	// paramRawMaterial is the "20.Raw Material" column of the flat sheet, also
	// used by the per-product sheet's "5.Raw Material." row.
	paramRawMaterial = "RAW_MATERIAL"
)

// WriteAllDataSheet renders the flat "all data" sheet into f: a single header
// row followed by one row per stage, 149 columns wide.
//
// Unlike the transposed per-product sheet, an absent parameter is left as a
// genuinely empty cell rather than a "-" placeholder — the flat sheet is a data
// extract meant to be filtered and pivoted, where a dash would poison every
// numeric column it lands in. Values that parse as numbers are written as real
// numeric cells at full float precision so downstream totals stay exact;
// everything else is written as a string.
//
// stages must already be in the order the rows should appear; the "No" column
// is the 1-based position within that order.
func WriteAllDataSheet(f *excelize.File, stages []Stage) error {
	// The caller may have pre-created the sheet to reserve its position as the
	// workbook's first; excelize appends new sheets at the end, and v2.8.1 has
	// no move-sheet operation, so position can only be claimed up front.
	idx, err := f.GetSheetIndex(allDataSheetName)
	if err != nil {
		return fmt.Errorf("look up %q sheet: %w", allDataSheetName, err)
	}
	if idx < 0 {
		if _, err := f.NewSheet(allDataSheetName); err != nil {
			return fmt.Errorf("create %q sheet: %w", allDataSheetName, err)
		}
	}
	if err := writeAllDataHeader(f); err != nil {
		return err
	}
	for i, stage := range stages {
		if err := writeAllDataRow(f, i+2, i+1, stage); err != nil {
			return err
		}
	}
	// The sheet is a wide extract; freezing the header keeps the column names
	// visible while scrolling through stages.
	if err := f.SetPanes(allDataSheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze %q header: %w", allDataSheetName, err)
	}
	return nil
}

// writeAllDataHeader writes row 1 — the 149 column names, verbatim.
func writeAllDataHeader(f *excelize.File) error {
	for i, col := range allDataColumns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return fmt.Errorf("all data header coordinate col=%d: %w", i+1, err)
		}
		if err := f.SetCellStr(allDataSheetName, cell, col.Header); err != nil {
			return fmt.Errorf("write all data header %s: %w", cell, err)
		}
	}
	return nil
}

// writeAllDataRow writes one stage's row. no is the printed sequence number.
func writeAllDataRow(f *excelize.File, excelRow, no int, stage Stage) error {
	for i, col := range allDataColumns {
		value := allDataCellValue(col, no, stage)
		if value == "" {
			// Absent params stay absent: an empty cell, never a zero or a dash.
			continue
		}
		cell, err := excelize.CoordinatesToCellName(i+1, excelRow)
		if err != nil {
			return fmt.Errorf("all data coordinate col=%d row=%d: %w", i+1, excelRow, err)
		}
		if err := setAllDataCell(f, cell, value); err != nil {
			return err
		}
	}
	return nil
}

// setAllDataCell writes raw as a real number when it parses as one, and as a
// string otherwise. Numbers keep full float precision — the flat sheet is not
// a print layout, so nothing is rounded for display here.
func setAllDataCell(f *excelize.File, cell, raw string) error {
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		if err := f.SetCellFloat(allDataSheetName, cell, number, -1, 64); err != nil {
			return fmt.Errorf("write all data number %s: %w", cell, err)
		}
		return nil
	}
	if err := f.SetCellStr(allDataSheetName, cell, raw); err != nil {
		return fmt.Errorf("write all data text %s: %w", cell, err)
	}
	return nil
}

// allDataCellValue resolves one column for one stage as its raw string form.
// An empty result means "write nothing".
func allDataCellValue(col allDataColumn, no int, stage Stage) string {
	if col.Kind == allDataIdentity {
		return allDataIdentityValue(col.Identity, no, stage)
	}
	switch col.ParamCode {
	case paramCostingLink:
		// The snapshot is authoritative; compose the documented
		// item-shade-machine form only when the master never stored one.
		return firstNonEmpty(snapshotValue(col.ParamCode, stage), composeCostingLink(stage))
	case paramOrionLink:
		return firstNonEmpty(snapshotValue(col.ParamCode, stage), composeOrionLink(stage))
	default:
		return snapshotValue(col.ParamCode, stage)
	}
}

// allDataIdentityValue resolves the nine leading identity columns from the
// stage record, falling back to the snapshot where the stage carries no value.
func allDataIdentityValue(identity, no int, stage Stage) string {
	switch identity {
	case identityNo:
		return strconv.Itoa(no)
	case identityLeftSysID:
		// The legacy Oracle sys id (cpm_flex_02), not our own primary key.
		return stage.LeftSysID
	case identityLeftNo:
		// cost_product_master's own serial id, which is what the legacy sheet's
		// "Left No" column tracks alongside the legacy sys id.
		if stage.ProductSysID <= 0 {
			return ""
		}
		return strconv.FormatInt(stage.ProductSysID, 10)
	case identityName:
		return stage.ProductName
	case identityYarnType:
		// The legacy product type label (cpm_flex_03) — "POY", "MELANGE".
		return stage.YarnType
	case identityItemCode:
		return stage.ItemCode
	case identityShadeCode:
		return stage.ShadeCode
	case identityShadeName:
		return stage.ShadeName
	case identityMachineCode:
		// Never denormalized onto the route seq; MC_NAME is a text param read
		// from current master data and holds the machine code.
		return stageMachineCode(stage)
	default:
		return ""
	}
}

// stageMachineCode reads MC_NAME without the HasCost gate that snapshotValue
// applies. MC_NAME is not a calculated value: applyCapText overlays it from
// current master data regardless of whether the stage has a cost row, so gating
// it would blank the Machine Code column for every uncalculated stage. The flat
// sheet's identity columns must be populated even where the costing is not.
func stageMachineCode(stage Stage) string {
	if stage.ParamSnapshot == nil {
		return ""
	}
	return strings.TrimSpace(stage.ParamSnapshot[paramMachineName])
}

// composeCostingLink builds the marketing costing link from stage identity:
// "<item code>-<shade code>-<machine code>". Returns empty unless every part is
// present, so a partial key is never mistaken for a real one.
func composeCostingLink(stage Stage) string {
	machine := stageMachineCode(stage)
	if stage.ItemCode == "" || stage.ShadeCode == "" || machine == "" {
		return ""
	}
	return stage.ItemCode + "-" + stage.ShadeCode + "-" + machine
}

// composeOrionLink builds the Orion link from stage identity:
// "<item code>-<shade code>".
func composeOrionLink(stage Stage) string {
	if stage.ItemCode == "" || stage.ShadeCode == "" {
		return ""
	}
	return stage.ItemCode + "-" + stage.ShadeCode
}

// firstNonEmpty returns the first non-empty argument, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
