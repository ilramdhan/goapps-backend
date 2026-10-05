package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

// fakeRow builds a synthetic result row keyed by column name (others empty).
func fakeRow(vals map[string]string) []string {
	r := make([]string, len(columns))
	for i, c := range columns {
		r[i] = vals[c.name]
	}
	return r
}

func newFake(t *testing.T, user string, rows ...[]string) *fakeoracle.Querier {
	t.Helper()
	q := fakeoracle.New()
	q.Register("DUAL", fakeoracle.Dataset{Columns: []string{"USER"}, Rows: [][]string{{user}}})
	q.Register("OT_STD_COST_PRODUCTS_MGT", fakeoracle.Dataset{Columns: Header(), Rows: rows})
	return q
}

var (
	rowCost = fakeRow(map[string]string{
		"item_code": "PTY0000001", "grade_code": "BX", "shade_code": "SH01", "fg_type": "DTY",
		"prod_type": "PTY", "grade_group": "B", "rule_basis": "COST", "rule_val_loss": ".1",
		"ax_chp_cost": "1.2", "ax_chp_con_kg": "1", "ax_conv": ".654321",
		"legacy_std": "1.75432", "flex_std": "1.75432",
	})
	rowSP = fakeRow(map[string]string{
		"item_code": "POY0000002", "grade_code": "CX", "shade_code": "NL", "fg_type": "FDY",
		"prod_type": "POY", "grade_group": "C", "rule_basis": "SPPTY", "rule_val_loss": "0.25",
		"rule_sell_price_raw": "1.5", "legacy_std": "1.25", "legacy_pvl": "-0.0000012345",
	})
	rowNoRule = fakeRow(map[string]string{
		"item_code": "ACY0000003", "grade_code": "ZZ", "shade_code": "NL", "legacy_std": "0",
	})
)

func TestBuildQuery_IsReadOnlyAndShaped(t *testing.T) {
	q, err := BuildQuery(DefaultSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := oracle.CheckReadOnly(q); err != nil {
		t.Fatalf("query not read-only: %v", err)
	}
	for _, want := range []string{
		"FROM MGTDAT.OT_STD_COST_PRODUCTS_MGT der",
		"JOIN MGTDAT.OT_STD_COST_PRODUCTS_MGT ax",
		"ax.FG_ITEM_GRADE = 'AX'",
		"LEFT JOIN MGTDAT.OM_GRADE_CODE_1 g",
		"LEFT JOIN MGTDAT.MGT_ITEM_COST_VAL_LOSS m",
		"LEFT JOIN MGTDAT.IM_VS_STATIC_VALUE sp",
		"WHERE der.FG_ITEM_GRADE <> 'AX'",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q", want)
		}
	}
	for _, bad := range []string{"FOR UPDATE", ";", "FLOAT", "BINARY_DOUBLE"} {
		if strings.Contains(strings.ToUpper(q), bad) {
			t.Errorf("query contains %q", bad)
		}
	}
	if _, err := BuildQuery("MGTDAT; DROP"); !errors.Is(err, ErrBadSchema) {
		t.Fatalf("bad schema accepted: %v", err)
	}
	uq, err := BuildQuery("")
	if err != nil || strings.Contains(uq, "MGTDAT.") {
		t.Fatalf("unqualified query: %v", err)
	}
}

func TestHeader_StableOrder(t *testing.T) {
	h := Header()
	if h[0] != "item_code" || h[1] != "grade_code" || h[2] != "shade_code" || len(h) != 37 {
		t.Fatalf("unexpected header %v", h)
	}
	seen := map[string]bool{}
	for _, n := range h {
		if seen[n] {
			t.Fatalf("duplicate column %s", n)
		}
		seen[n] = true
	}
}

func TestDSNUserAndCheckUser(t *testing.T) {
	u, err := DSNUser("oracle://ro_etl:secret@dev-host:1521/ALTHARADEV")
	if err != nil || u != "RO_ETL" {
		t.Fatalf("DSNUser = %q, %v", u, err)
	}
	for _, bad := range []string{"", "postgres://a:b@h/db", "oracle://h:1521/svc", "::"} {
		if _, err := DSNUser(bad); err == nil {
			t.Errorf("DSNUser(%q) accepted", bad)
		}
	}
	if err := CheckUser("RO_ETL", " ro_etl , other"); err != nil {
		t.Fatalf("allowed user refused: %v", err)
	}
	for _, tc := range []struct{ user, list string }{
		{"RO_ETL", ""},
		{"RO_ETL", "OTHER"},
		{"MGTDAT", "MGTDAT"},
		{"GOAPPS_IF", "goapps_if"},
		{"SYSTEM", "SYSTEM"},
	} {
		if err := CheckUser(tc.user, tc.list); !errors.Is(err, ErrUserNotAllowed) {
			t.Errorf("CheckUser(%q,%q) = %v, want ErrUserNotAllowed", tc.user, tc.list, err)
		}
	}
}

func TestVerifyIdentity(t *testing.T) {
	ctx := context.Background()
	if err := VerifyIdentity(ctx, newFake(t, "RO_ETL"), "RO_ETL"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIdentity(ctx, newFake(t, "MGTDAT"), "RO_ETL"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("mismatch not detected: %v", err)
	}
}

func TestFormatNum(t *testing.T) {
	for in, want := range map[string]string{
		"0": "0.00000", ".1": "0.10000", "-0.4": "-0.40000", "12.3456789": "12.3456789",
		"1.2300000000": "1.23000", "999.99999": "999.99999", "-.0000012345": "-0.0000012345",
	} {
		got, _ := FormatNum(decimal.RequireFromString(in))
		if got != want {
			t.Errorf("FormatNum(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestFetch_NormalisesSortsAndSummarises(t *testing.T) {
	dup := append([]string(nil), rowNoRule...)
	q := newFake(t, "RO_ETL", rowSP, rowNoRule, rowCost, dup)
	rows, sum, err := Fetch(context.Background(), q, DefaultSchema)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Rows != 4 || sum.DuplicateKeys != 1 || sum.WideCells != 2 {
		t.Fatalf("summary %+v", sum)
	}
	if sum.PerBasis["COST"] != 1 || sum.PerBasis["SPPTY"] != 1 || sum.PerBasis["<none>"] != 2 {
		t.Fatalf("per basis %+v", sum.PerBasis)
	}
	// COST: 1.2*1 + R5(0.654321-0.1)=0.55432 -> 1.75432; SP: 1.5-0.25; no rule: 0-0 = legacy 0.
	if sum.PerMatch["std_match"] != 4 || sum.PerMatch["std_diff"] != 0 {
		t.Fatalf("per match %+v", sum.PerMatch)
	}
	if rows[0][0] != "ACY0000003" || rows[2][0] != "POY0000002" || rows[3][0] != "PTY0000001" {
		t.Fatalf("not sorted: %v %v %v", rows[0][0], rows[2][0], rows[3][0])
	}
	cost := rows[3]
	if cost[colIndex("rule_val_loss")] != "0.10000" || cost[colIndex("ax_conv")] != "0.654321" ||
		cost[colIndex("legacy_pvl")] != "" || cost[colIndex("flex_std")] != "1.75432" {
		t.Fatalf("cost row not normalised: %v", cost)
	}
	for _, s := range q.Queries() {
		if err := oracle.CheckReadOnly(s); err != nil {
			t.Fatalf("unguarded statement: %v", err)
		}
	}
}

func TestFetch_RejectsBadNumber(t *testing.T) {
	bad := fakeRow(map[string]string{"item_code": "X", "legacy_std": "1,5"})
	if _, _, err := Fetch(context.Background(), newFake(t, "RO_ETL", bad), DefaultSchema); err == nil {
		t.Fatal("comma decimal accepted")
	}
}

func TestWriteFixture_DeterministicRoundTrip(t *testing.T) {
	rows := [][]string{rowCost, rowSP}
	var a, b bytes.Buffer
	shaA, err := WriteFixture(&a, rows)
	if err != nil {
		t.Fatal(err)
	}
	shaB, _ := WriteFixture(&b, rows)
	if shaA != shaB || !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("fixture bytes not deterministic")
	}
	gz, err := gzip.NewReader(&a)
	if err != nil {
		t.Fatal(err)
	}
	if !gz.ModTime.IsZero() || gz.Name != "" {
		t.Fatalf("gzip header not stable: %v %q", gz.ModTime, gz.Name)
	}
	recs, err := csv.NewReader(gz).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || strings.Join(recs[0], ",") != strings.Join(Header(), ",") {
		t.Fatalf("round trip %d records", len(recs))
	}
}

func TestExport_WritesFixtureAndChecksum(t *testing.T) {
	out := filepath.Join(t.TempDir(), "testdata", "golden.csv.gz")
	var stdout bytes.Buffer
	o := options{out: out, schema: DefaultSchema, expect: 2}
	if err := export(context.Background(), newFake(t, "RO_ETL", rowCost, rowSP), "RO_ETL", o, &stdout); err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(out + ".sha256")
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(sum)), "golden.csv.gz") {
		t.Fatalf("checksum file: %q %v", sum, err)
	}
	if !strings.Contains(stdout.String(), "rows: 2") || !strings.Contains(stdout.String(), "basis COST: 1") {
		t.Fatalf("summary output: %s", stdout.String())
	}

	o.expect = 9602
	if err := export(context.Background(), newFake(t, "RO_ETL", rowCost), "RO_ETL", o, &stdout); err == nil {
		t.Fatal("wrong row count accepted")
	}
	if err := export(context.Background(), newFake(t, "RO_ETL"), "RO_ETL", o, &stdout); err == nil {
		t.Fatal("empty result accepted")
	}
	if err := export(context.Background(), newFake(t, "OTHER", rowCost), "RO_ETL", o, &stdout); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("identity mismatch not refused: %v", err)
	}
}

func TestDefaultOutPath_MatchesGoldenTest(t *testing.T) {
	if DefaultOutPath != "internal/domain/erpintegration/testdata/golden_legacy_std_202608.csv.gz" {
		t.Fatalf("DefaultOutPath changed: %s", DefaultOutPath)
	}
	if _, err := os.Stat(filepath.Join("..", "..", filepath.Dir(DefaultOutPath), "..")); err != nil {
		t.Fatalf("erpintegration package dir missing: %v", err)
	}
}

func TestRun_RefusesWithoutAllowedUser(t *testing.T) {
	t.Setenv(envDSN, "oracle://mgtdat:x@127.0.0.1:1/none")
	t.Setenv(envUsers, "mgtdat")
	if err := run(nil, &bytes.Buffer{}); !errors.Is(err, ErrUserNotAllowed) {
		t.Fatalf("owner user not refused before connecting: %v", err)
	}
	t.Setenv(envDSN, "")
	if err := run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("empty DSN accepted")
	}
}
