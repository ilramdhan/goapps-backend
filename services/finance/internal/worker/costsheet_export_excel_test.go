package worker

// Internal test package: the cost sheet manifest, the sheet-name sanitizer, and
// the cell resolvers are unexported, so they can only be exercised from inside
// the worker package.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// Row-count pins for the cost sheet body.
//
//   - expectedCSVDataRowCount: every data row of
//     docs/export-product-cost/template_export_product_cost.csv is represented
//     in the manifest (the CSV's own separator lines 36/68/78/80 included).
//   - expectedSheetRowCount: manifest entries, i.e. the CSV rows PLUS the one
//     synthetic divider that opens the "others" section. That divider has no
//     CSV line of its own, so the sheet is one row taller than the CSV.
//   - expectedPrintableRowCount: the prefix that is actually printed. The
//     reference workbook's "parameter check" sheet ends at row 84, so CSV rows
//     85-95 sit below the divider, outside the print area.
const (
	expectedCSVDataRowCount   = 95
	expectedSheetRowCount     = expectedCSVDataRowCount + 1
	expectedPrintableRowCount = 84
)

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

func TestCostSheetRows_HasExactly95CSVRows(t *testing.T) {
	t.Parallel()
	assert.Len(t, costSheetRows, expectedSheetRowCount,
		"the cost sheet layout is fixed at %d CSV rows plus the others divider",
		expectedCSVDataRowCount)

	// Every manifest entry except the synthetic others divider maps to a CSV
	// data row, so dropping it leaves exactly the CSV's 95.
	csvRows := 0
	for i := range costSheetRows {
		if costSheetRows[i].Kind == kindSeparator && costSheetRows[i].Label == othersSeparatorLabel {
			continue
		}
		csvRows++
	}
	assert.Equal(t, expectedCSVDataRowCount, csvRows,
		"all %d CSV data rows must be represented", expectedCSVDataRowCount)
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
		{label: "Domestic Cost AX grd only.", kind: kindMissing},
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
			want:  "SH01/Navy",
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
			got, styleID := stageCellFor(tc.row, tc.stage, []Stage{tc.stage}, styles)
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
		{name: "both halves", code: "SH01", val: "Navy", want: "SH01/Navy"},
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
		"%d manifest rows, with no title or header block above them", expectedSheetRowCount)

	// Column A of the first manifest row (Excel row 1) is "1.Particulars." —
	// the sheet starts straight at the data, with no title or header rows.
	labelCell, err := f.GetCellValue(sheet, "A1")
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
	label, err := dst.GetCellValue(dstName, "A1")
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

// -----------------------------------------------------------------------------
// Number-format pinning
// -----------------------------------------------------------------------------

// TestCostSheetRows_Decimal4Set pins the EXACT set of rows printed with four
// decimals. TestCostSheetRows_KindInvariants only asserts that a numeric row
// carries one of the three known formats, which passed before row 13 was moved
// to four decimals too — so it cannot catch a silent revert. This test can:
// dropping row 13 back to numFmtDecimal would round its reference value 6.6858
// to 6.686 and lose the digit the template shows.
//
// The negative half matters as much as the positive: the reference workbook
// (<repo-root>/data-examples/export-product-cost/example-export-param.xlsx,
// sheet "parameter check") carries more than 3 dp in exactly three cells, so a
// fourth numFmtDecimal4 row must be a deliberate act with its own evidence,
// never an incidental copy-paste.
func TestCostSheetRows_Decimal4Set(t *testing.T) {
	t.Parallel()

	// Keyed by printed row number, with the label the number must still carry.
	wantDecimal4 := map[string]string{
		"13.": "MB Rate.",
		"79.": "Final Conversion excl MB.",
		"81.": "Cost lessQL,CO,Frwd.",
	}

	t.Run("each four-decimal row keeps its format", func(t *testing.T) {
		t.Parallel()
		byNum := make(map[string]sheetRow, len(costSheetRows))
		for _, row := range costSheetRows {
			if _, dup := byNum[row.Num]; dup && row.Num != "" {
				continue // two rows print "33."; neither is in the 4-dp set.
			}
			byNum[row.Num] = row
		}
		for num, label := range wantDecimal4 {
			row, ok := byNum[num]
			require.True(t, ok, "row %q vanished from the manifest", num)
			assert.Equal(t, label, row.Label, "row %q changed label", num)
			assert.Equal(t, numFmtDecimal4, row.NumFmt,
				"row %q (%q) must print four decimals", num, label)
		}
	})

	t.Run("no other row carries the four-decimal format", func(t *testing.T) {
		t.Parallel()
		var got []string
		for i := range costSheetRows {
			if costSheetRows[i].NumFmt == numFmtDecimal4 {
				got = append(got, costSheetRows[i].Num)
			}
		}
		assert.Len(t, got, len(wantDecimal4),
			"exactly %d rows may print four decimals; got %v", len(wantDecimal4), got)
		for _, num := range got {
			assert.Contains(t, wantDecimal4, num,
				"row %q gained numFmtDecimal4 without a reference-workbook cell to justify it", num)
		}
	})
}

// TestNumericCell_RoundsToDeclaredDecimals pins the rounding contract: the
// number stored in the cell must already be rounded to the decimals the cell's
// format displays, so re-totalling the sheet in Excel cannot drift by hidden
// sub-precision digits.
//
// The 6.6858 case is the row-13 regression in numeric form: under the
// three-decimal format the same input must become 6.686, under four it must
// survive intact.
func TestNumericCell_RoundsToDeclaredDecimals(t *testing.T) {
	t.Parallel()

	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	styles, err := newSheetStyles(f)
	require.NoError(t, err)

	tests := []struct {
		name   string
		numFmt string
		raw    string
		want   float64
	}{
		{
			name:   "four-decimal row keeps the fourth digit",
			numFmt: numFmtDecimal4,
			raw:    "6.6858",
			want:   6.6858,
		},
		{
			name:   "three-decimal row rounds the same input up",
			numFmt: numFmtDecimal,
			raw:    "6.6858",
			want:   6.686,
		},
		{
			name:   "four-decimal row rounds a fifth digit away",
			numFmt: numFmtDecimal4,
			raw:    "0.79275",
			want:   0.7928,
		},
		{
			name:   "integer row drops the fraction",
			numFmt: numFmtInt,
			raw:    "1234.6",
			want:   1235,
		},
		{
			name:   "row with no declared format follows the three-decimal default",
			numFmt: "",
			raw:    "6.6858",
			want:   6.686,
		},
		{
			name:   "negative values round away from zero, not toward it",
			numFmt: numFmtDecimal,
			raw:    "-0.0125",
			want:   -0.013,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			row := sheetRow{
				Label: "probe", ParamCode: "PROBE", Kind: kindSnapshot, NumFmt: tc.numFmt,
			}
			value, styleID := numericCell(row, stageWith(map[string]string{"PROBE": tc.raw}), styles)

			number, ok := value.(float64)
			require.True(t, ok, "value must stay a real number, got %T (%v)", value, value)
			assert.InDelta(t, tc.want, number, 1e-9)
			assert.Equal(t, styles.numericStyle(tc.numFmt), styleID)
		})
	}
}

// -----------------------------------------------------------------------------
// "Others" section (CSV rows 85-95) and print area
// -----------------------------------------------------------------------------

// TestCostSheetRows_OthersSection pins the tail of the manifest: the full CSV
// is 95 data rows, the last 11 of them live below the othersSeparatorLabel
// divider, and the two rows whose params were verified against the seed
// migrations carry those param codes. Trimming the manifest back to 84 rows,
// dropping the divider, or un-wiring row 93/95 all fail here.
func TestCostSheetRows_OthersSection(t *testing.T) {
	t.Parallel()

	require.Len(t, costSheetRows, expectedSheetRowCount)

	sepIdx := -1
	for i := range costSheetRows {
		if costSheetRows[i].Kind == kindSeparator && costSheetRows[i].Label == othersSeparatorLabel {
			sepIdx = i
			break
		}
	}
	require.NotEqual(t, -1, sepIdx, "the manifest must open the others section with a labeled separator")
	assert.Equal(t, expectedPrintableRowCount, sepIdx,
		"the others separator must sit immediately after the %d printed rows", expectedPrintableRowCount)
	assert.Equal(t, expectedPrintableRowCount, printableRowCount())

	// The 11 CSV rows 85-95, in order, below the divider.
	wantOthers := []struct {
		num       string
		label     string
		paramCode string
		kind      sheetRowKind
	}{
		{num: "85.", label: "R-AX..", kind: kindMissing},
		{num: "86.", label: "R-AE./A9/A.", kind: kindMissing},
		{num: "87.", label: "R-BC.", kind: kindMissing},
		{num: "88.", label: "R NS SP.", kind: kindMissing},
		{num: "89.", label: "R NS difference.", kind: kindMissing},
		{num: "90.", label: "B/C SP.", kind: kindMissing},
		{num: "91.", label: "R NS loss.", kind: kindMissing},
		{num: "92.", label: "R BC loss.", kind: kindMissing},
		{num: "93.", label: "Std loss as above.", paramCode: "QLTY_LOSS_DELIVERY_COST", kind: kindSnapshot},
		{num: "94.", label: "Addl Val Loss.", kind: kindMissing},
		{num: "95.", label: "Domestic cost with uneven packing.", paramCode: "DOMESTIC_COST_UNEVEN_PACK", kind: kindSnapshot},
	}

	others := costSheetRows[sepIdx+1:]
	require.Len(t, others, len(wantOthers))
	for i, want := range wantOthers {
		assert.Equal(t, want.num, others[i].Num, "others row %d number", i)
		assert.Equal(t, want.label, others[i].Label, "others row %d label", i)
		assert.Equal(t, want.paramCode, others[i].ParamCode, "others row %d param code", i)
		assert.Equal(t, want.kind, others[i].Kind, "others row %d kind", i)
	}
}

// TestBuildProductCostSheet_PrintAreaExcludesOthers reads the generated
// workbook's "_xlnm.Print_Area" defined name back and asserts it stops at the
// last row before the others section, so a user printing the sheet still gets
// the reference workbook's shape.
func TestBuildProductCostSheet_PrintAreaExcludesOthers(t *testing.T) {
	t.Parallel()

	stages := []Stage{
		stageWith(map[string]string{}),
		stageWith(map[string]string{}),
	}

	f, err := BuildProductCostSheet(stages)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)

	var got *excelize.DefinedName
	for _, dn := range f.GetDefinedName() {
		if dn.Name == printAreaDefinedName && dn.Scope == sheet {
			cp := dn
			got = &cp
			break
		}
	}
	require.NotNil(t, got, "the sheet must declare a %s defined name", printAreaDefinedName)

	lastPrintedRow := headerRowCount + expectedPrintableRowCount
	// Columns: A plus one per stage.
	lastCol, err := excelize.ColumnNumberToName(len(stages) + 1)
	require.NoError(t, err)
	assert.Equal(t,
		quoteSheetRef(sheet)+"!$A$1:$"+lastCol+"$"+strconv.Itoa(lastPrintedRow),
		got.RefersTo,
		"the print area must end at the last row before the others section")

	// The others rows are still in the sheet, just outside that range.
	rows, err := f.GetRows(sheet)
	require.NoError(t, err)
	assert.Len(t, rows, headerRowCount+expectedSheetRowCount)

	// The separator row itself is the first row past the print area.
	sepLabel, err := f.GetCellValue(sheet, "A"+strconv.Itoa(lastPrintedRow+1))
	require.NoError(t, err)
	assert.Equal(t, othersSeparatorLabel, sepLabel)
}

// TestBuildProductCostSheet_PrintAreaQuotesSpacedSheetName guards the formula
// reference itself: the default sheet name "Cost Sheet" and the single-product
// reference name "parameter check" both contain a space, and an unquoted
// space makes the reference invalid to Excel.
func TestBuildProductCostSheet_PrintAreaQuotesSpacedSheetName(t *testing.T) {
	t.Parallel()

	// No stages at all -> the sheet takes the (spaced) default name.
	f, err := BuildProductCostSheet(nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	require.Equal(t, defaultSheetName, sheet)
	require.Contains(t, sheet, " ", "this test is only meaningful for a spaced sheet name")

	for _, dn := range f.GetDefinedName() {
		if dn.Name == printAreaDefinedName && dn.Scope == sheet {
			assert.True(t, strings.HasPrefix(dn.RefersTo, "'"+sheet+"'!"),
				"a spaced sheet name must be single-quoted in the print-area reference, got %q", dn.RefersTo)
			return
		}
	}
	t.Fatalf("no %s defined name for sheet %q", printAreaDefinedName, sheet)
}

// TestBuildProductCostSheet_OthersRowsRenderValues checks the two wired others
// rows actually resolve their snapshot values in the rendered sheet.
func TestBuildProductCostSheet_OthersRowsRenderValues(t *testing.T) {
	t.Parallel()

	stages := []Stage{stageWith(map[string]string{
		"QLTY_LOSS_DELIVERY_COST":   "0.034",
		"DOMESTIC_COST_UNEVEN_PACK": "2.393",
	})}

	f, err := BuildProductCostSheet(stages)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	for label, want := range map[string]string{
		"Std loss as above.":                 "0.034",
		"Domestic cost with uneven packing.": "2.393",
	} {
		row := manifestRowIndexByLabel(t, label) + headerRowCount + 1
		cell, cErr := excelize.CoordinatesToCellName(2, row)
		require.NoError(t, cErr)
		raw, vErr := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
		require.NoError(t, vErr)
		assert.Equal(t, want, raw, "row %q must render its snapshot value", label)
	}
}
