package mblegacyfill

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type fakeSnap struct {
	vals    map[string]string
	holders map[string][]string
}

func (f fakeSnap) Current(e Entity, id, col string) (string, bool) {
	v, ok := f.vals[string(e)+"|"+id+"|"+col]
	if !ok {
		_, ok = f.vals[string(e)+"|"+id]
	}
	return v, ok
}
func (f fakeSnap) Holders(col, v string) []string { return f.holders[col+"\x00"+v] }

type fakeReader struct{ s fakeSnap }

func (f fakeReader) Load(context.Context, []Row) (Snapshot, error) { return f.s, nil }

func plan(t *testing.T, csv string, s fakeSnap) *Plan {
	t.Helper()
	p, err := ParseProposal(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	pl, err := BuildPlan(context.Background(), fakeReader{s}, p)
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

func TestPlanDecisions(t *testing.T) {
	const hdr = "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note\n"
	s := fakeSnap{
		vals: map[string]string{
			"head|h1|mbh_status": "A", "head|h2|mbh_status": "B", "head|h3|mbh_status": "NEW",
			"head|h4|mbh_denier": "2.60", "head|h5|mbh_vs_number": "", "head|h6|mbh_vs_number": "",
			"head|h7|mbh_vs_number": "", "head|h8|mbh_dev_code": "", "head|h9|mbh_dev_code": "",
			"head|h10|mbh_code": "",
		},
		holders: map[string][]string{"mbh_code\x00C1": {"other"}},
	}
	csv := hdr +
		"head,h1,1,status,A,X,s,N,OVERWRITE,\n" + // apply
		"head,h2,1,status,A,X,s,N,OVERWRITE,\n" + // changed
		"head,h3,1,status,A,NEW,s,N,OVERWRITE,\n" + // already
		"head,gone,1,status,A,X,s,N,OVERWRITE,\n" + // missing
		"head,h4,1,den,2.6,2.6,s,N,OVERWRITE,\n" + // already numeric
		"head,h5,1,vs,,V9,s,N,FILL_EMPTY,\n" + // dup target
		"head,h6,1,vs,,V9,s,N,FILL_EMPTY,\n" + // dup target
		"head,h7,1,vs,,0,s,N,FILL_EMPTY,\n" + // exempt
		"head,h8,1,dev,,D1,s,N,FILL_EMPTY,\n" + // apply
		"head,h9,1,dev,,D2,s,N,FILL_EMPTY,\n" + // apply
		"head,h10,1,code,,C1,s,N,FILL_EMPTY,\n" + // live holder
		"head,h1,1,entry_status,A,X,s,N,OVERWRITE,\n" // class X
	pl := plan(t, csv, s)
	want := []Decision{
		DecisionApply, DecisionChanged, DecisionAlready, DecisionReject, DecisionAlready,
		DecisionReject, DecisionReject, DecisionApply, DecisionApply, DecisionApply, DecisionReject, DecisionReject,
	}
	for i, w := range want {
		if pl.Rows[i].Decision != w {
			t.Errorf("row %d (%s/%s): got %s (%s) want %s", i, pl.Rows[i].Row.ID, pl.Rows[i].Row.Field, pl.Rows[i].Decision, pl.Rows[i].Reason, w)
		}
	}
	if pl.Rows[10].Decision != DecisionReject || pl.Rows[10].Class != ClassD {
		t.Errorf("code clash: %+v", pl.Rows[10])
	}
	if pl.Rows[11].Class != ClassX {
		t.Errorf("entry_status class: %+v", pl.Rows[11])
	}
}

func TestPlanSelfHolderNotClash(t *testing.T) {
	const csv = "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note\n" +
		"head,h1,1,orion,,O1,s,N,FILL_EMPTY,\n"
	s := fakeSnap{vals: map[string]string{"head|h1|mbh_orion_item_code": ""}, holders: map[string][]string{"mbh_orion_item_code\x00O1": {"h1"}}}
	if pl := plan(t, csv, s); pl.Rows[0].Decision != DecisionApply {
		t.Fatalf("%+v", pl.Rows[0])
	}
}

func TestPlanWriters(t *testing.T) {
	const csv = "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note\n" +
		"head,h1,1,status,A,X,s,N,OVERWRITE,\n" +
		"head,h1,1,status,A,X,s,N,REVIEW,\n"
	pl := plan(t, csv, fakeSnap{vals: map[string]string{"head|h1|mbh_status": "A"}})
	var b bytes.Buffer
	if err := WritePlanCSV(&b, pl); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.SplitN(b.String(), "\n", 2)[0], "class,decision,reason") || !strings.Contains(b.String(), ",L,APPLY,") {
		t.Fatalf("plan csv: %s", b.String())
	}
	b.Reset()
	if err := WriteSummaryCSV(&b, pl); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"decision,APPLY,1", "skipped_usulan,REVIEW,1", "class_decision,L/APPLY,1"} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("summary missing %q:\n%s", w, b.String())
		}
	}
}
