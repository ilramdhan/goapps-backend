package worker

// costsheet_export_excel.go renders the fixed 96-entry manifest in
// costsheet_rows.go into an A4 xlsx workbook, one column per route stage.
// Only the rows before the "others" separator are inside the print area; see
// applyPrintArea.
// See design doc
// docs/superpowers/specs/2026-08-04-cost-results-enhancements-design.md §3.

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// MaxStagesPerPage is the largest number of route stages that still renders
// legibly on one A4 portrait page. The sheet is set to fit one page wide, so
// exceeding this does not break the layout — Excel simply scales the print
// down further until the columns become unreadable. Callers (the bulk-export
// worker) should surface a warning in the job's result summary when a product
// has more stages than this.
const MaxStagesPerPage = 12

// Stage is one route stage — one column of the product cost sheet. It mirrors
// the application-layer costcalc.RouteCostSheetStage so the worker package does
// not import the application layer. ParamSnapshot is stringified; an absent key
// means the value was never calculated and must render as "-", never as zero.
type Stage struct {
	RouteLevel    int32
	RouteSeq      int32
	RouteName     string
	ItemCode      string
	ProductName   string
	ShadeCode     string
	ShadeName     string
	ProductSysID  int64
	HasCost       bool
	ParamSnapshot map[string]string
	// LeftSysID is the legacy Oracle sys id (cpm_flex_02) — the "Left Sys ID"
	// column of the flat "all data" sheet. Empty for products that were never
	// imported from the legacy system.
	LeftSysID string
	// YarnType is the legacy product type label (cpm_flex_03) — "POY",
	// "MELANGE". The "Yarn Type" column of the flat "all data" sheet.
	YarnType string
}

// Rendering constants for the sheet's fixed look.
const (
	// dashValue is printed wherever a value is absent. Never substitute a zero.
	dashValue = "-"

	defaultSheetName = "Cost Sheet"

	labelColWidth  = 30.0
	stageColWidth  = 13.0
	sheetRowHeight = 11.0

	fontName    = "Calibri"
	fontSize    = 7.0
	borderColor = "BFBFBF"

	// headerRowCount is the number of rows above the manifest. The sheet has no
	// title or column-header block — the target template starts straight at
	// "1.Particulars." on row 1 — so the manifest is written from row 1 down.
	headerRowCount = 0

	// maxSheetNameLen is Excel's hard limit on worksheet names.
	maxSheetNameLen = 31

	// printAreaDefinedName is the OOXML built-in defined name that holds a
	// worksheet's print area.
	printAreaDefinedName = "_xlnm.Print_Area"

	// labelSeparatorFill and stageSeparatorFill reproduce the dashed divider
	// rows of the CSV template. labelSeparatorFill (column A, 35 dashes) matches
	// the reference workbook on every separator row.
	//
	// ⚠ stageSeparatorFill is deliberately ONE width for all four separator rows,
	// even though the reference is itself inconsistent: unzipping
	// <repo-root>/data-examples/export-product-cost/example-export-param.xlsx
	// (sheet "parameter check", checked 2026-09-11) shows 21 dashes on row 36 but
	// only 17 on rows 68, 78 and 80. The CSV template carries the same 21/17/17/17
	// split. Since the dashes are pure visual filler — no formula, no total, no
	// reader ever parses them — reproducing an inconsistency was judged worse than
	// a uniform fill. The visible consequence is that rows 68, 78 and 80 render
	// four dashes wider than the reference. Do not "fix" this to 17 without
	// making row 36 divergent instead; if per-row widths are ever wanted, they
	// belong on the sheetRow manifest, not here.
	labelSeparatorFill = "-----------------------------------"
	stageSeparatorFill = "---------------------"
)

// Labels of the kindStage rows. The value for these rows comes from the stage
// identity rather than the parameter snapshot.
const (
	labelParticulars = "Particulars."
	labelProductName = "Product Name."
	labelItemCode    = "Item Code."
	labelItemName    = "Item Name."
	labelRawMaterial = "Raw Material."
	labelShade       = "Shade Code / Name."
)

// BuildProductCostSheet renders the fixed cost sheet manifest into a new
// workbook, one column per stage. Stages must already be ordered by route
// level and sequence; this function preserves the given order.
//
// Column A holds the printed row number plus label; columns B onward hold one
// stage each. Values come from each stage's ParamSnapshot, except for the
// kindStage rows, which come from the stage identity. Any value that is absent
// from the snapshot — or that belongs to a stage with HasCost false — renders
// as "-" rather than a fabricated zero.
//
// The caller owns the returned file and is responsible for closing it.
func BuildProductCostSheet(stages []Stage) (*excelize.File, error) {
	f := excelize.NewFile()

	sheet := sheetNameForStages(stages)
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		return nil, fmt.Errorf("rename default sheet: %w", err)
	}

	styles, err := newSheetStyles(f)
	if err != nil {
		return nil, err
	}
	if err := applyPageLayout(f, sheet, len(stages)); err != nil {
		return nil, err
	}
	if err := writeCostSheetBody(f, sheet, stages, styles); err != nil {
		return nil, err
	}
	if err := applyRowHeights(f, sheet); err != nil {
		return nil, err
	}
	if err := applyPrintArea(f, sheet, len(stages)); err != nil {
		return nil, err
	}
	return f, nil
}

// sanitizeSheetName turns an arbitrary product label into a valid, unique
// worksheet name: it strips the characters Excel forbids, truncates to 31
// characters, and appends a numeric suffix when the name is already present in
// taken. The chosen name is recorded in taken so repeated calls stay unique.
// A nil or empty taken map is allowed; an empty result falls back to a default
// name.
func sanitizeSheetName(name string, taken map[string]bool) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '[', ']', ':', '*', '?', '/', '\\':
			return -1
		default:
			return r
		}
	}, name)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		cleaned = defaultSheetName
	}
	cleaned = truncateRunes(cleaned, maxSheetNameLen)

	candidate := cleaned
	for i := 2; taken[candidate]; i++ {
		suffix := " (" + strconv.Itoa(i) + ")"
		candidate = truncateRunes(cleaned, maxSheetNameLen-len(suffix)) + suffix
	}
	if taken != nil {
		taken[candidate] = true
	}
	return candidate
}

// truncateRunes shortens s to at most limit runes, keeping multi-byte
// characters intact.
func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

// sheetNameForStages derives the worksheet name from the route-level-1 stage —
// the finished good itself (see costroute/graph.go's ValidateLevels: level 1
// is always the single stage producing the head product) — preferring its
// item code over its product name. Falls back to the first stage when no
// level-1 stage is present, which should not normally happen.
func sheetNameForStages(stages []Stage) string {
	if len(stages) == 0 {
		return defaultSheetName
	}
	target := stages[0]
	for _, s := range stages {
		if s.RouteLevel == 1 {
			target = s
			break
		}
	}
	if target.ItemCode != "" {
		return sanitizeSheetName(target.ItemCode, nil)
	}
	return sanitizeSheetName(target.ProductName, nil)
}

// =============================================================================
// Styles
// =============================================================================

// sheetStyles holds the style IDs reused across every cell of the sheet.
type sheetStyles struct {
	label   int
	text    int
	dash    int
	numeric map[string]int
}

func newSheetStyles(f *excelize.File) (*sheetStyles, error) {
	s := &sheetStyles{numeric: make(map[string]int, 2)}

	var err error
	if s.label, err = newStyle(f, styleSpec{horizontal: "left"}); err != nil {
		return nil, err
	}
	if s.text, err = newStyle(f, styleSpec{horizontal: "left", numFmt: numFmtText}); err != nil {
		return nil, err
	}
	if s.dash, err = newStyle(f, styleSpec{horizontal: "center", numFmt: numFmtText}); err != nil {
		return nil, err
	}
	for _, format := range []string{numFmtDecimal, numFmtDecimal4, numFmtInt} {
		id, sErr := newStyle(f, styleSpec{horizontal: "right", numFmt: format})
		if sErr != nil {
			return nil, sErr
		}
		s.numeric[format] = id
	}
	return s, nil
}

// numericStyle returns the style for a number format, falling back to the
// three-decimal style when the manifest row carries no explicit format.
func (s *sheetStyles) numericStyle(format string) int {
	if id, ok := s.numeric[format]; ok {
		return id
	}
	return s.numeric[numFmtDecimal]
}

// styleSpec describes the handful of style variations the sheet needs.
//
// It carries only the two axes that actually vary: every cell of this sheet is
// non-bold, non-wrapped, and fontSize tall. Earlier revisions also carried
// bold/size/wrap fields for a report-header block that has since been removed
// (headerRowCount is 0); they were left behind set-by-nobody and read-by-newStyle,
// so no linter flagged them. Verified 2026-09-11 that no caller in the service
// — production or test — ever set them before deleting; if a future variation
// needs one, add the field back together with the caller that sets it.
type styleSpec struct {
	horizontal string
	numFmt     string
}

func newStyle(f *excelize.File, spec styleSpec) (int, error) {
	style := &excelize.Style{
		Border: thinBorders(),
		Font:   &excelize.Font{Family: fontName, Size: fontSize},
		Alignment: &excelize.Alignment{
			Horizontal: spec.horizontal,
			Vertical:   "center",
		},
	}
	if spec.numFmt != "" {
		format := spec.numFmt
		style.CustomNumFmt = &format
	}
	id, err := f.NewStyle(style)
	if err != nil {
		return 0, fmt.Errorf("create style: %w", err)
	}
	return id, nil
}

func thinBorders() []excelize.Border {
	const thin = 1
	return []excelize.Border{
		{Type: "left", Color: borderColor, Style: thin},
		{Type: "top", Color: borderColor, Style: thin},
		{Type: "right", Color: borderColor, Style: thin},
		{Type: "bottom", Color: borderColor, Style: thin},
	}
}

// =============================================================================
// Page layout
// =============================================================================

func applyPageLayout(f *excelize.File, sheet string, stageCount int) error {
	const (
		a4PaperSize = 10
		fitToWidth  = 1
		fitToHeight = 0 // 0 means "as many pages tall as needed"
	)
	size, width, height := a4PaperSize, fitToWidth, fitToHeight
	orientation := "portrait"
	if err := f.SetPageLayout(sheet, &excelize.PageLayoutOptions{
		Size:        &size,
		Orientation: &orientation,
		FitToWidth:  &width,
		FitToHeight: &height,
	}); err != nil {
		return fmt.Errorf("set page layout: %w", err)
	}

	// Excel ignores fitToWidth/fitToHeight unless the sheet's fitToPage flag is
	// set, so the scaling must be enabled explicitly here as well.
	fitToPage := true
	if err := f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage}); err != nil {
		return fmt.Errorf("enable fit to page: %w", err)
	}

	sideMargin, endMargin := 0.2, 0.25
	if err := f.SetPageMargins(sheet, &excelize.PageLayoutMarginsOptions{
		Left:   &sideMargin,
		Right:  &sideMargin,
		Top:    &endMargin,
		Bottom: &endMargin,
	}); err != nil {
		return fmt.Errorf("set page margins: %w", err)
	}

	if err := f.SetColWidth(sheet, "A", "A", labelColWidth); err != nil {
		return fmt.Errorf("set label column width: %w", err)
	}
	if stageCount > 0 {
		firstCol, err := excelize.ColumnNumberToName(2)
		if err != nil {
			return fmt.Errorf("first stage column name: %w", err)
		}
		lastCol, err := excelize.ColumnNumberToName(stageCount + 1)
		if err != nil {
			return fmt.Errorf("last stage column name: %w", err)
		}
		if err := f.SetColWidth(sheet, firstCol, lastCol, stageColWidth); err != nil {
			return fmt.Errorf("set stage column widths: %w", err)
		}
	}

	// Freeze the label column only — there are no header rows to freeze.
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		XSplit:      1,
		TopLeftCell: "B1",
		ActivePane:  "topRight",
	}); err != nil {
		return fmt.Errorf("freeze panes: %w", err)
	}
	return nil
}

// applyPrintArea limits printing to the rows above the "others" separator, so
// the printed sheet keeps the reference workbook's 84-row shape while CSV rows
// 85-95 remain present on screen. Implemented with the built-in defined name
// "_xlnm.Print_Area" scoped to the sheet — excelize v2.8.1 whitelists exactly
// that name in SetDefinedName (see
// $GOMODCACHE/github.com/xuri/excelize/v2@v2.8.1/sheet.go:1655 and the
// builtInDefinedNames slice at templates.go:492).
func applyPrintArea(f *excelize.File, sheet string, stageCount int) error {
	lastRow := headerRowCount + printableRowCount()
	if lastRow <= 0 {
		return nil
	}
	lastCol, err := excelize.ColumnNumberToName(stageCount + 1)
	if err != nil {
		return fmt.Errorf("print area last column name: %w", err)
	}
	refersTo := fmt.Sprintf("%s!$A$1:$%s$%d", quoteSheetRef(sheet), lastCol, lastRow)
	if err := f.SetDefinedName(&excelize.DefinedName{
		Name:     printAreaDefinedName,
		RefersTo: refersTo,
		Scope:    sheet,
	}); err != nil {
		return fmt.Errorf("set print area %s: %w", refersTo, err)
	}
	return nil
}

// quoteSheetRef renders a worksheet name for use inside a formula reference.
// Excel requires single quotes around any sheet name that is not a bare
// identifier — the default name "Cost Sheet" and the reference name
// "parameter check" both contain spaces — and an embedded apostrophe is
// escaped by doubling it. sanitizeSheetName already strips the characters
// Excel forbids outright, so quoting is the only escaping needed here.
func quoteSheetRef(sheet string) string {
	if !strings.ContainsAny(sheet, " '") {
		return sheet
	}
	return "'" + strings.ReplaceAll(sheet, "'", "''") + "'"
}

func applyRowHeights(f *excelize.File, sheet string) error {
	total := headerRowCount + len(costSheetRows)
	for row := 1; row <= total; row++ {
		if err := f.SetRowHeight(sheet, row, sheetRowHeight); err != nil {
			return fmt.Errorf("set height of row %d: %w", row, err)
		}
	}
	return nil
}

// =============================================================================
// Body
// =============================================================================

func writeCostSheetBody(f *excelize.File, sheet string, stages []Stage, styles *sheetStyles) error {
	for i := range costSheetRows {
		manifestRow := costSheetRows[i]
		excelRow := i + headerRowCount + 1
		if err := writeManifestRow(f, sheet, excelRow, manifestRow, stages, styles); err != nil {
			return err
		}
	}
	return nil
}

func writeManifestRow(
	f *excelize.File,
	sheet string,
	excelRow int,
	row sheetRow,
	stages []Stage,
	styles *sheetStyles,
) error {
	if err := setCell(f, sheet, 1, excelRow, rowLabel(row), styles.label); err != nil {
		return err
	}
	for i, stage := range stages {
		value, styleID := stageCellFor(row, stage, stages, styles)
		if err := setCell(f, sheet, i+2, excelRow, value, styleID); err != nil {
			return err
		}
	}
	return nil
}

// rowLabel renders column A: the printed number plus the label. Separator rows
// without a label print the dashed filler instead.
func rowLabel(row sheetRow) any {
	if row.Kind == kindSeparator && row.Label == "" {
		return labelSeparatorFill
	}
	return row.Num + row.Label
}

// stageCellFor resolves one stage's value for one manifest row, together with
// the style it must be written in.
func stageCellFor(row sheetRow, stage Stage, stages []Stage, styles *sheetStyles) (any, int) {
	switch row.Kind {
	case kindSeparator:
		return stageSeparatorFill, styles.text
	case kindMissing:
		return dashValue, styles.dash
	case kindStage:
		return textOrDash(stageIdentityValue(row.Label, stage, stages), styles)
	case kindText:
		return textOrDash(snapshotValue(row.ParamCode, stage), styles)
	case kindSnapshot:
		return numericCell(row, stage, styles)
	default:
		return dashValue, styles.dash
	}
}

// snapshotValue reads a parameter from the stage snapshot. A stage without a
// cost row, or a parameter that was never calculated, yields the empty string
// so the caller renders "-".
func snapshotValue(paramCode string, stage Stage) string {
	if !stage.HasCost || paramCode == "" || stage.ParamSnapshot == nil {
		return ""
	}
	return strings.TrimSpace(stage.ParamSnapshot[paramCode])
}

// textOrDash writes a string cell, falling back to "-" when the value is
// absent.
func textOrDash(value string, styles *sheetStyles) (any, int) {
	if value == "" {
		return dashValue, styles.dash
	}
	return value, styles.text
}

// numericCell parses a snapshot value into a real number so the user can
// re-total the sheet in Excel. Values that are not parsable as numbers fall
// back to their raw string form rather than being dropped.
//
// The stored value is rounded to the same number of decimals the cell's number
// format displays, so the workbook's underlying data matches what is printed
// and re-totalling the sheet cannot drift by hidden sub-precision digits. A
// value that is exactly zero after rounding renders as "-" — the source
// template leaves genuinely-nil costs blank rather than printing 0 — while
// negatives (e.g. an oil gain) are preserved.
func numericCell(row sheetRow, stage Stage, styles *sheetStyles) (any, int) {
	raw := snapshotValue(row.ParamCode, stage)
	if raw == "" {
		return dashValue, styles.dash
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw, styles.text
	}
	number = roundTo(number, numFmtDecimals(row.NumFmt))
	if number == 0 {
		return dashValue, styles.dash
	}
	return number, styles.numericStyle(row.NumFmt)
}

// numFmtDecimals reports how many decimal places a manifest row's number
// format displays. Rows with no explicit format follow the sheet default of
// three decimals, matching sheetStyles.numericStyle.
func numFmtDecimals(format string) int {
	switch format {
	case numFmtInt:
		return 0
	case numFmtDecimal4:
		return 4
	case numFmtDecimal:
		return 3
	default:
		return 3
	}
}

// roundTo rounds v to places decimal places, half away from zero.
func roundTo(v float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(v*factor) / factor
}

// stageIdentityValue resolves the kindStage rows, whose values come from the
// route stage and product master rather than the parameter snapshot. stages is
// the full column set, needed because a stage's raw material is another stage.
func stageIdentityValue(label string, stage Stage, stages []Stage) string {
	switch label {
	case labelParticulars:
		return particularsValue(stage)
	case labelProductName, labelItemName:
		return stage.ProductName
	case labelItemCode:
		return stage.ItemCode
	case labelRawMaterial:
		return rawMaterialValue(stage, stages)
	case labelShade:
		return joinShade(stage.ShadeCode, stage.ShadeName)
	default:
		return ""
	}
}

// particularsValue renders the sheet's first row as "<yarn type>(<left no>)" —
// e.g. "POY(14671)" — matching the source template. The yarn type is the
// legacy product type label (cpm_flex_03) and the left no is the product
// master's own sys id. Products never imported from the legacy system carry no
// yarn type, so those fall back to the route name rather than printing a bare
// number in parentheses.
func particularsValue(stage Stage) string {
	if stage.YarnType == "" || stage.ProductSysID <= 0 {
		return stage.RouteName
	}
	return stage.YarnType + "(" + strconv.FormatInt(stage.ProductSysID, 10) + ")"
}

// rawMaterialValue resolves the "Raw Material." row. The RAW_MATERIAL text
// param is authoritative when the master stores one — it is the same value the
// flat sheet prints in its "20.Raw Material" column, and it is the only source
// for raw materials that are store items or groups (e.g. "SD"), which have no
// route stage of their own.
//
// When the master stores nothing, the value is composed from the stage's own
// upstream stage, which for a yarn route is the raw material: the next route
// level up, rendered in the established "<item code>-<shade code>-<machine
// code>" link form also used by 2.Marketing Costing Link.
func rawMaterialValue(stage Stage, stages []Stage) string {
	if v := snapshotValue(paramRawMaterial, stage); v != "" {
		return v
	}
	up := upstreamStage(stage, stages)
	if up == nil {
		return ""
	}
	return composeCostingLink(*up)
}

// upstreamStage returns the stage feeding the given one: the lowest route level
// strictly above it. Returns nil for the most upstream stage of the route,
// whose raw material is a purchased item rather than another stage.
func upstreamStage(stage Stage, stages []Stage) *Stage {
	var best *Stage
	for i := range stages {
		s := &stages[i]
		if s.RouteLevel <= stage.RouteLevel {
			continue
		}
		if best == nil || s.RouteLevel < best.RouteLevel ||
			(s.RouteLevel == best.RouteLevel && s.RouteSeq < best.RouteSeq) {
			best = s
		}
	}
	return best
}

// joinShade renders "code/name", or whichever half is present.
//
// ⚠ NO spaces around the slash. The reference workbook
// (data-examples/export-product-cost/example-export-param.xlsx, sheet
// "parameter check" row 10) holds "6912-01/RUSA BG AT" — unpadded. Do not
// "tidy" this into " / ": the row LABEL "10.Shade Code / Name." does carry
// spaces, and so does the unrelated OPU value ".4% / 2.4067", which is why
// padding looks correct here at a glance. It is not.
func joinShade(code, name string) string {
	switch {
	case code != "" && name != "":
		return code + "/" + name
	case code != "":
		return code
	default:
		return name
	}
}

// setCell writes one value with its style at 1-based column/row coordinates.
func setCell(f *excelize.File, sheet string, col, row int, value any, styleID int) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return fmt.Errorf("cell coordinate c=%d r=%d: %w", col, row, err)
	}
	if err := f.SetCellValue(sheet, cell, value); err != nil {
		return fmt.Errorf("write cell %s: %w", cell, err)
	}
	if err := f.SetCellStyle(sheet, cell, cell, styleID); err != nil {
		return fmt.Errorf("style cell %s: %w", cell, err)
	}
	return nil
}
