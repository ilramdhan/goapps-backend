// Package rmcost — V2 fix_rate inline edit handler. Updates one detail row's
// fix_rate, recomputes the FL chain for that detail, recomputes parent
// fl_rate (= MAX), and (when valuation_flag is AUTO/FL) refreshes cost_val.
package rmcost

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// EditFixRateHandler handles per-detail fix_rate edits.
type EditFixRateHandler struct {
	costRepo       rmcost.Repository
	costDetailRepo rmcost.CostDetailRepository
	costInputsRepo rmcost.CostInputsRepository
}

// NewEditFixRateHandler constructs the handler.
func NewEditFixRateHandler(
	costRepo rmcost.Repository,
	costDetailRepo rmcost.CostDetailRepository,
	costInputsRepo rmcost.CostInputsRepository,
) *EditFixRateHandler {
	return &EditFixRateHandler{
		costRepo:       costRepo,
		costDetailRepo: costDetailRepo,
		costInputsRepo: costInputsRepo,
	}
}

// EditFixRateCommand carries the single detail edit. FixRate=nil means "clear".
type EditFixRateCommand struct {
	CostDetailID uuid.UUID
	FixRate      *float64
	UpdatedBy    string
}

// EditFixRateResult bundles the updated detail and the recomputed parent cost.
type EditFixRateResult struct {
	Detail *rmcost.CostDetail
	Cost   *rmcost.Cost
}

// Handle applies the patch, recomputes the FL chain on the detail, and updates
// fl_rate / cost_val on the parent.
func (h *EditFixRateHandler) Handle(ctx context.Context, cmd EditFixRateCommand) (*EditFixRateResult, error) {
	if cmd.UpdatedBy == "" {
		return nil, rmcost.ErrEmptyCalculatedBy
	}
	detail, err := h.costDetailRepo.GetByID(ctx, cmd.CostDetailID)
	if err != nil {
		return nil, fmt.Errorf("load cost detail: %w", err)
	}

	// Recompute the fix-stage chain using the detail's stored inputs.
	snap := detail.Snapshot()
	in := DetailInputs{
		FreightRate:           derefOrZero(snap.FreightRate),
		AntiDumpingPct:        derefOrZero(snap.AntiDumpingPct),
		DutyPct:               derefOrZero(snap.DutyPct),
		TransportRate:         derefOrZero(snap.TransportRate),
		ValuationDefaultValue: derefOrZero(cmd.FixRate),
	}
	out := ComputeDetail(in, SourceQty{}) // only fix-stage matters here

	snap.ValuationDefaultValue = cmd.FixRate
	snap.FixRate = nilIfZero(out.FixRate)
	snap.FixFreightRate = nilIfZero(out.FixFreightRate)
	snap.FixRateBased = nilIfZero(out.FixRateBased)
	snap.FixAntiDumpingRate = nilIfZero(out.FixAntiDumpingRate)
	snap.FixDutyRate = nilIfZero(out.FixDutyRate)
	snap.FixTransportRate = nilIfZero(out.FixTransportRate)
	snap.FixLandedCost = nilIfZero(out.FixLandedCost)

	detail.AttachSnapshot(snap)
	detail.MarkUpdated(cmd.UpdatedBy)

	if err := h.costDetailRepo.UpdateSnapshot(ctx, detail); err != nil {
		return nil, fmt.Errorf("save detail: %w", err)
	}

	// Recompute parent FL = MAX(detail.fix_landed_cost) across the cost row.
	siblings, err := h.costDetailRepo.ListByCostID(ctx, detail.CostID())
	if err != nil {
		return nil, fmt.Errorf("list siblings: %w", err)
	}
	flMax := 0.0
	for _, s := range siblings {
		if v := derefOrZero(s.Snapshot().FixLandedCost); v > flMax {
			flMax = v
		}
	}

	// Recompute cost_val if flag is AUTO or FL. Otherwise leave as-is.
	cost, err := h.costRepo.GetByID(ctx, detail.CostID())
	if err != nil {
		return nil, fmt.Errorf("load cost: %w", err)
	}
	flag := "AUTO"
	if cost.V2Inputs() != nil {
		flag = cost.V2Inputs().ValuationFlag
	}

	var newCostVal *float64
	tot := totalsFromCost(cost)
	tot.FL = flMax
	if flag == "AUTO" || flag == "FL" {
		v := SelectValuation(tot, flag)
		newCostVal = &v
	} else {
		// Keep existing cost_val.
		newCostVal = cost.CostValuation()
	}

	// Persist fl_rate (and cost_val when relevant).
	if err := h.costInputsRepo.UpdateFLAndCostVal(ctx, cost.ID(), flMax, newCostVal, cmd.UpdatedBy); err != nil {
		return nil, fmt.Errorf("update fl/cost_val: %w", err)
	}

	updated, err := h.costRepo.GetByID(ctx, cost.ID())
	if err != nil {
		return nil, fmt.Errorf("reload cost: %w", err)
	}
	return &EditFixRateResult{Detail: detail, Cost: updated}, nil
}

// roundForLog kept here to avoid importing math in callers that don't need it.
//
//nolint:deadcode,unused // utility kept for diagnostic logging
func roundForLog(v float64, places int) float64 {
	mult := math.Pow(10, float64(places))
	return math.Round(v*mult) / mult
}
