package worker

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// The flat sheet's width is fixed by the legacy workbook it reproduces: nine
// identity columns (A..I) plus 140 parameter columns (J..ES).
const (
	allDataExpectedColumns  = 149
	allDataExpectedIdentity = 9
)

func TestAllDataColumns_Shape(t *testing.T) {
	require.Len(t, allDataColumns, allDataExpectedColumns)

	identity := 0
	seenHeaders := map[string]bool{}
	for i, col := range allDataColumns {
		require.NotEmpty(t, col.Header, "column %d has no header", i)
		require.False(t, seenHeaders[col.Header], "duplicate header %q", col.Header)
		seenHeaders[col.Header] = true

		if col.Kind == allDataIdentity {
			identity++
			require.Less(t, i, allDataExpectedIdentity, "identity column %q must lead the sheet", col.Header)
			require.Empty(t, col.ParamCode, "identity column %q must not carry a param code", col.Header)
			continue
		}
		require.NotEmpty(t, col.ParamCode, "param column %q has no param code", col.Header)
	}
	require.Equal(t, allDataExpectedIdentity, identity)
}

// The nine identity columns must appear in the exact order and wording of the
// legacy workbook — downstream consumers key off the header text.
func TestAllDataColumns_IdentityHeaders(t *testing.T) {
	want := []string{
		"No", "Left Sys ID", "Left No", "Name", "Yarn Type",
		"Item Code", "Shade Code", "Shade Name", "Machine Code",
	}
	got := make([]string, 0, len(want))
	for _, col := range allDataColumns[:allDataExpectedIdentity] {
		got = append(got, col.Header)
	}
	require.Equal(t, want, got)
}

func allDataFixture() []Stage {
	return []Stage{
		{
			RouteLevel:   1,
			RouteSeq:     1,
			ProductSysID: 14668,
			LeftSysID:    "2026091093759",
			ProductName:  "POY 250/72/RND/DSD/SIM/NS/1/O",
			YarnType:     "POY",
			ItemCode:     "POY0000264",
			ShadeCode:    "7A12-01",
			ShadeName:    "TISHA GY",
			HasCost:      true,
			ParamSnapshot: map[string]string{
				"MC_NAME":       "A1-8-S",
				"ORION_ITEM":    "POY0000264",
				"MC_EFFICIENCY": "98",
				"INTERMINGLE":   "NIM",
				// NO_BOB_PER_TROLLIES is deliberately absent — it must stay blank.
			},
		},
		{
			RouteLevel:   2,
			RouteSeq:     1,
			ProductSysID: 14673,
			ItemCode:     "PTY0000090",
			ShadeCode:    "6912-01",
			// HasCost false: every param column must be blank for this stage.
			HasCost:       false,
			ParamSnapshot: map[string]string{"MC_NAME": "BT-S"},
		},
	}
}

func TestWriteAllDataSheet_HeaderAndIdentity(t *testing.T) {
	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, WriteAllDataSheet(f, allDataFixture()))

	rows, err := f.GetRows(allDataSheetName)
	require.NoError(t, err)
	require.Len(t, rows, 3, "header plus one row per stage")
	require.Len(t, rows[0], allDataExpectedColumns)

	for i, col := range allDataColumns {
		require.Equal(t, col.Header, rows[0][i])
	}

	// "Left No" is the product's own sys id; "No" is the 1-based row position.
	require.Equal(t, []string{
		"1", "2026091093759", "14668", "POY 250/72/RND/DSD/SIM/NS/1/O", "POY",
		"POY0000264", "7A12-01", "TISHA GY", "A1-8-S",
	}, rows[1][:allDataExpectedIdentity])
	require.Equal(t, "2", rows[2][0])
	require.Equal(t, "14673", rows[2][2])
}

// Links prefer the snapshot but fall back to composition from identity, and a
// composition missing any part yields nothing rather than a partial key.
func TestWriteAllDataSheet_LinkColumns(t *testing.T) {
	stages := allDataFixture()
	stages[0].ParamSnapshot["COSTING_LINK"] = "FROM-SNAPSHOT"

	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, WriteAllDataSheet(f, stages))

	// K = snapshot wins; L = composed from item code + shade code.
	costingLink, err := f.GetCellValue(allDataSheetName, "K2")
	require.NoError(t, err)
	require.Equal(t, "FROM-SNAPSHOT", costingLink)

	orionLink, err := f.GetCellValue(allDataSheetName, "L2")
	require.NoError(t, err)
	require.Equal(t, "POY0000264-7A12-01", orionLink)

	// The second stage has no cost row, but MC_NAME comes from master data
	// rather than the calculation, so its link still composes.
	composed, err := f.GetCellValue(allDataSheetName, "K3")
	require.NoError(t, err)
	require.Equal(t, "PTY0000090-6912-01-BT-S", composed)
}

// Absent params are genuinely empty cells — never a zero and never the "-"
// placeholder that the transposed sheet uses.
func TestWriteAllDataSheet_AbsentParamsStayBlank(t *testing.T) {
	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, WriteAllDataSheet(f, allDataFixture()))

	for _, cell := range []string{"BH2", "U2", "ES2"} {
		got, err := f.GetCellValue(allDataSheetName, cell)
		require.NoError(t, err)
		require.Empty(t, got, "cell %s must be blank", cell)
	}

	// A stage without a cost row contributes identity plus only the columns
	// sourced outside the calculation: MC_NAME (master data) and the two links
	// composed from identity.
	fromMaster := map[string]bool{
		"2.Marketing Costing Link": true,
		"3.Orion Link":             true,
		"7.Machine Name":           true,
	}
	rows, err := f.GetRows(allDataSheetName)
	require.NoError(t, err)
	for i, v := range rows[2] {
		if i < allDataExpectedIdentity || fromMaster[allDataColumns[i].Header] {
			continue
		}
		require.Empty(t, v, "no-cost stage must have blank param column %q", allDataColumns[i].Header)
	}
}

// Numeric params land as real numeric cells at full precision, so the sheet can
// be re-totalled without reparsing text.
func TestWriteAllDataSheet_NumericCells(t *testing.T) {
	stages := allDataFixture()
	// The reference workbook prints 17 significant digits, but that string and
	// its shortest form denote the same float64 — so the assertion below is on
	// bit-exact round-trip, not on digit count.
	const wasteRaw = "32.718000000000004"
	stages[0].ParamSnapshot["WASTE_PERC"] = wasteRaw

	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, WriteAllDataSheet(f, stages))

	// A numeric cell carries no "t" attribute at all — that is what makes it a
	// number in the XLSX schema, and it is exactly how the reference workbook
	// stores its values. Anything string-typed here would break re-totalling.
	typ, err := f.GetCellType(allDataSheetName, "S2")
	require.NoError(t, err)
	require.Equal(t, excelize.CellTypeUnset, typ, "MC Efficiency must be a bare numeric cell")

	raw, err := f.GetCellValue(allDataSheetName, "S2", excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	require.Equal(t, "98", raw)

	raw, err = f.GetCellValue(allDataSheetName, "AD2", excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	got, err := strconv.ParseFloat(raw, 64)
	require.NoError(t, err)
	want, err := strconv.ParseFloat(wasteRaw, 64)
	require.NoError(t, err)
	require.Equal(t, want, got, "numbers round-trip bit-exact, never rounded for display")

	// Text params stay text even in a mostly-numeric sheet.
	typ, err = f.GetCellType(allDataSheetName, "AA2")
	require.NoError(t, err)
	require.Equal(t, excelize.CellTypeSharedString, typ, "Inter Migle is a text param")
}

func TestWriteAllDataSheet_NoStages(t *testing.T) {
	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, WriteAllDataSheet(f, nil))

	rows, err := f.GetRows(allDataSheetName)
	require.NoError(t, err)
	require.Len(t, rows, 1, "header is written even with no stages")
}

// The sheet may be pre-created by the caller to reserve slot 0; writing into it
// must not fail or duplicate it.
func TestWriteAllDataSheet_ReusesPreCreatedSheet(t *testing.T) {
	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, f.SetSheetName(f.GetSheetName(0), allDataSheetName))

	require.NoError(t, WriteAllDataSheet(f, allDataFixture()))
	require.Equal(t, []string{allDataSheetName}, f.GetSheetList())
}
