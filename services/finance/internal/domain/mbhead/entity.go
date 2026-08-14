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
	vsNumber               string
	noOfProcess            string
	shades                 []*Shade
}

// NewInput carries the field values for New.
//
// The 11 fields marked required in spec section 2.1 are plain (non-pointer) values: omitting
// one is now an error rather than a "leave unset" signal. The remaining pointer fields stay
// genuinely optional.
type NewInput struct {
	// Required (spec section 2.1).
	MBCosting    string
	MgtName      string
	DevCode      string
	VsNumber     string
	NoOfProcess  string
	ShadeCode    string
	ShadeName    string
	CrossSection string
	FinalProduct string
	Denier       float64
	Filament     int
	LdrPrsn      float64

	// Optional.
	OracleSysID    *string
	Dozing         *float64
	MBHCheckStatus *string
	MBHStatus      *string
	MBHCode        *string
	LustureCode    string
	MachineID      *uuid.UUID

	// Metadata.
	CreatedBy   string
	IsBoughtout bool
}

// validatedFields holds the parsed value objects for the required fields.
type validatedFields struct {
	mbCosting    string
	mgtName      MgtName
	devCode      DevCode
	vsNumber     VsNumber
	noOfProcess  NoOfProcess
	shadeCode    ShadeCode
	shadeName    ShadeName
	crossSection CrossSection
	finalProduct FinalProduct
	denier       Denier
	filament     Filament
	ldrPrsn      LdrPercent
}

// validateRequired parses every required field, returning on the first failure.
func validateRequired(in NewInput) (*validatedFields, error) {
	var (
		f   validatedFields
		err error
	)
	if f.mbCosting, err = newBoundedString(in.MBCosting, maxMBCostingLen, ErrEmptyMBCosting, ErrMBCostingTooLong); err != nil {
		return nil, err
	}
	if f.mgtName, err = NewMgtName(in.MgtName); err != nil {
		return nil, err
	}
	if f.devCode, err = NewDevCode(in.DevCode); err != nil {
		return nil, err
	}
	if f.vsNumber, err = NewVsNumber(in.VsNumber); err != nil {
		return nil, err
	}
	if f.noOfProcess, err = NewNoOfProcess(in.NoOfProcess); err != nil {
		return nil, err
	}
	if f.shadeCode, err = NewShadeCode(in.ShadeCode); err != nil {
		return nil, err
	}
	if f.shadeName, err = NewShadeName(in.ShadeName); err != nil {
		return nil, err
	}
	if f.crossSection, err = NewCrossSection(in.CrossSection); err != nil {
		return nil, err
	}
	if f.finalProduct, err = NewFinalProduct(in.FinalProduct); err != nil {
		return nil, err
	}
	if f.denier, err = NewDenier(in.Denier); err != nil {
		return nil, err
	}
	if f.filament, err = NewFilament(in.Filament); err != nil {
		return nil, err
	}
	if f.ldrPrsn, err = NewLdrPercent(in.LdrPrsn); err != nil {
		return nil, err
	}
	return &f, nil
}

// New creates a new MB Head entity, validating all 11 required recipe fields.
func New(in NewInput) (*Entity, error) {
	if in.CreatedBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	f, err := validateRequired(in)
	if err != nil {
		return nil, err
	}
	mgtName := f.mgtName.String()
	finalProduct := f.finalProduct.String()
	denier := f.denier.Float64()
	filament := f.filament.Int()
	ldrPrsn := f.ldrPrsn.Float64()
	return &Entity{
		id: uuid.New(), oracleSysID: in.OracleSysID, mbCosting: f.mbCosting, mgtName: &mgtName,
		denier: &denier, filament: &filament, dozing: in.Dozing,
		mbhCheckStatus: in.MBHCheckStatus, mbhStatus: in.MBHStatus, mbhLdrPrsn: &ldrPrsn,
		mbhFinalProduct: &finalProduct, mbhCode: in.MBHCode,
		isActive: true, createdAt: time.Now(), createdBy: in.CreatedBy,
		isBoughtout: in.IsBoughtout, devCode: f.devCode.String(), shadeCode: f.shadeCode.String(),
		shadeName: f.shadeName.String(), crossSection: f.crossSection.String(),
		lustureCode: in.LustureCode, machineID: in.MachineID,
		vsNumber: f.vsNumber.String(), noOfProcess: f.noOfProcess.String(),
	}, nil
}

// Reconstruct rebuilds an MB Head from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(
	id uuid.UUID, oracleSysID *string, mbCosting string, mgtName *string, denier *float64,
	filament *int, dozing *float64, mbhCheckStatus, mbhStatus *string, mbhLdrPrsn *float64,
	mbhFinalProduct, mbhCode *string, isActive bool, createdAt time.Time, createdBy string,
	updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string,
	entryStatus string, isBoughtout bool, currentVersion int32, machineFixedTotal *string,
	stateReason, devCode, shadeCode, shadeName, crossSection, lustureCode string,
	costProductID int64, costGeneratedAt *string, costGeneratedBy string,
	paramWaste, paramQualityLoss, paramEfficiency, paramDevExpense, paramPacking,
	paramMBProdPerDay *string, paramThroughputPerHour, paramNoOfProcess string,
	machineID *uuid.UUID, vsNumber, noOfProcess string,
) *Entity {
	return &Entity{
		id: id, oracleSysID: oracleSysID, mbCosting: mbCosting, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing,
		mbhCheckStatus: mbhCheckStatus, mbhStatus: mbhStatus, mbhLdrPrsn: mbhLdrPrsn,
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
		machineID: machineID, vsNumber: vsNumber, noOfProcess: noOfProcess,
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

// ParamNoOfProcess returns the snapshotted number-of-process parameter value, frozen at
// Validate time by FreezeParams. Distinct from NoOfProcess, which is the user's editable
// header choice (spec section 2.4).
func (e *Entity) ParamNoOfProcess() string { return e.paramNoOfProcess }

// VsNumber returns the VS number, unique among live records.
func (e *Entity) VsNumber() string { return e.vsNumber }

// NoOfProcess returns the user-selected number-of-process option code (S/D/T). This is the
// editable header value, not the frozen ParamNoOfProcess snapshot (spec section 2.4).
func (e *Entity) NoOfProcess() string { return e.noOfProcess }

// Shades returns the additional (non-header) shades attached to this MB head.
func (e *Entity) Shades() []*Shade { return e.shades }

// UpdateInput carries field mutations for Update.
//
// Fields stay pointer-typed so the domain can distinguish "not supplied" from "set to zero",
// but the 12 required fields (spec section 2.1) are validated whenever they ARE supplied:
// passing a pointer to an empty string is rejected, not silently accepted. Per the Phase 2
// proto change these 12 arrive full-replace from the delivery layer, so in practice they are
// always non-nil on the update path.
type UpdateInput struct {
	MBCosting       *string
	MgtName         *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHFinalProduct *string
	MBHCode         *string
	IsActive        *bool
	IsBoughtout     *bool
	DevCode         *string
	VsNumber        *string
	NoOfProcess     *string
	ShadeCode       *string
	ShadeName       *string
	CrossSection    *string
	LustureCode     *string
	MachineID       *uuid.UUID
}

// Update applies field changes to the entity, validating every required field supplied.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyRequiredStrings(in); err != nil {
		return err
	}
	if err := e.applyRequiredNumerics(in); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	e.applyRecipeIdentityFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// boundedStringRule binds one supplied value to its destination and its length bounds.
type boundedStringRule struct {
	src        *string
	dst        *string
	dstPtr     **string
	maxLen     int
	errEmpty   error
	errTooLong error
}

// applyRequiredStrings validates and assigns every required string field that was supplied.
func (e *Entity) applyRequiredStrings(in UpdateInput) error {
	rules := []boundedStringRule{
		{src: in.MBCosting, dst: &e.mbCosting, maxLen: maxMBCostingLen, errEmpty: ErrEmptyMBCosting, errTooLong: ErrMBCostingTooLong},
		{src: in.DevCode, dst: &e.devCode, maxLen: maxDevCodeLen, errEmpty: ErrEmptyDevCode, errTooLong: ErrDevCodeTooLong},
		{src: in.VsNumber, dst: &e.vsNumber, maxLen: maxVsNumberLen, errEmpty: ErrEmptyVsNumber, errTooLong: ErrVsNumberTooLong},
		{src: in.NoOfProcess, dst: &e.noOfProcess, maxLen: maxNoOfProcessLen, errEmpty: ErrEmptyNoOfProcess, errTooLong: ErrNoOfProcessTooLong},
		{src: in.ShadeCode, dst: &e.shadeCode, maxLen: maxShadeCodeLen, errEmpty: ErrEmptyShadeCode, errTooLong: ErrShadeCodeTooLong},
		{src: in.ShadeName, dst: &e.shadeName, maxLen: maxShadeNameLen, errEmpty: ErrEmptyShadeName, errTooLong: ErrShadeNameTooLong},
		{src: in.CrossSection, dst: &e.crossSection, maxLen: maxCrossSectionLen, errEmpty: ErrEmptyCrossSection, errTooLong: ErrCrossSectionTooLong},
		{src: in.MgtName, dstPtr: &e.mgtName, maxLen: maxMgtNameLen, errEmpty: ErrEmptyMgtName, errTooLong: ErrMgtNameTooLong},
		{src: in.MBHFinalProduct, dstPtr: &e.mbhFinalProduct, maxLen: maxFinalProductLen, errEmpty: ErrEmptyFinalProduct, errTooLong: ErrFinalProductTooLong},
	}
	for _, r := range rules {
		if err := applyBoundedString(r); err != nil {
			return err
		}
	}
	return nil
}

// applyBoundedString validates one rule and writes through whichever destination it carries.
func applyBoundedString(r boundedStringRule) error {
	if r.src == nil {
		return nil
	}
	v, err := newBoundedString(*r.src, r.maxLen, r.errEmpty, r.errTooLong)
	if err != nil {
		return err
	}
	if r.dst != nil {
		*r.dst = v
		return nil
	}
	*r.dstPtr = &v
	return nil
}

// applyRequiredNumerics validates and assigns the required numeric fields that were supplied.
func (e *Entity) applyRequiredNumerics(in UpdateInput) error {
	if in.Denier != nil {
		v, err := NewDenier(*in.Denier)
		if err != nil {
			return err
		}
		f := v.Float64()
		e.denier = &f
	}
	if in.Filament != nil {
		v, err := NewFilament(*in.Filament)
		if err != nil {
			return err
		}
		i := v.Int()
		e.filament = &i
	}
	if in.MBHLdrPrsn != nil {
		v, err := NewLdrPercent(*in.MBHLdrPrsn)
		if err != nil {
			return err
		}
		f := v.Float64()
		e.mbhLdrPrsn = &f
	}
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

// ReplaceShades swaps the head's additional shades for the supplied set, enforcing the
// max-3-shades rule (spec section 4.2): at most 2 children, distinct sequence numbers,
// distinct shade codes, and no child repeating the header's own shade code. Persistence is
// replace-on-save (spec section 4.4), so an empty slice clears all children.
//
// This is application-layer enforcement by necessity: no DB constraint can count sibling
// rows or compare a child against its parent's column.
func (e *Entity) ReplaceShades(shades []*Shade) error {
	if err := e.validateShades(shades); err != nil {
		return err
	}
	for _, s := range shades {
		s.SetParent(e.id)
	}
	e.shades = shades
	return nil
}

// AddShade appends one additional shade, re-validating the whole set afterwards.
func (e *Entity) AddShade(s *Shade) error {
	if s == nil {
		return ErrShadeNotFound
	}
	next := make([]*Shade, 0, len(e.shades)+1)
	next = append(next, e.shades...)
	next = append(next, s)
	return e.ReplaceShades(next)
}

// SetShades hydrates shades from persistence without re-validating, so legacy rows stay
// readable. Mirrors Reconstruct.
func (e *Entity) SetShades(shades []*Shade) { e.shades = shades }

// validateShades enforces the count, sequence-number and shade-code rules for a candidate set.
func (e *Entity) validateShades(shades []*Shade) error {
	if len(shades) > MaxAdditionalShades {
		return ErrTooManyShades
	}
	seenSeq := make(map[int32]struct{}, len(shades))
	seenCode := make(map[string]struct{}, len(shades))
	for _, s := range shades {
		if s == nil {
			return ErrShadeNotFound
		}
		if s.SeqNo() < MinShadeSeqNo || s.SeqNo() > MaxShadeSeqNo {
			return ErrInvalidShadeSeqNo
		}
		if _, dup := seenSeq[s.SeqNo()]; dup {
			return ErrDuplicateShadeSeqNo
		}
		seenSeq[s.SeqNo()] = struct{}{}
		if err := e.checkShadeCode(s.ShadeCode(), seenCode); err != nil {
			return err
		}
	}
	return nil
}

// checkShadeCode rejects a child code that repeats the header code or an earlier sibling.
func (e *Entity) checkShadeCode(code string, seen map[string]struct{}) error {
	if code == e.shadeCode {
		return ErrShadeCodeMatchesHeader
	}
	if _, dup := seen[code]; dup {
		return ErrDuplicateShadeCode
	}
	seen[code] = struct{}{}
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

// applyOptionalFields assigns the unvalidated optional fields. The required fields are handled
// by applyRequiredStrings / applyRequiredNumerics and are deliberately absent here.
func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.Dozing != nil {
		e.dozing = in.Dozing
	}
	if in.MBHCheckStatus != nil {
		e.mbhCheckStatus = in.MBHCheckStatus
	}
	if in.MBHStatus != nil {
		e.mbhStatus = in.MBHStatus
	}
	if in.MBHCode != nil {
		e.mbhCode = in.MBHCode
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	if in.IsBoughtout != nil {
		e.isBoughtout = *in.IsBoughtout
	}
}

// applyRecipeIdentityFields assigns the unvalidated identity fields.
func (e *Entity) applyRecipeIdentityFields(in UpdateInput) {
	if in.LustureCode != nil {
		e.lustureCode = *in.LustureCode
	}
	if in.MachineID != nil {
		e.machineID = in.MachineID
	}
}
