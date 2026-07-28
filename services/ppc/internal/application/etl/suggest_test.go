package etl

import (
	"context"
	"testing"
	"time"
)

// stubSuggestRepo is a configurable stub for SuggestRepo.
type stubSuggestRepo struct {
	overrideQty    float64
	overrideSource string
	overrideHas    bool

	gradeQty float64
	gradeHas bool

	normalBobs      int
	fullBobbins     int
	unfullBobbins   int
	transferredBobs int
	totalBobbins    int
	weightPerBob    float64
	bobbinsHas      bool

	stdFull   float64
	stdUnfull float64
	stdOK     bool
}

func (s *stubSuggestRepo) ProductionActualOverride(_ context.Context, _ int64, _ time.Time, _ string) (float64, string, bool, error) {
	return s.overrideQty, s.overrideSource, s.overrideHas, nil
}

func (s *stubSuggestRepo) GradeActualQtyKg(_ context.Context, _ int64) (float64, bool, error) {
	return s.gradeQty, s.gradeHas, nil
}

func (s *stubSuggestRepo) ProductionActualBobbins(_ context.Context, _ int64, _ time.Time, _ string) (int, int, int, int, int, float64, bool, error) {
	return s.normalBobs, s.fullBobbins, s.unfullBobbins, s.transferredBobs, s.totalBobbins, s.weightPerBob, s.bobbinsHas, nil
}

func (s *stubSuggestRepo) LotStdWeightsByWO(_ context.Context, _ int64) (float64, float64, bool, error) {
	return s.stdFull, s.stdUnfull, s.stdOK, nil
}

func TestSuggestChain(t *testing.T) {
	const (
		stdFull   = 4.0
		stdUnfull = 2.0
	)
	base := func() *stubSuggestRepo {
		return &stubSuggestRepo{stdFull: stdFull, stdUnfull: stdUnfull, stdOK: true}
	}

	tests := []struct {
		name       string
		repo       *stubSuggestRepo
		wantQty    float64
		wantSource SuggestSource
	}{
		{
			name: "manual override wins",
			repo: func() *stubSuggestRepo {
				r := base()
				r.overrideHas, r.overrideSource, r.overrideQty = true, "ADJUSTED", 123.5
				// data present but must be ignored
				r.gradeHas, r.gradeQty = true, 999
				return r
			}(),
			wantQty:    123.5,
			wantSource: SuggestManualOverride,
		},
		{
			name: "P1 packing done",
			repo: func() *stubSuggestRepo {
				r := base()
				r.gradeHas, r.gradeQty = true, 500.25
				return r
			}(),
			wantQty:    500.25,
			wantSource: SuggestPackingDone,
		},
		{
			name: "P2 QC released normal bobs",
			repo: func() *stubSuggestRepo {
				r := base()
				r.bobbinsHas = true
				r.normalBobs = 18 // 18 * 4.0 = 72
				return r
			}(),
			wantQty:    72.0,
			wantSource: SuggestQCReleased,
		},
		{
			name: "P3 SPG transferred",
			repo: func() *stubSuggestRepo {
				r := base()
				r.bobbinsHas = true
				r.transferredBobs = 10
				r.weightPerBob = 3.5 // 10 * 3.5 = 35
				return r
			}(),
			wantQty:    35.0,
			wantSource: SuggestSPGTransferred,
		},
		{
			name: "P4 TXT transferred bobbins",
			repo: func() *stubSuggestRepo {
				r := base()
				r.bobbinsHas = true
				r.totalBobbins = 20
				r.fullBobbins = 18  // 18*4 = 72
				r.unfullBobbins = 2 // 2*2 = 4 -> 76
				return r
			}(),
			wantQty:    76.0,
			wantSource: SuggestTXTTransferred,
		},
		{
			name:       "NO_DATA when nothing present",
			repo:       base(),
			wantQty:    0,
			wantSource: SuggestNoData,
		},
		{
			name: "P5 doff estimate falls through to NO_DATA (phase 2)",
			repo: func() *stubSuggestRepo {
				r := base()
				r.bobbinsHas = true // row exists but all counts zero
				return r
			}(),
			wantQty:    0,
			wantSource: SuggestNoData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSuggestService(tt.repo)
			qty, src, err := svc.Suggest(context.Background(), 1, time.Now(), "1")
			if err != nil {
				t.Fatalf("Suggest returned error: %v", err)
			}
			if qty != tt.wantQty {
				t.Errorf("qty = %g, want %g", qty, tt.wantQty)
			}
			if src != tt.wantSource {
				t.Errorf("source = %d, want %d", src, tt.wantSource)
			}
		})
	}
}
