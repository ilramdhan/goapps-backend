package worker

// costsheet_export_excel.go renders the fixed 95-row manifest in
// costsheet_rows.go into an A4 xlsx workbook, one column per route stage.
// See design doc
// docs/superpowers/specs/2026-08-04-cost-results-enhancements-design.md §3.

import (
	"fmt"
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
}

// Rendering constants for the sheet's fixed look.
const (
	// dashValue is printed wherever a value is absent. Never substitute a zero.
	dashValue = "-"

	defaultSheetName = "Cost Sheet"
	sheetTitle       = "Report : Find Color by Product"

	labelColWidth  = 30.0
	stageColWidth  = 13.0
	sheetRowHeight = 11.0

	fontName    = "Calibri"
	fontSize    = 7.0
	borderColor = "BFBFBF"

	// headerRowCount is the number of rows above the manifest: the title row
	// and the stage column header row.
	headerRowCount = 2

	// maxSheetNameLen is Excel's hard limit on worksheet names.
	maxSheetNameLen = 31

	// labelSeparatorFill and stageSeparatorFill reproduce the dashed divider
	// rows of the CSV template.
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
	if err := writeCostSheetHeader(f, sheet, stages, styles); err != nil {
		return nil, err
	}
	if err := writeCostSheetBody(f, sheet, stages, styles); err != nil {
		return nil, err
	}
	if err := applyRowHeights(f, sheet); err != nil {
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
	title     int
	colHeader int
	label     int
	text      int
	dash      int
	numeric   map[string]int
}

func newSheetStyles(f *excelize.File) (*sheetStyles, error) {
	s := &sheetStyles{numeric: make(map[string]int, 2)}

	var err error
	if s.title, err = newStyle(f, styleSpec{bold: true, size: 9, horizontal: "left"}); err != nil {
		return nil, err
	}
	if s.colHeader, err = newStyle(f, styleSpec{bold: true, horizontal: "center", wrap: true}); err != nil {
		return nil, err
	}
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
type styleSpec struct {
	bold       bool
	size       float64
	horizontal string
	numFmt     string
	wrap       bool
}

func newStyle(f *excelize.File, spec styleSpec) (int, error) {
	size := spec.size
	if size == 0 {
		size = fontSize
	}
	style := &excelize.Style{
		Border: thinBorders(),
		Font:   &excelize.Font{Bold: spec.bold, Family: fontName, Size: size},
		Alignment: &excelize.Alignment{
			Horizontal: spec.horizontal,
			Vertical:   "center",
			WrapText:   spec.wrap,
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

	// Freeze the label column and the two header rows.
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		XSplit:      1,
		YSplit:      headerRowCount,
		TopLeftCell: "B3",
		ActivePane:  "bottomRight",
	}); err != nil {
		return fmt.Errorf("freeze panes: %w", err)
	}
	return nil
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
// Header block
// =============================================================================

// writeCostSheetHeader writes row 1 (title) and row 2 (stage column headers).
func writeCostSheetHeader(f *excelize.File, sheet string, stages []Stage, styles *sheetStyles) error {
	if err := setCell(f, sheet, 1, 1, sheetTitleFor(stages), styles.title); err != nil {
		return err
	}
	if len(stages) > 0 {
		if err := mergeTitleRow(f, sheet, len(stages)); err != nil {
			return err
		}
	}

	if err := setCell(f, sheet, 1, headerRowCount, "Route Stage", styles.colHeader); err != nil {
		return err
	}
	for i, stage := range stages {
		if err := setCell(f, sheet, i+2, headerRowCount, stageHeader(stage), styles.colHeader); err != nil {
			return err
		}
	}
	return nil
}

// sheetTitleFor builds the title row. The signature carries no period, so the
// title is the template's fixed report caption, qualified with the target
// product when one is known.
func sheetTitleFor(stages []Stage) string {
	if len(stages) == 0 {
		return sheetTitle
	}
	target := stages[len(stages)-1]
	parts := make([]string, 0, 2)
	if target.ItemCode != "" {
		parts = append(parts, target.ItemCode)
	}
	if target.ProductName != "" {
		parts = append(parts, target.ProductName)
	}
	if len(parts) == 0 {
		return sheetTitle
	}
	return sheetTitle + " — " + strings.Join(parts, " ")
}

func mergeTitleRow(f *excelize.File, sheet string, stageCount int) error {
	last, err := excelize.CoordinatesToCellName(stageCount+1, 1)
	if err != nil {
		return fmt.Errorf("title merge coordinate: %w", err)
	}
	if err := f.MergeCell(sheet, "A1", last); err != nil {
		return fmt.Errorf("merge title row: %w", err)
	}
	return nil
}

// stageHeader labels a stage column with its route position and item code.
// Internal identifiers are never printed.
func stageHeader(stage Stage) string {
	position := "L" + strconv.FormatInt(int64(stage.RouteLevel), 10) +
		"." + strconv.FormatInt(int64(stage.RouteSeq), 10)
	name := stage.RouteName
	if name == "" {
		name = stage.ProductName
	}
	parts := []string{position}
	if name != "" {
		parts = append(parts, name)
	}
	if stage.ItemCode != "" {
		parts = append(parts, stage.ItemCode)
	}
	return strings.Join(parts, "\n")
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
		value, styleID := stageCellFor(row, stage, styles)
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
func stageCellFor(row sheetRow, stage Stage, styles *sheetStyles) (any, int) {
	switch row.Kind {
	case kindSeparator:
		return stageSeparatorFill, styles.text
	case kindMissing:
		return dashValue, styles.dash
	case kindStage:
		return textOrDash(stageIdentityValue(row.Label, stage), styles)
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
func numericCell(row sheetRow, stage Stage, styles *sheetStyles) (any, int) {
	raw := snapshotValue(row.ParamCode, stage)
	if raw == "" {
		return dashValue, styles.dash
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw, styles.text
	}
	return number, styles.numericStyle(row.NumFmt)
}

// stageIdentityValue resolves the kindStage rows, whose values come from the
// route stage and product master rather than the parameter snapshot.
func stageIdentityValue(label string, stage Stage) string {
	switch label {
	case labelParticulars:
		return stage.RouteName
	case labelProductName, labelItemName:
		return stage.ProductName
	case labelItemCode:
		return stage.ItemCode
	case labelRawMaterial:
		// The raw material of a stage is not carried on the stage record and is
		// not in the parameter snapshot either, so there is no source for it
		// here. It renders as "-" until the route's RM lines are plumbed
		// through.
		return ""
	case labelShade:
		return joinShade(stage.ShadeCode, stage.ShadeName)
	default:
		return ""
	}
}

// joinShade renders "code / name", or whichever half is present.
func joinShade(code, name string) string {
	switch {
	case code != "" && name != "":
		return code + " / " + name
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
