// Package rmgroup provides domain logic for raw-material grouping and landed-cost configuration.
package rmgroup

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Head — aggregate root for an RM group's cost configuration.
// =============================================================================

// Head is the aggregate root. It carries cost-formula inputs and the flags that
// select which stage rate feeds each purpose (valuation, marketing, simulation).
type Head struct {
	id                uuid.UUID
	code              Code
	name              string
	description       string
	colorant          string
	ciName            string
	costPercentage    float64
	costPerKg         float64
	flagValuation     Flag
	flagMarketing     Flag
	flagSimulation    Flag
	initValValuation  *float64
	initValMarketing  *float64
	initValSimulation *float64
	isActive          bool
	createdAt         time.Time
	createdBy         string
	updatedAt         *time.Time
	updatedBy         *string
	deletedAt         *time.Time
	deletedBy         *string
}

// NewHead creates a new Head with validation. Defaults: flags = CONS, isActive = true.
func NewHead(
	code Code,
	name string,
	description string,
	costPercentage float64,
	costPerKg float64,
	createdBy string,
) (*Head, error) {
	if code.IsEmpty() {
		return nil, ErrEmptyCode
	}
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 200 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	if costPercentage < 0 {
		return nil, ErrNegativeCostPercentage
	}
	if costPerKg < 0 {
		return nil, ErrNegativeCostPerKg
	}

	return &Head{
		id:             uuid.New(),
		code:           code,
		name:           name,
		description:    description,
		costPercentage: costPercentage,
		costPerKg:      costPerKg,
		flagValuation:  FlagCons,
		flagMarketing:  FlagCons,
		flagSimulation: FlagCons,
		isActive:       true,
		createdAt:      time.Now(),
		createdBy:      createdBy,
	}, nil
}

// ReconstructHead rebuilds a Head from persistence. Used by repositories only.
func ReconstructHead(
	id uuid.UUID,
	code Code,
	name, description, colorant, ciName string,
	costPercentage, costPerKg float64,
	flagValuation, flagMarketing, flagSimulation Flag,
	initValValuation, initValMarketing, initValSimulation *float64,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *Head {
	return &Head{
		id:                id,
		code:              code,
		name:              name,
		description:       description,
		colorant:          colorant,
		ciName:            ciName,
		costPercentage:    costPercentage,
		costPerKg:         costPerKg,
		flagValuation:     flagValuation,
		flagMarketing:     flagMarketing,
		flagSimulation:    flagSimulation,
		initValValuation:  initValValuation,
		initValMarketing:  initValMarketing,
		initValSimulation: initValSimulation,
		isActive:          isActive,
		createdAt:         createdAt,
		createdBy:         createdBy,
		updatedAt:         updatedAt,
		updatedBy:         updatedBy,
		deletedAt:         deletedAt,
		deletedBy:         deletedBy,
	}
}

// Getters (read-only exposure of internal state).

// ID returns the head UUID.
func (h *Head) ID() uuid.UUID { return h.id }

// Code returns the group code.
func (h *Head) Code() Code { return h.code }

// Name returns the group display name.
func (h *Head) Name() string { return h.name }

// Description returns the free-text description.
func (h *Head) Description() string { return h.description }

// Colorant returns the optional colorant tag.
func (h *Head) Colorant() string { return h.colorant }

// CIName returns the optional CI name tag.
func (h *Head) CIName() string { return h.ciName }

// CostPercentage returns the cost percentage multiplier used in the landed-cost formula.
func (h *Head) CostPercentage() float64 { return h.costPercentage }

// CostPerKg returns the per-kg overhead added to the landed cost.
func (h *Head) CostPerKg() float64 { return h.costPerKg }

// FlagValuation returns the stage flag used for valuation cost.
func (h *Head) FlagValuation() Flag { return h.flagValuation }

// FlagMarketing returns the stage flag used for marketing cost.
func (h *Head) FlagMarketing() Flag { return h.flagMarketing }

// FlagSimulation returns the stage flag used for simulation cost.
func (h *Head) FlagSimulation() Flag { return h.flagSimulation }

// InitValValuation returns the init_val override for valuation (nil when unset).
func (h *Head) InitValValuation() *float64 { return h.initValValuation }

// InitValMarketing returns the init_val override for marketing (nil when unset).
func (h *Head) InitValMarketing() *float64 { return h.initValMarketing }

// InitValSimulation returns the init_val override for simulation (nil when unset).
func (h *Head) InitValSimulation() *float64 { return h.initValSimulation }

// IsActive returns whether the group is active.
func (h *Head) IsActive() bool { return h.isActive }

// CreatedAt returns the creation timestamp.
func (h *Head) CreatedAt() time.Time { return h.createdAt }

// CreatedBy returns the creator.
func (h *Head) CreatedBy() string { return h.createdBy }

// UpdatedAt returns the last update timestamp.
func (h *Head) UpdatedAt() *time.Time { return h.updatedAt }

// UpdatedBy returns the last updater.
func (h *Head) UpdatedBy() *string { return h.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (h *Head) DeletedAt() *time.Time { return h.deletedAt }

// DeletedBy returns who soft-deleted the group.
func (h *Head) DeletedBy() *string { return h.deletedBy }

// IsDeleted reports whether the group is soft-deleted.
func (h *Head) IsDeleted() bool { return h.deletedAt != nil }

// =============================================================================
// Head — behavior methods
// =============================================================================

// UpdateInput carries all optional mutations for Head.Update. Callers set only the
// fields they intend to change (pointers stay nil otherwise). This keeps the Update
// signature stable as fields grow.
type UpdateInput struct {
	Name              *string
	Description       *string
	Colorant          *string
	CIName            *string
	CostPercentage    *float64
	CostPerKg         *float64
	FlagValuation     *Flag
	FlagMarketing     *Flag
	FlagSimulation    *Flag
	InitValValuation  *float64
	InitValMarketing  *float64
	InitValSimulation *float64
	IsActive          *bool

	// ClearInitValValuation forces the init_val_valuation to NULL.
	// Use when the caller wants to unset a previously-set init value.
	ClearInitValValuation  bool
	ClearInitValMarketing  bool
	ClearInitValSimulation bool
}

// Update applies a partial update to the head and records audit fields.
// Each field has a dedicated apply helper to keep cognitive complexity low.
func (h *Head) Update(in UpdateInput, updatedBy string) error {
	if h.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if updatedBy == "" {
		return ErrEmptyUpdatedBy
	}

	if err := h.applyNameField(in.Name); err != nil {
		return err
	}
	h.applyTextFields(in.Description, in.Colorant, in.CIName)
	if err := h.applyCostFields(in.CostPercentage, in.CostPerKg); err != nil {
		return err
	}
	if err := h.applyInitValues(in); err != nil {
		return err
	}
	if err := h.applyFlagFields(in.FlagValuation, in.FlagMarketing, in.FlagSimulation); err != nil {
		return err
	}
	if err := h.assertFlagInitConsistency(); err != nil {
		return err
	}
	if in.IsActive != nil {
		h.isActive = *in.IsActive
	}

	now := time.Now()
	h.updatedAt = &now
	h.updatedBy = &updatedBy
	return nil
}

func (h *Head) applyNameField(name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return ErrEmptyName
	}
	if len(*name) > 200 {
		return ErrNameTooLong
	}
	h.name = *name
	return nil
}

func (h *Head) applyTextFields(description, colorant, ciName *string) {
	if description != nil {
		h.description = *description
	}
	if colorant != nil {
		h.colorant = *colorant
	}
	if ciName != nil {
		h.ciName = *ciName
	}
}

func (h *Head) applyCostFields(costPercentage, costPerKg *float64) error {
	if costPercentage != nil {
		if *costPercentage < 0 {
			return ErrNegativeCostPercentage
		}
		h.costPercentage = *costPercentage
	}
	if costPerKg != nil {
		if *costPerKg < 0 {
			return ErrNegativeCostPerKg
		}
		h.costPerKg = *costPerKg
	}
	return nil
}

func (h *Head) applyInitValues(in UpdateInput) error {
	if err := assignInitVal(&h.initValValuation, in.InitValValuation, in.ClearInitValValuation); err != nil {
		return err
	}
	if err := assignInitVal(&h.initValMarketing, in.InitValMarketing, in.ClearInitValMarketing); err != nil {
		return err
	}
	return assignInitVal(&h.initValSimulation, in.InitValSimulation, in.ClearInitValSimulation)
}

func assignInitVal(target **float64, incoming *float64, reset bool) error {
	if reset {
		*target = nil
		return nil
	}
	if incoming == nil {
		return nil
	}
	if *incoming < 0 {
		return ErrNegativeInitValue
	}
	v := *incoming
	*target = &v
	return nil
}

func (h *Head) applyFlagFields(valuation, marketing, simulation *Flag) error {
	if valuation != nil {
		if !valuation.IsValid() {
			return ErrInvalidFlag
		}
		h.flagValuation = *valuation
	}
	if marketing != nil {
		if !marketing.IsValid() {
			return ErrInvalidFlag
		}
		h.flagMarketing = *marketing
	}
	if simulation != nil {
		if !simulation.IsValid() {
			return ErrInvalidFlag
		}
		h.flagSimulation = *simulation
	}
	return nil
}

// assertFlagInitConsistency enforces that a flag set to INIT has a non-nil init_val.
// Mirrors the chk_rm_group_init_val_* CHECK constraints on cst_rm_group_head.
func (h *Head) assertFlagInitConsistency() error {
	if h.flagValuation == FlagInit && h.initValValuation == nil {
		return ErrInitValueRequired
	}
	if h.flagMarketing == FlagInit && h.initValMarketing == nil {
		return ErrInitValueRequired
	}
	if h.flagSimulation == FlagInit && h.initValSimulation == nil {
		return ErrInitValueRequired
	}
	return nil
}

// SoftDelete marks the head as deleted.
func (h *Head) SoftDelete(deletedBy string) error {
	if h.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if deletedBy == "" {
		return ErrEmptyUpdatedBy
	}
	now := time.Now()
	h.deletedAt = &now
	h.deletedBy = &deletedBy
	h.isActive = false
	return nil
}

// =============================================================================
// Detail — items (RMs) assigned to a Head.
// =============================================================================

// Detail represents one item's membership in an RM group.
type Detail struct {
	id               uuid.UUID
	headID           uuid.UUID
	itemCode         ItemCode
	itemName         string
	itemTypeCode     string
	gradeCode        string
	itemGrade        string
	uomCode          string
	marketPercentage *float64
	marketValueRp    *float64
	sortOrder        int32
	isActive         bool
	isDummy          bool
	createdAt        time.Time
	createdBy        string
	updatedAt        *time.Time
	updatedBy        *string
	deletedAt        *time.Time
	deletedBy        *string
}

// NewDetail creates a new Detail with validation. Defaults: isActive = true, isDummy = false.
func NewDetail(headID uuid.UUID, itemCode ItemCode, createdBy string) (*Detail, error) {
	if itemCode.IsEmpty() {
		return nil, ErrEmptyItemCode
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Detail{
		id:        uuid.New(),
		headID:    headID,
		itemCode:  itemCode,
		isActive:  true,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// ReconstructDetail rebuilds a Detail from persistence. Used by repositories only.
//
//nolint:revive // Many fields required for persistence reconstitution.
func ReconstructDetail(
	id, headID uuid.UUID,
	itemCode ItemCode,
	itemName, itemTypeCode, gradeCode, itemGrade, uomCode string,
	marketPercentage, marketValueRp *float64,
	sortOrder int32,
	isActive, isDummy bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *Detail {
	return &Detail{
		id:               id,
		headID:           headID,
		itemCode:         itemCode,
		itemName:         itemName,
		itemTypeCode:     itemTypeCode,
		gradeCode:        gradeCode,
		itemGrade:        itemGrade,
		uomCode:          uomCode,
		marketPercentage: marketPercentage,
		marketValueRp:    marketValueRp,
		sortOrder:        sortOrder,
		isActive:         isActive,
		isDummy:          isDummy,
		createdAt:        createdAt,
		createdBy:        createdBy,
		updatedAt:        updatedAt,
		updatedBy:        updatedBy,
		deletedAt:        deletedAt,
		deletedBy:        deletedBy,
	}
}

// Detail getters.

// ID returns the detail UUID.
func (d *Detail) ID() uuid.UUID { return d.id }

// HeadID returns the owning head UUID.
func (d *Detail) HeadID() uuid.UUID { return d.headID }

// ItemCode returns the item code.
func (d *Detail) ItemCode() ItemCode { return d.itemCode }

// ItemName returns the item name.
func (d *Detail) ItemName() string { return d.itemName }

// ItemTypeCode returns the item type code.
func (d *Detail) ItemTypeCode() string { return d.itemTypeCode }

// GradeCode returns the grade code.
func (d *Detail) GradeCode() string { return d.gradeCode }

// ItemGrade returns the item grade.
func (d *Detail) ItemGrade() string { return d.itemGrade }

// UOMCode returns the unit-of-measure code.
func (d *Detail) UOMCode() string { return d.uomCode }

// MarketPercentage returns the per-item marketing percentage (nil when unset).
func (d *Detail) MarketPercentage() *float64 { return d.marketPercentage }

// MarketValueRp returns the per-item marketing value in rupiah (nil when unset).
func (d *Detail) MarketValueRp() *float64 { return d.marketValueRp }

// SortOrder returns the display order within the group.
func (d *Detail) SortOrder() int32 { return d.sortOrder }

// IsActive reports whether the detail contributes to rate aggregation.
func (d *Detail) IsActive() bool { return d.isActive }

// IsDummy reports whether the detail is a placeholder (excluded from aggregation regardless of IsActive).
func (d *Detail) IsDummy() bool { return d.isDummy }

// CreatedAt returns the creation timestamp.
func (d *Detail) CreatedAt() time.Time { return d.createdAt }

// CreatedBy returns the creator.
func (d *Detail) CreatedBy() string { return d.createdBy }

// UpdatedAt returns the last update timestamp.
func (d *Detail) UpdatedAt() *time.Time { return d.updatedAt }

// UpdatedBy returns the last updater.
func (d *Detail) UpdatedBy() *string { return d.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (d *Detail) DeletedAt() *time.Time { return d.deletedAt }

// DeletedBy returns who soft-deleted the detail.
func (d *Detail) DeletedBy() *string { return d.deletedBy }

// IsDeleted reports whether the detail is soft-deleted.
func (d *Detail) IsDeleted() bool { return d.deletedAt != nil }

// =============================================================================
// Detail — behavior methods
// =============================================================================

// DetailUpdateInput carries optional mutations for Detail.Update.
type DetailUpdateInput struct {
	ItemName         *string
	ItemTypeCode     *string
	GradeCode        *string
	ItemGrade        *string
	UOMCode          *string
	MarketPercentage *float64
	MarketValueRp    *float64
	SortOrder        *int32
	IsActive         *bool
	IsDummy          *bool

	// ClearMarketPercentage forces market_percentage to NULL.
	ClearMarketPercentage bool
	ClearMarketValueRp    bool
}

// Update applies a partial update to the detail.
func (d *Detail) Update(in DetailUpdateInput, updatedBy string) error {
	if d.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if updatedBy == "" {
		return ErrEmptyUpdatedBy
	}

	d.applyDetailTextFields(in)
	if err := d.applyDetailMarketFields(in); err != nil {
		return err
	}
	d.applyDetailFlagFields(in)

	now := time.Now()
	d.updatedAt = &now
	d.updatedBy = &updatedBy
	return nil
}

func (d *Detail) applyDetailTextFields(in DetailUpdateInput) {
	if in.ItemName != nil {
		d.itemName = *in.ItemName
	}
	if in.ItemTypeCode != nil {
		d.itemTypeCode = *in.ItemTypeCode
	}
	if in.GradeCode != nil {
		d.gradeCode = *in.GradeCode
	}
	if in.ItemGrade != nil {
		d.itemGrade = *in.ItemGrade
	}
	if in.UOMCode != nil {
		d.uomCode = *in.UOMCode
	}
}

func (d *Detail) applyDetailMarketFields(in DetailUpdateInput) error {
	if in.ClearMarketPercentage {
		d.marketPercentage = nil
	} else if in.MarketPercentage != nil {
		if *in.MarketPercentage < 0 {
			return ErrNegativeMarketPercentage
		}
		v := *in.MarketPercentage
		d.marketPercentage = &v
	}
	if in.ClearMarketValueRp {
		d.marketValueRp = nil
	} else if in.MarketValueRp != nil {
		if *in.MarketValueRp < 0 {
			return ErrNegativeMarketValue
		}
		v := *in.MarketValueRp
		d.marketValueRp = &v
	}
	return nil
}

func (d *Detail) applyDetailFlagFields(in DetailUpdateInput) {
	if in.SortOrder != nil {
		d.sortOrder = *in.SortOrder
	}
	if in.IsActive != nil {
		d.isActive = *in.IsActive
	}
	if in.IsDummy != nil {
		d.isDummy = *in.IsDummy
	}
}

// SoftDelete marks the detail as deleted.
func (d *Detail) SoftDelete(deletedBy string) error {
	if d.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if deletedBy == "" {
		return ErrEmptyUpdatedBy
	}
	now := time.Now()
	d.deletedAt = &now
	d.deletedBy = &deletedBy
	d.isActive = false
	return nil
}

// Activate sets the detail active. Callers must guarantee no other active detail
// holds the same item_code (enforced at DB via partial unique index).
func (d *Detail) Activate(updatedBy string) error {
	if d.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if updatedBy == "" {
		return ErrEmptyUpdatedBy
	}
	d.isActive = true
	now := time.Now()
	d.updatedAt = &now
	d.updatedBy = &updatedBy
	return nil
}

// Deactivate excludes the detail from rate aggregation while keeping audit history.
func (d *Detail) Deactivate(updatedBy string) error {
	if d.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if updatedBy == "" {
		return ErrEmptyUpdatedBy
	}
	d.isActive = false
	now := time.Now()
	d.updatedAt = &now
	d.updatedBy = &updatedBy
	return nil
}
