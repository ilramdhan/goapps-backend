package mbcomparisonfill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func xrow(item, shade, sysid string, cells map[string]string) ExcelRow {
	r := ExcelRow{Row: 3, ItemCode: item, Shade: shade, SysID: sysid, Cells: cells}
	return r
}

var fullCells = map[string]string{"MB_SP_CODE": "CMB1", "MB_SP_DYE": "DYE", "MB_SP_DENIER": "380", "MB_SP_FILAMENT": "108",
	"MB_SP_CC": "TBL", "MB_SP_DOZING": "1.45", "CONV_FACTOR_ACTUAL": "1.6743", "RP_CC": "TBL", "RP_DOZING_ACTUAL": "1.6743"}

func sameValues() map[string]string {
	return map[string]string{"MB_SP_CODE": "CMB1", "MB_SP_DYE": "DYE", "MB_SP_DENIER": "380.000000", "MB_SP_FILAMENT": "108.000000",
		"MB_SP_CC": "TBL", "MB_SP_DOZING": "1.450000", "CONV_FACTOR_ACTUAL": "1.674300", "RP_CC": "TBL", "RP_DOZING_ACTUAL": "1.674300"}
}

func allCAPP() map[string]bool {
	m := map[string]bool{}
	for _, p := range Params {
		m[p.Code] = true
	}
	return m
}

func TestBuildPlanDecisions(t *testing.T) {
	prod := func(id int64, locked bool, vals map[string]string, capp map[string]bool) Product {
		return Product{SysID: id, ItemCode: "POY1", Shade: "Y-145 T", Locked: locked, Values: vals, HasCAPP: capp}
	}
	diff := sameValues()
	diff["MB_SP_DOZING"] = "1.390000"
	cases := []struct {
		name     string
		row      ExcelRow
		prods    []Product
		orion    map[string]string
		decision Decision
		reason   string
		after    map[string]string
		capp     int
	}{
		{"same", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, false, sameValues(), allCAPP())}, nil, DecisionSkipSame, "same", nil, 0},
		{"changed dozing", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, false, diff, allCAPP())}, nil, DecisionApply, "changed:MB_SP_DOZING", map[string]string{"MB_SP_DOZING": "1.45"}, 0},
		{"new product values + capp", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, false, map[string]string{}, map[string]bool{})}, nil, DecisionApply, "", nil, 9},
		{"capp only", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, false, sameValues(), map[string]bool{"RP_DOZING_ACTUAL": true})}, nil, DecisionApply, "capp_only", map[string]string{}, 8},
		{"locked", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, true, diff, allCAPP())}, nil, DecisionReport, "product_locked", nil, 0},
		{"not found", xrow("POY9", "Y145T", "1", fullCells), nil, nil, DecisionReport, "product_not_found", nil, 0},
		{"multi", xrow("POY1", "Y145T", "1", fullCells), []Product{prod(1, false, diff, allCAPP()), prod(2, false, diff, allCAPP())}, nil, DecisionReview, "multiple_products(2)", nil, 0},
		{"na", ExcelRow{Row: 3, ItemCode: "POY1", Shade: "Y145T", HasNA: true, Cells: map[string]string{}}, []Product{prod(1, false, diff, allCAPP())}, nil, DecisionReport, "excel_na", nil, 0},
		{"malformed", ExcelRow{Row: 3, ItemCode: "POY1", Shade: "Y145T", Malformed: "MB_SP_DOZING=x", Cells: map[string]string{}}, []Product{prod(1, false, diff, allCAPP())}, nil, DecisionReport, "excel_malformed: MB_SP_DOZING=x", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			products := map[string][]Product{}
			for _, p := range c.prods {
				products[ProductKey(p.ItemCode, p.Shade)] = append(products[ProductKey(p.ItemCode, p.Shade)], p)
			}
			plan := BuildPlan([]ExcelRow{c.row}, products, c.orion)
			require.Len(t, plan.Rows, 1)
			got := plan.Rows[0]
			assert.Equal(t, c.decision, got.Decision)
			if c.reason != "" {
				assert.Equal(t, c.reason, got.Reason)
			}
			if c.after != nil {
				assert.Equal(t, c.after, got.After)
			}
			assert.Len(t, got.CAPP, c.capp)
		})
	}
}

func TestShadeNormalization(t *testing.T) {
	assert.Equal(t, "Y145T", ShadeKey("y-145 t"))
	assert.Equal(t, ProductKey("poy1", "X-438T"), ProductKey(" POY1 ", "x438 t"))
}

func TestOrionFallback(t *testing.T) {
	cells := map[string]string{"MB_SP_CODE": "", "MB_SP_DOZING": "1"}
	p := Product{SysID: 1, ItemCode: "P", Shade: "S", Values: map[string]string{}, HasCAPP: map[string]bool{}}
	prods := map[string][]Product{ProductKey("P", "S"): {p}}

	plan := BuildPlan([]ExcelRow{xrow("P", "S", "20200700001", cells)}, prods, map[string]string{"20200700001": "ORI-1"})
	assert.Equal(t, "ORI-1", plan.Rows[0].After["MB_SP_CODE"])

	plan = BuildPlan([]ExcelRow{xrow("P", "S", "20200700001", cells)}, prods, map[string]string{})
	_, ok := plan.Rows[0].After["MB_SP_CODE"]
	assert.False(t, ok, "orion empty -> code stays unset")
}

func TestDuplicateExcelKeyIsReview(t *testing.T) {
	p := Product{SysID: 1, ItemCode: "P", Shade: "S", Values: map[string]string{}, HasCAPP: map[string]bool{}}
	prods := map[string][]Product{ProductKey("P", "S"): {p}}
	plan := BuildPlan([]ExcelRow{xrow("P", "S", "1", fullCells), xrow("P", "s", "2", fullCells)}, prods, nil)
	assert.Equal(t, DecisionReview, plan.Rows[0].Decision)
	assert.Equal(t, DecisionReview, plan.Rows[1].Decision)
}

func TestCPPWithoutCAPPCount(t *testing.T) {
	p := Product{SysID: 1, ItemCode: "P", Shade: "S", Values: map[string]string{"MB_SP_DOZING": "1"}, HasCAPP: map[string]bool{}}
	plan := BuildPlan([]ExcelRow{xrow("P", "S", "1", map[string]string{"MB_SP_DOZING": "1"})}, map[string][]Product{ProductKey("P", "S"): {p}}, nil)
	assert.Equal(t, 1, plan.CPPWithoutCAPP)
}

func TestParseRowAndNormNum(t *testing.T) {
	rec := make([]string, minCols)
	rec[colItem], rec[colShade], rec[colSPDoz], rec[colConv], rec[colRPDoz], rec[colSysID] = "POY1", "Y1", "1.45", "1.6742999999999999", "1.6743", "99"
	r := ParseRow(5, rec)
	assert.False(t, r.HasNA)
	assert.Equal(t, "1.6743", mustNum(t, r.Cells["CONV_FACTOR_ACTUAL"]))
	rec[colRPDoz] = "#N/A"
	assert.True(t, ParseRow(5, rec).HasNA)
	rec[colRPDoz] = "1"
	rec = append(rec, "#N/A", "#N/A") // check/diff columns
	assert.True(t, ParseRow(5, rec).HasNA)
	rec = rec[:minCols]
	rec[colRPDoz] = "abc"
	assert.NotEmpty(t, ParseRow(5, rec).Malformed)
}

func mustNum(t *testing.T, s string) string {
	t.Helper()
	v, ok := NormNum(s)
	require.True(t, ok)
	return v
}

func TestPlanCSVRoundTripAndOrion(t *testing.T) {
	dir := t.TempDir()
	plan := &Plan{Rows: []PlanRow{{ExcelRow: 3, ItemCode: "P", Shade: "S", ProductID: 7, Decision: DecisionApply, Reason: "changed:X",
		Before: map[string]string{"MB_SP_DOZING": ""}, After: map[string]string{"MB_SP_DOZING": "1"}, CAPP: []string{"MB_SP_DOZING"}}}}
	var sb strings.Builder
	require.NoError(t, WritePlanCSV(&sb, plan))
	f := filepath.Join(dir, "p.csv")
	require.NoError(t, os.WriteFile(f, []byte(sb.String()), 0o600))
	got, err := ReadPlanCSV(f)
	require.NoError(t, err)
	assert.Equal(t, plan.Rows, got)

	o := filepath.Join(dir, "o.csv")
	require.NoError(t, os.WriteFile(o, []byte("CMBH_SYS_ID,CMBH_ORION_ITEM_CODE\n1,AAA\n2,\n"), 0o600))
	m, err := ParseOrion(o)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"1": "AAA"}, m)
}

type fakeWriter struct {
	pre    error
	called int
}

func (f *fakeWriter) Preflight(context.Context) error { return f.pre }
func (f *fakeWriter) ApplyBatch(_ context.Context, _ string, rows []PlanRow, _ bool) ([]RowResult, error) {
	f.called++
	out := make([]RowResult, len(rows))
	for i, r := range rows {
		out[i] = RowResult{PlanRow: r, Result: ResultApplied}
	}
	return out, nil
}
func (f *fakeWriter) Revert(_ context.Context, _ string, r RowResult) (RowResult, error) {
	r.Result = ResultReverted
	return r, nil
}

func TestApplyLimitBatchPreflightActor(t *testing.T) {
	rows := []PlanRow{{Decision: DecisionApply}, {Decision: DecisionSkipSame}, {Decision: DecisionApply}, {Decision: DecisionApply}}
	w := &fakeWriter{}
	res, err := Apply(context.Background(), w, "mbcmp-local", rows, 2, 1, true)
	require.NoError(t, err)
	assert.Len(t, res, 2)
	assert.Equal(t, 2, w.called)

	_, err = Apply(context.Background(), &fakeWriter{pre: assert.AnError}, "mbcmp-local", rows, 0, 0, true)
	require.ErrorIs(t, err, assert.AnError)
	_, err = Apply(context.Background(), w, "bob", rows, 0, 0, true)
	require.Error(t, err)

	rev, err := Revert(context.Background(), w, "mbcmp-local", []RowResult{{Result: ResultApplied}, {Result: ResultWouldApply}})
	require.NoError(t, err)
	assert.Len(t, rev, 1)
}
