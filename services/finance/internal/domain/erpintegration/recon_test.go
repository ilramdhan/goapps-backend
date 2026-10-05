package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
)

func rnd(s string) decimal.NullDecimal { return decimal.NewNullDecimal(decimal.RequireFromString(s)) }

func rKey(item string) ErpKey { return ErpKey{ItemCode: item, GradeCode: "A", ShadeCode: "S1"} }

func rStd(item, std string) StdRow {
	return StdRow{Key: rKey(item), Status: DeriveOK, StdCost: rnd(std)}
}

func rCombo(item, rate string, items, stamped, variants int64) AdjReadBackCombo {
	return AdjReadBackCombo{Key: rKey(item), Items: items, Stamped: stamped, RateVariants: variants,
		MaxRate: rnd(rate), Flex13: "5"}
}

func TestClassifyRecon_Statuses(t *testing.T) {
	std := []StdRow{
		rStd("M1", "1.20000"), rStd("D1", "1.20000"), rStd("D2", "1.20000"), rStd("N1", "3"),
		{Key: rKey("X1"), Status: DeriveNoRule},
	}
	adj := []AdjReadBackCombo{
		rCombo("M1", "1.2000049", 2, 2, 1), // C-12: ROUND(rate,5) = std
		rCombo("D1", "1.2000050", 1, 1, 1), // rounds to 1.20001
		rCombo("D2", "1.2", 2, 1, 1),       // one item not stamped
		rCombo("Z9", "9", 4, 0, 1),         // not covered
		rCombo("X1", "1", 1, 0, 1),         // non-OK std row: not covered either
	}
	c := ClassifyRecon(std, adj, 5)
	want := map[string]ReconStatus{"M1": ReconMatch, "D1": ReconDiff, "D2": ReconDiff, "N1": ReconNotInAdj}
	if len(c.Rows) != len(want) {
		t.Fatalf("rows = %d, want %d (non-OK rows are not classified)", len(c.Rows), len(want))
	}
	for _, r := range c.Rows {
		if want[r.Key.ItemCode] != r.Status {
			t.Errorf("%s = %s, want %s", r.Key.ItemCode, r.Status, want[r.Key.ItemCode])
		}
	}
	if c.Counts.Match != 1 || c.Counts.Diff != 2 || c.Counts.NotInAdj != 1 ||
		c.Counts.NotCoveredCombos != 2 || c.Counts.NotCoveredItems != 5 {
		t.Fatalf("counts = %+v", c.Counts)
	}
	if c.AllMatch() {
		t.Fatal("AllMatch with diffs")
	}
	if ok := ClassifyRecon(std[:1], adj[:1], 5); !ok.AllMatch() {
		t.Fatalf("clean set not AllMatch: %+v", ok.Counts)
	}
}

func TestClassifyRecon_WrongBatchStampIsDiff(t *testing.T) {
	c := ClassifyRecon([]StdRow{rStd("M1", "1.2")}, []AdjReadBackCombo{rCombo("M1", "1.2", 1, 1, 1)}, 6)
	if c.Rows[0].Status != ReconDiff {
		t.Fatalf("status = %s", c.Rows[0].Status)
	}
}

func TestMergeAdjReadBack_TrimsAndFolds(t *testing.T) {
	a := rCombo("M1", "1.2", 1, 1, 1)
	b := rCombo("M1", "1.3", 2, 0, 1)
	b.Key.ItemCode = "M1  "
	b.QtyKg = rnd("2")
	got := MergeAdjReadBack([]AdjReadBackCombo{b, a, rCombo("A0", "1", 1, 1, 1)})
	if len(got) != 2 || got[0].Key.ItemCode != "A0" {
		t.Fatalf("merged = %+v", got)
	}
	m := got[1]
	if m.Items != 3 || m.Stamped != 1 || m.RateVariants != 2 || m.MaxRate.Decimal.String() != "1.3" || m.QtyKg.Decimal.String() != "2" {
		t.Fatalf("merged combo = %+v", m)
	}
}

func TestExpectedGsbStatusFor(t *testing.T) {
	cases := []struct {
		key, gsb      string
		done, notDone bool
	}{
		{CallKeyW2ValuateAdj, GsbValuated, true, false},
		{CallKeyW2ValuateAdj, GsbPushed, false, true},
		{CallKeyW2ValuateAdj, GsbFailed, false, false},
		{CallKeyW2ApproveAdj, GsbApproved, true, false},
		{CallKeyW2LockBatch, GsbLocked, true, false},
		{CallKeyW2LockBatch, GsbApproved, false, true},
		{CallKeyW2RestoreAdj, GsbFailed, true, false},
		{CallKeyW1InsertBatch, GsbPushed, false, false},
	}
	for _, tc := range cases {
		d, n := ExpectedGsbStatusFor(tc.key, tc.gsb)
		if d != tc.done || n != tc.notDone {
			t.Errorf("%s/%s = %v,%v want %v,%v", tc.key, tc.gsb, d, n, tc.done, tc.notDone)
		}
	}
}
