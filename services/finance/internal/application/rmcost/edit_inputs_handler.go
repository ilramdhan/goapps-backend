// Package rmcost — V2 inline edit handlers (no full recalc).
package rmcost

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// EditInputsHandler edits the per-row marketing snapshot inputs, simulation
// rate, and flags on a Cost row, then recomputes SP/PP/FP, cost_mark, cost_sim
// without touching the per-detail rows. CL/SL/FL stay as-is because they
// depend on the underlying details.
type EditInputsHandler struct {
	costRepo       rmcost.Repository
	costInputsRepo rmcost.CostInputsRepository
}

// NewEditInputsHandler constructs the handler.
func NewEditInputsHandler(costRepo rmcost.Repository, costInputsRepo rmcost.CostInputsRepository) *EditInputsHandler {
	return &EditInputsHandler{costRepo: costRepo, costInputsRepo: costInputsRepo}
}

// EditInputsCommand carries optional patches (nil = no change) and clear flags.
type EditInputsCommand struct {
	RMCostID uuid.UUID

	MarketingFreightRate    *float64
	MarketingAntiDumpingPct *float64
	MarketingDutyPct        *float64
	MarketingTransportRate  *float64
	MarketingDefaultValue   *float64
	SimulationRate          *float64
	ValuationFlag           *string
	MarketingFlag           *string

	ClearMarketingFreightRate    bool
	ClearMarketingAntiDumpingPct bool
	ClearMarketingDutyPct        bool
	ClearMarketingTransportRate  bool
	ClearMarketingDefaultValue   bool
	ClearSimulationRate          bool

	UpdatedBy string
}

// Handle applies the patches, recomputes SP/PP/FP/cost_mark/cost_sim, and
// returns the updated Cost.
func (h *EditInputsHandler) Handle(ctx context.Context, cmd EditInputsCommand) (*rmcost.Cost, error) {
	if cmd.UpdatedBy == "" {
		return nil, rmcost.ErrEmptyCalculatedBy
	}
	cost, err := h.costRepo.GetByID(ctx, cmd.RMCostID)
	if err != nil {
		return nil, fmt.Errorf("load cost: %w", err)
	}

	cur := currentV2Inputs(cost)
	applyEditPatches(&cur, cmd)

	// Build a HeaderInputsV2 from the patched values for the formulas.
	hv2 := HeaderInputsV2{
		MarketingFreightRate:    derefOrZero(cur.MarketingFreightRate),
		MarketingAntiDumpingPct: derefOrZero(cur.MarketingAntiDumpingPct),
		MarketingDutyPct:        derefOrZero(cur.MarketingDutyPct),
		MarketingTransportRate:  derefOrZero(cur.MarketingTransportRate),
		MarketingDefaultValue:   derefOrZero(cur.MarketingDefaultValue),
		SimulationRate:          derefOrZero(cur.SimulationRate),
		ValuationFlag:           cur.ValuationFlag,
		MarketingFlag:           cur.MarketingFlag,
	}
	if hv2.ValuationFlag == "" {
		hv2.ValuationFlag = "AUTO"
	}
	if hv2.MarketingFlag == "" {
		hv2.MarketingFlag = "AUTO"
	}

	// Use existing CL/SL/FL/CR/SR/PR group totals (they don't change here).
	tot := totalsFromCost(cost)
	proj := ComputeMarketingProjections(tot, hv2)
	costSim := ComputeSimulation(hv2.SimulationRate, hv2)
	costMkt := SelectMarketing(proj, hv2.MarketingFlag)

	v2Rates := buildV2Rates(tot, proj)

	if err := h.costInputsRepo.UpdateInputs(ctx, cost.ID(), cur, v2Rates, costMkt, costSim, cmd.UpdatedBy); err != nil {
		return nil, fmt.Errorf("update inputs: %w", err)
	}
	// Re-load to return the canonical row.
	updated, err := h.costRepo.GetByID(ctx, cmd.RMCostID)
	if err != nil {
		return nil, fmt.Errorf("reload cost: %w", err)
	}
	return updated, nil
}

// currentV2Inputs returns the current V2 inputs of cost, defaulting to AUTO flags
// when none persisted.
func currentV2Inputs(cost *rmcost.Cost) rmcost.V2Inputs {
	in := cost.V2Inputs()
	if in == nil {
		return rmcost.V2Inputs{ValuationFlag: "AUTO", MarketingFlag: "AUTO"}
	}
	cp := *in
	if cp.ValuationFlag == "" {
		cp.ValuationFlag = "AUTO"
	}
	if cp.MarketingFlag == "" {
		cp.MarketingFlag = "AUTO"
	}
	return cp
}

func applyEditPatches(cur *rmcost.V2Inputs, cmd EditInputsCommand) {
	cur.MarketingFreightRate = patchFloat(cur.MarketingFreightRate, cmd.MarketingFreightRate, cmd.ClearMarketingFreightRate)
	cur.MarketingAntiDumpingPct = patchFloat(cur.MarketingAntiDumpingPct, cmd.MarketingAntiDumpingPct, cmd.ClearMarketingAntiDumpingPct)
	cur.MarketingDutyPct = patchFloat(cur.MarketingDutyPct, cmd.MarketingDutyPct, cmd.ClearMarketingDutyPct)
	cur.MarketingTransportRate = patchFloat(cur.MarketingTransportRate, cmd.MarketingTransportRate, cmd.ClearMarketingTransportRate)
	cur.MarketingDefaultValue = patchFloat(cur.MarketingDefaultValue, cmd.MarketingDefaultValue, cmd.ClearMarketingDefaultValue)
	cur.SimulationRate = patchFloat(cur.SimulationRate, cmd.SimulationRate, cmd.ClearSimulationRate)
	if cmd.ValuationFlag != nil {
		cur.ValuationFlag = *cmd.ValuationFlag
	}
	if cmd.MarketingFlag != nil {
		cur.MarketingFlag = *cmd.MarketingFlag
	}
}

func patchFloat(cur, in *float64, clear bool) *float64 {
	if clear {
		return nil
	}
	if in == nil {
		return cur
	}
	v := *in
	return &v
}

func totalsFromCost(cost *rmcost.Cost) GroupTotals {
	v := cost.V2Rates()
	if v == nil {
		return GroupTotals{}
	}
	return GroupTotals{
		CR: derefOrZero(v.CR),
		SR: derefOrZero(v.SR),
		PR: derefOrZero(v.PR),
		CL: derefOrZero(v.CL),
		SL: derefOrZero(v.SL),
		FL: derefOrZero(v.FL),
	}
}

func buildV2Rates(tot GroupTotals, proj MarketingProjections) rmcost.V2Rates {
	return rmcost.V2Rates{
		CR: nilIfZero(tot.CR), SR: nilIfZero(tot.SR), PR: nilIfZero(tot.PR),
		CL: nilIfZero(tot.CL), SL: nilIfZero(tot.SL), FL: nilIfZero(tot.FL),
		SP: nilIfZero(proj.SP), PP: nilIfZero(proj.PP), FP: nilIfZero(proj.FP),
	}
}
