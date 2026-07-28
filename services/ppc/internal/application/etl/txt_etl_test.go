package etl

import (
	"context"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// stubTxtSource serves fixture rows and records the watermark it was asked for.
type stubTxtSource struct {
	rows         []oracle.TxtProductionRow
	gotWatermark time.Time
}

func (s *stubTxtSource) ListTxtProduction(_ context.Context, watermark time.Time) ([]oracle.TxtProductionRow, error) {
	s.gotWatermark = watermark
	return s.rows, nil
}

// stubTxtRepo records upserts and simulates WO matching / lot weights.
type stubTxtRepo struct {
	watermark     time.Time
	advancedTo    time.Time
	matchOK       bool
	matchArea     string
	lotFull       float64
	lotUnfull     float64
	lotOK         bool
	upserts       []postgres.ProductionActualUpsert
	advanceCalled bool
}

func (r *stubTxtRepo) GetWatermark(_ context.Context, _ string) (time.Time, error) {
	return r.watermark, nil
}

func (r *stubTxtRepo) AdvanceWatermark(_ context.Context, _ string, ts time.Time) error {
	r.advanceCalled = true
	r.advancedTo = ts
	return nil
}

func (r *stubTxtRepo) MatchWO(_ context.Context, _, _ string) (int64, string, bool, error) {
	if !r.matchOK {
		return 0, "", false, nil
	}
	return 42, r.matchArea, true, nil
}

func (r *stubTxtRepo) LotStdWeights(_ context.Context, _ string) (float64, float64, bool, error) {
	return r.lotFull, r.lotUnfull, r.lotOK, nil
}

func (r *stubTxtRepo) UpsertProductionActual(_ context.Context, in postgres.ProductionActualUpsert) error {
	r.upserts = append(r.upserts, in)
	return nil
}

func fixtureDate() time.Time { return time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC) }

func TestTxtETLNilSourceNoOp(t *testing.T) {
	repo := &stubTxtRepo{}
	uc := NewTxtProductionETL(nil, repo, 5)
	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.OracleUp {
		t.Errorf("expected OracleUp false on nil source")
	}
	if repo.advanceCalled {
		t.Errorf("watermark should not advance when Oracle unavailable")
	}
}

func TestTxtETLAggregatesAndComputesQty(t *testing.T) {
	updated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	// Two DOFF rows for the same lot/machine/date/shift must aggregate.
	src := &stubTxtSource{rows: []oracle.TxtProductionRow{
		{
			LotNo: "qU04qB006", MachineNo: "SM2", Area: "TXT",
			TrnDate: fixtureDate(), TrnShift: "1", DoffNo: 1,
			TotalBobbins: 10, FullBobbins: 9, UnfullBobbins: 1,
			NormalBobs: 9, DowngradeBobs: 1, LastUpdated: updated.Add(-time.Minute),
		},
		{
			LotNo: "qU04qB006", MachineNo: "SM2", Area: "TXT",
			TrnDate: fixtureDate(), TrnShift: "1", DoffNo: 2,
			TotalBobbins: 10, FullBobbins: 9, UnfullBobbins: 1,
			NormalBobs: 9, DowngradeBobs: 1, LastUpdated: updated,
		},
	}}
	repo := &stubTxtRepo{
		watermark: time.Unix(0, 0).UTC(),
		matchOK:   true, matchArea: "TXT",
		lotFull: 4.0, lotUnfull: 2.0, lotOK: true,
	}
	uc := NewTxtProductionETL(src, repo, 5)

	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Pulled != 2 || res.Upserted != 1 || res.Unmatched != 0 {
		t.Fatalf("counts pulled=%d upserted=%d unmatched=%d, want 2/1/0", res.Pulled, res.Upserted, res.Unmatched)
	}
	if len(repo.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(repo.upserts))
	}
	got := repo.upserts[0]
	// Aggregated: full=18, unfull=2 -> 18*4 + 2*2 = 76.
	if got.QtyBobbin != 76.0 {
		t.Errorf("QtyBobbin = %g, want 76.0", got.QtyBobbin)
	}
	if got.TotalBobbins != 20 || got.FullBobbins != 18 || got.UnfullBobbins != 2 {
		t.Errorf("aggregated bobbins = total %d full %d unfull %d, want 20/18/2",
			got.TotalBobbins, got.FullBobbins, got.UnfullBobbins)
	}
	// Watermark advanced to maxSeen - buffer(5m).
	wantAdvance := updated.Add(-5 * time.Minute)
	if !repo.advancedTo.Equal(wantAdvance) {
		t.Errorf("advancedTo = %v, want %v", repo.advancedTo, wantAdvance)
	}
}

func TestTxtETLUnmatchedIsSkipped(t *testing.T) {
	src := &stubTxtSource{rows: []oracle.TxtProductionRow{
		{
			LotNo: "nope", MachineNo: "X1", Area: "TXT",
			TrnDate: fixtureDate(), TrnShift: "1", DoffNo: 1,
			TotalBobbins: 5, FullBobbins: 5, LastUpdated: time.Now(),
		},
	}}
	repo := &stubTxtRepo{watermark: time.Unix(0, 0).UTC(), matchOK: false}
	uc := NewTxtProductionETL(src, repo, 0)

	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Unmatched != 1 || res.Upserted != 0 {
		t.Errorf("unmatched=%d upserted=%d, want 1/0", res.Unmatched, res.Upserted)
	}
	if len(repo.upserts) != 0 {
		t.Errorf("expected no upserts for unmatched row")
	}
}
