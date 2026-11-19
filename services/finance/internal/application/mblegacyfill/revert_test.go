package mblegacyfill

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseResultCSVAndShadeCount(t *testing.T) {
	var b bytes.Buffer
	rows := []RowResult{
		{Row: Row{Entity: EntityHead, ID: "h1", Field: "shade_code"}, Result: ResultApplied, BeforeJSON: `{"mbh_shade_code":""}`, AfterJSON: `{"mbh_shade_code":"AX-7","shade_master_created":"AX-7"}`},
		{Row: Row{Entity: EntitySpin, ID: "s1", Field: "shade_code"}, Result: ResultApplied, AfterJSON: `{"shade_master_created":"AX7"}`},
		{Row: Row{Entity: EntityHead, ID: "h2", Field: "notes"}, Result: ResultSkipped},
	}
	if err := WriteResultCSV(&b, rows); err != nil {
		t.Fatal(err)
	}
	got, err := ParseResultCSV(3, strings.NewReader(b.String()))
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %d", err, len(got))
	}
	if got[0].File != 3 || got[0].Line != 2 || got[0].Entity != EntityHead || got[0].BeforeJSON == "" {
		t.Fatalf("%+v", got[0])
	}
	if n := CountShadeMastersCreated(got); n != 1 {
		t.Fatalf("want 1 distinct nk, got %d", n)
	}
	if _, err := ParseResultCSV(0, strings.NewReader("a,b\n")); err == nil {
		t.Fatal("expected missing column error")
	}
}

func TestColumnKind(t *testing.T) {
	if k, ok := ColumnKind(EntitySpin, "mbs_ldr_is_actual"); !ok || k != KindBool {
		t.Fatal("spin extra")
	}
	if _, ok := ColumnKind(EntityHead, "mbh_entry_status"); ok {
		t.Fatal("must not be revertable")
	}
	if k, ok := ColumnKind(EntityHead, "mbh_denier"); !ok || k != KindNum {
		t.Fatal("registry col")
	}
}
