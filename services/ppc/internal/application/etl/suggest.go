// Package etl implements the PPC Oracle ETL use cases (TXT/TWT production, sales
// order staging) and the WO-actual suggest chain. Application-layer code here
// must not import generated proto; the suggest source is a domain enum mapped to
// proto by the delivery layer.
package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shared"
)

// SuggestSource is the domain-level provenance of a suggested WO-actual quantity,
// mapped to ppc.v1.QtySource by the gRPC delivery layer.
type SuggestSource int

// Suggest source priorities (highest wins). Values mirror the proto QtySource
// ordering but stay independent of generated code.
const (
	SuggestNoData         SuggestSource = iota // No data available.
	SuggestManualOverride                      // Operator ADJUSTED value; never overwritten.
	SuggestPackingDone                         // P1: wo_grade_actual sum (packing done).
	SuggestQCReleased                          // P2: TXT normal bobs QC released.
	SuggestSPGTransferred                      // P3: SPG transferred bobs (Phase 2).
	SuggestTXTTransferred                      // P4: TXT transferred bobbins.
	SuggestDoffEstimate                        // P5: SPG doff estimate (Phase 2).
)

// qtySourceAdjusted is the wpa_qty_source marker for operator-adjusted rows.
const qtySourceAdjusted = "ADJUSTED"

// SuggestRepo is the narrow read surface the SuggestService needs. Implemented by
// postgres.ETLRepository; stubbed in tests.
type SuggestRepo interface {
	ProductionActualOverride(ctx context.Context, woID int64, date time.Time, shift string) (qtyActual float64, source string, has bool, err error)
	GradeActualQtyKg(ctx context.Context, woID int64) (qty float64, has bool, err error)
	ProductionActualBobbins(ctx context.Context, woID int64, date time.Time, shift string) (normalBobs, fullBobbins, unfullBobbins, transferredBobs, totalBobbins int, weightPerBob float64, has bool, err error)
	LotStdWeightsByWO(ctx context.Context, woID int64) (full, unfull float64, ok bool, err error)
}

// SuggestService computes the suggested WO-actual quantity via the priority chain
// (design §6). Manual overrides always win; otherwise the highest-confidence data
// source available is used.
type SuggestService struct {
	repo SuggestRepo
}

// NewSuggestService builds the suggest service.
func NewSuggestService(repo SuggestRepo) *SuggestService {
	return &SuggestService{repo: repo}
}

// Suggest returns the suggested quantity in kilograms and its provenance for a
// (wo, date, shift). It applies, in order: manual override, P1 packing-done, P2
// TXT QC-released, P3 SPG transferred, P4 TXT transferred, P5 doff estimate, then
// NO_DATA.
func (s *SuggestService) Suggest(ctx context.Context, woID int64, date time.Time, shift string) (float64, SuggestSource, error) {
	if qty, src, done, err := s.checkOverride(ctx, woID, date, shift); err != nil || done {
		return qty, src, err
	}
	if qty, has, err := s.repo.GradeActualQtyKg(ctx, woID); err != nil {
		return 0, SuggestNoData, err
	} else if has {
		return qty, SuggestPackingDone, nil
	}
	return s.suggestFromBobbins(ctx, woID, date, shift)
}

// checkOverride returns done=true when an operator ADJUSTED value is present and
// must win over the data chain.
func (s *SuggestService) checkOverride(ctx context.Context, woID int64, date time.Time, shift string) (float64, SuggestSource, bool, error) {
	qtyActual, source, has, err := s.repo.ProductionActualOverride(ctx, woID, date, shift)
	if err != nil {
		return 0, SuggestNoData, false, err
	}
	if has && source == qtySourceAdjusted {
		return qtyActual, SuggestManualOverride, true, nil
	}
	return 0, SuggestNoData, false, nil
}

// suggestFromBobbins applies the bobbin-based branches P2-P5.
func (s *SuggestService) suggestFromBobbins(ctx context.Context, woID int64, date time.Time, shift string) (float64, SuggestSource, error) {
	normalBobs, fullBobbins, unfullBobbins, transferredBobs, totalBobbins, weightPerBob, has, err :=
		s.repo.ProductionActualBobbins(ctx, woID, date, shift)
	if err != nil {
		return 0, SuggestNoData, err
	}
	if !has {
		return 0, SuggestNoData, nil
	}

	stdFull, stdUnfull, ok, err := s.repo.LotStdWeightsByWO(ctx, woID)
	if err != nil {
		return 0, SuggestNoData, err
	}
	if !ok {
		return 0, SuggestNoData, fmt.Errorf("suggest wo=%d: lot std weights missing", woID)
	}

	// P2: TXT normal bobs QC released.
	if normalBobs > 0 {
		return shared.QtyTXTNormal(normalBobs, 0, stdFull, stdUnfull), SuggestQCReleased, nil
	}
	// P3: SPG transferred bobs (Phase 2 data; usually 0 until SPG ETL lands).
	if transferredBobs > 0 {
		return float64(transferredBobs) * weightPerBob, SuggestSPGTransferred, nil
	}
	// P4: TXT transferred bobbins (full + unfull by std weight).
	if totalBobbins > 0 {
		return shared.QtyTXT(fullBobbins, unfullBobbins, stdFull, stdUnfull), SuggestTXTTransferred, nil
	}
	// P5: SPG doff estimate — Phase 2, no data yet. Falls through to NO_DATA.
	return 0, SuggestNoData, nil
}
