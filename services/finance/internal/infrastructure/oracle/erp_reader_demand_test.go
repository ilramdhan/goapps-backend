package oracle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

const (
	fixtureDemand       = "../../testutil/fakeoracle/testdata/erp/adj_demand_202608_sample.csv"
	fixtureProbe        = "../../testutil/fakeoracle/testdata/erp/adj_head_probe_202608.csv"
	fixtureProbeEmpty   = "../../testutil/fakeoracle/testdata/erp/adj_head_probe_empty.csv"
	fixtureProbeNoPost  = "../../testutil/fakeoracle/testdata/erp/adj_head_probe_unposted.csv"
	demandView          = "MGTDAT.V_GOAPPS_ADJ_DEMAND"
	demandBaseItemTable = "MGTDAT.OT_ADJ_ITEM"
	adjHeadTable        = "MGTDAT.OT_ADJ_HEAD"
)

// recorder wraps the fake and records bind arguments.
type recorder struct {
	*fakeoracle.Querier
	args [][]any
}

func (r *recorder) QueryRO(ctx context.Context, q string, args ...any) (oracle.Rows, error) {
	r.args = append(r.args, args)
	return r.Querier.QueryRO(ctx, q, args...)
}

func newRecorder(t *testing.T, table, path string) *recorder {
	t.Helper()
	q := fakeoracle.New()
	if err := q.LoadCSV(table, path); err != nil {
		t.Fatal(err)
	}
	return &recorder{Querier: q}
}

func assertReadOnly(t *testing.T, q *fakeoracle.Querier) {
	t.Helper()
	if len(q.Queries()) == 0 {
		t.Fatal("no query was sent")
	}
	for _, s := range q.Queries() {
		if err := oracle.CheckReadOnly(s); err != nil {
			t.Errorf("non-read-only SQL %q: %v", s, err)
		}
		if strings.Contains(strings.ToUpper(s), "FOR UPDATE") {
			t.Errorf("FOR UPDATE in reader SQL: %q", s)
		}
	}
}

func dec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestErpDemandReader_View(t *testing.T) {
	rec := newRecorder(t, demandView, fixtureDemand)
	r, err := oracle.NewErpDemandReader(rec, "view", "production")
	if err != nil {
		t.Fatal(err)
	}
	if r.Source() != erpintegration.DemandSourceView {
		t.Fatalf("source = %q", r.Source())
	}
	rows, err := r.LoadAdjDemand(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	assertReadOnly(t, rec.Querier)
	if got := rec.Queries()[0]; !strings.Contains(got, "FROM MGTDAT.V_GOAPPS_ADJ_DEMAND") || !strings.Contains(got, "WHERE PERIOD = :1") {
		t.Errorf("unexpected view SQL: %q", got)
	}
	if len(rec.args[0]) != 1 || rec.args[0][0] != "202608" {
		t.Errorf("binds = %v, want [202608]", rec.args[0])
	}

	r0 := rows[0]
	if r0.TxnCode != erpintegration.TxnInvAdj || r0.ItemCode != "FGX0001" || r0.GradeCode != "AX" || r0.ShadeCode != "SH01" ||
		r0.HeadCount != 2 || r0.ItemCount != 5 || r0.RateVariants != 1 {
		t.Errorf("row 0 mapping wrong: %+v", r0)
	}
	if !r0.QtyKg.Equal(dec(t, "1234.567")) || !r0.MinRate.Valid || !r0.MinRate.Decimal.Equal(dec(t, "1.23456")) ||
		!r0.AdjVal.Decimal.Equal(dec(t, "1524.1111111")) {
		t.Errorf("row 0 decimals wrong: %+v", r0)
	}
	// Row 1: trimmed code, blank shade, NULL rates/value.
	r1 := rows[1]
	if r1.ItemCode != "FGX0002" || r1.ShadeCode != "" || r1.MinRate.Valid || r1.MaxRate.Valid || r1.AdjVal.Valid {
		t.Errorf("row 1 NULL/trim handling wrong: %+v", r1)
	}
	if !r1.QtyKg.Equal(dec(t, "0.001")) {
		t.Errorf("row 1 qty = %s", r1.QtyKg)
	}
	r2 := rows[2]
	if r2.TxnCode != erpintegration.TxnMbInvAdj || r2.ApprovedItems != 3 || r2.GoappsBatch != "17" || r2.GoappsSource != "GOAPPS" ||
		!r2.MinRate.Decimal.IsZero() || !r2.MinRate.Valid {
		t.Errorf("row 2 mapping wrong: %+v", r2)
	}
	r3 := rows[3]
	if r3.PostedItems != 2 || !r3.QtyKg.Equal(dec(t, "-12.3456789")) || r3.GradeCode != "BX" {
		t.Errorf("row 3 mapping wrong: %+v", r3)
	}
}

func TestErpDemandReader_BaseTables(t *testing.T) {
	rec := newRecorder(t, demandBaseItemTable, fixtureDemand)
	r, err := oracle.NewErpDemandReader(rec, " BASE_TABLES ", "development")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := r.LoadAdjDemand(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	assertReadOnly(t, rec.Querier)
	sql := rec.Queries()[0]
	for _, want := range []string{"MGTDAT.OT_ADJ_HEAD", "MGTDAT.OT_ADJ_ITEM", "'INVADJ', 'MBINVADJ', 'MBINVADJRP'", "/ 1000"} {
		if !strings.Contains(sql, want) {
			t.Errorf("base SQL missing %q", want)
		}
	}
	if len(rec.args[0]) != 2 || rec.args[0][0] != "202608" || rec.args[0][1] != "202608" {
		t.Errorf("binds = %v", rec.args[0])
	}
}

// TestErpDemandReader_SameColumns asserts I-4: both sources scan the same
// column contract, so the same fixture yields identical rows.
func TestErpDemandReader_SameColumns(t *testing.T) {
	view, err := oracle.NewErpDemandReader(newRecorder(t, demandView, fixtureDemand), "view", "")
	if err != nil {
		t.Fatal(err)
	}
	base, err := oracle.NewErpDemandReader(newRecorder(t, demandBaseItemTable, fixtureDemand), "base_tables", "staging")
	if err != nil {
		t.Fatal(err)
	}
	a, err := view.LoadAdjDemand(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	b, err := base.LoadAdjDemand(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ItemCode != b[i].ItemCode || !a[i].QtyKg.Equal(b[i].QtyKg) || a[i].PostedItems != b[i].PostedItems {
			t.Errorf("row %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestNewErpDemandReader_FailsClosed(t *testing.T) {
	q := fakeoracle.New()
	for _, env := range []string{"production", "PROD", " Production "} {
		if _, err := oracle.NewErpDemandReader(q, "base_tables", env); !errors.Is(err, erpintegration.ErrBaseTablesInProduction) {
			t.Errorf("env %q: expected ErrBaseTablesInProduction, got %v", env, err)
		}
	}
	if _, err := oracle.NewErpDemandReader(q, "tables", "development"); !errors.Is(err, erpintegration.ErrInvalidDemandSource) {
		t.Errorf("expected ErrInvalidDemandSource, got %v", err)
	}
	r, err := oracle.NewErpDemandReader(q, "", "production")
	if err != nil || r.Source() != erpintegration.DemandSourceView {
		t.Errorf("empty source must default to view: %v %v", r, err)
	}
}

func TestErpDemandReader_Errors(t *testing.T) {
	ctx := context.Background()
	r, _ := oracle.NewErpDemandReader(fakeoracle.New(), "view", "")
	if _, err := r.LoadAdjDemand(ctx, "2026-08"); !errors.Is(err, erpintegration.ErrInvalidBatchPeriod) {
		t.Errorf("expected ErrInvalidBatchPeriod, got %v", err)
	}
	nilR, _ := oracle.NewErpDemandReader(nil, "view", "")
	if _, err := nilR.LoadAdjDemand(ctx, "202608"); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Errorf("expected ErrNoReadConnection, got %v", err)
	}
	boom := errors.New("ORA-03113: end-of-file on communication channel")
	q := fakeoracle.New()
	q.FailWith(boom)
	r, _ = oracle.NewErpDemandReader(q, "view", "")
	if _, err := r.LoadAdjDemand(ctx, "202608"); !errors.Is(err, boom) {
		t.Errorf("expected wrapped Oracle error, got %v", err)
	}

	cols := []string{"PERIOD", "TXN_CODE", "ITEM_CODE", "ITEM_NAME", "GRADE_CODE_1", "GRADE_CODE_2", "HEAD_COUNT", "ITEM_COUNT",
		"RATE_VARIANTS", "QTY_KG", "MIN_RATE", "MAX_RATE", "ADJ_VAL", "APPROVED_ITEMS", "POSTED_ITEMS", "GOAPPS_BATCH", "GOAPPS_SOURCE"}
	bad := map[string][]string{
		"period mismatch": {"202607", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "1", "", "", "", "0", "0", "", ""},
		"null qty":        {"202608", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "", "", "", "", "0", "0", "", ""},
		"bad qty":         {"202608", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "1,5", "", "", "", "0", "0", "", ""},
		"bad min rate":    {"202608", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "1", "x", "", "", "0", "0", "", ""},
		"bad max rate":    {"202608", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "1", "1", "x", "", "0", "0", "", ""},
		"bad adj val":     {"202608", "INVADJ", "FGX1", "", "AX", "S", "1", "1", "1", "1", "1", "1", "x", "0", "0", "", ""},
		"blank item":      {"202608", "INVADJ", "  ", "", "AX", "S", "1", "1", "1", "1", "", "", "", "0", "0", "", ""},
		"bad int":         {"202608", "INVADJ", "FGX1", "", "AX", "S", "one", "1", "1", "1", "", "", "", "0", "0", "", ""},
	}
	for name, row := range bad {
		q := fakeoracle.New()
		q.Register(demandView, fakeoracle.Dataset{Columns: cols, Rows: [][]string{row}})
		r, _ := oracle.NewErpDemandReader(q, "view", "")
		if _, err := r.LoadAdjDemand(ctx, "202608"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestErpAdjReader_ProbeHeads(t *testing.T) {
	rec := newRecorder(t, adjHeadTable, fixtureProbe)
	r := oracle.NewErpAdjReader(rec)
	c, err := r.ProbeHeads(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	want := erpintegration.AdjHeadCounts{Heads: 7, Posted: 2, Approved: 3, NullStatus: 1, Eligible: 3}
	if c != want {
		t.Errorf("counts = %+v, want %+v", c, want)
	}
	assertReadOnly(t, rec.Querier)
	sql := rec.Queries()[0]
	if !strings.Contains(sql, "NVL(h.ADJH_APPR_STATUS, 0) != 3 AND h.ADJH_POST_STATUS IS NULL") {
		t.Errorf("probe must use the §S-R7 NVL predicate: %q", sql)
	}
	if len(rec.args[0]) != 2 || rec.args[0][0] != "202608" {
		t.Errorf("binds = %v", rec.args[0])
	}
	n, err := r.ProbePosted(context.Background(), "202608")
	if err != nil || n != 2 {
		t.Errorf("ProbePosted = %d, %v", n, err)
	}
}

func TestErpAdjReader_ProbeHeadsEmptyPeriod(t *testing.T) {
	r := oracle.NewErpAdjReader(newRecorder(t, adjHeadTable, fixtureProbeEmpty))
	c, err := r.ProbeHeads(context.Background(), "202608")
	if err != nil {
		t.Fatal(err)
	}
	if c != (erpintegration.AdjHeadCounts{}) {
		t.Errorf("NULL sums must read as zero: %+v", c)
	}
}

func TestErpAdjReader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := oracle.NewErpAdjReader(fakeoracle.New()).ProbeHeads(ctx, "20268"); !errors.Is(err, erpintegration.ErrInvalidBatchPeriod) {
		t.Errorf("expected ErrInvalidBatchPeriod, got %v", err)
	}
	if _, err := oracle.NewErpAdjReader(nil).ProbePosted(ctx, "202608"); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Errorf("expected ErrNoReadConnection, got %v", err)
	}
	boom := errors.New("ORA-12170: TNS connect timeout")
	q := fakeoracle.New()
	q.FailWith(boom)
	if _, err := oracle.NewErpAdjReader(q).ProbePosted(ctx, "202608"); !errors.Is(err, boom) {
		t.Errorf("expected wrapped error, got %v", err)
	}
	empty := fakeoracle.New()
	empty.Register(adjHeadTable, fakeoracle.Dataset{Columns: []string{"HEADS", "POSTED", "APPROVED", "NULL_STATUS", "ELIGIBLE"}})
	if _, err := oracle.NewErpAdjReader(empty).ProbeHeads(ctx, "202608"); err == nil {
		t.Error("expected an error for a probe with no result row")
	}
	badScan := fakeoracle.New()
	badScan.Register(adjHeadTable, fakeoracle.Dataset{Columns: []string{"HEADS"}, Rows: [][]string{{"1"}}})
	if _, err := oracle.NewErpAdjReader(badScan).ProbeHeads(ctx, "202608"); err == nil {
		t.Error("expected a scan error")
	}
}

func TestAdjPostedProbe(t *testing.T) {
	ctx := context.Background()
	posted := oracle.NewAdjPostedProbe(oracle.NewErpAdjReader(newRecorder(t, adjHeadTable, fixtureProbe)))
	if ok, err := posted.IsAdjPosted(ctx, "202608"); err != nil || !ok {
		t.Errorf("expected posted, got %v %v", ok, err)
	}
	unposted := oracle.NewAdjPostedProbe(oracle.NewErpAdjReader(newRecorder(t, adjHeadTable, fixtureProbeNoPost)))
	if ok, err := unposted.IsAdjPosted(ctx, "202608"); err != nil || ok {
		t.Errorf("expected not posted, got %v %v", ok, err)
	}
	// Fail closed: errors propagate (the unlock handler refuses on error).
	boom := errors.New("ORA-03113")
	q := fakeoracle.New()
	q.FailWith(boom)
	if _, err := oracle.NewAdjPostedProbe(oracle.NewErpAdjReader(q)).IsAdjPosted(ctx, "202608"); !errors.Is(err, boom) {
		t.Errorf("expected error, got %v", err)
	}
	if _, err := oracle.NewAdjPostedProbe(nil).IsAdjPosted(ctx, "202608"); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Errorf("expected ErrNoReadConnection, got %v", err)
	}
}
