// Package rmcost provides application layer handlers for RM landed-cost calculation jobs.
package rmcost

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// SourceDataReader loads the per-stage consumption/stock/PO records that feed the
// landed-cost engine. Implementations read from `cst_item_cons_stk_po` filtered
// to (period, item_codes) and return the per-stage qty/val pointers needed by
// rmcost.AggregateRates.
type SourceDataReader interface {
	FetchRateInputs(ctx context.Context, period string, itemCodes []string) ([]rmcost.RateInputs, int, error)
	// FetchItemUOMs returns a map of item_code -> uom for the given period, for
	// items whose uom column is non-empty in the sync feed. Used as a fallback
	// when the rm_group detail rows were created without a UOM.
	FetchItemUOMs(ctx context.Context, period string, itemCodes []string) (map[string]string, error)
}

// CalculateCommand requests calculation for one group (GroupHeadID non-nil) or
// all active groups (GroupHeadID nil) in the given period.
type CalculateCommand struct {
	Period        string
	GroupHeadID   *uuid.UUID
	JobID         *uuid.UUID
	TriggerReason rmcost.HistoryTriggerReason
	CalculatedBy  string
}

// CalculateResult summarises the outcome of a calculation pass.
type CalculateResult struct {
	Period    string
	Processed int
	Skipped   int
	Costs     []*rmcost.Cost
}

// CalculateHandler runs the full pipeline per group and persists the result.
type CalculateHandler struct {
	groupRepo rmgroup.Repository
	costRepo  rmcost.Repository
	source    SourceDataReader
}

// NewCalculateHandler builds a CalculateHandler.
func NewCalculateHandler(
	groupRepo rmgroup.Repository,
	costRepo rmcost.Repository,
	source SourceDataReader,
) *CalculateHandler {
	return &CalculateHandler{groupRepo: groupRepo, costRepo: costRepo, source: source}
}

// Handle validates inputs, expands the target group list, and processes each one
// independently. A group whose active-detail list is empty produces a row with
// all-zero rates — this matches the plan's §6 "all-zero edge case" behaviour.
func (h *CalculateHandler) Handle(ctx context.Context, cmd CalculateCommand) (*CalculateResult, error) {
	if cmd.CalculatedBy == "" {
		return nil, rmcost.ErrEmptyCalculatedBy
	}
	if err := rmcost.ValidatePeriod(cmd.Period); err != nil {
		return nil, err
	}
	reason := cmd.TriggerReason
	if reason == "" {
		reason = rmcost.TriggerManualUI
	}
	if !reason.IsValid() {
		return nil, fmt.Errorf("invalid trigger reason %q", cmd.TriggerReason)
	}

	heads, err := h.resolveTargets(ctx, cmd.GroupHeadID)
	if err != nil {
		return nil, err
	}

	result := &CalculateResult{Period: cmd.Period}
	for _, head := range heads {
		if !head.IsActive() || head.IsDeleted() {
			result.Skipped++
			continue
		}
		cost, err := h.processHead(ctx, head, cmd.Period, cmd.JobID, reason, cmd.CalculatedBy)
		if err != nil {
			return nil, fmt.Errorf("process head %s: %w", head.Code(), err)
		}
		result.Costs = append(result.Costs, cost)
		result.Processed++
	}
	return result, nil
}

// resolveTargets returns the list of heads to calculate. When groupHeadID is
// non-nil only that head is loaded; otherwise every active head is fetched via
// the repository list API in batches.
func (h *CalculateHandler) resolveTargets(ctx context.Context, groupHeadID *uuid.UUID) ([]*rmgroup.Head, error) {
	if groupHeadID != nil {
		head, err := h.groupRepo.GetHeadByID(ctx, *groupHeadID)
		if err != nil {
			return nil, fmt.Errorf("load head %s: %w", *groupHeadID, err)
		}
		return []*rmgroup.Head{head}, nil
	}

	active := true
	page := 1
	const pageSize = 100
	var all []*rmgroup.Head
	for {
		filter := rmgroup.ListFilter{IsActive: &active, Page: page, PageSize: pageSize, SortBy: "code", SortOrder: "asc"}
		filter.Validate()
		heads, total, err := h.groupRepo.ListHeads(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("list heads page %d: %w", page, err)
		}
		all = append(all, heads...)
		if int64(len(all)) >= total || len(heads) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// processHead runs one calculation pass for a single group head: load active
// details, fetch source rows, calculate, upsert cost + history in one tx.
func (h *CalculateHandler) processHead(
	ctx context.Context,
	head *rmgroup.Head,
	period string,
	jobID *uuid.UUID,
	reason rmcost.HistoryTriggerReason,
	calculatedBy string,
) (*rmcost.Cost, error) {
	details, err := h.groupRepo.ListActiveDetailsByHeadID(ctx, head.ID())
	if err != nil {
		return nil, fmt.Errorf("list active details: %w", err)
	}

	itemCodes := make([]string, 0, len(details))
	for _, d := range details {
		if d.IsDummy() {
			continue
		}
		itemCodes = append(itemCodes, d.ItemCode().String())
	}

	inputs, sourceCount, err := h.fetchInputs(ctx, period, itemCodes)
	if err != nil {
		return nil, err
	}

	header := toHeaderInputs(head)
	computed := rmcost.CalculateCost(inputs, header)

	// Pick a representative UOM from the first non-dummy detail with a non-empty
	// UOM code. Groups usually hold items of the same unit; this gives operators
	// a useful display value instead of "—". When all details were added without
	// a UOM, fall back to looking up the UOM from the sync feed by item_code.
	uomCode := pickGroupUOM(details)
	if uomCode == "" && len(itemCodes) > 0 {
		uomCode, err = h.lookupUOMFromSource(ctx, period, itemCodes)
		if err != nil {
			return nil, err
		}
	}

	cost, err := h.buildOrUpdateCost(ctx, head, period, uomCode, computed, calculatedBy)
	if err != nil {
		return nil, err
	}

	hist := buildHistory(cost, head, computed, sourceCount, jobID, reason, calculatedBy)
	if err := h.costRepo.Upsert(ctx, cost, hist); err != nil {
		return nil, fmt.Errorf("upsert cost + history: %w", err)
	}
	return cost, nil
}

func (h *CalculateHandler) fetchInputs(ctx context.Context, period string, itemCodes []string) ([]rmcost.RateInputs, int, error) {
	if len(itemCodes) == 0 {
		return nil, 0, nil
	}
	inputs, n, err := h.source.FetchRateInputs(ctx, period, itemCodes)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch rate inputs: %w", err)
	}
	return inputs, n, nil
}

// buildOrUpdateCost loads the existing (period, rm_code) row if any and applies
// the fresh Computed values; otherwise constructs a brand-new Cost aggregate.
func (h *CalculateHandler) buildOrUpdateCost(
	ctx context.Context,
	head *rmgroup.Head,
	period, uomCode string,
	computed rmcost.Computed,
	calculatedBy string,
) (*rmcost.Cost, error) {
	existing, err := h.costRepo.GetByPeriodAndCode(ctx, period, head.Code().String())
	if err != nil && !errors.Is(err, rmcost.ErrNotFound) {
		return nil, fmt.Errorf("lookup existing cost: %w", err)
	}
	if existing != nil {
		if err := existing.ApplyComputed(computed, calculatedBy); err != nil {
			return nil, fmt.Errorf("apply computed: %w", err)
		}
		if uomCode != "" {
			existing.SetUOMCode(uomCode)
		}
		return existing, nil
	}
	cost, err := rmcost.NewGroupCost(period, head.Code().String(), head.ID(), head.Name(), uomCode, computed, calculatedBy)
	if err != nil {
		return nil, fmt.Errorf("new cost: %w", err)
	}
	return cost, nil
}

// lookupUOMFromSource returns the first non-empty UOM from cst_item_cons_stk_po
// for the given period+item_codes. Used to backfill the display UOM when none
// of the rm_group details carry one. Returns "" when the feed has no UOM for
// any of the items (not an error).
func (h *CalculateHandler) lookupUOMFromSource(ctx context.Context, period string, itemCodes []string) (string, error) {
	uoms, err := h.source.FetchItemUOMs(ctx, period, itemCodes)
	if err != nil {
		return "", fmt.Errorf("fetch item uoms: %w", err)
	}
	// Prefer order of the detail list for determinism.
	for _, code := range itemCodes {
		if u := uoms[code]; u != "" {
			return u, nil
		}
	}
	return "", nil
}

// pickGroupUOM returns the UOM code from the first non-dummy detail whose
// UOMCode is non-empty. Returns "" when no candidate exists.
func pickGroupUOM(details []*rmgroup.Detail) string {
	for _, d := range details {
		if d.IsDummy() {
			continue
		}
		if u := d.UOMCode(); u != "" {
			return u
		}
	}
	return ""
}

// toHeaderInputs maps the rmgroup.Head fields required by the calc engine. The
// rmcost package never imports rmgroup — this adapter lives in the application
// layer so the domain boundary stays clean.
func toHeaderInputs(head *rmgroup.Head) rmcost.HeaderInputs {
	return rmcost.HeaderInputs{
		CostPercentage:    head.CostPercentage(),
		CostPerKg:         head.CostPerKg(),
		FlagValuation:     rmcost.Stage(head.FlagValuation()),
		FlagMarketing:     rmcost.Stage(head.FlagMarketing()),
		FlagSimulation:    rmcost.Stage(head.FlagSimulation()),
		InitValValuation:  head.InitValValuation(),
		InitValMarketing:  head.InitValMarketing(),
		InitValSimulation: head.InitValSimulation(),
	}
}

func buildHistory(
	cost *rmcost.Cost,
	head *rmgroup.Head,
	computed rmcost.Computed,
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
		Rates:              computed.Rates,
		CostPercentage:     head.CostPercentage(),
		CostPerKg:          head.CostPerKg(),
		FlagValuation:      computed.FlagValuation,
		FlagMarketing:      computed.FlagMarketing,
		FlagSimulation:     computed.FlagSimulation,
		InitValValuation:   head.InitValValuation(),
		InitValMarketing:   head.InitValMarketing(),
		InitValSimulation:  head.InitValSimulation(),
		CostValuation:      cost.CostValuation(),
		CostMarketing:      cost.CostMarketing(),
		CostSimulation:     cost.CostSimulation(),
		FlagValuationUsed:  computed.FlagValuationUsed,
		FlagMarketingUsed:  computed.FlagMarketingUsed,
		FlagSimulationUsed: computed.FlagSimulationUsed,
		SourceItemCount:    sourceCount,
		TriggerReason:      reason,
		CalculatedAt:       time.Now(),
		CalculatedBy:       calculatedBy,
	}
}
