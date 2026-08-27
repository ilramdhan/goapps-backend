// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the MB Head domain.
type Entity struct {
	id             uuid.UUID
	oracleSysID    *string
	mbCosting      string
	mgtName        *string
	denier         *float64
	filament       *int
	dozing         *float64
	mbhCheckStatus *string
	// mbhCheckStatusCalc maps to mbh_check_status_calc (migration 000487) — the
	// DERIVED check status. ⛔ Distinct from mbhCheckStatus, which is the FROZEN
	// Oracle import trace and is never written from a derivation path (K-1 opt 2).
	// nil = never calculated by the application; 207 legacy heads stay nil forever.
	mbhCheckStatusCalc     *string
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

	// Recipe fields added by the MB Recipe consolidation (P5).
	//
	// vsNumber maps to mbh_vs_number and noOfProcess to mbh_no_of_process. Both are
	// pointers because an absent field must persist as NULL, never as an invented
	// default (D13) — a legacy payload that omits them has to keep working.
	//
	// ⛔ noOfProcess (mbh_no_of_process, the live user choice S/D/T) is NOT
	// paramNoOfProcess (mbh_param_no_of_process, the frozen VALIDATE snapshot).
	// Merging them would retroactively change historical cost snapshots — the only
	// legitimate writer of paramNoOfProcess is FreezeParams.
	vsNumber         *string
	noOfProcess      *string
	additionalShades []Shade

	// Lock columns (000485). Storage was prepared by P5; the BEHAVIOR (Lock,
	// RequestUnlock, GrantUnlock, RejectUnlock, IsEditable) landed with P10.
	//
	// 🔴 mbh_is_locked is NULLable WITHOUT DEFAULT (deliberate deviation recorded in
	// 000485), so NULL means "not locked". Every read must go through
	// COALESCE(mbh_is_locked, FALSE) — ⛔ never `mbh_is_locked = FALSE`, which skips
	// NULL. In Go the distinction disappears: this bool is false for such rows.
	isLocked          bool
	lockedAt          *time.Time
	lockedBy          *string
	unlockRequestedAt *time.Time
	unlockRequestedBy *string
	unlockReason      *string

	// preUnlockStatus remembers WHICH locked state (APPROVED or VALIDATED) the head
	// was parked from when an unlock was requested, so RejectUnlock can put it back
	// exactly there instead of guessing.
	//
	// 🔴 It has ⛔ NO COLUMN of its own. On a freshly loaded entity it is restored by
	// the repository from mst_mb_workflow_log (the row whose to_state is
	// UNLOCK_REQUESTED), which is the source the design names. Empty string means
	// "unknown" and RejectUnlock refuses rather than inventing a target.
	preUnlockStatus string
}

// NewParams carries the construction arguments for New. It replaces the former
// 20-positional-parameter signature (K-5): positional arguments at that width are
// unreadable and trivially mis-ordered at the call site.
type NewParams struct {
	MBCosting   string
	OracleSysID *string
	MgtName     *string
	Denier      *float64
	Filament    *int
	Dozing      *float64
	// ⛔ MBHCheckStatus is ABSENT by design (§11 item 106, decision K-1). A newly
	// created head has no Oracle import trace — that column is written only by the
	// historical Oracle import, never by this application. Reconstruct still carries
	// it, because READING the stored trace back is required and untouched.
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHRunLdrPct    *float64
	MBHFinalProduct *string
	MBHCode         *string
	CreatedBy       string
	IsBoughtout     bool
	DevCode         string
	ShadeCode       string
	ShadeName       string
	CrossSection    string
	LustureCode     string
	MachineID       *uuid.UUID

	// VSNumber is free text (B9/OQ-17): no format, no normalization, no uniqueness
	// in the domain. The literal "NA" is an accepted value. Uniqueness for changed
	// values is an application-layer concern.
	VSNumber *string
	// NoOfProcess is the live user choice. Nil stays NULL — ⛔ no default is encoded
	// here, the default is an open user decision (U-B).
	NoOfProcess *string
}

// New creates a new MB Head entity with validation.
func New(p NewParams) (*Entity, error) {
	if p.MBCosting == "" {
		return nil, ErrEmptyMBCosting
	}
	if len(p.MBCosting) > 100 {
		return nil, ErrMBCostingTooLong
	}
	if p.CreatedBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	e := &Entity{
		id: uuid.New(), oracleSysID: p.OracleSysID, mbCosting: p.MBCosting, mgtName: p.MgtName,
		denier: p.Denier, filament: p.Filament, dozing: p.Dozing,
		mbhStatus: p.MBHStatus, mbhLdrPrsn: p.MBHLdrPrsn,
		mbhRunLdrPct:    p.MBHRunLdrPct,
		mbhFinalProduct: p.MBHFinalProduct, mbhCode: p.MBHCode,
		isActive: true, createdAt: time.Now(), createdBy: p.CreatedBy,
		isBoughtout: p.IsBoughtout, devCode: p.DevCode, shadeCode: p.ShadeCode,
		shadeName: p.ShadeName, crossSection: p.CrossSection, lustureCode: p.LustureCode,
		machineID: p.MachineID,
		vsNumber:  p.VSNumber, noOfProcess: p.NoOfProcess,
	}
	// Seed the derived column for a brand-new head. The row lands in the DB with
	// mbh_entry_status = 'DRAFT' (column DEFAULT, migration 000445:6-7) — the INSERT
	// does not carry the column — so DRAFT is passed explicitly here.
	//
	// ⛔ e.entryStatus is deliberately LEFT AT ITS ZERO VALUE: New has never wired it,
	// several tests depend on that, and setting it here would change what
	// entry_status a create response reports. That is a separate change, not this one.
	// The value still comes from the SAME pure function every other path uses — no
	// hardcoded literal, no second implementation of the rules.
	e.mbhCheckStatusCalc = DeriveCheckStatus(StatusDraft, p.IsBoughtout)
	return e, nil
}

// PersistedExtras carries the P5 recipe/lock columns back from storage.
//
// It is a separate hydration step rather than six more positional parameters on
// Reconstruct, which already carries 40. Reconstruct and this method deliberately
// perform NO validation: legacy rows must round-trip unchanged (K8).
type PersistedExtras struct {
	// MBHCheckStatusCalc is the stored derived check status. nil means the row has
	// never been calculated by the application (the permanent state of 207 legacy
	// rows — there is no backfill), ⛔ not "no status".
	MBHCheckStatusCalc *string
	VSNumber           *string
	NoOfProcess        *string
	AdditionalShades   []Shade
	IsLocked           bool
	LockedAt           *time.Time
	LockedBy           *string
	UnlockRequestedAt  *time.Time
	UnlockRequestedBy  *string
	UnlockReason       *string
	// PreUnlockStatus is the locked state an UNLOCK_REQUESTED head came from, read
	// back from mst_mb_workflow_log. Empty when the head is not parked in
	// UNLOCK_REQUESTED — or when the log holds no such row.
	PreUnlockStatus string
}

// HydrateExtras applies persisted P5 columns onto a Reconstruct-ed entity. It is
// unvalidated on purpose — see PersistedExtras.
func (e *Entity) HydrateExtras(x PersistedExtras) {
	e.mbhCheckStatusCalc = x.MBHCheckStatusCalc
	e.vsNumber = x.VSNumber
	e.noOfProcess = x.NoOfProcess
	e.additionalShades = x.AdditionalShades
	e.isLocked = x.IsLocked
	e.lockedAt = x.LockedAt
	e.lockedBy = x.LockedBy
	e.unlockRequestedAt = x.UnlockRequestedAt
	e.unlockRequestedBy = x.UnlockRequestedBy
	e.unlockReason = x.UnlockReason
	e.preUnlockStatus = x.PreUnlockStatus
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

// MBHCheckStatus returns the optional Oracle check status. ⛔ FROZEN import trace —
// read-only for the application. The derived value is MBHCheckStatusCalc.
func (e *Entity) MBHCheckStatus() *string { return e.mbhCheckStatus }

// MBHCheckStatusCalc returns the DERIVED check status (mbh_check_status_calc), nil
// when it has never been calculated. nil is a legitimate, permanent value for the
// 207 legacy heads — callers ⛔ must not coerce it to "".
func (e *Entity) MBHCheckStatusCalc() *string { return e.mbhCheckStatusCalc }

// RecomputeCheckStatusCalc recalculates the derived check status from the entity's
// current entry status and bought-out flag, and stores it on the entity.
//
// 🔴 When DeriveCheckStatus has no rule for the current state it returns nil, and
// this method then leaves the existing value UNTOUCHED rather than clearing it —
// an undecided state must not silently erase a previously computed value.
//
// ⛔ It never writes mbhCheckStatus. Persisting the result is the repository's job.
func (e *Entity) RecomputeCheckStatusCalc() {
	if v := DeriveCheckStatus(e.entryStatus, e.isBoughtout); v != nil {
		e.mbhCheckStatusCalc = v
	}
}

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

// StateReason returns the reason recorded for the current ~~UnApprove/Revoke~~
// Reject/ReturnToDraft/unlock transition, empty otherwise.
//
// 🔴 2026-08-26 — UnApprove and Revoke no longer produce a reason because both were
// removed as features. Reasons stored by them on pre-existing rows stay readable.
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

// VSNumber returns the VS Number (mbh_vs_number), nil when never set. Free text —
// the domain applies no format, no normalization and no uniqueness rule.
func (e *Entity) VSNumber() *string { return e.vsNumber }

// NoOfProcess returns the live Number-of-Process choice (mbh_no_of_process), nil
// when unset. ⛔ Distinct from ParamNoOfProcess, which is the frozen snapshot.
func (e *Entity) NoOfProcess() *string { return e.noOfProcess }

// IsLocked reports whether the recipe is locked. NULL in storage reads as false.
func (e *Entity) IsLocked() bool { return e.isLocked }

// UnlockRequestedAt returns when an unlock was requested, nil when none is pending.
func (e *Entity) UnlockRequestedAt() *time.Time { return e.unlockRequestedAt }

// UnlockRequestedBy returns who requested the unlock, nil when none is pending.
func (e *Entity) UnlockRequestedBy() *string { return e.unlockRequestedBy }

// LockedAt returns when the recipe was locked, nil when it never was.
func (e *Entity) LockedAt() *time.Time { return e.lockedAt }

// LockedBy returns who locked the recipe, nil when it never was locked.
func (e *Entity) LockedBy() *string { return e.lockedBy }

// UnlockReason returns the reason given for the pending unlock request, nil when
// none. 🔴 It is ⛔ NOT cleared by GrantUnlock or RejectUnlock — the trail of WHY
// an unlock was asked for stays readable (principle U-2).
func (e *Entity) UnlockReason() *string { return e.unlockReason }

// PreUnlockStatus returns the locked state an UNLOCK_REQUESTED head came from
// (APPROVED or VALIDATED), or "" when the head is not parked or the origin is
// unknown. Persisted nowhere of its own — restored from mst_mb_workflow_log.
func (e *Entity) PreUnlockStatus() string { return e.preUnlockStatus }

// IsEditable reports whether the recipe's content may be mutated right now.
//
// 🔴 A locked head is NOT editable, and a head parked in UNLOCK_REQUESTED is not
// editable either — the unlock has been asked for, not yet granted. Editability
// returns only once GrantUnlock has run. A soft-deleted head is never editable.
func (e *Entity) IsEditable() bool {
	if e.IsDeleted() {
		return false
	}
	if e.entryStatus == StatusUnlockRequested {
		return false
	}
	return !e.isLocked
}

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	MBCosting *string
	MgtName   *string
	Denier    *float64
	Filament  *int
	Dozing    *float64
	// ⛔ MBHCheckStatus is ABSENT by design (§11 item 106, decision K-1). The frozen
	// Oracle trace has no update path: Update() must be structurally incapable of
	// touching it, not merely careful not to. Do not re-add this field.
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
	// VSNumber / NoOfProcess follow the existing pointer-nil-check convention: nil
	// means "field absent from the payload", so the stored value is left untouched
	// (D13). ⛔ Absent is never coerced into a default.
	VSNumber    *string
	NoOfProcess *string
}

// Update applies optional field changes to the entity.
//
// 🔴 P10: a LOCKED recipe refuses every mutation with ErrHeadLocked — ⛔ never a
// silent success that quietly drops the caller's edits. The same applies to a head
// parked in UNLOCK_REQUESTED: asking for an unlock is not the same as getting one.
// See IsEditable, which is the single place this rule is expressed.
//
// ⚠ In practice this changes nothing for existing data: mbh_is_locked is NULL on
// every legacy row (000485) and NULL means "not locked".
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if !e.IsEditable() {
		return ErrHeadLocked
	}
	if err := e.applyMBCosting(in.MBCosting); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	e.applyRecipeIdentityFields(in)
	e.applyRecipeExtraFields(in)
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
	e.RecomputeCheckStatusCalc()
	return nil
}

// Approve transitions SUBMITTED → APPROVED, or UN_APPROVED → APPROVED (revalidate path).
//
// 🔴 FIXED 2026-08-26 — it now also LOCKS the entity in memory. APPROVED is a
// lockOnEnter state, so the SQL layer (DeriveLockEffect → lockClauses) has always
// written mbh_is_locked = TRUE for this move, but the in-memory entity was left with
// isLocked = false. The gRPC response is built from that same entity, so the caller was
// told mbhIsLocked=false about a row the database had just locked.
//
// ⛔ This does NOT duplicate or contradict the SQL effect: lockOnEnter is the SAME
// predicate DeriveLockEffect consults, so the two agree by construction, and Lock() is
// idempotent (a second call only refreshes actor/timestamp). The entity is not the
// producer of the SQL — the repository derives its clauses from the state names alone —
// so no extra column write is created here.
//
// ⛔ No actor is passed: these transition methods take none, and Lock("") deliberately
// leaves lockedBy untouched rather than inventing an actor. The authoritative
// mbh_locked_by is written by lockClauses from the transition's real actorUserID.
func (e *Entity) Approve() error {
	if !canTransition(e.entryStatus, StatusApproved) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusApproved
	e.lockOnEnterState()
	e.RecomputeCheckStatusCalc()
	return nil
}

// lockOnEnterState locks the entity when its CURRENT status is one the workflow locks on
// entry (APPROVED, VALIDATED). Sharing lockOnEnter with DeriveLockEffect is the point:
// the in-memory flag and the persisted column can never disagree about which states lock.
func (e *Entity) lockOnEnterState() {
	if lockOnEnter(e.entryStatus) {
		e.Lock("")
	}
}

// Validate transitions to VALIDATED: from SUBMITTED (the 2026-08-26 "Opsi A" Approve
// path), from APPROVED (legacy rows, still reachable through the ValidateMBHead RPC), or
// from DRAFT directly for boughtout MBs.
//
// 🔴 REWRITTEN 2026-08-26 (USER DECISION, "Opsi A") — the boughtout branch used to pin
// entryStatus == StatusDraft, i.e. a boughtout recipe could ONLY be validated straight
// out of DRAFT, via the Validate button. That button was removed from the UI, so a
// boughtout recipe now travels Submit → Approve like every other one, and its status at
// this point is SUBMITTED, not DRAFT. Keeping the old pin would have STRANDED every
// boughtout recipe in SUBMITTED with no legal move to VALIDATED.
//
// ⛔ The DRAFT shortcut was NOT removed — it is now expressed through the shared state
// map (allowedTransitions[StatusDraft] already lists StatusValidated), so a boughtout
// recipe can still be validated straight from DRAFT by the surviving ValidateMBHead RPC.
// Both branches now consult canTransition, which means the boughtout path became WIDER,
// never narrower, and no previously-legal boughtout move was lost.
//
// The residual difference between the two kinds of recipe is enforced one layer up: the
// application ValidateHandler gates which ORIGIN statuses an own-production recipe may
// validate from (design.md §2.1), because the state map alone cannot express it.
func (e *Entity) Validate() error {
	if !canTransition(e.entryStatus, StatusValidated) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusValidated
	e.currentVersion++
	// VALIDATED is a lockOnEnter state; see Approve for why the entity locks itself here.
	e.lockOnEnterState()
	e.RecomputeCheckStatusCalc()
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

// UnApprove ~~transitions APPROVED → UN_APPROVED, requiring a reason.~~
//
// 🔴 USER DECISION 2026-08-26 — Un-approve was REMOVED as a feature. The workflow is
// now DRAFT → SUBMITTED → APPROVED (locked); a locked recipe is reopened through the
// P10 Request Unlock flow, ⛔ never by un-approving it. This method therefore ALWAYS
// returns ErrInvalidTransition and mutates nothing.
//
// ⛔ Deliberately NOT deleted, and neither is StatusUnApproved: the RPC still exists
// in the proto contract (removing it would be a breaking change) and production
// already holds rows sitting in UN_APPROVED. Those rows stay readable and can still
// move on to APPROVED; only the entrance to UN_APPROVED was closed.
//
// The reason parameter is retained so the signature keeps documenting the removed
// contract, but it is ignored — the transition is refused before any argument matters.
func (e *Entity) UnApprove(_ string) error {
	return ErrInvalidTransition
}

// Reject transitions SUBMITTED → REJECTED, requiring a reason (user decision K-2).
// ⛔ Not terminal: a REJECTED head may still be Revoked (K-24) and may return to DRAFT.
func (e *Entity) Reject(reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	if !canTransition(e.entryStatus, StatusRejected) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusRejected
	e.stateReason = reason
	e.RecomputeCheckStatusCalc()
	return nil
}

// ReturnToDraft transitions REJECTED → DRAFT so the author can rework the recipe
// (user decision K-29, 2026-08-23).
//
// 🔴 The reason is OPTIONAL here — deliberately unlike Reject/UnApprove/Revoke, which
// all mandate one. Sending work back to its author is not an accusation, so nothing
// needs to be justified.
//
// 🔴 stateReason is PRESERVED, ⛔ never cleared. When the caller supplies a non-empty
// reason it OVERWRITES the stored one; when it is empty the previous value stays
// readable. That is the whole point of K-29: the author must still be able to see WHY
// the MB was rejected while fixing it. It also follows the global principle U-2 —
// don't erase the trail.
func (e *Entity) ReturnToDraft(reason string) error {
	if !canTransition(e.entryStatus, StatusDraft) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusDraft
	if reason != "" {
		e.stateReason = reason
	}
	e.RecomputeCheckStatusCalc()
	return nil
}

// Revoke ~~transitions any non-terminal state to REVOKED, requiring a reason. Terminal —
// no further transitions are possible after Revoke.~~
//
// 🔴 USER DECISION 2026-08-26 — Revoke was REMOVED as a feature. Switching a recipe on
// or off is an admin concern served by the is_active flag, so a terminal REVOKED status
// is not needed. This method therefore ALWAYS returns ErrInvalidTransition and mutates
// nothing (canRevoke now always reports false).
//
// ⛔ Deliberately NOT deleted, and neither is StatusRevoked: the RPC still exists in the
// proto contract (removing it would be a breaking change) and production already holds
// rows sitting in REVOKED. Those rows stay readable; only the entrance was closed.
//
// The reason parameter is retained so the signature keeps documenting the removed
// contract, but it is ignored — the transition is refused before any argument matters.
func (e *Entity) Revoke(_ string) error {
	return ErrInvalidTransition
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

func (e *Entity) applyRecipeExtraFields(in UpdateInput) {
	if in.VSNumber != nil {
		e.vsNumber = in.VSNumber
	}
	if in.NoOfProcess != nil {
		e.noOfProcess = in.NoOfProcess
	}
}
