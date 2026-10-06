package oracle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

var adjSnapshotColumns = []string{
	"ADJH_SYS_ID", "ADJI_SYS_ID", "ADJH_TXN_CODE", "ADJH_APPR_STATUS", "ADJH_POST_STATUS",
	"ADJI_ITEM_CODE", "ADJI_GRADE_CODE_1", "ADJI_GRADE_CODE_2", "ADJI_QTY_BU", "ADJI_ITEM_DESC",
	"ADJI_RATE", "ADJI_VAL",
	"F01", "F02", "F03", "F04", "F05", "F06", "F07", "F08", "F09", "F10", "F11", "F12", "F13", "F14",
}

func snapshotRow(head, item, txn, appr, post, qty, rate, val, f1 string) []string {
	r := []string{head, item, txn, appr, post, "ITM" + item, "G1", "S1", qty, "desc " + item, rate, val, f1}
	for len(r) < len(adjSnapshotColumns) {
		r = append(r, "")
	}
	return r
}

func newSnapshotFake(rows ...[]string) *fakeoracle.Querier {
	q := fakeoracle.New()
	// Registered on one table only: fakeoracle matches the first dataset whose
	// table the query references, and the query joins head and item.
	q.Register(adjHeadTable, fakeoracle.Dataset{Columns: adjSnapshotColumns, Rows: rows})
	return q
}

func TestErpAdjSnapshotReader_ScansRowsAndNulls(t *testing.T) {
	q := newSnapshotFake(
		snapshotRow("10", "101", "INVADJ", "", "", "2500", "1.25", "3.125", "0.5"),
		snapshotRow("11", "111", "MBINVADJ", "3", "P", "", "", "", ""),
	)
	r := oracle.NewErpAdjSnapshotReader(q)
	got, err := r.SnapshotAdjRows(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, q)
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2", len(got))
	}
	a := got[0]
	if a.HeadSysID != 10 || a.ItemSysID != 101 || a.TxnCode != "INVADJ" || a.ItemCode != "ITM101" ||
		a.GradeCode != "G1" || a.ShadeCode != "S1" || a.ItemDesc != "desc 101" {
		t.Fatalf("row 0 identity = %+v", a)
	}
	if a.HeadApprStatus != nil || a.HeadPostStatus != nil {
		t.Fatalf("row 0 status should be NULL: %+v", a)
	}
	if !a.QtyBu.Valid || !a.QtyBu.Decimal.Equal(dec(t, "2500")) ||
		!a.Rate.Valid || !a.Rate.Decimal.Equal(dec(t, "1.25")) ||
		!a.Val.Valid || !a.Val.Decimal.Equal(dec(t, "3.125")) {
		t.Fatalf("row 0 numbers = %+v", a)
	}
	if a.Flex[0] == nil || *a.Flex[0] != "0.5" || a.Flex[1] != nil {
		t.Fatalf("row 0 flex = %v", a.Flex)
	}
	if !a.Eligible() {
		t.Fatal("row 0 must be eligible")
	}
	b := got[1]
	if b.HeadApprStatus == nil || *b.HeadApprStatus != 3 || b.HeadPostStatus == nil || *b.HeadPostStatus != "P" {
		t.Fatalf("row 1 status = %+v", b)
	}
	if b.QtyBu.Valid || b.Rate.Valid || b.Val.Valid {
		t.Fatalf("row 1 numbers should be NULL: %+v", b)
	}
	if b.Eligible() {
		t.Fatal("row 1 (posted, approved) must not be eligible")
	}
	if !strings.Contains(q.Queries()[0], "ORDER BY") {
		t.Fatal("snapshot query must be deterministically ordered")
	}
}

func TestErpAdjSnapshotReader_BindsPeriod(t *testing.T) {
	rec := &recorder{Querier: newSnapshotFake()}
	r := oracle.NewErpAdjSnapshotReader(rec)
	got, err := r.SnapshotAdjRows(context.Background(), "202608")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(rec.args) != 1 || len(rec.args[0]) != 2 || rec.args[0][0] != "202608" || rec.args[0][1] != "202608" {
		t.Fatalf("binds = %v", rec.args)
	}
}

func TestErpAdjSnapshotReader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := oracle.NewErpAdjSnapshotReader(newSnapshotFake()).SnapshotAdjRows(ctx, "2026-08"); err == nil {
		t.Fatal("bad period must fail")
	}
	if _, err := oracle.NewErpAdjSnapshotReader(nil).SnapshotAdjRows(ctx, "202608"); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Fatalf("nil querier err = %v", err)
	}
	boom := errors.New("boom")
	q := newSnapshotFake()
	q.FailWith(boom)
	if _, err := oracle.NewErpAdjSnapshotReader(q).SnapshotAdjRows(ctx, "202608"); !errors.Is(err, boom) {
		t.Fatalf("query err = %v", err)
	}
	bad := newSnapshotFake(snapshotRow("10", "101", "INVADJ", "", "", "x1", "", "", ""))
	if _, err := oracle.NewErpAdjSnapshotReader(bad).SnapshotAdjRows(ctx, "202608"); err == nil {
		t.Fatal("bad decimal must fail")
	}
	nullID := newSnapshotFake(snapshotRow("", "101", "INVADJ", "", "", "", "", "", ""))
	if _, err := oracle.NewErpAdjSnapshotReader(nullID).SnapshotAdjRows(ctx, "202608"); err == nil {
		t.Fatal("NULL head id must fail")
	}
}
