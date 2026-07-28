package etl

import (
	"context"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// stubSpgSource serves fixture SPG rows.
type stubSpgSource struct {
	rows []oracle.SpgProductionRow
}

func (s *stubSpgSource) ListSpgProduction(_ context.Context, _ time.Time) ([]oracle.SpgProductionRow, error) {
	return s.rows, nil
}

// stubSpgRepo records SPG upserts and simulates WO matching / lot weights.
type stubSpgRepo struct {
	matchOK   bool
	lotFull   float64
	lotUnfull float64
	lotOK     bool
	upserts   []postgres.SpgProductionActualUpsert
}

func (r *stubSpgRepo) GetWatermark(_ context.Context, _ string) (time.Time, error) {
	return time.Unix(0, 0), nil
}
func (r *stubSpgRepo) AdvanceWatermark(_ context.Context, _ string, _ time.Time) error { return nil }
func (r *stubSpgRepo) MatchWO(_ context.Context, _, _ string) (int64, string, bool, error) {
	if !r.matchOK {
		return 0, "", false, nil
	}
	return 7, "SPG", true, nil
}
func (r *stubSpgRepo) LotStdWeights(_ context.Context, _ string) (float64, float64, bool, error) {
	return r.lotFull, r.lotUnfull, r.lotOK, nil
}
func (r *stubSpgRepo) UpsertSpgProductionActual(_ context.Context, in postgres.SpgProductionActualUpsert) error {
	r.upserts = append(r.upserts, in)
	return nil
}

func TestSpgProductionETL_NilSource_NoOp(t *testing.T) {
	etl := NewSpgProductionETL(nil, &stubSpgRepo{}, 5)
	res, err := etl.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.OracleUp || res.Pulled != 0 {
		t.Errorf("nil source should be a no-op, got %+v", res)
	}
}

func TestSpgProductionETL_DualQty_MeasuredWeight(t *testing.T) {
	// DOFF_OPTION=1 (Full). Two doffs of the same lot+line+date roll up.
	// Doff A: GROSS=10, TRANSFERRED=8, weight 5.0kg -> doffed 50, transferred 40.
	// Doff B: GROSS=6,  TRANSFERRED=6, weight 4.0kg -> doffed 24, transferred 24.
	src := &stubSpgSource{rows: []oracle.SpgProductionRow{
		{
			LotNo: "L1", MachineLine: "A1", DoffDate: date(t, "2026-07-01"),
			PositionNo: 1, DoffNo: 1, DoffOption: 1,
			GrossBobbins: 10, TransferredBob: 8, CutBobbins: 1, NotTransfer: 1,
			NormalBobs: 7, DowngradeBobs: 1, NotCheckedBobs: 0, WeightPerBob: 5.0,
			LastUpdated: time.Unix(100, 0),
		},
		{
			LotNo: "L1", MachineLine: "A1", DoffDate: date(t, "2026-07-01"),
			PositionNo: 2, DoffNo: 2, DoffOption: 1,
			GrossBobbins: 6, TransferredBob: 6, CutBobbins: 0, NotTransfer: 0,
			NormalBobs: 6, DowngradeBobs: 0, NotCheckedBobs: 0, WeightPerBob: 4.0,
			LastUpdated: time.Unix(200, 0),
		},
	}}
	repo := &stubSpgRepo{matchOK: true}
	etl := NewSpgProductionETL(src, repo, 0)

	res, err := etl.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Upserted != 1 {
		t.Fatalf("Upserted = %d, want 1 (rolled up)", res.Upserted)
	}
	up := repo.upserts[0]
	if up.GrossBobbins != 16 || up.TransferredBobs != 14 {
		t.Errorf("bobbins gross=%d transferred=%d, want 16/14", up.GrossBobbins, up.TransferredBobs)
	}
	// Doffed (efficiency basis) = 10*5 + 6*4 = 74.
	if up.QtyDoffedKg != 74.0 {
		t.Errorf("QtyDoffedKg = %g, want 74", up.QtyDoffedKg)
	}
	// Transferred (fulfillment basis) = 8*5 + 6*4 = 64.
	if up.QtyTransferredKg != 64.0 {
		t.Errorf("QtyTransferredKg = %g, want 64", up.QtyTransferredKg)
	}
	// Shift-average weight per bob = (10*5 + 6*4) / 16 = 4.625.
	if up.WeightPerBob < 4.62 || up.WeightPerBob > 4.63 {
		t.Errorf("WeightPerBob = %g, want ~4.625", up.WeightPerBob)
	}
}

func TestSpgProductionETL_StdWeightFallback_FullDoff(t *testing.T) {
	// No measured weight -> back-fill from lot std weights. DOFF_OPTION 1 = Full
	// (opposite of TXT). GROSS=10 * stdFull 6.0 = 60 doffed; TRANSFERRED=9 * 6 = 54.
	src := &stubSpgSource{rows: []oracle.SpgProductionRow{{
		LotNo: "L2", MachineLine: "B2", DoffDate: date(t, "2026-07-02"),
		DoffNo: 1, DoffOption: 1,
		GrossBobbins: 10, TransferredBob: 9, CutBobbins: 1, NotTransfer: 0,
		NormalBobs: 9, WeightPerBob: 0, LastUpdated: time.Unix(300, 0),
	}}}
	repo := &stubSpgRepo{matchOK: true, lotOK: true, lotFull: 6.0, lotUnfull: 3.0}
	etl := NewSpgProductionETL(src, repo, 0)

	if _, err := etl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	up := repo.upserts[0]
	if up.QtyDoffedKg != 60.0 {
		t.Errorf("QtyDoffedKg = %g, want 60 (10 * stdFull 6.0)", up.QtyDoffedKg)
	}
	if up.QtyTransferredKg != 54.0 {
		t.Errorf("QtyTransferredKg = %g, want 54 (9 * stdFull 6.0)", up.QtyTransferredKg)
	}
}

func TestSpgProductionETL_Unmatched_Skipped(t *testing.T) {
	src := &stubSpgSource{rows: []oracle.SpgProductionRow{{
		LotNo: "X", MachineLine: "Z", DoffDate: date(t, "2026-07-03"),
		GrossBobbins: 5, WeightPerBob: 4.0, LastUpdated: time.Unix(400, 0),
	}}}
	repo := &stubSpgRepo{matchOK: false}
	etl := NewSpgProductionETL(src, repo, 0)

	res, err := etl.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Unmatched != 1 || res.Upserted != 0 {
		t.Errorf("unmatched=%d upserted=%d, want 1/0", res.Unmatched, res.Upserted)
	}
}

// date parses an ISO date for fixtures.
func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad fixture date %q: %v", s, err)
	}
	return d
}
