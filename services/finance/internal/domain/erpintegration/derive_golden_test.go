package erpintegration

// derive_golden_test.go is the AC-02 golden test (plan-05 P4-T3, design
// Part 2 §6.5): every legacy derived std row of period 202608 is re-derived
// by the pure engine from the legacy AX components and the 000548 seed rule
// set, and must equal legacy at 5 dp (decimal.Equal) plus FormatFlex text.
//
// The real fixture (testdata/golden_legacy_std_202608.csv.gz) is produced
// once by the user with `go run ./tools/golden-export` (read-only Oracle,
// P4-T2). Until it is present TestDeriveGolden_Legacy202608 SKIPS; the P4
// exit requires it present and green. The synthetic tests below always run
// and prove the harness (load, derive, compare, report) is not vacuous.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

const (
	goldenFixturePath = "testdata/golden_legacy_std_202608.csv.gz"
	// goldenSeedPath is the canonical 000548 seed rule set; P2's
	// TestRuleSetSeedGolden asserts it is identical to the migration.
	goldenSeedPath   = "../erprule/testdata/ruleset_seed_canonical.golden.json"
	goldenWantRows   = 9602
	goldenMaxDiffOut = 50
	goldenNone       = "<none>"
)

// goldenRequiredColumns are the fixture columns the harness reads (a subset
// of tools/golden-export Header(); extra columns are ignored).
var goldenRequiredColumns = []string{
	"item_code", "grade_code", "shade_code", "fg_type", "prod_type", "grade_group",
	"rule_basis", "ax_chp_cost", "ax_chp_con_kg", "ax_conv",
	"legacy_basis", "legacy_selling", "legacy_val_loss", "legacy_std", "legacy_conv",
	"legacy_ax_cost", "legacy_ax_conv_cost", "legacy_pvl",
	"legacy_conv1", "legacy_conv2", "legacy_conv4", "legacy_conv5",
	"flex_std", "flex_conv", "flex_ax_cost", "flex_pvl",
}

// goldenRow is one fixture row, by column name.
type goldenRow struct {
	line int
	col  map[string]string
}

func (r goldenRow) get(name string) string { return strings.TrimSpace(r.col[name]) }

func (r goldenRow) key() ErpKey {
	return ErpKey{ItemCode: r.get("item_code"), GradeCode: r.get("grade_code"), ShadeCode: r.get("shade_code")}
}

// loadGolden reads a gzip CSV fixture with a header row.
func loadGolden(r io.Reader) ([]goldenRow, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	cr := csv.NewReader(gz)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	for _, c := range goldenRequiredColumns {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("fixture is missing column %q", c)
		}
	}
	var rows []goldenRow
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		m := make(map[string]string, len(idx))
		for name, i := range idx {
			m[name] = rec[i]
		}
		rows = append(rows, goldenRow{line: line, col: m})
	}
}

// goldenNum parses a fixture numeric cell; "" is NULL (Valid=false).
func goldenNum(r goldenRow, name string) (decimal.NullDecimal, error) {
	s := r.get(name)
	if s == "" {
		return decimal.NullDecimal{}, nil
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("line %d column %s: invalid number %q", r.line, name, s)
	}
	return valid(v), nil
}

// goldenInput rebuilds the engine input from the legacy AX row: the AX
// FG_CHP_COST is cpc_total_rm_cost and FG_CHP_COST×kg + FG_CONVER_COST is
// cpc_cost_per_unit, so R5(CostPerUnit − TotalRMCost) = legacy AX conv. A
// NULL AX cost or conv is NVL'd to 0 (the resulting diff is reported).
func goldenInput(r goldenRow) (DeriveInput, error) {
	chp, err := goldenNum(r, "ax_chp_cost")
	if err != nil {
		return DeriveInput{}, err
	}
	kg, err := goldenNum(r, "ax_chp_con_kg")
	if err != nil {
		return DeriveInput{}, err
	}
	conv, err := goldenNum(r, "ax_conv")
	if err != nil {
		return DeriveInput{}, err
	}
	k := decimal.NewFromInt(1)
	if kg.Valid {
		k = kg.Decimal
	}
	key := r.key()
	return DeriveInput{
		Key:  key,
		Kind: ItemKindForCode(key.ItemCode),
		Ax: &AxComponents{
			CostPerUnit: chp.Decimal.Mul(k).Add(conv.Decimal),
			TotalRMCost: chp.Decimal,
			FgType:      r.get("fg_type"),
		},
	}, nil
}

// goldenDiff is one mismatching field of one row.
type goldenDiff struct {
	Field, Want, Got string
}

// goldenNumFields pairs each legacy numeric column with the engine value.
func goldenNumFields(s StdRow) []struct {
	col string
	got decimal.NullDecimal
} {
	return []struct {
		col string
		got decimal.NullDecimal
	}{
		{"legacy_std", s.StdCost}, {"legacy_conv", s.ConvCost},
		{"legacy_ax_cost", s.AxCost}, {"legacy_pvl", s.ProdValLoss},
		{"legacy_conv1", s.ConvCost1}, {"legacy_conv2", s.ConvCost2},
		{"legacy_conv4", s.ConvCost4}, {"legacy_conv5", s.ConvCost5},
		{"legacy_ax_conv_cost", s.AxConvCost}, {"legacy_selling", s.SellingPrice},
		{"legacy_val_loss", s.ValueLoss},
	}
}

func nullText(v decimal.NullDecimal) string {
	if !v.Valid {
		return "NULL"
	}
	return v.Decimal.StringFixed(ScaleR5)
}

// compareGolden returns every field where the engine row differs from the
// legacy row. A non-OK engine status is itself a diff.
func compareGolden(r goldenRow, s StdRow) []goldenDiff {
	var out []goldenDiff
	add := func(f, want, got string) { out = append(out, goldenDiff{Field: f, Want: want, Got: got}) }
	if s.Status != DeriveOK {
		msg := string(s.Status)
		if len(s.Issues) > 0 {
			msg = s.Issues[len(s.Issues)-1].Message
		}
		add("status", string(DeriveOK), msg)
	}
	if want := r.get("grade_group"); want != string(s.GradeGroup) {
		add("grade_group", want, string(s.GradeGroup))
	}
	if want := r.get("prod_type"); want != string(s.ProdType) {
		add("prod_type", want, string(s.ProdType))
	}
	if want := r.get("legacy_basis"); want != string(s.Basis) {
		add("basis", want, string(s.Basis))
	}
	for _, f := range goldenNumFields(s) {
		want, err := goldenNum(r, f.col)
		if err != nil {
			add(f.col, r.get(f.col), err.Error())
			continue
		}
		if want.Valid != f.got.Valid || (want.Valid && !want.Decimal.Equal(f.got.Decimal)) {
			add(f.col, nullText(want), nullText(f.got))
		}
	}
	out = append(out, compareGoldenFlex(r, s)...)
	return out
}

// compareGoldenFlex compares Oracle's FM990D00000 text with FormatFlex.
func compareGoldenFlex(r goldenRow, s StdRow) []goldenDiff {
	var out []goldenDiff
	for _, f := range []struct {
		col string
		got decimal.NullDecimal
	}{
		{"flex_std", s.StdCost}, {"flex_conv", s.ConvCost},
		{"flex_ax_cost", s.AxCost}, {"flex_pvl", s.ProdValLoss},
	} {
		want := r.get(f.col)
		if want == "" {
			continue // "where present" (design §6.5)
		}
		got, err := FormatFlexNull(f.got)
		gotText := string(got)
		if err != nil {
			gotText = err.Error()
		}
		if want != gotText {
			out = append(out, goldenDiff{Field: f.col, Want: want, Got: gotText})
		}
	}
	return out
}

// goldenRowResult is the outcome of one fixture row.
type goldenRowResult struct {
	row   goldenRow
	std   StdRow
	diffs []goldenDiff
}

// goldenReport aggregates a golden run.
type goldenReport struct {
	Rows, Matched int
	PerBasis      map[string][2]int // legacy rule_basis -> {rows, matched}
	PerStatus     map[DeriveStatus]int
	PerField      map[string]int // mismatch count per field
	Failures      []goldenRowResult
}

// runGolden derives every fixture row and compares it with legacy.
func runGolden(rows []goldenRow, rs *erprule.RuleSet) (goldenReport, error) {
	rep := goldenReport{
		PerBasis:  map[string][2]int{},
		PerStatus: map[DeriveStatus]int{},
		PerField:  map[string]int{},
	}
	inputs := make([]DeriveInput, 0, len(rows))
	for _, r := range rows {
		in, err := goldenInput(r)
		if err != nil {
			return rep, err
		}
		inputs = append(inputs, in)
	}
	std, _ := Derive(inputs, rs)
	for i, r := range rows {
		diffs := compareGolden(r, std[i])
		basis := r.get("rule_basis")
		if basis == "" {
			basis = goldenNone
		}
		b := rep.PerBasis[basis]
		b[0]++
		rep.Rows++
		rep.PerStatus[std[i].Status]++
		if len(diffs) == 0 {
			b[1]++
			rep.Matched++
		} else {
			rep.Failures = append(rep.Failures, goldenRowResult{row: r, std: std[i], diffs: diffs})
			for _, df := range diffs {
				rep.PerField[df.Field]++
			}
		}
		rep.PerBasis[basis] = b
	}
	return rep, nil
}

func sortedMapKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// String renders the breakdown and the first goldenMaxDiffOut failures.
func (rep goldenReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "golden: %d/%d rows match\n", rep.Matched, rep.Rows)
	for _, k := range sortedMapKeys(rep.PerBasis) {
		v := rep.PerBasis[k]
		fmt.Fprintf(&b, "  basis %-6s rows=%d match=%d diff=%d\n", k, v[0], v[1], v[0]-v[1])
	}
	statuses := make([]string, 0, len(rep.PerStatus))
	for s := range rep.PerStatus {
		statuses = append(statuses, string(s))
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		fmt.Fprintf(&b, "  status %-14s %d\n", s, rep.PerStatus[DeriveStatus(s)])
	}
	for _, k := range sortedMapKeys(rep.PerField) {
		fmt.Fprintf(&b, "  field %-20s mismatches=%d\n", k, rep.PerField[k])
	}
	for i, f := range rep.Failures {
		if i == goldenMaxDiffOut {
			fmt.Fprintf(&b, "  ... %d more failing rows not shown\n", len(rep.Failures)-goldenMaxDiffOut)
			break
		}
		fmt.Fprintf(&b, "  line %d %s basis=%s status=%s:", f.row.line, f.row.key(), f.row.get("rule_basis"), f.std.Status)
		for _, df := range f.diffs {
			fmt.Fprintf(&b, " %s want=%s got=%s;", df.Field, df.Want, df.Got)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// loadSeedRuleSet parses the canonical 000548 seed rule set.
func loadSeedRuleSet(t *testing.T) *erprule.RuleSet {
	t.Helper()
	raw, err := os.ReadFile(goldenSeedPath)
	require.NoError(t, err, "read seed rule set")
	rs, err := erprule.ParseCanonical(bytes.TrimSpace(raw))
	require.NoError(t, err, "parse seed rule set")
	require.Equal(t, 115, rs.RuleCount(), "seed rules")
	require.Equal(t, 3, rs.PriceCount(), "seed prices")
	require.Equal(t, 22, rs.GradeCount(), "seed grade groups")
	return rs
}

// verifyGoldenChecksum checks <fixture>.sha256 ("<hex>  <name>") if present.
func verifyGoldenChecksum(t *testing.T, fixture []byte, path string) {
	t.Helper()
	side, err := os.ReadFile(path + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		t.Logf("no %s.sha256 next to the fixture; checksum not verified", filepath.Base(path))
		return
	}
	require.NoError(t, err)
	fields := strings.Fields(string(side))
	require.NotEmpty(t, fields, "empty checksum file")
	sum := sha256.Sum256(fixture)
	require.Equal(t, fields[0], hex.EncodeToString(sum[:]), "fixture sha256 differs from %s.sha256", filepath.Base(path))
}

// TestDeriveGolden_Legacy202608 is AC-02: 9,602/9,602 at 5 dp.
func TestDeriveGolden_Legacy202608(t *testing.T) {
	raw, err := os.ReadFile(goldenFixturePath)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("golden fixture %s is absent: AC-02 NOT verified. The user must run the read-only "+
			"exporter once (P4-T2, QG-USER-SQL) from services/finance: "+
			"ORACLE_RO_DSN='oracle://<ro_user>:<pw>@<dev_host>:<port>/<svc>' GOLDEN_RO_USERS='<ro_user>' "+
			"go run ./tools/golden-export ; the P4 exit requires this test present and green", goldenFixturePath)
	}
	require.NoError(t, err)
	verifyGoldenChecksum(t, raw, goldenFixturePath)

	rows, err := loadGolden(bytes.NewReader(raw))
	require.NoError(t, err)
	rep, err := runGolden(rows, loadSeedRuleSet(t))
	require.NoError(t, err)
	t.Log(rep.String())

	assert.Equal(t, goldenWantRows, rep.Rows, "fixture row count (recon §3)")
	if rep.Matched != rep.Rows {
		t.Fatalf("AC-02 FAILED: %d of %d rows differ from legacy. Do not change the engine "+
			"unless the deviation is listed in the design; stop and report.\n%s",
			rep.Rows-rep.Matched, rep.Rows, rep)
	}
}

// --- synthetic fixture: always runs, proves the harness -----------------

// syntheticGoldenHeader is the exporter's 37-column header (tools/golden-export Header()).
var syntheticGoldenHeader = []string{
	"item_code", "grade_code", "shade_code", "item_type", "fg_type", "row_fg_type", "prod_type",
	"grade_group", "rule_basis", "rule_val_loss", "rule_sell_price_raw",
	"ax_chp_item_code", "ax_chp_cost", "ax_chp_con_kg", "ax_conv",
	"ax_conv1", "ax_conv2", "ax_conv4", "ax_conv5", "ax_ax_conv_cost", "ax_std",
	"legacy_basis", "legacy_selling", "legacy_val_loss", "legacy_std", "legacy_conv",
	"legacy_ax_cost", "legacy_ax_conv_cost", "legacy_pvl",
	"legacy_conv1", "legacy_conv2", "legacy_conv4", "legacy_conv5",
	"flex_std", "flex_conv", "flex_ax_cost", "flex_pvl",
}

// syntheticGoldenRows are hand-computed from the legacy formulas (research
// §1.3 / §1.5) with the 000548 seed rules, NOT from the engine:
//
//	COST : conv = R5(AX.conv − loss); std = R5(chp×kg + conv)
//	SPxx : std = R5(sell − loss); conv* = 0
//	all  : ax_cost = R5(chp×kg + R5(AX.conv)); pvl = R5(std − ax_cost)
func syntheticGoldenRows() []map[string]string {
	base := func(item, grade, fg, pt, group, basis, loss, sell, chp, conv string) map[string]string {
		return map[string]string{
			"item_code": item, "grade_code": grade, "shade_code": "NL", "item_type": item[:3],
			"fg_type": fg, "row_fg_type": fg, "prod_type": pt, "grade_group": group,
			"rule_basis": basis, "rule_val_loss": loss, "rule_sell_price_raw": sell,
			"ax_chp_item_code": "CHP01", "ax_chp_cost": chp, "ax_chp_con_kg": "1.00000", "ax_conv": conv,
			"ax_conv1": conv, "ax_conv2": conv, "ax_conv4": conv, "ax_conv5": conv, "ax_ax_conv_cost": conv,
			"legacy_basis": basis, "legacy_val_loss": loss, "legacy_ax_conv_cost": conv,
		}
	}
	set := func(m map[string]string, kv ...string) map[string]string {
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	tiers := func(m map[string]string, v string) map[string]string {
		return set(m, "legacy_conv", v, "legacy_conv1", v, "legacy_conv2", v, "legacy_conv4", v, "legacy_conv5", v, "flex_conv", v)
	}
	return []map[string]string{
		// COST, Type 1/POY/POYA loss 0: conv 0.54321, std 1.77777, pvl 0.
		tiers(set(base("POY150D48F", "Aa", "Type 1", "POY", "POYA", "COST", "0.00000", "", "1.23456", "0.54321"),
			"ax_std", "1.77777", "legacy_selling", "0.00000", "legacy_std", "1.77777", "legacy_ax_cost", "1.77777",
			"legacy_pvl", "0.00000", "flex_std", "1.77777", "flex_ax_cost", "1.77777", "flex_pvl", "0.00000"), "0.54321"),
		// COST, Type 1/PTY/NS loss 0.05: conv 0.35, std 1.45, ax 1.5, pvl -0.05.
		tiers(set(base("PTY075D36F", "A", "Type 1", "PTY", "NS", "COST", "0.05000", "", "1.10000", "0.40000"),
			"ax_std", "1.50000", "legacy_selling", "0.00000", "legacy_std", "1.45000", "legacy_ax_cost", "1.50000",
			"legacy_pvl", "-0.05000", "flex_std", "1.45000", "flex_ax_cost", "1.50000", "flex_pvl", "-0.05000"), "0.35000"),
		// SPPTY, Type 1/POY/BC loss 0.5, sell 1.3: std 0.8, ax 1.5, pvl -0.7.
		tiers(set(base("POY300D96F", "B", "Type 1", "POY", "BC", "SPPTY", "0.50000", "1.3", "1.20000", "0.30000"),
			"ax_std", "1.50000", "legacy_selling", "1.30000", "legacy_std", "0.80000", "legacy_ax_cost", "1.50000",
			"legacy_pvl", "-0.70000", "flex_std", "0.80000", "flex_ax_cost", "1.50000", "flex_pvl", "-0.70000"), "0.00000"),
		// SPITY, Type 10/ITY/BB loss 0.6, sell 1.5: std 0.9, ax 1.05, pvl -0.15.
		tiers(set(base("ITY150D48F", "BB", "Type 10", "ITY", "BB", "SPITY", "0.60000", "1.5", "0.80000", "0.25000"),
			"ax_std", "1.05000", "legacy_selling", "1.50000", "legacy_std", "0.90000", "legacy_ax_cost", "1.05000",
			"legacy_pvl", "-0.15000", "flex_std", "0.90000", "flex_ax_cost", "1.05000", "flex_pvl", "-0.15000"), "0.00000"),
		// SPBSD, ACY prefix -> PTY (C-18 fallback), Type 12/PTY/JLT loss 0.4,
		// sell 1.4: std 1.0, ax 2.5, pvl -1.5.
		tiers(set(base("ACY150D48F", "JLT", "Type 12", "PTY", "JLT", "SPBSD", "0.40000", "1.4", "2.00000", "0.50000"),
			"ax_std", "2.50000", "legacy_selling", "1.40000", "legacy_std", "1.00000", "legacy_ax_cost", "2.50000",
			"legacy_pvl", "-1.50000", "flex_std", "1.00000", "flex_ax_cost", "2.50000", "flex_pvl", "-1.50000"), "0.00000"),
	}
}

// syntheticGoldenGz renders rows as the exporter does (gzip CSV + header).
func syntheticGoldenGz(t *testing.T, header []string, rows []map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	cw := csv.NewWriter(gz)
	require.NoError(t, cw.Write(header))
	for _, m := range rows {
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = m[h]
		}
		require.NoError(t, cw.Write(rec))
	}
	cw.Flush()
	require.NoError(t, cw.Error())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func runSynthetic(t *testing.T, rows []map[string]string) goldenReport {
	t.Helper()
	loaded, err := loadGolden(bytes.NewReader(syntheticGoldenGz(t, syntheticGoldenHeader, rows)))
	require.NoError(t, err)
	require.Len(t, loaded, len(rows))
	rep, err := runGolden(loaded, loadSeedRuleSet(t))
	require.NoError(t, err)
	return rep
}

func TestDeriveGolden_SyntheticFixture_AllBasesMatch(t *testing.T) {
	rep := runSynthetic(t, syntheticGoldenRows())
	require.Equal(t, rep.Rows, rep.Matched, "%s", rep)
	assert.Equal(t, 5, rep.Rows)
	assert.Equal(t, map[string][2]int{"COST": {2, 2}, "SPPTY": {1, 1}, "SPITY": {1, 1}, "SPBSD": {1, 1}}, rep.PerBasis)
	assert.Equal(t, map[DeriveStatus]int{DeriveOK: 5}, rep.PerStatus)
	assert.Empty(t, rep.Failures)
}

// Each mutation must be caught: the harness is not vacuous.
func TestDeriveGolden_SyntheticFixture_DetectsDiffs(t *testing.T) {
	cases := []struct {
		name, field, value, wantField string
	}{
		{"std last digit", "legacy_std", "1.77778", "legacy_std"},
		{"pvl", "legacy_pvl", "0.00001", "legacy_pvl"},
		{"conv tier", "legacy_conv4", "0.54320", "legacy_conv4"},
		{"basis", "legacy_basis", "SPPTY", "basis"},
		{"flex text", "flex_std", "1.7778", "flex_std"},
		{"legacy NULL", "legacy_ax_cost", "", "legacy_ax_cost"},
		{"grade group input", "grade_group", "NS", "grade_group"},
		{"unknown grade", "grade_code", "ZZ", "status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := syntheticGoldenRows()
			rows[0][tc.field] = tc.value
			rep := runSynthetic(t, rows)
			assert.Equal(t, 4, rep.Matched)
			require.Len(t, rep.Failures, 1)
			assert.Equal(t, 1, rep.PerField[tc.wantField], "%s", rep)
			assert.Contains(t, rep.String(), "line 2 ")
			assert.Contains(t, rep.String(), tc.wantField)
		})
	}
}

func TestDeriveGolden_SyntheticFixture_NoRuleIsADiff(t *testing.T) {
	// Legacy with no MICVL rule wrote a zero-cost row (NULL basis); the
	// engine returns NO_RULE (design §6.2), so such a row must fail AC-02
	// loudly rather than pass silently.
	rows := syntheticGoldenRows()
	rows[0]["fg_type"] = "Type 99"
	rep := runSynthetic(t, rows)
	require.Len(t, rep.Failures, 1)
	assert.Equal(t, DeriveNoRule, rep.Failures[0].std.Status)
	assert.Equal(t, 1, rep.PerStatus[DeriveNoRule])
}

func TestDeriveGolden_ReportTruncatesAt50(t *testing.T) {
	rows := make([]map[string]string, 0, 60)
	for i := 0; i < 60; i++ {
		r := syntheticGoldenRows()[1]
		r["legacy_std"] = "9.99999"
		rows = append(rows, r)
	}
	rep := runSynthetic(t, rows)
	assert.Len(t, rep.Failures, 60)
	s := rep.String()
	assert.Equal(t, goldenMaxDiffOut, strings.Count(s, "\n  line "))
	assert.Contains(t, s, "10 more failing rows not shown")
	assert.Contains(t, s, "golden: 0/60 rows match")
}

func TestLoadGolden_Errors(t *testing.T) {
	_, err := loadGolden(bytes.NewReader([]byte("not gzip")))
	require.Error(t, err)

	hdr := append([]string{}, syntheticGoldenHeader...)
	hdr = hdr[1:] // drop item_code
	_, err = loadGolden(bytes.NewReader(syntheticGoldenGz(t, hdr, nil)))
	require.ErrorContains(t, err, `missing column "item_code"`)

	rows := syntheticGoldenRows()
	rows[0]["ax_conv"] = "1,5"
	loaded, err := loadGolden(bytes.NewReader(syntheticGoldenGz(t, syntheticGoldenHeader, rows)))
	require.NoError(t, err)
	_, err = runGolden(loaded, nil)
	require.ErrorContains(t, err, "ax_conv")
}

func TestVerifyGoldenChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.csv.gz")
	data := []byte("fixture")
	verifyGoldenChecksum(t, data, path) // absent sidecar: logs only

	sum := sha256.Sum256(data)
	require.NoError(t, os.WriteFile(path+".sha256", []byte(hex.EncodeToString(sum[:])+"  f.csv.gz\n"), 0o600))
	verifyGoldenChecksum(t, data, path)
}
