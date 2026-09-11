package mbhead_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
)

// stubCostCalcDetailReader returns a fixed row set and captures the filter it was given.
type stubCostCalcDetailReader struct {
	rows []mbhead.CostCalcDetailRow
	got  mbhead.CostCalcDetailFilter
	err  error

	// skipped is what the observability probe reports: mb_codes whose selected cost
	// snapshot carries an EMPTY cpc_rm_cost_detail array, so they contribute no rows.
	skipped    []string
	skippedErr error
	// gotSkipped captures the filter the probe was given, to pin that it mirrors the
	// row read's filter exactly.
	gotSkipped mbhead.CostCalcDetailFilter
}

func (s *stubCostCalcDetailReader) ListCostCalcDetailRows(
	_ context.Context, f mbhead.CostCalcDetailFilter,
) ([]mbhead.CostCalcDetailRow, error) {
	s.got = f
	return s.rows, s.err
}

func (s *stubCostCalcDetailReader) ListCostCalcDetailSkippedMBCodes(
	_ context.Context, f mbhead.CostCalcDetailFilter,
) ([]string, error) {
	s.gotSkipped = f
	return s.skipped, s.skippedErr
}

func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }

// referenceRow reproduces the first data row of the reference export
// (data-examples/export-product-cost/example-export-cost.txt, CSTMB2607000001).
// Every number here is a value observed in that file, not a value invented for
// the test — so the identity assertions below are pinned to real production data.
func referenceRow() mbhead.CostCalcDetailRow {
	return mbhead.CostCalcDetailRow{
		MBCode:          "CSTMB2607000001",
		MBName:          str("ALLOY GREY TR-712-A"),
		TotalFix:        f64(939.33),
		PctWaste:        f64(2),
		PctQualityLoss:  f64(0.6),
		PctEfficiency:   f64(94),
		DevExpense:      f64(3),
		Packing:         f64(0.1),
		ProdPerDay:      f64(16),
		ThroughputPerHr: f64(55),
		NoProcess:       f64(1),
		MBNetProd:       f64(827.2),
		MBWasteVal:      f64(0.03529329),
		MBFixedTotal:    f64(1.135553675),
		MBCostOthers:    f64(0.204407854),
		MBRMCost:        f64(1.729371),
		MBConvCost:      f64(1.375255),
		MBTotalCost:     f64(3.104626),
		MBCostPerUnit:   f64(3.104626),
		CalcVersion:     9,
		CalcStatus:      "APPROVED",
		// RowNo is the per-period presentation ordinal: CSTMB2607000001 has code
		// suffix 000001, so 1. ⚠ The reference file shows 52939 here — that value is
		// cost_route_seq.crs_seq_id, and it is DELIBERATELY not reproduced (unstable:
		// cost_route_seq rows are deleted and reinserted). See CostCalcDetailRow.RowNo.
		RowNo:        1,
		RMType:       "GROUP",
		RMRef:        "202006004",
		RMGroupName:  str("DYE0000015"),
		Komposisi:    f64(0.0011),
		RMRateActual: f64(14.774611),
		RMRateTier:   "CL",
		CostActual:   f64(0.016252),
	}
}

// TestCostCalcDetail_ReferenceIdentities pins the arithmetic relationships the
// coordinator verified across all 21,813 rows of the reference export. They are
// asserted here so a future change to the param codes this export reads (e.g.
// swapping MB_THROUGHPUT back to the THROUGHPUT_PER_HOUR picklist, or reading
// MB_FIXED_COST instead of MB_FIXED_TOTAL) fails loudly instead of silently
// emitting a differently-sourced number.
func TestCostCalcDetail_ReferenceIdentities(t *testing.T) {
	r := referenceRow()
	const eps = 1e-6

	// mb_net_prod == throughput_per_hr * prod_per_day * pct_efficiency/100.
	// Held for 21813/21813 rows. This is the proof that throughput_per_hr is the
	// numeric MB_THROUGHPUT param and NOT the THROUGHPUT_PER_HOUR picklist, whose
	// options (30/40/50/60/70) cannot produce the observed 34/20/55.
	wantNet := *r.ThroughputPerHr * *r.ProdPerDay * *r.PctEfficiency / 100
	if math.Abs(wantNet-*r.MBNetProd) > eps {
		t.Errorf("mb_net_prod identity: got %v, want %v", *r.MBNetProd, wantNet)
	}

	// mb_fixed_total == total_fix / mb_net_prod (4000/4000 sampled). Pins total_fix
	// to MACHINE_MB_FIXED_TOTAL and the result param to MB_FIXED_TOTAL.
	wantFixed := *r.TotalFix / *r.MBNetProd
	if math.Abs(wantFixed-*r.MBFixedTotal) > eps {
		t.Errorf("mb_fixed_total identity: got %v, want %v", *r.MBFixedTotal, wantFixed)
	}

	// mb_total_cost == mb_rm_cost + mb_conv_cost (21813/21813).
	wantTotal := *r.MBRMCost + *r.MBConvCost
	if math.Abs(wantTotal-*r.MBTotalCost) > eps {
		t.Errorf("mb_total_cost identity: got %v, want %v", *r.MBTotalCost, wantTotal)
	}

	// mb_cost_per_unit is byte-identical to mb_total_cost in every row — both are
	// MB_FINAL_COST.
	if *r.MBCostPerUnit != *r.MBTotalCost {
		t.Errorf("mb_cost_per_unit must equal mb_total_cost: got %v vs %v", *r.MBCostPerUnit, *r.MBTotalCost)
	}

	// cost_actual == komposisi * rm_rate_actual (21460/21460 GROUP rows).
	wantCost := *r.Komposisi * *r.RMRateActual
	if math.Abs(wantCost-*r.CostActual) > eps {
		t.Errorf("cost_actual identity: got %v, want %v", *r.CostActual, wantCost)
	}
}

// TestCostCalcDetail_HeaderIsExactly29SnakeCaseColumns pins the contract with the
// downstream tooling that keys on these exact names.
func TestCostCalcDetail_HeaderIsExactly29SnakeCaseColumns(t *testing.T) {
	want := []string{
		"mb_code", "mb_name", "total_fix", "pct_waste", "pct_quality_loss",
		"pct_efficiency", "dev_expense", "packing", "prod_per_day",
		"throughput_per_hr", "no_process", "mb_net_prod", "mb_waste_val",
		"mb_fixed_total", "mb_cost_others", "mb_rm_cost", "mb_conv_cost",
		"mb_total_cost", "mb_cost_per_unit", "calc_version", "calc_status", "row_no",
		"rm_type", "rm_ref", "rm_group_name", "komposisi", "rm_rate_actual",
		"rm_rate_tier", "cost_actual",
	}

	rows := sheetRows(t, []mbhead.CostCalcDetailRow{referenceRow()})
	if len(rows) < 1 {
		t.Fatal("workbook has no header row")
	}
	got := rows[0]
	if len(got) != len(want) {
		t.Fatalf("column count: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestCostCalcDetail_ReferenceRowRendersVerbatim checks the observed reference values
// survive into the sheet in the right columns.
func TestCostCalcDetail_ReferenceRowRendersVerbatim(t *testing.T) {
	rows := sheetRows(t, []mbhead.CostCalcDetailRow{referenceRow()})
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 data row, got %d rows", len(rows))
	}
	data := rows[1]

	for _, tc := range []struct {
		col  int
		name string
		want string
	}{
		{0, "mb_code", "CSTMB2607000001"},
		{1, "mb_name", "ALLOY GREY TR-712-A"},
		{2, "total_fix", "939.33"},
		{9, "throughput_per_hr", "55"},
		{11, "mb_net_prod", "827.2"},
		{19, "calc_version", "9"},
		{20, "calc_status", "APPROVED"},
		// 1, not the reference file's 52939. The reference value is crs_seq_id, an
		// unstable id we deliberately do not export. See CostCalcDetailRow.RowNo.
		{21, "row_no", "1"},
		{22, "rm_type", "GROUP"},
		{23, "rm_ref", "202006004"},
		{24, "rm_group_name", "DYE0000015"},
		{27, "rm_rate_tier", "CL"},
	} {
		if data[tc.col] != tc.want {
			t.Errorf("%s (col %d): got %q, want %q", tc.name, tc.col, data[tc.col], tc.want)
		}
	}
}

// TestCostCalcDetail_ProductRowsRenderBlankTier pins the rule that PRODUCT-type rows
// (nested-MB references, which have no cst_rm_cost row) carry an EMPTY tier. In the
// reference dataset the blank count (353) equals the PRODUCT row count exactly.
//
// ⛔ A blank here must never be replaced by a placeholder label: downstream tooling
// distinguishes "no RM cost row exists" from any real tier.
func TestCostCalcDetail_ProductRowsRenderBlankTier(t *testing.T) {
	product := referenceRow()
	product.RMType = "PRODUCT"
	product.RMRef = "product:4711"
	product.RMGroupName = nil // a PRODUCT ref never resolves to an rm group
	product.RMRateTier = ""

	rows := sheetRows(t, []mbhead.CostCalcDetailRow{product})
	data := rows[1]
	if data[22] != "PRODUCT" {
		t.Errorf("rm_type: got %q, want PRODUCT", data[22])
	}
	if data[24] != "" {
		t.Errorf("rm_group_name for a PRODUCT ref must be blank, got %q", data[24])
	}
	if data[27] != "" {
		t.Errorf("rm_rate_tier for a PRODUCT row must be blank, got %q", data[27])
	}
}

// TestCostCalcDetail_AbsentNumbersStayBlank pins D13: a param the calc never produced
// renders as an empty cell, never as a fabricated 0.
func TestCostCalcDetail_AbsentNumbersStayBlank(t *testing.T) {
	r := referenceRow()
	r.MBFixedTotal = nil
	r.RMRateActual = nil

	rows := sheetRows(t, []mbhead.CostCalcDetailRow{r})
	data := rows[1]
	if data[13] != "" {
		t.Errorf("absent mb_fixed_total must be blank, got %q", data[13])
	}
	if data[26] != "" {
		t.Errorf("absent rm_rate_actual must be blank, got %q", data[26])
	}
}

// TestCostCalcDetail_DefaultsCalculationTypeOnly pins that ACTUAL is the ONLY defaulted
// filter — every other absent filter is forwarded as absent (D13).
func TestCostCalcDetail_DefaultsCalculationTypeOnly(t *testing.T) {
	reader := &stubCostCalcDetailReader{}
	h := mbhead.NewExportCostCalcDetailHandler(reader)

	if _, _, err := h.Handle(context.Background(), mbhead.ExportCostCalcDetailCommand{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.got.CalculationType != mbhead.DefaultCostCalcDetailType {
		t.Errorf("calculation type: got %q, want %q", reader.got.CalculationType, mbhead.DefaultCostCalcDetailType)
	}
	if reader.got.Period != "" || reader.got.CalcStatus != "" {
		t.Errorf("absent period/status must stay absent, got %q/%q", reader.got.Period, reader.got.CalcStatus)
	}
	if reader.got.IsActive != nil {
		t.Errorf("absent active filter must stay nil, got %v", *reader.got.IsActive)
	}
	if reader.got.IncludeRejected {
		t.Error("IncludeRejected must default to false (exclude rejected heads)")
	}

	// An explicit type is forwarded verbatim, never overridden.
	if _, _, err := h.Handle(context.Background(), mbhead.ExportCostCalcDetailCommand{
		CalculationType: "SELLING", CalcStatus: "APPROVED",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reader.got.CalculationType != "SELLING" || reader.got.CalcStatus != "APPROVED" {
		t.Errorf("explicit filters must pass through, got %q/%q", reader.got.CalculationType, reader.got.CalcStatus)
	}
}

// TestCostCalcDetail_WorkbookIsNotPasswordProtected pins the hard requirement that the
// export is plaintext: it must open with no password at all.
func TestCostCalcDetail_WorkbookIsNotPasswordProtected(t *testing.T) {
	content := workbookBytes(t, []mbhead.CostCalcDetailRow{referenceRow()})

	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("workbook must open with NO password, got: %v", err)
	}
	defer func() { _ = f.Close() }()
}

// workbookBytes runs the handler over rows and returns the raw workbook.
func workbookBytes(t *testing.T, rows []mbhead.CostCalcDetailRow) []byte {
	t.Helper()
	return workbookBytesFrom(t, &stubCostCalcDetailReader{rows: rows})
}

// workbookBytesFrom runs the handler over a fully configured reader stub.
func workbookBytesFrom(t *testing.T, reader *stubCostCalcDetailReader) []byte {
	t.Helper()
	h := mbhead.NewExportCostCalcDetailHandler(reader)
	content, name, err := h.Handle(context.Background(), mbhead.ExportCostCalcDetailCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if name != "mb_cost_calc_detail_export.xlsx" {
		t.Errorf("file name: got %q", name)
	}
	return content
}

// captureWarnLogs redirects the package-level zerolog logger into a buffer for the
// duration of one test and returns the accumulated output.
//
// ⚠ Tests using it must NOT call t.Parallel: they mutate the global logger.
func captureWarnLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.WarnLevel)
	defer func() { log.Logger = prev }()
	fn()
	return buf.String()
}

// TestCostCalcDetail_EmptyRMDetailMBIsCountedAndLogged pins the whole point of the skip
// accounting: an MB whose selected cost snapshot holds an EMPTY cpc_rm_cost_detail array
// flattens to ZERO rows and so vanishes from the workbook. Before this it vanished with
// NO log, NO count and NO warning.
//
// ⭐ Production case, not a hypothetical: CSTMB2609000099 ("TW0509 50% TIO2 PBT
// MASTERBATCH") is VALIDATED, active, version 9, has an APPROVED 202607/ACTUAL cost row, a
// route head and a route sequence — and zero RM lines on every one of its 21 cost rows.
// The upstream data defect is out of scope; this test pins that the export no longer hides
// it.
func TestCostCalcDetail_EmptyRMDetailMBIsCountedAndLogged(t *testing.T) {
	reader := &stubCostCalcDetailReader{
		rows:    []mbhead.CostCalcDetailRow{referenceRow()},
		skipped: []string{"CSTMB2609000099"},
	}

	out := captureWarnLogs(t, func() {
		h := mbhead.NewExportCostCalcDetailHandler(reader)
		if _, _, err := h.Handle(context.Background(), mbhead.ExportCostCalcDetailCommand{}); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	})

	if !strings.Contains(out, `"skipped_count":1`) {
		t.Errorf("the skipped MB must be COUNTED, got log:\n%s", out)
	}
	if !strings.Contains(out, "CSTMB2609000099") {
		t.Errorf("the skipped MB must be NAMED in the log, got log:\n%s", out)
	}
	if !strings.Contains(out, "masterbatch CSTMB2609000099 has no rm cost lines and was skipped") {
		t.Errorf("the warning must mirror the costsheet export phrasing, got log:\n%s", out)
	}

	// The probe must see the SAME filter as the row read, or it would report heads the
	// dump never considered.
	if reader.gotSkipped != reader.got {
		t.Errorf("skip probe filter %+v must equal the row filter %+v", reader.gotSkipped, reader.got)
	}
}

// TestCostCalcDetail_SkipAccountingDoesNotChangeEmittedRows is the HARD CONSTRAINT test.
// The dump emits 21813 rows for period 202607 and that number matches the reference
// workbook EXACTLY. The skip accounting is observability ONLY: the emitted rows, their
// count, the column set and the ordering must be byte-for-byte what they were before.
//
// ⛔ If this fails, the skip accounting has started feeding the workbook — revert it
// rather than adjusting this test.
func TestCostCalcDetail_SkipAccountingDoesNotChangeEmittedRows(t *testing.T) {
	second := referenceRow()
	second.MBCode = "CSTMB2607000002"
	second.RMRef = "202006005"
	data := []mbhead.CostCalcDetailRow{referenceRow(), second}

	var without, with [][]string
	// The logger is swapped for both runs so the two workbooks differ in NOTHING but
	// the presence of skipped MBs.
	captureWarnLogs(t, func() {
		without = sheetRowsFrom(t, &stubCostCalcDetailReader{rows: data})
		with = sheetRowsFrom(t, &stubCostCalcDetailReader{
			rows:    data,
			skipped: []string{"CSTMB2609000099", "CSTMB2609000100"},
		})
	})

	if len(with) != len(without) {
		t.Fatalf("row count changed: got %d rows with skip accounting, want %d", len(with), len(without))
	}
	if len(with) != 3 {
		t.Fatalf("expected header + 2 data rows, got %d", len(with))
	}
	for i := range without {
		if len(with[i]) != len(without[i]) {
			t.Fatalf("row %d column count changed: got %d, want %d", i, len(with[i]), len(without[i]))
		}
		for j := range without[i] {
			if with[i][j] != without[i][j] {
				t.Errorf("row %d col %d changed: got %q, want %q", i, j, with[i][j], without[i][j])
			}
		}
	}
	// Ordering is part of the contract: mb_code order must survive untouched.
	if with[1][0] != "CSTMB2607000001" || with[2][0] != "CSTMB2607000002" {
		t.Errorf("row ordering changed: got %q then %q", with[1][0], with[2][0])
	}
	// And a skipped MB must never leak INTO the sheet.
	for _, row := range with[1:] {
		if row[0] == "CSTMB2609000099" || row[0] == "CSTMB2609000100" {
			t.Errorf("a skipped MB must never be emitted as a data row, got %q", row[0])
		}
	}
}

// TestCostCalcDetail_SkipProbeFailureIsNotFatal pins that the observability read can
// never fail an export that already has its rows.
func TestCostCalcDetail_SkipProbeFailureIsNotFatal(t *testing.T) {
	reader := &stubCostCalcDetailReader{
		rows:       []mbhead.CostCalcDetailRow{referenceRow()},
		skippedErr: errors.New("boom"),
	}

	var content []byte
	out := captureWarnLogs(t, func() {
		h := mbhead.NewExportCostCalcDetailHandler(reader)
		var err error
		content, _, err = h.Handle(context.Background(), mbhead.ExportCostCalcDetailCommand{})
		if err != nil {
			t.Fatalf("a failed skip probe must NOT fail the export, got: %v", err)
		}
	})

	if len(content) == 0 {
		t.Error("the workbook must still be produced when the skip probe fails")
	}
	if !strings.Contains(out, "Could not determine skipped cost-calc-detail masterbatches") {
		t.Errorf("a failed skip probe must be logged, got log:\n%s", out)
	}
}

// TestCostCalcDetail_NoSkipsLogsNothing pins that the clean case stays quiet — the log
// line is a signal, not noise on every export.
func TestCostCalcDetail_NoSkipsLogsNothing(t *testing.T) {
	out := captureWarnLogs(t, func() {
		_ = workbookBytesFrom(t, &stubCostCalcDetailReader{
			rows: []mbhead.CostCalcDetailRow{referenceRow()},
		})
	})
	if strings.Contains(out, "skipped") {
		t.Errorf("no skipped MBs must produce no skip log, got:\n%s", out)
	}
}

// sheetRows opens the produced workbook and returns its rows as strings.
func sheetRows(t *testing.T, rows []mbhead.CostCalcDetailRow) [][]string {
	t.Helper()
	return sheetRowsFrom(t, &stubCostCalcDetailReader{rows: rows})
}

// sheetRowsFrom opens the workbook produced from a reader stub and returns its rows.
func sheetRowsFrom(t *testing.T, reader *stubCostCalcDetailReader) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(workbookBytesFrom(t, reader)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := f.GetRows("MB Cost Calc Detail")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	return got
}
