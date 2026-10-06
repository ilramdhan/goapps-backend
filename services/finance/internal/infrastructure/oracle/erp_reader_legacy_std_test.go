package oracle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

const (
	fixtureLegacyStd = "../../testutil/fakeoracle/testdata/erp/legacy_std_sample.csv"
	legacyStdTable   = "MGTDAT.OT_STD_COST_PRODUCTS_MGT"
)

func TestLegacyStdReader_List_AllRows(t *testing.T) {
	rec := newRecorder(t, legacyStdTable, fixtureLegacyStd)
	rows, err := oracle.NewLegacyStdReader(rec).List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, rec.Querier)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (blank item skipped): %+v", len(rows), rows)
	}
	want0 := erpintegration.LegacyStdRow{
		ItemCode: "FGX0001", GradeCode: "AX", ShadeCode: "sh01", ItemType: "FGX", FgType: "DTY",
		ChpItemCode: "CHPX001", MsBatchItem: "MB0001", PrdPerDay: "1250.5",
	}
	if rows[0] != want0 {
		t.Errorf("row0 = %+v, want %+v", rows[0], want0)
	}
	if r := rows[2]; r.ItemCode != "FGX0002" || r.GradeCode != "AX" || r.ShadeCode != "" || r.FgType != "FDY" || r.PrdPerDay != "" {
		t.Errorf("row2 not trimmed/NULL-mapped: %+v", r)
	}
	if len(rec.args) != 1 || len(rec.args[0]) != 0 {
		t.Errorf("whole-table read must not bind args: %v", rec.args)
	}
	q := rec.Queries()[0]
	if strings.Contains(q, "FG_ITEM_CR_DT") {
		t.Errorf("unexpected period filter: %s", q)
	}
	if !strings.Contains(q, "CAST(NULL AS VARCHAR2(64)) FG_PRD_PER_DAY") {
		t.Errorf("FG_PRD_PER_DAY must not be read by default: %s", q)
	}
	if !strings.Contains(q, "TO_CHAR(FG_MS_BATCH_ITEM)") {
		t.Errorf("MS batch must be read as text: %s", q)
	}
}

func TestLegacyStdReader_List_PeriodAndPrdFlag(t *testing.T) {
	rec := newRecorder(t, legacyStdTable, fixtureLegacyStd)
	r := oracle.NewLegacyStdReader(rec).WithPrdPerDay(true)
	if _, err := r.List(context.Background(), " 202608 "); err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, rec.Querier)
	if len(rec.args) != 1 || len(rec.args[0]) != 1 || rec.args[0][0] != "202608" {
		t.Errorf("period bind = %v, want [202608]", rec.args)
	}
	q := rec.Queries()[0]
	if !strings.Contains(q, "ADD_MONTHS(TO_DATE(:1, 'YYYYMM'), 1)") {
		t.Errorf("missing period filter: %s", q)
	}
	if !strings.Contains(q, "TO_CHAR(FG_PRD_PER_DAY, 'TM9'") {
		t.Errorf("FG_PRD_PER_DAY not read with flag: %s", q)
	}
}

func TestLegacyStdReader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := oracle.NewLegacyStdReader(nil).List(ctx, ""); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Errorf("nil querier err = %v", err)
	}
	var nilReader *oracle.LegacyStdReader
	if _, err := nilReader.List(ctx, ""); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Errorf("nil reader err = %v", err)
	}
	q := fakeoracle.New()
	if _, err := oracle.NewLegacyStdReader(q).List(ctx, "2026-08"); err == nil {
		t.Error("invalid period accepted")
	}
	if len(q.Queries()) != 0 {
		t.Error("invalid period must not query")
	}
	boom := errors.New("boom")
	q.FailWith(boom)
	if _, err := oracle.NewLegacyStdReader(q).List(ctx, ""); !errors.Is(err, boom) {
		t.Errorf("query failure err = %v", err)
	}
	bad := fakeoracle.New()
	bad.Register(legacyStdTable, fakeoracle.Dataset{
		Columns: []string{"FG_ITEM_CODE", "FG_ITEM_GRADE", "FG_ITEM_SHADE", "FG_ITEM_TYPE", "FG_TYPE", "FG_CHP_ITEM_CODE", "FG_MS_BATCH_ITEM", "FG_PRD_PER_DAY"},
		Rows:    [][]string{{"FGX0001", "AX", "S", "FGX", "DTY", "", "", "not-a-number"}},
	})
	if _, err := oracle.NewLegacyStdReader(bad).WithPrdPerDay(true).List(ctx, ""); err == nil {
		t.Error("bad decimal accepted")
	}
}
