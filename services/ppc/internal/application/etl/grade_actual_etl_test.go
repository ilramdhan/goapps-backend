package etl

import (
	"context"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// stubGradeSource serves fixture grade-actual rows and records the watermark.
type stubGradeSource struct {
	rows         []oracle.GradeActualRow
	gotWatermark time.Time
}

func (s *stubGradeSource) ListGradeActuals(_ context.Context, watermark time.Time) ([]oracle.GradeActualRow, error) {
	s.gotWatermark = watermark
	return s.rows, nil
}

// stubGradeRepo records grade-actual upserts and simulates lot->WO matching.
type stubGradeRepo struct {
	watermark     time.Time
	advancedTo    time.Time
	advanceCalled bool
	matchByLot    map[string]int64
	upserts       []postgres.GradeActualUpsert
}

func (r *stubGradeRepo) GetWatermark(_ context.Context, _ string) (time.Time, error) {
	return r.watermark, nil
}

func (r *stubGradeRepo) AdvanceWatermark(_ context.Context, _ string, ts time.Time) error {
	r.advanceCalled = true
	r.advancedTo = ts
	return nil
}

func (r *stubGradeRepo) MatchWOByLot(_ context.Context, lotNo string) (int64, bool, error) {
	id, ok := r.matchByLot[lotNo]
	return id, ok, nil
}

func (r *stubGradeRepo) UpsertGradeActual(_ context.Context, in postgres.GradeActualUpsert) error {
	r.upserts = append(r.upserts, in)
	return nil
}

func TestGradeActualETLNilSourceNoOp(t *testing.T) {
	repo := &stubGradeRepo{}
	uc := NewGradeActualETL(nil, repo, 5)
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

func TestGradeActualETLMapsMatchedRows(t *testing.T) {
	updated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	packing := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	src := &stubGradeSource{rows: []oracle.GradeActualRow{
		{
			OriginalLotNo: "qU04qB006", Grade: "AX", Dept: "TXT",
			TotalQtyKg: 120.5, TotalBobbinCount: 30,
			LastPackingDate: packing, LastUpdated: updated.Add(-time.Minute),
		},
		{
			OriginalLotNo: "qU04qB006", Grade: "A9", Dept: "TXT",
			TotalQtyKg: 15.0, TotalBobbinCount: 4,
			LastPackingDate: packing, LastUpdated: updated,
		},
	}}
	repo := &stubGradeRepo{
		watermark:  time.Unix(0, 0).UTC(),
		matchByLot: map[string]int64{"qU04qB006": 42},
	}
	uc := NewGradeActualETL(src, repo, 5)

	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Pulled != 2 || res.Upserted != 2 || res.Unmatched != 0 {
		t.Fatalf("counts pulled=%d upserted=%d unmatched=%d, want 2/2/0", res.Pulled, res.Upserted, res.Unmatched)
	}
	if len(repo.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(repo.upserts))
	}
	first := repo.upserts[0]
	if first.WOID != 42 || first.Grade != "AX" || first.TotalQtyKg != 120.5 || first.BobbinCount != 30 {
		t.Errorf("upsert[0] = %+v, unexpected mapping", first)
	}
	// Watermark advanced to maxSeen - buffer(5m).
	wantAdvance := updated.Add(-5 * time.Minute)
	if !repo.advancedTo.Equal(wantAdvance) {
		t.Errorf("advancedTo = %v, want %v", repo.advancedTo, wantAdvance)
	}
}

func TestGradeActualETLUnmatchedLotSkipped(t *testing.T) {
	src := &stubGradeSource{rows: []oracle.GradeActualRow{
		{OriginalLotNo: "nope", Grade: "B", Dept: "TWT", TotalQtyKg: 5, LastUpdated: time.Now()},
	}}
	repo := &stubGradeRepo{watermark: time.Unix(0, 0).UTC(), matchByLot: map[string]int64{}}
	uc := NewGradeActualETL(src, repo, 0)

	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Unmatched != 1 || res.Upserted != 0 {
		t.Errorf("unmatched=%d upserted=%d, want 1/0", res.Unmatched, res.Upserted)
	}
	if len(repo.upserts) != 0 {
		t.Errorf("expected no upserts for unmatched lot")
	}
}
