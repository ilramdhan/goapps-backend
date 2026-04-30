// Package rmcost — V2 RM Cost engine extensions.
// Adds per-detail snapshot entity (CostDetail), V2 attach methods on Cost for
// snapshot/computed-rate columns, and the V2 ComputedV2 struct produced by the
// new calculation engine.
package rmcost

import (
	"time"

	"github.com/google/uuid"
)

// V2Inputs is the bag of editable per-row marketing inputs + flags that the
// V2 engine snapshots onto cst_rm_cost.
type V2Inputs struct {
	MarketingFreightRate    *float64
	MarketingAntiDumpingPct *float64 // whole percent (5 = 5%) at storage
	MarketingDutyPct        *float64 // whole percent at storage
	MarketingTransportRate  *float64
	MarketingDefaultValue   *float64
	SimulationRate          *float64
	ValuationFlag           string // "AUTO" / "CR" / ... (validated upstream)
	MarketingFlag           string // "AUTO" / "SP" / ...
}

// V2Rates is the per-stage computed rate snapshot persisted on cst_rm_cost
// alongside the V2 inputs. cl/sl/fl come from the per-detail engine; cr/sr/pr
// are group totals; sp/pp/fp are marketing projections.
type V2Rates struct {
	CL *float64
	SL *float64
	FL *float64
	SP *float64
	PP *float64
	FP *float64
	CR *float64
	SR *float64
	PR *float64
}

// ComputedV2 carries the full output of one V2 calculation pass. Replaces the
// V1 Computed when the engine runs in V2 mode.
type ComputedV2 struct {
	V2Inputs       V2Inputs
	V2Rates        V2Rates
	CostValuation  float64
	CostMarketing  float64
	CostSimulation float64
}

// AttachV2 sets the V2 snapshot inputs and computed rates on a Cost.
func (c *Cost) AttachV2(in V2Inputs, r V2Rates) {
	c.v2Inputs = &in
	c.v2Rates = &r
}

// V2Inputs returns the V2 marketing-snapshot inputs (nil when V1 row).
func (c *Cost) V2Inputs() *V2Inputs { return c.v2Inputs }

// V2Rates returns the V2 computed-rate snapshot (nil when V1 row).
func (c *Cost) V2Rates() *V2Rates { return c.v2Rates }

// ApplyV2Computed overwrites both V2 snapshots and the cost_* values from a
// fresh ComputedV2 pass. Used when full recalc runs.
func (c *Cost) ApplyV2Computed(comp ComputedV2, recalculatedBy string) error {
	if recalculatedBy == "" {
		return ErrEmptyCalculatedBy
	}
	now := time.Now()
	by := recalculatedBy
	costVal := comp.CostValuation
	costMkt := comp.CostMarketing
	costSim := comp.CostSimulation
	c.costValuation = &costVal
	c.costMarketing = &costMkt
	c.costSimulation = &costSim
	c.v2Inputs = &comp.V2Inputs
	c.v2Rates = &comp.V2Rates
	c.calculatedAt = &now
	c.calculatedBy = &by
	c.updatedAt = &now
	c.updatedBy = &by
	return nil
}

// ApplyV2Recompute overwrites only the cost_* values + V2Rates fields that
// depend on the inline-edit inputs (SP/PP/FP from new marketing inputs,
// recomputed cost_marketing and cost_simulation). cl/sl/fl/cr/sr/pr stay as is
// because they depend on the underlying detail rows.
func (c *Cost) ApplyV2Recompute(rates V2Rates, costMkt, costSim float64, by string) {
	now := time.Now()
	user := by
	c.costMarketing = &costMkt
	c.costSimulation = &costSim
	c.v2Rates = &rates
	c.updatedAt = &now
	c.updatedBy = &user
}

// CostDetail is the per-(cost, item, grade) snapshot of every intermediate column
// produced by the V2 engine. Mirrors the cst_rm_cost_detail table 1:1.
//
//nolint:revive // Wide struct mirrors the persistence row one-for-one.
type CostDetail struct {
	id            uuid.UUID
	costID        uuid.UUID
	period        string
	groupHeadID   uuid.UUID
	groupDetailID *uuid.UUID
	itemCode      string
	itemName      string
	gradeCode     string

	// Per-detail inputs (snapshot from cst_rm_group_detail).
	freightRate           *float64
	antiDumpingPct        *float64
	dutyPct               *float64
	transportRate         *float64
	valuationDefaultValue *float64

	// Consumption stage outputs.
	consVal              *float64
	consQty              *float64
	consRate             *float64
	consFreightVal       *float64
	consValBased         *float64
	consRateBased        *float64
	consAntiDumpingVal   *float64
	consAntiDumpingRate  *float64
	consDutyVal          *float64
	consDutyRate         *float64
	consTransportVal     *float64
	consTransportRate    *float64
	consLandedCost       *float64

	// Stock stage outputs.
	stockVal              *float64
	stockQty              *float64
	stockRate             *float64
	stockFreightVal       *float64
	stockValBased         *float64
	stockRateBased        *float64
	stockAntiDumpingVal   *float64
	stockAntiDumpingRate  *float64
	stockDutyVal          *float64
	stockDutyRate         *float64
	stockTransportVal     *float64
	stockTransportRate    *float64
	stockLandedCost       *float64

	// PO stage.
	poVal  *float64
	poQty  *float64
	poRate *float64

	// Fix stage outputs (driven by editable fix_rate).
	fixRate             *float64
	fixFreightRate      *float64
	fixRateBased        *float64
	fixAntiDumpingRate  *float64
	fixDutyRate         *float64
	fixTransportRate    *float64
	fixLandedCost       *float64

	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
}

// NewCostDetail constructs a fresh CostDetail.
func NewCostDetail(costID, groupHeadID uuid.UUID, period, itemCode, itemName, gradeCode, createdBy string) (*CostDetail, error) {
	if err := ValidatePeriod(period); err != nil {
		return nil, err
	}
	if itemCode == "" {
		return nil, ErrEmptyRMCode
	}
	if createdBy == "" {
		return nil, ErrEmptyCalculatedBy
	}
	return &CostDetail{
		id:          uuid.New(),
		costID:      costID,
		groupHeadID: groupHeadID,
		period:      period,
		itemCode:    itemCode,
		itemName:    itemName,
		gradeCode:   gradeCode,
		createdAt:   time.Now(),
		createdBy:   createdBy,
	}, nil
}

// ReconstructCostDetail rebuilds a CostDetail from persistence.
//
//nolint:revive // Persistence reconstitution takes many fields by design.
func ReconstructCostDetail(
	id, costID, groupHeadID uuid.UUID,
	period, itemCode, itemName, gradeCode string,
	groupDetailID *uuid.UUID,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *CostDetail {
	return &CostDetail{
		id:            id,
		costID:        costID,
		groupHeadID:   groupHeadID,
		period:        period,
		itemCode:      itemCode,
		itemName:      itemName,
		gradeCode:     gradeCode,
		groupDetailID: groupDetailID,
		createdAt:     createdAt,
		createdBy:     createdBy,
		updatedAt:     updatedAt,
		updatedBy:     updatedBy,
	}
}

// CostDetailSnapshot is the bag of per-stage computed values written by the
// engine into a CostDetail. AttachSnapshot copies every pointer, so callers
// can reuse the struct across iterations.
//
//nolint:revive // Wide DTO mirrors persistence one-for-one.
type CostDetailSnapshot struct {
	FreightRate           *float64
	AntiDumpingPct        *float64
	DutyPct               *float64
	TransportRate         *float64
	ValuationDefaultValue *float64
	ConsVal               *float64
	ConsQty               *float64
	ConsRate              *float64
	ConsFreightVal        *float64
	ConsValBased          *float64
	ConsRateBased         *float64
	ConsAntiDumpingVal    *float64
	ConsAntiDumpingRate   *float64
	ConsDutyVal           *float64
	ConsDutyRate          *float64
	ConsTransportVal      *float64
	ConsTransportRate     *float64
	ConsLandedCost        *float64
	StockVal              *float64
	StockQty              *float64
	StockRate             *float64
	StockFreightVal       *float64
	StockValBased         *float64
	StockRateBased        *float64
	StockAntiDumpingVal   *float64
	StockAntiDumpingRate  *float64
	StockDutyVal          *float64
	StockDutyRate         *float64
	StockTransportVal     *float64
	StockTransportRate    *float64
	StockLandedCost       *float64
	POVal                 *float64
	POQty                 *float64
	PORate                *float64
	FixRate               *float64
	FixFreightRate        *float64
	FixRateBased          *float64
	FixAntiDumpingRate    *float64
	FixDutyRate           *float64
	FixTransportRate      *float64
	FixLandedCost         *float64
}

// AttachSnapshot copies every field from snap into the detail.
func (d *CostDetail) AttachSnapshot(snap CostDetailSnapshot) {
	d.freightRate = snap.FreightRate
	d.antiDumpingPct = snap.AntiDumpingPct
	d.dutyPct = snap.DutyPct
	d.transportRate = snap.TransportRate
	d.valuationDefaultValue = snap.ValuationDefaultValue
	d.consVal = snap.ConsVal
	d.consQty = snap.ConsQty
	d.consRate = snap.ConsRate
	d.consFreightVal = snap.ConsFreightVal
	d.consValBased = snap.ConsValBased
	d.consRateBased = snap.ConsRateBased
	d.consAntiDumpingVal = snap.ConsAntiDumpingVal
	d.consAntiDumpingRate = snap.ConsAntiDumpingRate
	d.consDutyVal = snap.ConsDutyVal
	d.consDutyRate = snap.ConsDutyRate
	d.consTransportVal = snap.ConsTransportVal
	d.consTransportRate = snap.ConsTransportRate
	d.consLandedCost = snap.ConsLandedCost
	d.stockVal = snap.StockVal
	d.stockQty = snap.StockQty
	d.stockRate = snap.StockRate
	d.stockFreightVal = snap.StockFreightVal
	d.stockValBased = snap.StockValBased
	d.stockRateBased = snap.StockRateBased
	d.stockAntiDumpingVal = snap.StockAntiDumpingVal
	d.stockAntiDumpingRate = snap.StockAntiDumpingRate
	d.stockDutyVal = snap.StockDutyVal
	d.stockDutyRate = snap.StockDutyRate
	d.stockTransportVal = snap.StockTransportVal
	d.stockTransportRate = snap.StockTransportRate
	d.stockLandedCost = snap.StockLandedCost
	d.poVal = snap.POVal
	d.poQty = snap.POQty
	d.poRate = snap.PORate
	d.fixRate = snap.FixRate
	d.fixFreightRate = snap.FixFreightRate
	d.fixRateBased = snap.FixRateBased
	d.fixAntiDumpingRate = snap.FixAntiDumpingRate
	d.fixDutyRate = snap.FixDutyRate
	d.fixTransportRate = snap.FixTransportRate
	d.fixLandedCost = snap.FixLandedCost
}

// Snapshot returns the current per-stage values as a CostDetailSnapshot.
//
//nolint:gocyclo,gocognit // Pure field copy is large but trivial in cognitive load.
func (d *CostDetail) Snapshot() CostDetailSnapshot {
	return CostDetailSnapshot{
		FreightRate:           d.freightRate,
		AntiDumpingPct:        d.antiDumpingPct,
		DutyPct:               d.dutyPct,
		TransportRate:         d.transportRate,
		ValuationDefaultValue: d.valuationDefaultValue,
		ConsVal:               d.consVal,
		ConsQty:               d.consQty,
		ConsRate:              d.consRate,
		ConsFreightVal:        d.consFreightVal,
		ConsValBased:          d.consValBased,
		ConsRateBased:         d.consRateBased,
		ConsAntiDumpingVal:    d.consAntiDumpingVal,
		ConsAntiDumpingRate:   d.consAntiDumpingRate,
		ConsDutyVal:           d.consDutyVal,
		ConsDutyRate:          d.consDutyRate,
		ConsTransportVal:      d.consTransportVal,
		ConsTransportRate:     d.consTransportRate,
		ConsLandedCost:        d.consLandedCost,
		StockVal:              d.stockVal,
		StockQty:              d.stockQty,
		StockRate:             d.stockRate,
		StockFreightVal:       d.stockFreightVal,
		StockValBased:         d.stockValBased,
		StockRateBased:        d.stockRateBased,
		StockAntiDumpingVal:   d.stockAntiDumpingVal,
		StockAntiDumpingRate:  d.stockAntiDumpingRate,
		StockDutyVal:          d.stockDutyVal,
		StockDutyRate:         d.stockDutyRate,
		StockTransportVal:     d.stockTransportVal,
		StockTransportRate:    d.stockTransportRate,
		StockLandedCost:       d.stockLandedCost,
		POVal:                 d.poVal,
		POQty:                 d.poQty,
		PORate:                d.poRate,
		FixRate:               d.fixRate,
		FixFreightRate:        d.fixFreightRate,
		FixRateBased:          d.fixRateBased,
		FixAntiDumpingRate:    d.fixAntiDumpingRate,
		FixDutyRate:           d.fixDutyRate,
		FixTransportRate:      d.fixTransportRate,
		FixLandedCost:         d.fixLandedCost,
	}
}

// SetGroupDetailID attaches a snapshot link to the source group detail.
func (d *CostDetail) SetGroupDetailID(id *uuid.UUID) { d.groupDetailID = id }

// CostDetail getters.

// ID returns the detail UUID.
func (d *CostDetail) ID() uuid.UUID { return d.id }

// CostID returns the parent cost row UUID.
func (d *CostDetail) CostID() uuid.UUID { return d.costID }

// Period returns the YYYYMM period.
func (d *CostDetail) Period() string { return d.period }

// GroupHeadID returns the source group head UUID.
func (d *CostDetail) GroupHeadID() uuid.UUID { return d.groupHeadID }

// GroupDetailID returns the source group detail UUID (nil if unlinked).
func (d *CostDetail) GroupDetailID() *uuid.UUID { return d.groupDetailID }

// ItemCode returns the item code.
func (d *CostDetail) ItemCode() string { return d.itemCode }

// ItemName returns the item name snapshot.
func (d *CostDetail) ItemName() string { return d.itemName }

// GradeCode returns the grade code.
func (d *CostDetail) GradeCode() string { return d.gradeCode }

// CreatedAt returns when this snapshot was inserted.
func (d *CostDetail) CreatedAt() time.Time { return d.createdAt }

// CreatedBy returns who inserted this snapshot.
func (d *CostDetail) CreatedBy() string { return d.createdBy }

// UpdatedAt returns when this snapshot was last touched.
func (d *CostDetail) UpdatedAt() *time.Time { return d.updatedAt }

// UpdatedBy returns who last touched this snapshot.
func (d *CostDetail) UpdatedBy() *string { return d.updatedBy }

// MarkUpdated stamps updated_at/updated_by. Used after fix_rate edit recompute.
func (d *CostDetail) MarkUpdated(by string) {
	now := time.Now()
	d.updatedAt = &now
	d.updatedBy = &by
}
