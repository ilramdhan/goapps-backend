package oracle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

const (
	gsbTable = "MGTDAT.CST_GOAPPS_STD_BATCH"
	gscTable = "MGTDAT.CST_GOAPPS_STD_COST"
)

var (
	gsbColumns = []string{"GSB_BATCH_ID", "GSB_PERIOD", "GSB_SEQ", "GSB_STATUS", "GSB_RULE_HASH",
		"GSB_ROW_COUNT", "GSB_SUM_STD", "GSB_SUM_CONV", "GSB_SUM_PVL"}
	gscColumns = []string{"ITEM", "GRADE", "SHADE", "SOURCE", "BASIS", "FG", "CHP", "AXCONV", "CONV",
		"SELL", "VLOSS", "AX", "STD", "PVL"}
	reconAdjColumns = []string{"ITEM", "GRADE", "SHADE", "ITEMS", "VARIANTS", "MAX_RATE", "QTY_KG", "VAL", "FLEX_13", "STAMPED"}
)

func TestErpReconReader_ReadBackBatch(t *testing.T) {
	q := fakeoracle.New()
	q.Register(gsbTable, fakeoracle.Dataset{Columns: gsbColumns, Rows: [][]string{
		{"7", "202608", "2", "VALUATED", "abc", "2", "3.70000", "0.5", ""},
	}})
	q.Register(gscTable, fakeoracle.Dataset{Columns: gscColumns, Rows: [][]string{
		{"POY001 ", "A", "S1", "GOAPPS_AX", "AX", "FDY", "", "", "0.5", "", "", "", "1.2", ""},
		{"POY002", "A", "S1", "GOAPPS_DERIVED", "SPPTY", "", "", "", "", "", "", "", "2.5", "0.1"},
	}})
	r := oracle.NewErpReconReader(q)
	got, err := r.ReadBackBatch(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, q)
	if !got.Found || got.Period != "202608" || got.Seq != 2 || got.Status != erpintegration.GsbValuated || got.RuleHash != "abc" {
		t.Fatalf("header = %+v", got)
	}
	if got.Header.RowCount() != 2 || got.Header.SumStd().String() != "3.7" || !got.Header.SumPvl().IsZero() {
		t.Fatalf("totals = %d %s %s", got.Header.RowCount(), got.Header.SumStd(), got.Header.SumPvl())
	}
	if len(got.CostRows) != 2 {
		t.Fatalf("cost rows = %d", len(got.CostRows))
	}
	a := got.CostRows[0]
	if a.Key.ItemCode != "POY001" || a.Source != erpintegration.SourceAX || a.Status != erpintegration.DeriveOK ||
		!a.StdCost.Valid || a.StdCost.Decimal.String() != "1.2" || a.ChpCost.Valid {
		t.Fatalf("cost row 0 = %+v", a)
	}
}

func TestErpReconReader_BatchNotFound(t *testing.T) {
	q := fakeoracle.New()
	q.Register(gsbTable, fakeoracle.Dataset{Columns: gsbColumns})
	got, err := oracle.NewErpReconReader(q).ReadBackBatch(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if got.Found || len(q.Queries()) != 1 {
		t.Fatalf("found=%v queries=%d (no cost read without a header)", got.Found, len(q.Queries()))
	}
}

func TestErpReconReader_ReadBackAdj(t *testing.T) {
	q := fakeoracle.New()
	q.Register(adjHeadTable, fakeoracle.Dataset{Columns: reconAdjColumns, Rows: [][]string{
		{"POY001", "A", "S1", "3", "1", "1.2", "4.5", "5.4", "7", "3"},
		{"POY009", "B", "S2", "1", "0", "", "", "", "", "0"},
	}})
	rec := &recorder{Querier: q}
	got, err := oracle.NewErpReconReader(rec).ReadBackAdj(context.Background(), "202608", 7)
	if err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, q)
	if len(rec.args) != 1 || len(rec.args[0]) != 3 || rec.args[0][0] != "7" || rec.args[0][1] != "202608" {
		t.Fatalf("binds = %v", rec.args)
	}
	if len(got) != 2 || got[0].Items != 3 || got[0].Stamped != 3 || got[0].RateVariants != 1 ||
		got[0].MaxRate.Decimal.String() != "1.2" || got[0].Flex13 != "7" {
		t.Fatalf("combo 0 = %+v", got)
	}
	if got[1].MaxRate.Valid || got[1].QtyKg.Valid {
		t.Fatalf("combo 1 NULL aggregates = %+v", got[1])
	}
}

func TestErpReconReader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := oracle.NewErpReconReader(nil).ReadBackBatch(ctx, 1); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Fatalf("nil querier: %v", err)
	}
	if _, err := oracle.NewErpReconReader(fakeoracle.New()).ReadBackAdj(ctx, "2026-08", 1); err == nil {
		t.Fatal("bad period accepted")
	}
	q := fakeoracle.New()
	q.FailWith(errors.New("ora down"))
	if _, err := oracle.NewErpReconReader(q).ReadBackAdj(ctx, "202608", 1); err == nil {
		t.Fatal("query error swallowed")
	}
}
