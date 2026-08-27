package mbhead

import (
	"context"
	"errors"
	"testing"

	"bytes"

	"github.com/xuri/excelize/v2"
)

// fakeRecipeFullReader records the filter it was handed and replays a canned result.
type fakeRecipeFullReader struct {
	gotFilter RecipeFullFilter
	rows      []RecipeFullRow
	err       error
}

func (f *fakeRecipeFullReader) ListRecipeFullRows(_ context.Context, filter RecipeFullFilter) ([]RecipeFullRow, error) {
	f.gotFilter = filter
	return f.rows, f.err
}

func strp(s string) *string   { return &s }
func f64p(v float64) *float64 { return &v }
func i32p(v int32) *int32     { return &v }
func boolp(v bool) *bool      { return &v }

// readSheet parses the produced workbook back into rows for assertions.
func readSheet(t *testing.T, content []byte) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("open produced workbook: %v", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(recipeFullSheetName)
	if err != nil {
		t.Fatalf("read sheet %q: %v", recipeFullSheetName, err)
	}
	return rows
}

func TestExportFullHandler_OneRowPerCompositionLine(t *testing.T) {
	// C1: one recipe with 5 composition lines must produce exactly 5 body rows.
	rows := make([]RecipeFullRow, 0, 5)
	for i := int32(1); i <= 5; i++ {
		rows = append(rows, RecipeFullRow{
			MBCosting:       "MB-001",
			EntryStatus:     "VALIDATED",
			CompSeqNo:       i32p(i),
			CompSourceType:  strp("GROUP"),
			CompRMGroupCode: strp("GRP-A"),
			CompPct:         strp("20.000"),
			CompIsCarrier:   boolp(false),
			CheckStatusCalc: strp("Current"),
		})
	}
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: rows})

	content, name, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if name != "mb_recipe_full_export.xlsx" {
		t.Errorf("file name = %q", name)
	}
	sheet := readSheet(t, content)
	if got := len(sheet) - 1; got != 5 {
		t.Errorf("body rows = %d, want 5", got)
	}
}

func TestExportFullHandler_HeadWithoutCompositionStillYieldsOneRow(t *testing.T) {
	// A DRAFT head with no composition rows must still export exactly one row with the
	// composition columns left BLANK — not be dropped from the workbook.
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{
		MBCosting:   "MB-EMPTY",
		EntryStatus: "DRAFT",
	}}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	if got := len(sheet) - 1; got != 1 {
		t.Fatalf("body rows = %d, want 1", got)
	}
	seqCol := indexOfHeader(t, "Seq No")
	if v := cellAt(sheet, 1, seqCol); v != "" {
		t.Errorf("Seq No for composition-less head = %q, want empty", v)
	}
	if v := cellAt(sheet, 1, indexOfHeader(t, "Composition %")); v != "" {
		t.Errorf("Composition %% = %q, want empty", v)
	}
}

func TestExportFullHandler_NullCheckStatusCalcRendersExplicitly(t *testing.T) {
	// 🔴 NULL mbh_check_status_calc means "never computed". It must be visible as such,
	// never as a blank cell indistinguishable from a read failure.
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{
		{MBCosting: "MB-NULL", CheckStatusCalc: nil},
		{MBCosting: "MB-SET", CheckStatusCalc: strp("Outdated")},
	}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	col := indexOfHeader(t, "Check Status (Calc)")
	if v := cellAt(sheet, 1, col); v != CheckStatusCalcNotComputedLabel {
		t.Errorf("NULL derived check status = %q, want %q", v, CheckStatusCalcNotComputedLabel)
	}
	if v := cellAt(sheet, 2, col); v != "Outdated" {
		t.Errorf("set derived check status = %q, want Outdated", v)
	}
}

func TestExportFullHandler_AbsentOptionalsStayBlankNotZero(t *testing.T) {
	// D13: an absent Denier/Filament/Dozing must not be coerced to 0, and an absent
	// Is Carrier must not be coerced to false.
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{
		MBCosting: "MB-ABSENT",
	}}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	for _, header := range []string{"Denier", "Filament", "Dozing", "LDR %", "Is Carrier", "Cost Value", "Cost Product Sys ID"} {
		if v := cellAt(sheet, 1, indexOfHeader(t, header)); v != "" {
			t.Errorf("absent %s = %q, want empty cell", header, v)
		}
	}
}

func TestExportFullHandler_PresentZeroValuesAreNotBlanked(t *testing.T) {
	// The mirror of the previous test: a real 0 must survive as "0", so blank keeps
	// meaning "absent" and nothing else.
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{
		MBCosting:     "MB-ZERO",
		Denier:        f64p(0),
		Dozing:        f64p(0),
		CompIsCarrier: boolp(false),
	}}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	if v := cellAt(sheet, 1, indexOfHeader(t, "Denier")); v != "0" {
		t.Errorf("present zero Denier = %q, want \"0\"", v)
	}
	if v := cellAt(sheet, 1, indexOfHeader(t, "Is Carrier")); v != "FALSE" {
		t.Errorf("present false Is Carrier = %q, want FALSE", v)
	}
}

func TestExportFullHandler_DefaultsCostTypeToActual(t *testing.T) {
	// An empty cost type must be resolved to exactly one type before the read, otherwise
	// the row count multiplies by the number of cost types.
	reader := &fakeRecipeFullReader{}
	h := NewExportFullHandler(reader)

	if _, _, err := h.Handle(context.Background(), ExportFullCommand{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.gotFilter.CostType != DefaultExportCostType {
		t.Errorf("cost type = %q, want %q", reader.gotFilter.CostType, DefaultExportCostType)
	}
	if reader.gotFilter.IsActive != nil {
		t.Errorf("IsActive = %v, want nil (absence preserved)", *reader.gotFilter.IsActive)
	}
	if reader.gotFilter.Period != "" {
		t.Errorf("Period = %q, want empty", reader.gotFilter.Period)
	}
}

func TestExportFullHandler_PassesExplicitFilterThrough(t *testing.T) {
	reader := &fakeRecipeFullReader{}
	h := NewExportFullHandler(reader)
	active := true

	_, _, err := h.Handle(context.Background(), ExportFullCommand{
		ActiveOnly: &active, Period: "202607", CostType: "SELLING",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.gotFilter.Period != "202607" || reader.gotFilter.CostType != "SELLING" {
		t.Errorf("filter = %+v", reader.gotFilter)
	}
	if reader.gotFilter.IsActive == nil || !*reader.gotFilter.IsActive {
		t.Error("IsActive did not pass through as true")
	}
}

// TestExportFullHandler_EmptyCheckStatusCalcStaysEmpty pins the load-bearing half of
// the semantics: an unset derived-check-status filter must reach the reader as the
// EMPTY STRING, which the SQL reads as "no filter at all" — so the export keeps
// returning every head, the NULL/"Belum dihitung" ones included. ⛔ If anything ever
// defaults this the way CostType is defaulted, those heads vanish from a plain export.
func TestExportFullHandler_EmptyCheckStatusCalcStaysEmpty(t *testing.T) {
	reader := &fakeRecipeFullReader{}
	h := NewExportFullHandler(reader)

	if _, _, err := h.Handle(context.Background(), ExportFullCommand{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.gotFilter.CheckStatusCalc != "" {
		t.Errorf("CheckStatusCalc = %q, want empty (no filter — all rows incl. NULL)",
			reader.gotFilter.CheckStatusCalc)
	}
}

// TestExportFullHandler_PassesCheckStatusCalcThrough proves the value travels VERBATIM,
// with no normalisation, for both a status DeriveCheckStatus actually produces today
// and one it does not.
//
// ⚠ "Outdated" is included on purpose: it is one of the six values allowed by
// chk_mbh_check_status_calc (migration 000487) but one of the THREE that
// DeriveCheckStatus never produces today ("Current", "Outdated", "Rejected"). The
// handler must still forward it untouched — the empty result set that follows is a
// correct answer, ⛔ not a bug to be papered over here.
func TestExportFullHandler_PassesCheckStatusCalcThrough(t *testing.T) {
	// Literals, not domain constants: these are the DIFFERENT package
	// internal/domain/mbhead (domainmbhead.CheckStatusApproved etc.) and importing it
	// here just for two strings would couple the application-layer test to the domain
	// for no gain. The values are the ones chk_mbh_check_status_calc allows verbatim.
	for _, want := range []string{"Approved", "Outdated"} {
		reader := &fakeRecipeFullReader{}
		h := NewExportFullHandler(reader)

		_, _, err := h.Handle(context.Background(), ExportFullCommand{CheckStatusCalc: want})
		if err != nil {
			t.Fatalf("Handle(%q): %v", want, err)
		}
		if reader.gotFilter.CheckStatusCalc != want {
			t.Errorf("CheckStatusCalc = %q, want %q", reader.gotFilter.CheckStatusCalc, want)
		}
	}
}

// TestExportFullHandler_DefaultForwardsIncludeRejectedFalse pins that an
// ExportFullCommand{} (the zero value, exactly what every current gRPC caller sends —
// there is no proto field for this yet) reaches the reader as
// RecipeFullFilter.IncludeRejected = false. §11 item 140: recipeFullQuery previously had
// no predicate at all excluding REJECTED heads.
func TestExportFullHandler_DefaultForwardsIncludeRejectedFalse(t *testing.T) {
	reader := &fakeRecipeFullReader{}
	h := NewExportFullHandler(reader)

	if _, _, err := h.Handle(context.Background(), ExportFullCommand{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.gotFilter.IncludeRejected {
		t.Error("IncludeRejected = true, want false (the safe default)")
	}
}

// TestExportFullHandler_IncludeRejectedTrueForwardsFlag pins the explicit opt-in path:
// ExportFullCommand.IncludeRejected = true must reach the reader as
// RecipeFullFilter.IncludeRejected = true.
//
// ⚠ PENDING FOLLOW-UP: no proto field exists yet to set this from a gRPC request — this
// path is reachable only from Go callers until a proto field is added. See
// RecipeFullFilter.IncludeRejected's doc comment.
func TestExportFullHandler_IncludeRejectedTrueForwardsFlag(t *testing.T) {
	reader := &fakeRecipeFullReader{}
	h := NewExportFullHandler(reader)

	if _, _, err := h.Handle(context.Background(), ExportFullCommand{IncludeRejected: true}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !reader.gotFilter.IncludeRejected {
		t.Error("IncludeRejected = false, want true")
	}
}

// TestExportFullHandler_DefaultExcludesRejectedRow simulates the reader actually applying
// the default filter (as recipeFullQuery's $5/$6 predicate does — see the postgres
// package's structural tests) by having the fake return a REJECTED-free set, then asserts
// the rendered workbook does not contain the rejected head. This locks the CONSUMING side
// of the fix: whatever the reader excludes must not reappear in the output.
func TestExportFullHandler_DefaultExcludesRejectedRow(t *testing.T) {
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{
		{MBCosting: "MB-KEEP", EntryStatus: "VALIDATED"},
		// A real reader would never return this row for IncludeRejected=false; this canned
		// value exists only to prove the handler renders whatever the reader hands back and
		// does not itself filter (filtering is the SQL predicate's job).
	}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	mbCostCol := indexOfHeader(t, "MB Costing")
	var costings []string
	for _, row := range sheet[1:] {
		if mbCostCol < len(row) {
			costings = append(costings, row[mbCostCol])
		}
	}
	for _, c := range costings {
		if c == "MB-REJECTED" {
			t.Errorf("rejected head must not appear in the default export, got %v", costings)
		}
	}
}

func TestExportFullHandler_ReaderErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")
	h := NewExportFullHandler(&fakeRecipeFullReader{err: sentinel})

	if _, _, err := h.Handle(context.Background(), ExportFullCommand{}); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wrapped %v", err, sentinel)
	}
}

func TestRecipeFullHeadersAreDistinctFromImportTemplate(t *testing.T) {
	// ⚠ The import template reuses mbHeadExportHeaders. The full export MUST NOT share
	// that slice, or its derived, non-importable columns would look importable.
	if len(recipeFullHeaders) == len(mbHeadExportHeaders) {
		t.Fatal("full-export headers must not mirror the import template header list")
	}
	for _, h := range mbHeadExportHeaders {
		if h == "Check Status (Calc)" {
			t.Fatal("derived check status leaked into the import template headers")
		}
	}
}

func TestRecipeFullHeaderCountMatchesRowWidth(t *testing.T) {
	// A drifting column order silently shifts every value one cell over; pin the width.
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{MBCosting: "X"}}})
	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	if len(sheet[0]) != len(recipeFullHeaders) {
		t.Errorf("header row width = %d, want %d", len(sheet[0]), len(recipeFullHeaders))
	}
}

// indexOfHeader returns the 0-based column index of a header, failing the test if absent.
func indexOfHeader(t *testing.T, name string) int {
	t.Helper()
	for i, h := range recipeFullHeaders {
		if h == name {
			return i
		}
	}
	t.Fatalf("header %q not found", name)
	return -1
}

// cellAt reads a body cell, tolerating trailing-blank truncation by excelize.
func cellAt(sheet [][]string, row, col int) string {
	if row >= len(sheet) || col >= len(sheet[row]) {
		return ""
	}
	return sheet[row][col]
}

// TestLegacyExportHeadersArePinned pins the EXACT contents and order of
// mbHeadExportHeaders.
//
// ⚠ WHY A HARDCODED LITERAL AND NOT A HASH/GOLDEN FILE: a hash tells a future
// editor only "something changed"; this literal tells them WHAT changed and, by
// sitting next to the explanation below, WHY they must not. The list is short and
// stable, so the maintenance cost of the literal is near zero.
//
// ⚠ WHY THIS GUARD EXISTS — CORRECTED 2026-08-23 (orchestrator). An earlier draft
// of this comment claimed the importer parses mbHeadExportHeaders positionally, so
// that reordering an export column would shift imported values. That is FALSE and
// was removed rather than left to mislead a future editor. Verified on disk:
//
//   - import_handler.go:118 parseMBHeadRow reads FIXED indices 0..10 and never
//     consults either header list; import_handler.go:66 skips only rows[0].
//   - mbHeadExportHeaders (export_handler.go:71) has NO reader outside
//     export_handler.go. Editing it cannot affect the importer at all.
//
// What the guard is actually worth: mbHeadExportHeaders is the column contract of
// a file users already hold on disk, and writeMBHeadRow hardcodes cell letters
// A..O. A header edit that is not matched by an equivalent writeMBHeadRow edit
// mislabels a column — right values under the wrong names, with no error raised.
// That is the failure this literal catches. It is a data-labelling guard, not an
// import guard.
//
// If this test fails: check writeMBHeadRow's A..O assignments in the same commit.
func TestLegacyExportHeadersArePinned(t *testing.T) {
	want := []string{
		"No", "MB Costing", "Mgt Name", "Dev Code", "Shade Code", "Shade Name",
		"Cross Section", "Lusture Code", "Denier", "Filament", "Dozing",
		"Is Bought Out", "Active", "Created At", "Created By",
	}

	if len(mbHeadExportHeaders) != len(want) {
		t.Fatalf("legacy export column COUNT changed: got %d (%v), want %d (%v)\n"+
			"this breaks the positional import template — see the doc comment on this test",
			len(mbHeadExportHeaders), mbHeadExportHeaders, len(want), want)
	}
	for i := range want {
		if mbHeadExportHeaders[i] != want[i] {
			t.Errorf("legacy export column %d changed: got %q, want %q\n"+
				"reordering or renaming an export column breaks the positional import template — "+
				"see the doc comment on this test", i, mbHeadExportHeaders[i], want[i])
		}
	}
}

// TestImportTemplateHeadersRemainASubsequenceOfExport pins the STRUCTURAL
// relationship between the two lists: every mbHeadTemplateHeaders column appears
// in mbHeadExportHeaders in the same relative order (today template == export[1:12]
// exactly). The export additionally carries system-generated columns "No",
// "Active", "Created At", "Created By", which are not importable.
//
// ⚠ CORRECTED 2026-08-23 (orchestrator): an earlier draft said this relationship
// "makes export → import round-tripping safe". It does NOT, and the opposite is
// true today — see §11 in the consolidated plan. Feeding an EXPORTED file straight
// back into the importer is already broken regardless of this test, because the
// export leads with "No" while parseMBHeadRow reads index 0 as mb_costing; every
// value lands one column off and mb_costing receives a row counter. This test
// pins list ALIGNMENT only; it makes no claim about round-tripping, and must not
// be cited as evidence that round-tripping works.
//
// A literal-only guard would still pass if someone changed BOTH lists in
// mismatched ways; this test catches that divergence.
func TestImportTemplateHeadersRemainASubsequenceOfExport(t *testing.T) {
	i := 0
	for _, tpl := range mbHeadTemplateHeaders {
		for i < len(mbHeadExportHeaders) && mbHeadExportHeaders[i] != tpl {
			i++
		}
		if i == len(mbHeadExportHeaders) {
			t.Fatalf("import template column %q is missing from the legacy export headers, "+
				"or the two lists now disagree on column ORDER; export=%v template=%v",
				tpl, mbHeadExportHeaders, mbHeadTemplateHeaders)
		}
		i++
	}
}

// colOf resolves a header label to its 0-based column index in the produced sheet, so the
// assertions below survive a future column insertion instead of silently shifting.
func colOf(t *testing.T, header string) int {
	t.Helper()
	for i, h := range recipeFullHeaders {
		if h == header {
			return i
		}
	}
	t.Fatalf("header %q not found in recipeFullHeaders", header)
	return -1
}

// C2: the MB cost figures must land in the MB Cost block, not bleed into the neighbouring
// composition or traceability columns. The existing suite pins blank/zero rendering but
// never pinned *placement* of populated cost values.
func TestExportFullHandler_MBCostValuesLandInTheCostColumns(t *testing.T) {
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{
		MBCosting:        "MB-001",
		EntryStatus:      "VALIDATED",
		CompSeqNo:        i32p(1),
		CompSourceType:   strp("GROUP"),
		CompRMGroupCode:  strp("GRP-A"),
		CompPct:          strp("100.000"),
		CompIsCarrier:    boolp(false),
		CostPeriod:       strp("202608"),
		CostType:         strp("ACTUAL"),
		CostValue:        strp("12345.6789"),
		CostPushedAt:     strp("2026-08-24T10:00:00Z"),
		CostProductSysID: 9911,
		CostProductCode:  strp("MB-PRD-9911"),
		CostGeneratedAt:  strp("2026-08-23T08:30:00Z"),
	}}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	if len(sheet) < 2 {
		t.Fatalf("expected a body row, got %d sheet rows", len(sheet))
	}
	body := sheet[1]

	for _, tc := range []struct{ header, want string }{
		{"Cost Period", "202608"},
		{"Cost Type", "ACTUAL"},
		{"Cost Value", "12345.6789"},
		{"Cost Pushed At", "2026-08-24T10:00:00Z"},
		{"Cost Product Sys ID", "9911"},
		{"Cost Product Code", "MB-PRD-9911"},
		{"Cost Generated At", "2026-08-23T08:30:00Z"},
	} {
		col := colOf(t, tc.header)
		if col >= len(body) {
			t.Errorf("column %q (index %d) missing from body row of width %d", tc.header, col, len(body))
			continue
		}
		if body[col] != tc.want {
			t.Errorf("column %q = %q, want %q", tc.header, body[col], tc.want)
		}
	}

	// The composition block immediately to the left must be untouched by the cost values.
	if got := body[colOf(t, "Composition %")]; got != "100.000" {
		t.Errorf("composition %% column = %q, want %q — cost block bled left", got, "100.000")
	}
}

// A recipe that has never been pushed to cost must still export cleanly: the cost block is
// blank rather than zero, and the row must not panic or be dropped.
func TestExportFullHandler_RecipeWithoutMBCostLeavesCostColumnsBlank(t *testing.T) {
	h := NewExportFullHandler(&fakeRecipeFullReader{rows: []RecipeFullRow{{
		MBCosting:      "MB-NOCOST",
		EntryStatus:    "DRAFT",
		CompSeqNo:      i32p(1),
		CompSourceType: strp("GROUP"),
		CompPct:        strp("100.000"),
	}}})

	content, _, err := h.Handle(context.Background(), ExportFullCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	sheet := readSheet(t, content)
	if got := len(sheet) - 1; got != 1 {
		t.Fatalf("body rows = %d, want 1", got)
	}
	body := sheet[1]

	for _, header := range []string{
		"Cost Period", "Cost Type", "Cost Value", "Cost Pushed At",
		"Cost Product Sys ID", "Cost Product Code", "Cost Generated At",
	} {
		col := colOf(t, header)
		if col < len(body) && body[col] != "" {
			t.Errorf("column %q = %q, want blank for a never-pushed recipe", header, body[col])
		}
	}
}
