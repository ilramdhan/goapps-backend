package worker

// Internal test package: the cost sheet manifest, the sheet-name sanitizer, and
// the cell resolvers are unexported, so they can only be exercised from inside
// the worker package.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// expectedSheetRowCount is the fixed height of the cost sheet body, matching the
// 95 data rows of docs/export-product-cost/template_export_product_cost.csv.
const expectedSheetRowCount = 95

const (
	testItemCode    = "PTY0001305"
	testProductName = "Regular"
	testRouteName   = "PTY(3427)"
)

// stageWith builds a stage carrying a full parameter snapshot.
func stageWith(snapshot map[string]string) Stage {
	return Stage{
		RouteLevel:    2,
		RouteSeq:      1,
		RouteName:     testRouteName,
		ItemCode:      testItemCode,
		ProductName:   testProductName,
		ShadeCode:     "SH01",
		ShadeName:     "Navy",
		ProductSysID:  3427,
		HasCost:       true,
		ParamSnapshot: snapshot,
	}
}

// -----------------------------------------------------------------------------
// Row manifest
// -----------------------------------------------------------------------------

func TestCostSheetRows_HasExactly95Rows(t *testing.T) {
	t.Parallel()
	assert.Len(t, costSheetRows, expectedSheetRowCount,
		"the cost sheet layout is fixed at %d rows by the CSV template", expectedSheetRowCount)
}

func TestCostSheetRows_KindInvariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		assert func(t *testing.T, idx int, row sheetRow)
	}{
		{
			name: "snapshot and text rows always carry a param code",
			assert: func(t *testing.T, idx int, row sheetRow) {
				if row.Kind == kindSnapshot || row.Kind == kindText {
					assert.NotEmpty(t, row.ParamCode, "row %d (%q) needs a param code", idx, row.Label)
				}
			},
		},
		{
			name: "separator and missing rows never carry a param code",
			assert: func(t *testing.T, idx int, row sheetRow) {
				if row.Kind == kindSeparator || row.Kind == kindMissing {
					assert.Empty(t, row.ParamCode, "row %d (%q) must have no param code", idx, row.Label)
				}
			},
		},
		{
			name: "numeric rows use a known number format",
			assert: func(t *testing.T, idx int, row sheetRow) {
				if row.Kind == kindSnapshot {
					assert.Contains(t, []string{numFmtDecimal, numFmtDecimal4, numFmtInt}, row.NumFmt,
						"row %d (%q) has an unknown number format", idx, row.Label)
				}
			},
		},
		{
			name: "every non-separator row is labeled",
			assert: func(t *testing.T, idx int, row sheetRow) {
				if row.Kind != kindSeparator {
					assert.NotEmpty(t, row.Label, "row %d must be labeled", idx)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i := range costSheetRows {
				tc.assert(t, i, costSheetRows[i])
			}
		})
	}
}

func TestCostSheetRows_ContainsExpectedAnchors(t *testing.T) {
	t.Parallel()

	byLabel := make(map[string]sheetRow, len(costSheetRows))
	for _, row := range costSheetRows {
		byLabel[row.Label] = row
	}

	tests := []struct {
		label     string
		paramCode string
		kind      sheetRowKind
	}{
		{label: labelParticulars, kind: kindStage},
		{label: labelItemCode, kind: kindStage},
		{label: labelShade, kind: kindStage},
		{label: "RM Rate.", paramCode: "RM_RATE", kind: kindSnapshot},
		{label: "Machine Name.", paramCode: "MC_NAME", kind: kindText},
		{label: "Fixed Cost.", kind: kindMissing},
		{label: "Domestic cost with uneven packing.", paramCode: "DOMESTIC_COST_UNEVEN_PACK", kind: kindSnapshot},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			row, ok := byLabel[tc.label]
			require.True(t, ok, "manifest is missing row %q", tc.label)
			assert.Equal(t, tc.kind, row.Kind)
			assert.Equal(t, tc.paramCode, row.ParamCode)
		})
	}
}

// -----------------------------------------------------------------------------
// Sheet naming
// -----------------------------------------------------------------------------

func TestSanitizeSheetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		taken map[string]bool
		want  string
	}{
		{name: "plain item code passes through", input: testItemCode, want: testItemCode},
		{
			name:  "forbidden characters are stripped",
			input: "PTY/001[A]:B*C?D\\E",
			want:  "PTY001ABCDE",
		},
		{
			name:  "over-long names are truncated to Excel's limit",
			input: "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
			want:  "ABCDEFGHIJKLMNOPQRSTUVWXYZ01234",
		},
		{name: "empty input falls back to the default name", input: "   ", want: defaultSheetName},
		{
			name:  "collision gets a numeric suffix",
			input: testItemCode,
			taken: map[string]bool{testItemCode: true},
			want:  testItemCode + " (2)",
		},
		{
			name:  "second collision increments the suffix",
			input: testItemCode,
			taken: map[string]bool{testItemCode: true, testItemCode + " (2)": true},
			want:  testItemCode + " (3)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeSheetName(tc.input, tc.taken)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, len([]rune(got)), maxSheetNameLen)
			if tc.taken != nil {
				assert.True(t, tc.taken[got], "the chosen name must be recorded as taken")
			}
		})
	}
}

func TestSanitizeSheetName_NilTakenMapIsAllowed(t *testing.T) {
	t.Parallel()
	assert.Equal(t, testItemCode, sanitizeSheetName(testItemCode, nil))
}

func TestSheetNameForStages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stages []Stage
		want   string
	}{
		{name: "no stages falls back to the default", stages: nil, want: defaultSheetName},
		{
			name: "the level-1 stage's item code names the sheet (level 1 is the finished good)",
			stages: []Stage{
				{RouteLevel: 2, ItemCode: "POY0000433"},
				{RouteLevel: 1, ItemCode: testItemCode},
			},
			want: testItemCode,
		},
		{
			name: "falls back to the first stage when no level-1 stage is present",
			stages: []Stage{
				{RouteLevel: 3, ItemCode: "POY0000433"},
				{RouteLevel: 2, ItemCode: testItemCode},
			},
			want: "POY0000433",
		},
		{
			name:   "product name is used when the item code is blank",
			stages: []Stage{{ProductName: testProductName}},
			want:   testProductName,
		},
		{
			name:   "a stage with neither falls back to the default",
			stages: []Stage{{}},
			want:   defaultSheetName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sheetNameForStages(tc.stages))
		})
	}
}

// -----------------------------------------------------------------------------
// Placeholder ("-") behavior
// -----------------------------------------------------------------------------

func TestStageCellFor_DashPlaceholders(t *testing.T) {
	t.Parallel()

	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	styles, err := newSheetStyles(f)
	require.NoError(t, err)

	numericRow := sheetRow{Label: "RM Rate.", ParamCode: "RM_RATE", Kind: kindSnapshot, NumFmt: numFmtDecimal}
	textRow := sheetRow{Label: "MB / SP Dye Name.", ParamCode: "MB_SP_DYE", Kind: kindText, NumFmt: numFmtText}

	tests := []struct {
		name  string
		row   sheetRow
		stage Stage
		want  any
	}{
		{
			name:  "absent snapshot key renders a dash, never zero",
			row:   numericRow,
			stage: stageWith(map[string]string{"OTHER": "1.5"}),
			want:  dashValue,
		},
		{
			name:  "nil snapshot renders a dash",
			row:   numericRow,
			stage: stageWith(nil),
			want:  dashValue,
		},
		{
			name: "stage without a cost row renders a dash even when the key exists",
			row:  numericRow,
			stage: func() Stage {
				s := stageWith(map[string]string{"RM_RATE": "1.082"})
				s.HasCost = false
				return s
			}(),
			want: dashValue,
		},
		{
			name:  "blank snapshot value renders a dash",
			row:   numericRow,
			stage: stageWith(map[string]string{"RM_RATE": "   "}),
			want:  dashValue,
		},
		{
			name:  "kindMissing always renders a dash",
			row:   sheetRow{Label: "Fixed Cost.", Kind: kindMissing},
			stage: stageWith(map[string]string{"RM_RATE": "1.082"}),
			want:  dashValue,
		},
		{
			name:  "absent text value renders a dash",
			row:   textRow,
			stage: stageWith(map[string]string{}),
			want:  dashValue,
		},
		{
			name:  "present numeric value is written as a real number",
			row:   numericRow,
			stage: stageWith(map[string]string{"RM_RATE": "1.082"}),
			want:  1.082,
		},
		{
			name:  "unparsable numeric value falls back to its raw string",
			row:   numericRow,
			stage: stageWith(map[string]string{"RM_RATE": "N/A"}),
			want:  "N/A",
		},
		{
			name:  "present text value is written verbatim",
			row:   textRow,
			stage: stageWith(map[string]string{"MB_SP_DYE": "MB-RED-01"}),
			want:  "MB-RED-01",
		},
		{
			name:  "separator rows render the dashed filler",
			row:   sheetRow{Kind: kindSeparator},
			stage: stageWith(map[string]string{}),
			want:  stageSeparatorFill,
		},
		{
			name:  "stage identity rows read the stage, not the snapshot",
			row:   sheetRow{Label: labelItemCode, Kind: kindStage},
			stage: stageWith(map[string]string{}),
			want:  testItemCode,
		},
		{
			name:  "shade row joins code and name",
			row:   sheetRow{Label: labelShade, Kind: kindStage},
			stage: stageWith(map[string]string{}),
			want:  "SH01 / Navy",
		},
		{
			name:  "raw material has no source yet and renders a dash",
			row:   sheetRow{Label: labelRawMaterial, Kind: kindStage},
			stage: stageWith(map[string]string{}),
			want:  dashValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, styleID := stageCellFor(tc.row, tc.stage, styles)
			assert.Equal(t, tc.want, got)
			assert.NotZero(t, styleID, "every cell must be styled")
		})
	}
}

func TestJoinShade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		val  string
		want string
	}{
		{name: "both halves", code: "SH01", val: "Navy", want: "SH01 / Navy"},
		{name: "code only", code: "SH01", want: "SH01"},
		{name: "name only", val: "Navy", want: "Navy"},
		{name: "neither", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, joinShade(tc.code, tc.val))
		})
	}
}

// -----------------------------------------------------------------------------
// Workbook builder
// -----------------------------------------------------------------------------

func TestBuildProductCostSheet_Shape(t *testing.T) {
	t.Parallel()

	stages := []Stage{
		{
			RouteLevel: 1, RouteSeq: 1, RouteName: "POY(3155)",
			ItemCode: "POY0000433", ProductName: testProductName, HasCost: true,
			ParamSnapshot: map[string]string{"RM_RATE": "1.082", "NO_OF_END": "24"},
		},
		stageWith(map[string]string{"RM_RATE": "1.352", "MB_SP_DYE": "MB-RED-01"}),
	}

	f, err := BuildProductCostSheet(stages)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	assert.Equal(t, "POY0000433", sheet, "the sheet is named after the level-1 stage's item code (the finished good)")

	rows, err := f.GetRows(sheet)
	require.NoError(t, err)
	assert.Len(t, rows, headerRowCount+expectedSheetRowCount,
		"title row + header row + %d manifest rows", expectedSheetRowCount)

	// Row 2 carries the stage column headers, one per stage plus the label column.
	stageHeaderCell, err := f.GetCellValue(sheet, "B2")
	require.NoError(t, err)
	assert.Contains(t, stageHeaderCell, "POY0000433")

	// Column A of the first manifest row (Excel row 3) is "1.Particulars.".
	labelCell, err := f.GetCellValue(sheet, "A3")
	require.NoError(t, err)
	assert.Equal(t, "1."+labelParticulars, labelCell)
}

func TestBuildProductCostSheet_NumbersStayNumeric(t *testing.T) {
	t.Parallel()

	stages := []Stage{stageWith(map[string]string{"RM_RATE": "1.352"})}

	f, err := BuildProductCostSheet(stages)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	rmRateRow := manifestRowIndexByLabel(t, "RM Rate.") + headerRowCount + 1
	cell, err := excelize.CoordinatesToCellName(2, rmRateRow)
	require.NoError(t, err)

	numeric, err := isNumericCell(f, sheet, cell)
	require.NoError(t, err)
	assert.True(t, numeric, "numeric values must stay editable numbers, not strings")

	raw, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	assert.Equal(t, "1.352", raw)

	// A "-" placeholder must NOT be mistaken for a number.
	dashRow := manifestRowIndexByLabel(t, "Oil Cost.") + headerRowCount + 1
	dashCell, err := excelize.CoordinatesToCellName(2, dashRow)
	require.NoError(t, err)
	dashNumeric, err := isNumericCell(f, sheet, dashCell)
	require.NoError(t, err)
	assert.False(t, dashNumeric, "the %q placeholder is text, not a number", dashValue)
}

// TestCopySheet_PreservesNumericCells guards the workbook-merge path: excelize
// reports untyped numeric cells as CellTypeUnset, so a naive CellTypeNumber
// check silently turns every number in the combined workbook into a string.
func TestCopySheet_PreservesNumericCells(t *testing.T) {
	t.Parallel()

	src, err := BuildProductCostSheet([]Stage{stageWith(map[string]string{"RM_RATE": "1.352"})})
	require.NoError(t, err)
	defer func() { require.NoError(t, src.Close()) }()

	dst := excelize.NewFile()
	defer func() { require.NoError(t, dst.Close()) }()

	const dstName = "Merged"
	require.NoError(t, copySheet(src, dst, dstName))

	rmRateRow := manifestRowIndexByLabel(t, "RM Rate.") + headerRowCount + 1
	cell, err := excelize.CoordinatesToCellName(2, rmRateRow)
	require.NoError(t, err)

	numeric, err := isNumericCell(dst, dstName, cell)
	require.NoError(t, err)
	assert.True(t, numeric, "merging must not degrade numbers into strings")

	raw, err := dst.GetCellValue(dstName, cell, excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	assert.Equal(t, "1.352", raw)

	// The label column must survive as text.
	label, err := dst.GetCellValue(dstName, "A3")
	require.NoError(t, err)
	assert.Equal(t, "1."+labelParticulars, label)
}

func TestBuildProductCostSheet_AbsentValuesRenderDash(t *testing.T) {
	t.Parallel()

	// A stage with no cost row at all: every data cell must be "-".
	stages := []Stage{{RouteLevel: 1, RouteSeq: 1, ItemCode: testItemCode, HasCost: false}}

	f, err := BuildProductCostSheet(stages)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	dashes, separators := 0, 0
	for i := range costSheetRows {
		cell, cErr := excelize.CoordinatesToCellName(2, i+headerRowCount+1)
		require.NoError(t, cErr)
		value, vErr := f.GetCellValue(sheet, cell)
		require.NoError(t, vErr)
		switch value {
		case dashValue:
			dashes++
		case stageSeparatorFill:
			separators++
		default:
			// The item code row is stage identity and is populated.
			assert.Equal(t, testItemCode, value, "unexpected value at row %d", i)
		}
	}
	assert.Positive(t, dashes)
	assert.Equal(t, separatorRowCount(), separators)
	assert.Equal(t, expectedSheetRowCount, dashes+separators+1)
}

func TestBuildProductCostSheet_NoStages(t *testing.T) {
	t.Parallel()

	f, err := BuildProductCostSheet(nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	assert.Equal(t, defaultSheetName, sheet)

	rows, err := f.GetRows(sheet)
	require.NoError(t, err)
	assert.Len(t, rows, headerRowCount+expectedSheetRowCount)
}

func TestBuildProductCostSheet_PageSetupFitsOnePageWide(t *testing.T) {
	t.Parallel()

	f, err := BuildProductCostSheet([]Stage{stageWith(map[string]string{})})
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	layout, err := f.GetPageLayout(sheet)
	require.NoError(t, err)

	require.NotNil(t, layout.FitToWidth)
	assert.Equal(t, 1, *layout.FitToWidth, "the sheet must print exactly one page wide")
	require.NotNil(t, layout.FitToHeight)
	assert.Equal(t, 0, *layout.FitToHeight, "height is unconstrained")
	require.NotNil(t, layout.Orientation)
	assert.Equal(t, "portrait", *layout.Orientation)
	require.NotNil(t, layout.Size)
	assert.Equal(t, 10, *layout.Size, "A4 paper")

	props, err := f.GetSheetProps(sheet)
	require.NoError(t, err)
	require.NotNil(t, props.FitToPage)
	assert.True(t, *props.FitToPage, "Excel ignores fit-to-width unless fitToPage is set")
}

// manifestRowIndexByLabel returns the zero-based manifest index of a labeled row.
func manifestRowIndexByLabel(t *testing.T, label string) int {
	t.Helper()
	for i := range costSheetRows {
		if costSheetRows[i].Label == label {
			return i
		}
	}
	t.Fatalf("manifest has no row labeled %q", label)
	return -1
}

// separatorRowCount counts the dashed divider rows in the manifest.
func separatorRowCount() int {
	count := 0
	for i := range costSheetRows {
		if costSheetRows[i].Kind == kindSeparator {
			count++
		}
	}
	return count
}
