// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the MB Head domain.
type Entity struct {
	id                     uuid.UUID
	oracleSysID            *string
	mbCosting              string
	mgtName                *string
	denier                 *float64
	filament               *int
	dozing                 *float64
	mbhCheckStatus         *string
	mbhStatus              *string
	mbhLdrPrsn             *float64
	mbhRunLdrPct           *float64
	mbhFinalProduct        *string
	mbhCode                *string
	isActive               bool
	createdAt              time.Time
	createdBy              string
	updatedAt              *time.Time
	updatedBy              *string
	deletedAt              *time.Time
	deletedBy              *string
	entryStatus            string
	isBoughtout            bool
	currentVersion         int32
	machineFixedTotal      *string
	machineID              *uuid.UUID
	stateReason            string
	devCode                string
	shadeCode              string
	shadeName              string
	crossSection           string
	lustureCode            string
	costProductID          int64
	costGeneratedAt        *string
	costGeneratedBy        string
	paramWaste             *string
	paramQualityLoss       *string
	paramEfficiency        *string
	paramDevExpense        *string
	paramPacking           *string
	paramMBProdPerDay      *string
	paramThroughputPerHour string
	paramNoOfProcess       string
}

// New creates a new MB Head entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(mbCosting string, oracleSysID, mgtName *string, denier *float64, filament *int, dozing *float64, mbhCheckStatus, mbhStatus *string, mbhLdrPrsn, mbhRunLdrPct *float64, mbhFinalProduct, mbhCode *string, createdBy string, isBoughtout bool, devCode, shadeCode, shadeName, crossSection, lustureCode string, machineID *uuid.UUID) (*Entity, error) {
	if mbCosting == "" {
		return nil, ErrEmptyMBCosting
	}
	if len(mbCosting) > 100 {
		return nil, ErrMBCostingTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), oracleSysID: oracleSysID, mbCosting: mbCosting, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing,
		mbhCheckStatus: mbhCheckStatus, mbhStatus: mbhStatus, mbhLdrPrsn: mbhLdrPrsn,
		mbhRunLdrPct:    mbhRunLdrPct,
		mbhFinalProduct: mbhFinalProduct, mbhCode: mbhCode,
		isActive: true, createdAt: time.Now(), createdBy: createdBy,
		isBoughtout: isBoughtout, devCode: devCode, shadeCode: shadeCode,
		shadeName: shadeName, crossSection: crossSection, lustureCode: lustureCode,
		machineID: machineID,
	}, nil
}

// Reconstruct rebuilds an MB Head from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(
	id uuid.UUID, oracleSysID *string, mbCosting string, mgtName *string, denier *float64,
	filament *int, dozing *float64, mbhCheckStatus, mbhStatus *string, mbhLdrPrsn, mbhRunLdrPct *float64,
	mbhFinalProduct, mbhCode *string, isActive bool, createdAt time.Time, createdBy string,
	updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string,
	entryStatus string, isBoughtout bool, currentVersion int32, machineFixedTotal *string,
	stateReason, devCode, shadeCode, shadeName, crossSection, lustureCode string,
	costProductID int64, costGeneratedAt *string, costGeneratedBy string,
	paramWaste, paramQualityLoss, paramEfficiency, paramDevExpense, paramPacking,
	paramMBProdPerDay *string, paramThroughputPerHour, paramNoOfProcess string,
	machineID *uuid.UUID,
) *Entity {
	return &Entity{
		id: id, oracleSysID: oracleSysID, mbCosting: mbCosting, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing,
		mbhCheckStatus: mbhCheckStatus, mbhStatus: mbhStatus, mbhLdrPrsn: mbhLdrPrsn,
		mbhRunLdrPct:    mbhRunLdrPct,
		mbhFinalProduct: mbhFinalProduct, mbhCode: mbhCode,
		isActive:  isActive,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
		entryStatus: entryStatus, isBoughtout: isBoughtout, currentVersion: currentVersion,
		machineFixedTotal: machineFixedTotal, stateReason: stateReason, devCode: devCode,
		shadeCode: shadeCode, shadeName: shadeName, crossSection: crossSection,
		lustureCode: lustureCode, costProductID: costProductID,
		costGeneratedAt: costGeneratedAt, costGeneratedBy: costGeneratedBy,
		paramWaste: paramWaste, paramQualityLoss: paramQualityLoss,
		paramEfficiency: paramEfficiency, paramDevExpense: paramDevExpense,
		paramPacking: paramPacking, paramMBProdPerDay: paramMBProdPerDay,
		paramThroughputPerHour: paramThroughputPerHour, paramNoOfProcess: paramNoOfProcess,
		machineID: machineID,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// OracleSysID returns the optional Oracle system ID for import reconciliation.
func (e *Entity) OracleSysID() *string { return e.oracleSysID }

// MBCosting returns the batch cost code identifier.
func (e *Entity) MBCosting() string { return e.mbCosting }

// MgtName returns the optional management display name.
func (e *Entity) MgtName() *string { return e.mgtName }

// Denier returns the optional yarn denier value.
func (e *Entity) Denier() *float64 { return e.denier }

// Filament returns the optional number of filaments.
func (e *Entity) Filament() *int { return e.filament }

// Dozing returns the optional dozing percentage.
func (e *Entity) Dozing() *float64 { return e.dozing }

// MBHCheckStatus returns the optional Oracle check status.
func (e *Entity) MBHCheckStatus() *string { return e.mbhCheckStatus }

// MBHStatus returns the optional Oracle head status.
func (e *Entity) MBHStatus() *string { return e.mbhStatus }

// MBHLdrPrsn returns the optional Oracle leader person value.
func (e *Entity) MBHLdrPrsn() *float64 { return e.mbhLdrPrsn }

// MBHRunLdrPct returns the optional Oracle CMBH_RUN_LDR_PRSN — the actual LDR
// percentage used in production. D30: this is the authoritative LDR for costing,
// while MBHLdrPrsn holds the planned value set while the product is still new.
func (e *Entity) MBHRunLdrPct() *float64 { return e.mbhRunLdrPct }

// MBHFinalProduct returns the optional Oracle final product code.
func (e *Entity) MBHFinalProduct() *string { return e.mbhFinalProduct }

// MBHCode returns the optional Oracle MB head code.
func (e *Entity) MBHCode() *string { return e.mbhCode }

// IsActive returns whether the MB head is active.
func (e *Entity) IsActive() bool { return e.isActive }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater.
func (e *Entity) UpdatedBy() *string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() *time.Time { return e.deletedAt }

// DeletedBy returns who soft-deleted the record.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted returns true if the MB head has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// EntryStatus returns the MB Costing workflow state (distinct from legacy Status/CheckStatus).
func (e *Entity) EntryStatus() string { return e.entryStatus }

// IsBoughtout returns whether this MB uses the boughtout shortcut workflow.
func (e *Entity) IsBoughtout() bool { return e.isBoughtout }

// CurrentVersion returns the current composition version number.
func (e *Entity) CurrentVersion() int32 { return e.currentVersion }

// MachineFixedTotal returns the fixed machine cost total, nil if not yet calculated.
func (e *Entity) MachineFixedTotal() *string { return e.machineFixedTotal }

// MachineID returns the assigned machine (mst_machine.mc_id), nil if not yet assigned.
func (e *Entity) MachineID() *uuid.UUID { return e.machineID }

// StateReason returns the reason recorded for the current UnApprove/Revoke transition, empty otherwise.
func (e *Entity) StateReason() string { return e.stateReason }

// DevCode returns the development code associated with this MB.
func (e *Entity) DevCode() string { return e.devCode }

// ShadeCode returns the shade code associated with this MB.
func (e *Entity) ShadeCode() string { return e.shadeCode }

// ShadeName returns the shade name associated with this MB.
func (e *Entity) ShadeName() string { return e.shadeName }

// CrossSection returns the cross-section descriptor for this MB.
func (e *Entity) CrossSection() string { return e.crossSection }

// LustureCode returns the lusture code associated with this MB.
func (e *Entity) LustureCode() string { return e.lustureCode }

// CostProductID returns the linked cost product's ID, zero if not yet generated.
func (e *Entity) CostProductID() int64 { return e.costProductID }

// CostGeneratedAt returns the timestamp the linked cost product was generated, nil if not yet generated.
func (e *Entity) CostGeneratedAt() *string { return e.costGeneratedAt }

// CostGeneratedBy returns the user who generated the linked cost product, empty if not yet generated.
func (e *Entity) CostGeneratedBy() string { return e.costGeneratedBy }

// ParamWaste returns the snapshotted waste parameter value, nil if not set.
func (e *Entity) ParamWaste() *string { return e.paramWaste }

// ParamQualityLoss returns the snapshotted quality-loss parameter value, nil if not set.
func (e *Entity) ParamQualityLoss() *string { return e.paramQualityLoss }

// ParamEfficiency returns the snapshotted efficiency parameter value, nil if not set.
func (e *Entity) ParamEfficiency() *string { return e.paramEfficiency }

// ParamDevExpense returns the snapshotted development-expense parameter value, nil if not set.
func (e *Entity) ParamDevExpense() *string { return e.paramDevExpense }

// ParamPacking returns the snapshotted packing parameter value, nil if not set.
func (e *Entity) ParamPacking() *string { return e.paramPacking }

// ParamMBProdPerDay returns the snapshotted MB-production-per-day parameter value, nil if not set.
func (e *Entity) ParamMBProdPerDay() *string { return e.paramMBProdPerDay }

// ParamThroughputPerHour returns the snapshotted throughput-per-hour parameter value.
func (e *Entity) ParamThroughputPerHour() string { return e.paramThroughputPerHour }

// ParamNoOfProcess returns the snapshotted number-of-process parameter value.
func (e *Entity) ParamNoOfProcess() string { return e.paramNoOfProcess }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	MBCosting       *string
	MgtName         *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHRunLdrPct    *float64
	MBHFinalProduct *string
	MBHCode         *string
	IsActive        *bool
	DevCode         *string
	ShadeCode       *string
	ShadeName       *string
	CrossSection    *string
	LustureCode     *string
	MachineID       *uuid.UUID
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyMBCosting(in.MBCosting); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	e.applyRecipeIdentityFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the MB head as deleted.
func (e *Entity) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	e.deletedAt = &now
	e.deletedBy = &deletedBy
	e.isActive = false
	return nil
}

func (e *Entity) applyMBCosting(mbCosting *string) error {
	if mbCosting == nil {
		return nil
	}
	if *mbCosting == "" {
		return ErrEmptyMBCosting
	}
	if len(*mbCosting) > 100 {
		return ErrMBCostingTooLong
	}
	e.mbCosting = *mbCosting
	return nil
}

// Submit transitions DRAFT → SUBMITTED. Returns ErrInvalidTransition if the current
// state does not allow it.
func (e *Entity) Submit() error {
	if !canTransition(e.entryStatus, StatusSubmitted) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusSubmitted
	return nil
}

// Approve transitions SUBMITTED → APPROVED, or UN_APPROVED → APPROVED (revalidate path).
func (e *Entity) Approve() error {
	if !canTransition(e.entryStatus, StatusApproved) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusApproved
	return nil
}

// Validate transitions APPROVED → VALIDATED for own-production MBs, or DRAFT → VALIDATED
// directly for boughtout MBs (shortcut gated by IsBoughtout, checked by the caller/handler
// layer per design.md §2.1 — this method only enforces the underlying state-name transition).
func (e *Entity) Validate() error {
	if e.isBoughtout {
		if e.entryStatus != StatusDraft {
			return ErrInvalidTransition
		}
	} else if !canTransition(e.entryStatus, StatusValidated) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusValidated
	e.currentVersion++
	return nil
}

// FreezeParams snapshots the 8 recipe parameter values onto the entity. Called once by
// ValidateHandler immediately before Validate() — scalar params take a numeric string value,
// picklist params take the selected option code. The caller resolves these from mst_mb_param's
// current live defaults; this method only assigns, it does not validate completeness.
//
//nolint:revive // Many parameters required — one per frozen field, mirrors Reconstruct's shape.
func (e *Entity) FreezeParams(waste, qualityLoss, efficiency, devExpense, packing, mbProdPerDay *string, throughputPerHour, noOfProcess string) {
	e.paramWaste = waste
	e.paramQualityLoss = qualityLoss
	e.paramEfficiency = efficiency
	e.paramDevExpense = devExpense
	e.paramPacking = packing
	e.paramMBProdPerDay = mbProdPerDay
	e.paramThroughputPerHour = throughputPerHour
	e.paramNoOfProcess = noOfProcess
}

// UnApprove transitions APPROVED → UN_APPROVED, requiring a reason.
func (e *Entity) UnApprove(reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	if !canTransition(e.entryStatus, StatusUnApproved) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusUnApproved
	e.stateReason = reason
	return nil
}

// Revoke transitions any non-terminal state to REVOKED, requiring a reason. Terminal —
// no further transitions are possible after Revoke.
func (e *Entity) Revoke(reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	if !canRevoke(e.entryStatus) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusRevoked
	e.stateReason = reason
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.MgtName != nil {
		e.mgtName = in.MgtName
	}
	if in.Denier != nil {
		e.denier = in.Denier
	}
	if in.Filament != nil {
		e.filament = in.Filament
	}
	if in.Dozing != nil {
		e.dozing = in.Dozing
	}
	if in.MBHCheckStatus != nil {
		e.mbhCheckStatus = in.MBHCheckStatus
	}
	if in.MBHStatus != nil {
		e.mbhStatus = in.MBHStatus
	}
	if in.MBHLdrPrsn != nil {
		e.mbhLdrPrsn = in.MBHLdrPrsn
	}
	if in.MBHRunLdrPct != nil {
		e.mbhRunLdrPct = in.MBHRunLdrPct
	}
	if in.MBHFinalProduct != nil {
		e.mbhFinalProduct = in.MBHFinalProduct
	}
	if in.MBHCode != nil {
		e.mbhCode = in.MBHCode
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
}

func (e *Entity) applyRecipeIdentityFields(in UpdateInput) {
	if in.DevCode != nil {
		e.devCode = *in.DevCode
	}
	if in.ShadeCode != nil {
		e.shadeCode = *in.ShadeCode
	}
	if in.ShadeName != nil {
		e.shadeName = *in.ShadeName
	}
	if in.CrossSection != nil {
		e.crossSection = *in.CrossSection
	}
	if in.LustureCode != nil {
		e.lustureCode = *in.LustureCode
	}
	if in.MachineID != nil {
		e.machineID = in.MachineID
	}
}
