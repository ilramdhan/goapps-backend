// Package rmcost — V2 calculation handler. Runs the V2 per-detail engine,
// snapshots intermediates to cst_rm_cost_detail, and persists V2 columns on
// cst_rm_cost. The V1 handler (calculate_handler.go) is left untouched for
// the gRPC paths that still call it; the worker can switch to V2 incrementally.
package rmcost

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// V2SourceReader fetches per-(item, grade) source qty/val for the V2 engine.
// Implemented by postgres.SyncDataRepository.
type V2SourceReader interface {
	FetchSourceQtyByItemGrade(ctx context.Context, period string, keys []postgres.ItemGradeKey) (map[postgres.ItemGradeKey]postgres.V2SourceQty, error)
}

// CalculateHandlerV2 runs the V2 engine for one group head + period.
type CalculateHandlerV2 struct {
	groupRepo      rmgroup.Repository
	costRepo       rmcost.Repository
	costDetailRepo rmcost.CostDetailRepository
	source         V2SourceReader
	uomSource      SourceDataReader // reused for FetchItemUOMs fallback
}

// NewCalculateHandlerV2 constructs the V2 handler.
func NewCalculateHandlerV2(
	groupRepo rmgroup.Repository,
	costRepo rmcost.Repository,
	costDetailRepo rmcost.CostDetailRepository,
	source V2SourceReader,
	uomSource SourceDataReader,
) *CalculateHandlerV2 {
	return &CalculateHandlerV2{
		groupRepo:      groupRepo,
		costRepo:       costRepo,
		costDetailRepo: costDetailRepo,
		source:         source,
		uomSource:      uomSource,
	}
}

// HandleOneGroup runs the V2 engine for a single group head and period.
// Returns the upserted Cost row. Detail snapshots are persisted via UpsertAll.
//
//nolint:gocognit // Pipeline is procedural by design — pure cyclo cost.
func (h *CalculateHandlerV2) HandleOneGroup(
	ctx context.Context,
	headID uuid.UUID,
	period, calculatedBy string,
) (*rmcost.Cost, error) {
	if err := rmcost.ValidatePeriod(period); err != nil {
		return nil, err
	}
	if calculatedBy == "" {
		return nil, rmcost.ErrEmptyCalculatedBy
	}

	head, err := h.groupRepo.GetHeadByID(ctx, headID)
	if err != nil {
		return nil, fmt.Errorf("load head: %w", err)
	}
	details, err := h.groupRepo.ListActiveDetailsByHeadID(ctx, headID)
	if err != nil {
		return nil, fmt.Errorf("list details: %w", err)
	}

	keys := make([]postgres.ItemGradeKey, 0, len(details))
	itemCodes := make([]string, 0, len(details))
	for _, d := range details {
		if d.IsDummy() {
			continue
		}
		keys = append(keys, postgres.ItemGradeKey{
			ItemCode:  d.ItemCode().String(),
			GradeCode: d.GradeCode(),
		})
		itemCodes = append(itemCodes, d.ItemCode().String())
	}
	srcMap, err := h.source.FetchSourceQtyByItemGrade(ctx, period, keys)
	if err != nil {
		return nil, fmt.Errorf("fetch v2 source: %w", err)
	}

	// Build per-detail outputs.
	outs := make([]DetailOutput, 0, len(details))
	for _, d := range details {
		if d.IsDummy() {
			continue
		}
		key := postgres.ItemGradeKey{ItemCode: d.ItemCode().String(), GradeCode: d.GradeCode()}
		src := srcMap[key] // zero V2SourceQty when not found
		in := detailInputsFromGroupDetail(d)
		out := ComputeDetail(in, V2SourceFromPG(src))
		outs = append(outs, out)
	}

	totals := AggregateGroupTotals(outs)

	// Determine simulation_rate: preserve from existing cost row if any (user edits).
	existing, err := h.costRepo.GetByPeriodAndCode(ctx, period, head.Code().String())
	if err != nil && !errors.Is(err, rmcost.ErrNotFound) {
		return nil, fmt.Errorf("lookup existing cost: %w", err)
	}
	simRate := 0.0
	if existing != nil && existing.V2Inputs() != nil && existing.V2Inputs().SimulationRate != nil {
		simRate = *existing.V2Inputs().SimulationRate
	}

	// Build V2 header inputs from the rmgroup head + the existing simulation rate.
	hv2 := headerInputsV2FromHead(head, simRate)

	// Marketing projections, simulation, selection.
	proj := ComputeMarketingProjections(totals, hv2)
	costSim := ComputeSimulation(simRate, hv2)
	costVal := SelectValuation(totals, hv2.ValuationFlag)
	costMkt := SelectMarketing(proj, hv2.MarketingFlag)

	// UOM resolution (reuse V1 logic — pick from details, fallback to feed).
	uomCode := pickGroupUOM(details)
	if uomCode == "" && len(itemCodes) > 0 {
		if uoms, e := h.uomSource.FetchItemUOMs(ctx, period, itemCodes); e == nil {
			for _, c := range itemCodes {
				if u := uoms[c]; u != "" {
					uomCode = u
					break
				}
			}
		}
	}

	// Build or apply on Cost.
	cost, err := h.buildOrApplyCost(existing, head, period, uomCode, totals, proj, hv2, costVal, costMkt, costSim, calculatedBy)
	if err != nil {
		return nil, err
	}

	// Persist Cost (V2 columns flow through upsertCost).
	hist := buildHistoryV2(cost, head, totals, len(outs), nil, rmcost.TriggerManualUI, calculatedBy)
	if err := h.costRepo.Upsert(ctx, cost, hist); err != nil {
		return nil, fmt.Errorf("upsert cost: %w", err)
	}

	// Build cost details bound to the now-known cost.ID().
	costDetails, err := rebuildCostDetailsWithCostID(details, outs, head.ID(), cost.ID(), period, calculatedBy)
	if err != nil {
		return nil, err
	}
	if err := h.costDetailRepo.UpsertAll(ctx, cost.ID(), costDetails); err != nil {
		return nil, fmt.Errorf("upsert cost details: %w", err)
	}
	return cost, nil
}

// buildOrApplyCost loads existing or creates a new Cost, then attaches V2 inputs/rates.
func (h *CalculateHandlerV2) buildOrApplyCost(
	existing *rmcost.Cost,
	head *rmgroup.Head,
	period, uomCode string,
	totals GroupTotals,
	proj MarketingProjections,
	hv2 HeaderInputsV2,
	costVal, costMkt, costSim float64,
	calculatedBy string,
) (*rmcost.Cost, error) {
	// Build a V1-Computed shim with the V2 cost values so existing schema fields
	// (cost_val/cost_mark/cost_sim, flag_*_used) stay populated for back-compat.
	v1Computed := rmcost.Computed{
		Rates:              rmcost.StageRates{}, // V1 stages unused in V2 path
		CostValuation:      costVal,
		CostMarketing:      costMkt,
		CostSimulation:     costSim,
		FlagValuation:      rmcost.Stage(head.FlagValuation()),
		FlagMarketing:      rmcost.Stage(head.FlagMarketing()),
		FlagSimulation:     rmcost.Stage(head.FlagSimulation()),
		FlagValuationUsed:  rmcost.Stage(head.FlagValuation()),
		FlagMarketingUsed:  rmcost.Stage(head.FlagMarketing()),
		FlagSimulationUsed: rmcost.Stage(head.FlagSimulation()),
	}

	v2In := rmcost.V2Inputs{
		MarketingFreightRate:    nilIfZero(hv2.MarketingFreightRate),
		MarketingAntiDumpingPct: nilIfZero(hv2.MarketingAntiDumpingPct),
		MarketingDutyPct:        nilIfZero(hv2.MarketingDutyPct),
		MarketingTransportRate:  nilIfZero(hv2.MarketingTransportRate),
		MarketingDefaultValue:   nilIfZero(hv2.MarketingDefaultValue),
		SimulationRate:          nilIfZero(hv2.SimulationRate),
		ValuationFlag:           hv2.ValuationFlag,
		MarketingFlag:           hv2.MarketingFlag,
	}
	v2Rates := rmcost.V2Rates{
		CR: nilIfZero(totals.CR), SR: nilIfZero(totals.SR), PR: nilIfZero(totals.PR),
		CL: nilIfZero(totals.CL), SL: nilIfZero(totals.SL), FL: nilIfZero(totals.FL),
		SP: nilIfZero(proj.SP), PP: nilIfZero(proj.PP), FP: nilIfZero(proj.FP),
	}

	if existing != nil {
		if err := existing.ApplyComputed(v1Computed, calculatedBy); err != nil {
			return nil, fmt.Errorf("apply computed: %w", err)
		}
		if uomCode != "" {
			existing.SetUOMCode(uomCode)
		}
		existing.AttachV2(v2In, v2Rates)
		return existing, nil
	}
	cost, err := rmcost.NewGroupCost(period, head.Code().String(), head.ID(), head.Name(), uomCode, v1Computed, calculatedBy)
	if err != nil {
		return nil, fmt.Errorf("new cost: %w", err)
	}
	cost.AttachV2(v2In, v2Rates)
	return cost, nil
}

// =============================================================================
// Helper conversions
// =============================================================================

func detailInputsFromGroupDetail(d *rmgroup.Detail) DetailInputs {
	in := d.ValuationInputs()
	return DetailInputs{
		FreightRate:           derefOrZero(in.FreightRate),
		AntiDumpingPct:        derefOrZero(in.AntiDumpingPct),
		DutyPct:               derefOrZero(in.DutyPct),
		TransportRate:         derefOrZero(in.TransportRate),
		ValuationDefaultValue: derefOrZero(in.DefaultValue),
	}
}

func headerInputsV2FromHead(h *rmgroup.Head, simRate float64) HeaderInputsV2 {
	mi := h.MarketingInputs()
	out := HeaderInputsV2{
		MarketingFreightRate:    derefOrZero(mi.FreightRate),
		MarketingAntiDumpingPct: derefOrZero(mi.AntiDumpingPct),
		MarketingDutyPct:        h.CostPercentage(),
		MarketingTransportRate:  h.CostPerKg(),
		MarketingDefaultValue:   derefOrZero(mi.DefaultValue),
		SimulationRate:          simRate,
		ValuationFlag:           string(mi.ValuationFlag),
		MarketingFlag:           string(mi.MarketingFlag),
	}
	if out.ValuationFlag == "" {
		out.ValuationFlag = "AUTO"
	}
	if out.MarketingFlag == "" {
		out.MarketingFlag = "AUTO"
	}
	return out
}

// V2SourceFromPG converts a postgres-layer V2SourceQty to the engine's SourceQty.
func V2SourceFromPG(src postgres.V2SourceQty) SourceQty {
	return SourceQty{
		ConsVal: src.ConsVal, ConsQty: src.ConsQty,
		StockVal: src.StockVal, StockQty: src.StockQty,
		POVal: src.POVal, POQty: src.POQty,
	}
}

func rebuildCostDetailsWithCostID(
	details []*rmgroup.Detail,
	outs []DetailOutput,
	headID, costID uuid.UUID,
	period, createdBy string,
) ([]*rmcost.CostDetail, error) {
	result := make([]*rmcost.CostDetail, 0, len(outs))
	idx := 0
	for _, d := range details {
		if d.IsDummy() {
			continue
		}
		out := outs[idx]
		idx++
		cd, err := rmcost.NewCostDetail(costID, headID, period, d.ItemCode().String(), d.ItemName(), d.GradeCode(), createdBy)
		if err != nil {
			return nil, fmt.Errorf("new cost detail: %w", err)
		}
		gid := d.ID()
		cd.SetGroupDetailID(&gid)
		cd.AttachSnapshot(detailOutputToSnapshot(out))
		result = append(result, cd)
	}
	return result, nil
}

func detailOutputToSnapshot(o DetailOutput) rmcost.CostDetailSnapshot {
	return rmcost.CostDetailSnapshot{
		FreightRate:           nilIfZero(o.Inputs.FreightRate),
		AntiDumpingPct:        nilIfZero(o.Inputs.AntiDumpingPct),
		DutyPct:               nilIfZero(o.Inputs.DutyPct),
		TransportRate:         nilIfZero(o.Inputs.TransportRate),
		ValuationDefaultValue: nilIfZero(o.Inputs.ValuationDefaultValue),
		ConsVal:               nilIfZero(o.Source.ConsVal),
		ConsQty:               nilIfZero(o.Source.ConsQty),
		ConsRate:              nilIfZero(o.ConsRate),
		ConsFreightVal:        nilIfZero(o.ConsFreightVal),
		ConsValBased:          nilIfZero(o.ConsValBased),
		ConsRateBased:         nilIfZero(o.ConsRateBased),
		ConsAntiDumpingVal:    nilIfZero(o.ConsAntiDumpingVal),
		ConsAntiDumpingRate:   nilIfZero(o.ConsAntiDumpingRate),
		ConsDutyVal:           nilIfZero(o.ConsDutyVal),
		ConsDutyRate:          nilIfZero(o.ConsDutyRate),
		ConsTransportVal:      nilIfZero(o.ConsTransportVal),
		ConsTransportRate:     nilIfZero(o.ConsTransportRate),
		ConsLandedCost:        nilIfZero(o.ConsLandedCost),
		StockVal:              nilIfZero(o.Source.StockVal),
		StockQty:              nilIfZero(o.Source.StockQty),
		StockRate:             nilIfZero(o.StockRate),
		StockFreightVal:       nilIfZero(o.StockFreightVal),
		StockValBased:         nilIfZero(o.StockValBased),
		StockRateBased:        nilIfZero(o.StockRateBased),
		StockAntiDumpingVal:   nilIfZero(o.StockAntiDumpingVal),
		StockAntiDumpingRate:  nilIfZero(o.StockAntiDumpingRate),
		StockDutyVal:          nilIfZero(o.StockDutyVal),
		StockDutyRate:         nilIfZero(o.StockDutyRate),
		StockTransportVal:     nilIfZero(o.StockTransportVal),
		StockTransportRate:    nilIfZero(o.StockTransportRate),
		StockLandedCost:       nilIfZero(o.StockLandedCost),
		POVal:                 nilIfZero(o.Source.POVal),
		POQty:                 nilIfZero(o.Source.POQty),
		PORate:                nilIfZero(o.PORate),
		FixRate:               nilIfZero(o.FixRate),
		FixFreightRate:        nilIfZero(o.FixFreightRate),
		FixRateBased:          nilIfZero(o.FixRateBased),
		FixAntiDumpingRate:    nilIfZero(o.FixAntiDumpingRate),
		FixDutyRate:           nilIfZero(o.FixDutyRate),
		FixTransportRate:      nilIfZero(o.FixTransportRate),
		FixLandedCost:         nilIfZero(o.FixLandedCost),
	}
}

func buildHistoryV2(
	cost *rmcost.Cost,
	head *rmgroup.Head,
	totals GroupTotals,
	sourceCount int,
	jobID *uuid.UUID,
	reason rmcost.HistoryTriggerReason,
	calculatedBy string,
) rmcost.History {
	costID := cost.ID()
	headID := head.ID()
	return rmcost.History{
		ID:                 uuid.New(),
		RMCostID:           &costID,
		JobID:              jobID,
		Period:             cost.Period(),
		RMCode:             cost.RMCode(),
		RMType:             cost.RMType(),
		GroupHeadID:        &headID,
		Rates:              cost.Rates(),
		CostPercentage:     head.CostPercentage(),
		CostPerKg:          head.CostPerKg(),
		FlagValuation:      cost.FlagValuation(),
		FlagMarketing:      cost.FlagMarketing(),
		FlagSimulation:     cost.FlagSimulation(),
		InitValValuation:   head.InitValValuation(),
		InitValMarketing:   head.InitValMarketing(),
		InitValSimulation:  head.InitValSimulation(),
		CostValuation:      cost.CostValuation(),
		CostMarketing:      cost.CostMarketing(),
		CostSimulation:     cost.CostSimulation(),
		FlagValuationUsed:  cost.FlagValuationUsed(),
		FlagMarketingUsed:  cost.FlagMarketingUsed(),
		FlagSimulationUsed: cost.FlagSimulationUsed(),
		SourceItemCount:    sourceCount,
		TriggerReason:      reason,
		CalculatedAt:       cost.CreatedAt(),
		CalculatedBy:       calculatedBy,
	}
}

func nilIfZero(v float64) *float64 {
	if v == 0 {
		return nil
	}
	x := v
	return &x
}

func derefOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
