// Package rmcost provides the landed-cost calculation engine and persistence contract
// for the RM cost aggregates produced from grouped raw-material consumption data.
package rmcost

import (
	"regexp"
	"time"

	"github.com/google/uuid"
)

// RMType distinguishes a group-level cost row from an item-level cost row.
// Mirrors the chk_rm_type CHECK constraint on cst_rm_cost.
type RMType string

const (
	// RMTypeGroup indicates the cost row aggregates a whole RM group.
	RMTypeGroup RMType = "GROUP"
	// RMTypeItem indicates the cost row is computed for a single item (future phase).
	RMTypeItem RMType = "ITEM"
)

// IsValid reports whether the RMType is one of the recognized values.
func (t RMType) IsValid() bool {
	switch t {
	case RMTypeGroup, RMTypeItem:
		return true
	default:
		return false
	}
}

// String returns the canonical string form.
func (t RMType) String() string { return string(t) }

// periodPattern validates a 6-character YYYYMM period (e.g. "202604").
var periodPattern = regexp.MustCompile(`^\d{6}$`)

// ValidatePeriod returns ErrInvalidPeriod when the supplied string is not a 6-digit YYYYMM value.
func ValidatePeriod(period string) error {
	if !periodPattern.MatchString(period) {
		return ErrInvalidPeriod
	}
	// Month digits must be 01-12.
	month := period[4:]
	if month < "01" || month > "12" {
		return ErrInvalidPeriod
	}
	return nil
}

// Cost is the aggregate root representing the computed landed cost for a single
// (period, rm_code) pair. It is populated by the worker and then upserted to
// `cst_rm_cost`; callers retrieve it via the Repository to display in the UI.
//
//nolint:revive // Wide struct mirrors the persistence row one-for-one.
type Cost struct {
	id          uuid.UUID
	period      string
	rmCode      string
	rmType      RMType
	groupHeadID *uuid.UUID
	itemCode    *string
	rmName      string
	uomCode     string

	// Per-stage snapshot of the aggregated rates (before selection).
	rates StageRates

	// Landed cost per purpose (raw — UI formats for display).
	costValuation  *float64
	costMarketing  *float64
	costSimulation *float64

	// Configured flags at calc time.
	flagValuation  Stage
	flagMarketing  Stage
	flagSimulation Stage

	// Stages actually used after cascade/INIT resolution.
	flagValuationUsed  Stage
	flagMarketingUsed  Stage
	flagSimulationUsed Stage

	calculatedAt *time.Time
	calculatedBy *string

	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
}

// Computed carries the output of one calculation pass. Construct via CalculateCost,
// then pass to NewCost / ApplyComputed to persist.
type Computed struct {
	Rates              StageRates
	CostValuation      float64
	CostMarketing      float64
	CostSimulation     float64
	FlagValuation      Stage
	FlagMarketing      Stage
	FlagSimulation     Stage
	FlagValuationUsed  Stage
	FlagMarketingUsed  Stage
	FlagSimulationUsed Stage
}

// HeaderInputs captures the RM group header fields the engine needs. Passed by
// the application layer; rmcost does not import rmgroup (keeps domain boundaries clean).
type HeaderInputs struct {
	CostPercentage    float64
	CostPerKg         float64
	FlagValuation     Stage
	FlagMarketing     Stage
	FlagSimulation    Stage
	InitValValuation  *float64
	InitValMarketing  *float64
	InitValSimulation *float64
}

// CalculateCost runs the full pipeline (AggregateRates → SelectRate × 3 → LandedCost × 3)
// for one group in one period. Pure function — no I/O, no state.
func CalculateCost(items []RateInputs, h HeaderInputs) Computed {
	rates := AggregateRates(items)
	valRate, valUsed := SelectRate(rates, h.FlagValuation, h.InitValValuation)
	mktRate, mktUsed := SelectRate(rates, h.FlagMarketing, h.InitValMarketing)
	simRate, simUsed := SelectRate(rates, h.FlagSimulation, h.InitValSimulation)
	return Computed{
		Rates:              rates,
		CostValuation:      LandedCost(h.CostPercentage, valRate, h.CostPerKg),
		CostMarketing:      LandedCost(h.CostPercentage, mktRate, h.CostPerKg),
		CostSimulation:     LandedCost(h.CostPercentage, simRate, h.CostPerKg),
		FlagValuation:      h.FlagValuation,
		FlagMarketing:      h.FlagMarketing,
		FlagSimulation:     h.FlagSimulation,
		FlagValuationUsed:  valUsed,
		FlagMarketingUsed:  mktUsed,
		FlagSimulationUsed: simUsed,
	}
}

// NewGroupCost creates a new Cost row for rm_type=GROUP. The worker calls this
// after calculation and then persists via Repository.Upsert.
func NewGroupCost(
	period, rmCode string,
	groupHeadID uuid.UUID,
	rmName, uomCode string,
	comp Computed,
	calculatedBy string,
) (*Cost, error) {
	if err := ValidatePeriod(period); err != nil {
		return nil, err
	}
	if rmCode == "" {
		return nil, ErrEmptyRMCode
	}
	if calculatedBy == "" {
		return nil, ErrEmptyCalculatedBy
	}
	now := time.Now()
	headID := groupHeadID
	by := calculatedBy
	costVal := comp.CostValuation
	costMkt := comp.CostMarketing
	costSim := comp.CostSimulation
	return &Cost{
		id:                 uuid.New(),
		period:             period,
		rmCode:             rmCode,
		rmType:             RMTypeGroup,
		groupHeadID:        &headID,
		rmName:             rmName,
		uomCode:            uomCode,
		rates:              comp.Rates,
		costValuation:      &costVal,
		costMarketing:      &costMkt,
		costSimulation:     &costSim,
		flagValuation:      comp.FlagValuation,
		flagMarketing:      comp.FlagMarketing,
		flagSimulation:     comp.FlagSimulation,
		flagValuationUsed:  comp.FlagValuationUsed,
		flagMarketingUsed:  comp.FlagMarketingUsed,
		flagSimulationUsed: comp.FlagSimulationUsed,
		calculatedAt:       &now,
		calculatedBy:       &by,
		createdAt:          now,
		createdBy:          calculatedBy,
	}, nil
}

// ReconstructCost rebuilds a Cost from persistence. Used by repositories only.
//
//nolint:revive // Persistence reconstitution takes many fields by design.
func ReconstructCost(
	id uuid.UUID,
	period, rmCode string,
	rmType RMType,
	groupHeadID *uuid.UUID,
	itemCode *string,
	rmName, uomCode string,
	rates StageRates,
	costValuation, costMarketing, costSimulation *float64,
	flagValuation, flagMarketing, flagSimulation Stage,
	flagValuationUsed, flagMarketingUsed, flagSimulationUsed Stage,
	calculatedAt *time.Time,
	calculatedBy *string,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Cost {
	return &Cost{
		id:                 id,
		period:             period,
		rmCode:             rmCode,
		rmType:             rmType,
		groupHeadID:        groupHeadID,
		itemCode:           itemCode,
		rmName:             rmName,
		uomCode:            uomCode,
		rates:              rates,
		costValuation:      costValuation,
		costMarketing:      costMarketing,
		costSimulation:     costSimulation,
		flagValuation:      flagValuation,
		flagMarketing:      flagMarketing,
		flagSimulation:     flagSimulation,
		flagValuationUsed:  flagValuationUsed,
		flagMarketingUsed:  flagMarketingUsed,
		flagSimulationUsed: flagSimulationUsed,
		calculatedAt:       calculatedAt,
		calculatedBy:       calculatedBy,
		createdAt:          createdAt,
		createdBy:          createdBy,
		updatedAt:          updatedAt,
		updatedBy:          updatedBy,
	}
}

// ApplyComputed overwrites the per-stage rates, costs, and flag-used fields with
// the output of a fresh CalculateCost pass. The caller passes `recalculatedBy`,
// and the Cost records it on the calculated_* and updated_* audit columns.
func (c *Cost) ApplyComputed(comp Computed, recalculatedBy string) error {
	if recalculatedBy == "" {
		return ErrEmptyCalculatedBy
	}
	now := time.Now()
	by := recalculatedBy
	costVal := comp.CostValuation
	costMkt := comp.CostMarketing
	costSim := comp.CostSimulation
	c.rates = comp.Rates
	c.costValuation = &costVal
	c.costMarketing = &costMkt
	c.costSimulation = &costSim
	c.flagValuation = comp.FlagValuation
	c.flagMarketing = comp.FlagMarketing
	c.flagSimulation = comp.FlagSimulation
	c.flagValuationUsed = comp.FlagValuationUsed
	c.flagMarketingUsed = comp.FlagMarketingUsed
	c.flagSimulationUsed = comp.FlagSimulationUsed
	c.calculatedAt = &now
	c.calculatedBy = &by
	c.updatedAt = &now
	c.updatedBy = &by
	return nil
}

// SetUOMCode updates the unit-of-measure code. Used by recalc to refresh a
// stale UOM on an existing Cost row when group details change.
func (c *Cost) SetUOMCode(code string) { c.uomCode = code }

// Cost getters.

// ID returns the cost row UUID.
func (c *Cost) ID() uuid.UUID { return c.id }

// Period returns the YYYYMM period string.
func (c *Cost) Period() string { return c.period }

// RMCode returns the rm_code (group code for rm_type=GROUP, item code for ITEM).
func (c *Cost) RMCode() string { return c.rmCode }

// RMType returns the RM type discriminator.
func (c *Cost) RMType() RMType { return c.rmType }

// GroupHeadID returns the owning group head ID (nil when rm_type=ITEM).
func (c *Cost) GroupHeadID() *uuid.UUID { return c.groupHeadID }

// ItemCode returns the item code (nil when rm_type=GROUP).
func (c *Cost) ItemCode() *string { return c.itemCode }

// RMName returns the display name of the RM (group or item name).
func (c *Cost) RMName() string { return c.rmName }

// UOMCode returns the unit-of-measure code for the RM.
func (c *Cost) UOMCode() string { return c.uomCode }

// Rates returns the per-stage aggregated rate snapshot.
func (c *Cost) Rates() StageRates { return c.rates }

// CostValuation returns the computed valuation landed cost (nil when never calculated).
func (c *Cost) CostValuation() *float64 { return c.costValuation }

// CostMarketing returns the computed marketing landed cost (nil when never calculated).
func (c *Cost) CostMarketing() *float64 { return c.costMarketing }

// CostSimulation returns the computed simulation landed cost (nil when never calculated).
func (c *Cost) CostSimulation() *float64 { return c.costSimulation }

// FlagValuation returns the flag configured on the group header at calc time.
func (c *Cost) FlagValuation() Stage { return c.flagValuation }

// FlagMarketing returns the flag configured on the group header at calc time.
func (c *Cost) FlagMarketing() Stage { return c.flagMarketing }

// FlagSimulation returns the flag configured on the group header at calc time.
func (c *Cost) FlagSimulation() Stage { return c.flagSimulation }

// FlagValuationUsed returns the stage actually used after cascade/INIT resolution.
func (c *Cost) FlagValuationUsed() Stage { return c.flagValuationUsed }

// FlagMarketingUsed returns the stage actually used after cascade/INIT resolution.
func (c *Cost) FlagMarketingUsed() Stage { return c.flagMarketingUsed }

// FlagSimulationUsed returns the stage actually used after cascade/INIT resolution.
func (c *Cost) FlagSimulationUsed() Stage { return c.flagSimulationUsed }

// CalculatedAt returns the last calculation timestamp (nil when never calculated).
func (c *Cost) CalculatedAt() *time.Time { return c.calculatedAt }

// CalculatedBy returns who last ran the calculation (nil when never calculated).
func (c *Cost) CalculatedBy() *string { return c.calculatedBy }

// CreatedAt returns the creation timestamp.
func (c *Cost) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Cost) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Cost) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Cost) UpdatedBy() *string { return c.updatedBy }

// =============================================================================
// History — append-only audit trail (aud_rm_cost_history).
// =============================================================================

// HistoryTriggerReason enumerates the reasons a calculation was run. The DB
// stores the raw string; callers must pass one of these canonical values.
type HistoryTriggerReason string

// HistoryTriggerReason canonical values.
const (
	// TriggerOracleSyncChain = auto-chained after a successful Oracle sync for the synced period.
	TriggerOracleSyncChain HistoryTriggerReason = "oracle-sync-chain"
	// TriggerGroupUpdate = recalculated because the group header changed.
	TriggerGroupUpdate HistoryTriggerReason = "group-update"
	// TriggerDetailChange = recalculated because an item was added/removed/toggled in the group.
	TriggerDetailChange HistoryTriggerReason = "detail-change"
	// TriggerManualUI = recalculated on explicit user request from the UI.
	TriggerManualUI HistoryTriggerReason = "manual-ui"
)

// IsValid reports whether the trigger reason is one of the recognized values.
func (r HistoryTriggerReason) IsValid() bool {
	switch r {
	case TriggerOracleSyncChain, TriggerGroupUpdate, TriggerDetailChange, TriggerManualUI:
		return true
	default:
		return false
	}
}

// History is a single row of the append-only audit trail. Stores a full snapshot
// of the inputs AND outputs of one calculation pass so operators can diff and
// replay.
//
//nolint:revive // Wide struct mirrors the persistence row one-for-one.
type History struct {
	ID             uuid.UUID
	RMCostID       *uuid.UUID
	JobID          *uuid.UUID
	Period         string
	RMCode         string
	RMType         RMType
	GroupHeadID    *uuid.UUID
	Rates          StageRates
	CostPercentage float64
	CostPerKg      float64

	FlagValuation      Stage
	FlagMarketing      Stage
	FlagSimulation     Stage
	InitValValuation   *float64
	InitValMarketing   *float64
	InitValSimulation  *float64
	CostValuation      *float64
	CostMarketing      *float64
	CostSimulation     *float64
	FlagValuationUsed  Stage
	FlagMarketingUsed  Stage
	FlagSimulationUsed Stage

	SourceItemCount int
	TriggerReason   HistoryTriggerReason
	CalculatedAt    time.Time
	CalculatedBy    string
}
