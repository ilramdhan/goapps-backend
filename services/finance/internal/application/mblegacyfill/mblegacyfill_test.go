package mblegacyfill

import (
	"os"
	"strings"
	"testing"
)

const fixture = `entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note
head,h1,1,no_of_process,,S,sysid,N,FILL_EMPTY,
head,h1,1,code,A,B,sysid,N,OVERWRITE,
head,h2,2,entry_status,X,Y,sysid,N,OVERWRITE_STATUS,
head,h2,2,bogus,,Y,sysid,N,FILL_EMPTY,
spin,s1,3,run_ldr,,5.0,sysid,N,FILL_EMPTY,"also set mbs_ldr_type='ACTUAL', mbs_ldr_is_actual=TRUE"
spin,s1,3,code,,5,sysid,N,FILL_EMPTY,
head,h3,4,status,A,B,sysid,N,REVIEW,
head,h3,4,status,A,B,sysid,N,REVIEW_NEW_SHADE,
head,h3,4,status,A,B,sysid,N,SKIP_CSV_OWNED,
head,h3,4,entry_status,A,B,sysid,N,REVIEW,
`

func TestParseProposal(t *testing.T) {
	p, err := ParseProposal(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 10 || len(p.Rows) != 3 || len(p.Rejected) != 3 || p.SkippedTotal() != 4 {
		t.Fatalf("total=%d rows=%d rej=%d skipped=%v", p.Total, len(p.Rows), len(p.Rejected), p.Skipped)
	}
	if p.Skipped["REVIEW"] != 2 || p.Skipped["REVIEW_NEW_SHADE"] != 1 || p.Skipped["SKIP_CSV_OWNED"] != 1 {
		t.Fatalf("skipped=%v", p.Skipped)
	}
	if p.Rows[1].Spec.Column != "mbh_code" || p.Rows[1].Spec.Class != ClassD {
		t.Fatalf("spec=%+v", p.Rows[1].Spec)
	}
	if p.Rows[2].Spec.Column != "mbs_run_ldr_pct" || !strings.Contains(p.Rows[2].Note, "ACTUAL") {
		t.Fatalf("row=%+v", p.Rows[2])
	}
}

func TestParseProposalBadHeader(t *testing.T) {
	if _, err := ParseProposal(strings.NewReader("a,b,c\n")); err == nil {
		t.Fatal("want error")
	}
	bad := strings.Replace(fixture, "usulan", "usul", 1)
	if _, err := ParseProposal(strings.NewReader(bad)); err == nil {
		t.Fatal("want error")
	}
}

func TestLookup(t *testing.T) {
	cases := []struct {
		e  Entity
		f  string
		ok bool
		c  Class
	}{
		{EntityHead, "code", true, ClassD}, {EntitySpin, "code", false, ""},
		{EntityHead, "entry_status", false, ""}, {EntityHead, "param_x", false, ""},
		{EntitySpin, "cc", true, ClassL}, {EntityHead, "cc", false, ""},
		{EntityHead, "shade_name", true, ClassS}, {EntityHead, "fil", true, ClassD},
		{EntityHead, "status", true, ClassL},
	}
	for _, c := range cases {
		s, ok := Lookup(c.e, c.f)
		if ok != c.ok || (ok && s.Class != c.c) {
			t.Errorf("%s/%s: ok=%v class=%v", c.e, c.f, ok, s.Class)
		}
	}
	if s, _ := Lookup(EntityHead, "fil"); s.Kind != KindInt || s.Column != "mbh_filament" {
		t.Errorf("fil spec %+v", s)
	}
}

func TestEqual(t *testing.T) {
	cases := []struct {
		k    Kind
		a, b string
		want bool
	}{
		{KindNum, "2.6", "2.60", true}, {KindNum, " 2.6 ", "2.61", false},
		{KindInt, "36", "36.0", true}, {KindText, " A ", "A", true},
		{KindText, "a", "A", false}, {KindNum, "", "  ", true},
		{KindNum, "", "0", false}, {KindNum, "x", "x", true},
	}
	for _, c := range cases {
		if got := Equal(c.k, c.a, c.b); got != c.want {
			t.Errorf("Equal(%s,%q,%q)=%v", c.k, c.a, c.b, got)
		}
	}
}

func TestRealProposalSanity(t *testing.T) {
	path := "../../../../../../docs/mb-active-sync/oracle-update-proposal.csv"
	if _, err := os.Stat(path); err != nil {
		t.Skip("proposal file missing")
	}
	p, err := ParseProposalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 8442 {
		t.Fatalf("total=%d", p.Total)
	}
	if got := len(p.Rows) + len(p.Rejected) + p.SkippedTotal(); got != p.Total {
		t.Fatalf("sum=%d", got)
	}
	if p.Skipped["REVIEW"] != 478 || p.Skipped["SKIP_CSV_OWNED"] != 131 || p.Skipped["REVIEW_NEW_SHADE"] != 31 {
		t.Fatalf("skipped=%v", p.Skipped)
	}
	t.Logf("rows=%d rejected=%d", len(p.Rows), len(p.Rejected))
}
