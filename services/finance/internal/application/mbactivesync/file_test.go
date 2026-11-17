package mbactivesync

import (
	"errors"
	"os"
	"testing"
)

const origHeader = "\xEF\xBB\xBFItem Code (POY);Name;Shade Code;Shade Name;63.MB/SP Code;64.MB/SP Dye Name;71.RP-Doz;SYSID OF MB\r\n"

func TestParseOriginal(t *testing.T) {
	in := origHeader +
		"POY0000283;POY 380/144/TBL/DBR/SIM/NS/1/O;Y145T;THRUSH YELLOW BR;CMB0000084;MGT THRUST YELLOW TR-1359-B; 1,39 ;20200701379\r\n" +
		"POY0000283;POY 380/144/TBL/DBR/SIM/NS/1/O;X438T;FIRDAUS GREEN BR;;MGT FIRDAUS;1,67;20200701459\r\n" +
		"POY0000283;POY 380/144/TBL/DBR/SIM/NS/1/O;X438T;FIRDAUS GREEN BR;;MGT FIRDAUS;1,67;20200701459\r\n" +
		"\r\n"
	rows, err := ParseBytes([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3 (duplicates are kept)", len(rows))
	}
	r := rows[0]
	if r.Ln != 2 || r.POY != "POY0000283" || r.Shade != "Y145T" || r.MBCode != "CMB0000084" || r.SysID != "20200701379" {
		t.Errorf("row0=%+v", r)
	}
	if !r.DozOK || r.Doz != 1.39 || r.DozRaw != "1,39" {
		t.Errorf("doz=%v %v %q", r.Doz, r.DozOK, r.DozRaw)
	}
	if r.PType != "POY" || r.Den == nil || *r.Den != 380 || r.Fil == nil || *r.Fil != 144 || r.CS != "TBL" {
		t.Errorf("spec=%+v", r)
	}
	if rows[1].MBCode != "" {
		t.Errorf("blank MB code must be empty, got %q", rows[1].MBCode)
	}
	if rows[1].Ln != 3 || rows[2].Ln != 4 {
		t.Errorf("ln=%d,%d", rows[1].Ln, rows[2].Ln)
	}
}

func TestParseNormalized(t *testing.T) {
	in := "2,POY0000283,\"POY 380/144/TBL/DBR\",Y145T,THRUSH YELLOW BR,CMB0000084,\"MGT A, B\",\"1,39\",20200701379\n" +
		"3,POY0000283,POY 380/144/TBL,Y145T,X,,MGT,\"0,57\",20200701380\n"
	rows, err := ParseBytes([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Ln != 2 || rows[0].DyeName != "MGT A, B" || rows[0].Doz != 1.39 || rows[1].MBCode != "" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestParseSpecUnparsed(t *testing.T) {
	in := "1,POY1,WEIRD NAME,S,N,,D,\"1,0\",1\n2,POY1,DTY abc/xx,S,N,,D,\"1,0\",1\n"
	rows, err := ParseBytes([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Den != nil || r.Fil != nil {
			t.Errorf("expected nil den/fil: %+v", r)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for name, in := range map[string]string{
		"empty":     "",
		"bad cols":  "1,a,b\n",
		"bad ln":    "x,p,n,s,sn,m,d,\"1,0\",1\n",
		"bad doz":   "1,p,n,s,sn,m,d,abc,1\n",
		"no sysid":  "1,p,n,s,sn,m,d,\"1,0\",\n",
		"bad hdr":   "A;B\r\n",
		"orig cols": origHeader + "a;b;c\r\n",
	} {
		if _, err := ParseBytes([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseBlankDoz(t *testing.T) {
	rows, err := ParseBytes([]byte("1,p,n,s,sn,m,d,,1\n"))
	if err != nil || rows[0].DozOK {
		t.Fatalf("err=%v rows=%+v", err, rows)
	}
}

const (
	realNormalized = "../../../../../../docs/mb-active-sync/mb-active-normalized.csv"
	realOriginal   = "../../../../../../docs/mb-spin-fg/mb-active.csv"
)

func loadReal(t *testing.T, path string) []FileRow {
	t.Helper()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s not present", path)
	}
	rows, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRealNormalizedFile(t *testing.T) {
	rows := loadReal(t, realNormalized)
	if len(rows) != 4097 || DistinctSysIDs(rows) != 2464 {
		t.Fatalf("rows=%d sysids=%d want 4097/2464", len(rows), DistinctSysIDs(rows))
	}
}

func TestRealOriginalMatchesNormalized(t *testing.T) {
	a := loadReal(t, realNormalized)
	b := loadReal(t, realOriginal)
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.POY != y.POY || x.Shade != y.Shade || x.MBCode != y.MBCode || x.SysID != y.SysID || x.Doz != y.Doz {
			t.Fatalf("row %d differs: %+v vs %+v", i, x, y)
		}
	}
}
