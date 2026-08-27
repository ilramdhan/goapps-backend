package mbdozing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DefaultImpactLimit is the row cap applied when the caller sends limit = 0.
const DefaultImpactLimit = 20

// ImpactCommand identifies the MB spin whose downstream impact is previewed.
type ImpactCommand struct {
	// MBSID is the MB spin UUID.
	MBSID uuid.UUID
	// Limit caps the returned rows. 0 means DefaultImpactLimit.
	Limit int
}

// ImpactResult is the outcome of the preview use case.
type ImpactResult struct {
	// Rows are the affected products, already capped by the limit.
	Rows []mbdozing.ImpactRow
	// Totals are the un-truncated counts.
	Totals mbdozing.Totals
	// Truncated reports that Totals.TotalAffected exceeds len(Rows).
	Truncated bool
	// Note reports a data anomaly that made the preview inconclusive; empty
	// when the preview is complete.
	Note string
}

// ImpactPreviewHandler resolves an MB spin and reads its downstream products.
//
// READ-ONLY (user decision K-18): it issues SELECTs only.
type ImpactPreviewHandler struct {
	spinRepo   mbspin.Repository
	impactRepo mbdozing.ImpactRepository
}

// NewImpactPreviewHandler constructs an ImpactPreviewHandler.
func NewImpactPreviewHandler(spinRepo mbspin.Repository, impactRepo mbdozing.ImpactRepository) *ImpactPreviewHandler {
	return &ImpactPreviewHandler{spinRepo: spinRepo, impactRepo: impactRepo}
}

// Handle resolves the spin to its ORION item code and previews affected products.
func (h *ImpactPreviewHandler) Handle(ctx context.Context, cmd ImpactCommand) (*ImpactResult, error) {
	limit := cmd.Limit
	if limit <= 0 {
		limit = DefaultImpactLimit
	}

	spin, err := h.spinRepo.GetByID(ctx, cmd.MBSID)
	if err != nil {
		return nil, err
	}

	// Products bind to a spin through its ORION item code (parameter MB_SP_CODE,
	// bound to the MB_SPIN lookup master). A spin without one cannot be joined
	// to any product — report that as a note rather than guessing another key or
	// returning a misleading empty-but-complete result.
	code := spin.OrionItemCode()
	if code == nil || *code == "" {
		return &ImpactResult{
			Rows: []mbdozing.ImpactRow{},
			Note: fmt.Sprintf(
				"MB spin %s has no ORION item code, so no cost products can be linked to it; the impact list is inconclusive rather than empty",
				cmd.MBSID),
		}, nil
	}

	rows, totals, err := h.impactRepo.ImpactBySpin(ctx, *code, limit)
	if err != nil {
		return nil, err
	}

	return &ImpactResult{
		Rows:      rows,
		Totals:    totals,
		Truncated: totals.TotalAffected > len(rows),
	}, nil
}
