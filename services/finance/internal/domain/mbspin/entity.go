// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the MB Spin domain.
type Entity struct {
	id              uuid.UUID
	oracleSysID     *string
	orionItemCode   *string
	headID          uuid.UUID
	mgtName         string
	denier          *float64
	filament        *int
	dozing          *float64
	mbCosting       *string
	cc              *string
	costRateMkt     *float64
	mbsStatus       *string
	mbsLdrPrsn      *float64
	mbsRunLdrPct    *float64
	mbsFinalProduct *string
	ldrIsFixed      *bool
	dozingIsFixed   *bool
	isActive        bool
	// Lineage / recalc trail — migration 000484. Populated via HydrateLineage,
	// deliberately NOT parameters of New/Reconstruct (both already oversized).
	parentSpinID *uuid.UUID
	duplicatedAt *time.Time
	duplicatedBy *string
	lastRecalcAt *time.Time
	lastRecalcBy *string
	// Cost-product ownership — migration 000490. Traceability only (D18).
	costProductID *int64
	createdAt     time.Time
	createdBy     string
	updatedAt     *time.Time
	updatedBy     *string
	deletedAt     *time.Time
	deletedBy     *string
}

// New creates a new MB Spin entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(headID uuid.UUID, mgtName string, oracleSysID, orionItemCode *string, denier *float64, filament *int, dozing *float64, mbCosting *string, cc *string, costRateMkt *float64, mbsStatus *string, mbsLdrPrsn, mbsRunLdrPct *float64, mbsFinalProduct *string, ldrIsFixed, dozingIsFixed *bool, createdBy string) (*Entity, error) {
	if headID == uuid.Nil {
		return nil, ErrInvalidHeadID
	}
	if mgtName == "" {
		return nil, ErrEmptyMgtName
	}
	if len(mgtName) > 100 {
		return nil, ErrMgtNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), oracleSysID: oracleSysID, orionItemCode: orionItemCode, headID: headID, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing, mbCosting: mbCosting,
		cc: cc, costRateMkt: costRateMkt,
		mbsStatus: mbsStatus, mbsLdrPrsn: mbsLdrPrsn, mbsRunLdrPct: mbsRunLdrPct, mbsFinalProduct: mbsFinalProduct,
		ldrIsFixed: ldrIsFixed, dozingIsFixed: dozingIsFixed,
		isActive: true, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds an MB Spin from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, oracleSysID, orionItemCode *string, headID uuid.UUID, mgtName string, denier *float64, filament *int, dozing *float64, mbCosting *string, cc *string, costRateMkt *float64, mbsStatus *string, mbsLdrPrsn, mbsRunLdrPct *float64, mbsFinalProduct *string, ldrIsFixed, dozingIsFixed *bool, isActive bool, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, oracleSysID: oracleSysID, orionItemCode: orionItemCode, headID: headID, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing, mbCosting: mbCosting,
		cc: cc, costRateMkt: costRateMkt,
		mbsStatus: mbsStatus, mbsLdrPrsn: mbsLdrPrsn, mbsRunLdrPct: mbsRunLdrPct, mbsFinalProduct: mbsFinalProduct,
		ldrIsFixed: ldrIsFixed, dozingIsFixed: dozingIsFixed,
		isActive: isActive, createdAt: createdAt, createdBy: createdBy,
		updatedAt: updatedAt, updatedBy: updatedBy, deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// OracleSysID returns the optional Oracle system ID.
func (e *Entity) OracleSysID() *string { return e.oracleSysID }

// OrionItemCode returns the optional Oracle ORION ERP item code (CMBS_ORION_ITEM_CODE).
func (e *Entity) OrionItemCode() *string { return e.orionItemCode }

// HeadID returns the parent MB head UUID.
func (e *Entity) HeadID() uuid.UUID { return e.headID }

// MgtName returns the management display name.
func (e *Entity) MgtName() string { return e.mgtName }

// Denier returns the optional spin denier.
func (e *Entity) Denier() *float64 { return e.denier }

// Filament returns the optional number of filaments.
func (e *Entity) Filament() *int { return e.filament }

// Dozing returns the optional dozing percentage.
func (e *Entity) Dozing() *float64 { return e.dozing }

// MBCosting returns the optional spin cost code.
func (e *Entity) MBCosting() *string { return e.mbCosting }

// CC returns the optional MB/SP cost code.
func (e *Entity) CC() *string { return e.cc }

// CostRateMkt returns the optional MB rate MKT USD/kg.
func (e *Entity) CostRateMkt() *float64 { return e.costRateMkt }

// MBSStatus returns the optional Oracle spin status.
func (e *Entity) MBSStatus() *string { return e.mbsStatus }

// MBSLdrPrsn returns the optional Oracle leader person value.
func (e *Entity) MBSLdrPrsn() *float64 { return e.mbsLdrPrsn }

// MBSRunLdrPct returns the optional Oracle CMBS_RUN_LDR_PRSN — the actual LDR
// percentage used in production. D30: this is the authoritative LDR for costing
// (mbsLdrPrsn is the planned value; dozing is the retired contaminated legacy column).
func (e *Entity) MBSRunLdrPct() *float64 { return e.mbsRunLdrPct }

// MBSFinalProduct returns the optional Oracle final product code.
func (e *Entity) MBSFinalProduct() *string { return e.mbsFinalProduct }

// LDRIsFixed returns the raw fix/actual marker for the LDR value.
// nil means "unknown" — see IsFixedLDR for the recalc-safe interpretation.
func (e *Entity) LDRIsFixed() *bool { return e.ldrIsFixed }

// DozingIsFixed returns the raw fix/actual marker for the dozing value.
// nil means "unknown" — see IsFixedDozing for the recalc-safe interpretation.
func (e *Entity) DozingIsFixed() *bool { return e.dozingIsFixed }

// IsFixedLDR reports whether the LDR value was filled by a human as an actual
// and therefore MUST NOT be overwritten by automatic recalculation (recalc rule
// #3, phase P13). This is the single authoritative predicate — do not
// re-derive this logic anywhere else.
//
// nil (unknown) is deliberately treated as FIXED: the ~2699 pre-existing Oracle
// rows all carry NULL, so erring toward "do not recalc" means the first recalc
// run cannot silently move numbers. Migration 000486 documents this contract.
func (e *Entity) IsFixedLDR() bool { return e.ldrIsFixed == nil || *e.ldrIsFixed }

// IsFixedDozing reports whether the dozing value was filled by a human as an
// actual. Same nil-means-fixed contract as IsFixedLDR; kept separate because
// recalc rule #3 is per-VALUE, not per-row.
func (e *Entity) IsFixedDozing() bool { return e.dozingIsFixed == nil || *e.dozingIsFixed }

// IsActive returns whether the spin is active.
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

// IsDeleted returns true if the spin is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	MgtName         *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	MBCosting       *string
	CC              *string
	CostRateMkt     *float64
	MBSStatus       *string
	MBSLdrPrsn      *float64
	MBSRunLdrPct    *float64
	MBSFinalProduct *string
	LDRIsFixed      *bool
	DozingIsFixed   *bool
	IsActive        *bool
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyMgtName(in.MgtName); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the spin as deleted.
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

func (e *Entity) applyMgtName(mgtName *string) error {
	if mgtName == nil {
		return nil
	}
	if *mgtName == "" {
		return ErrEmptyMgtName
	}
	if len(*mgtName) > 100 {
		return ErrMgtNameTooLong
	}
	e.mgtName = *mgtName
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.Denier != nil {
		e.denier = in.Denier
	}
	if in.Filament != nil {
		e.filament = in.Filament
	}
	if in.Dozing != nil {
		e.dozing = in.Dozing
	}
	if in.MBCosting != nil {
		e.mbCosting = in.MBCosting
	}
	if in.CC != nil {
		e.cc = in.CC
	}
	if in.CostRateMkt != nil {
		e.costRateMkt = in.CostRateMkt
	}
	if in.MBSStatus != nil {
		e.mbsStatus = in.MBSStatus
	}
	if in.MBSLdrPrsn != nil {
		e.mbsLdrPrsn = in.MBSLdrPrsn
	}
	if in.MBSRunLdrPct != nil {
		e.mbsRunLdrPct = in.MBSRunLdrPct
	}
	if in.MBSFinalProduct != nil {
		e.mbsFinalProduct = in.MBSFinalProduct
	}
	if in.LDRIsFixed != nil {
		e.ldrIsFixed = in.LDRIsFixed
	}
	if in.DozingIsFixed != nil {
		e.dozingIsFixed = in.DozingIsFixed
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
}

// =============================================================================
// Lineage / recalc trail (migration 000484) + cost-product ownership (000490)
// =============================================================================

// StatusRnD is the persisted spin status meaning "Research & Development".
//
// ⚠ Spelled "R and D" with spaces — not "RND", not "R&D". The value is compared
// and written verbatim against the Oracle-imported mbs_status column, so the
// exact spacing is load-bearing. Mirrors mbhead.DefaultMBHStatus.
//
// Every duplicated spin is born with this status (D5): a clone is always an
// R&D draft, never an inherited production/actual row.
const StatusRnD = "R and D"

// Lineage carries the migration-000484 duplication/recalc trail columns plus the
// migration-000490 cost-product ownership column back from storage.
//
// It is a separate hydration step rather than six more positional parameters on
// Reconstruct, which already carries 25 — the same reason mbhead uses
// PersistedExtras/HydrateExtras. Like that pair, this performs NO validation:
// the ~2699 legacy Oracle rows carry NULL in all six columns and must round-trip
// unchanged.
type Lineage struct {
	// ParentSpinID is the spin this row was duplicated FROM. nil = not a clone.
	ParentSpinID *uuid.UUID
	// DuplicatedAt / DuplicatedBy are set only on the duplicate path. nil = not a clone.
	DuplicatedAt *time.Time
	DuplicatedBy *string
	// LastRecalcAt / LastRecalcBy record the most recent automatic recalculation.
	// nil = this row has never been through the recalc path (the permanent state
	// of every legacy row — there is no backfill), ⛔ not "recalculated at zero".
	LastRecalcAt *time.Time
	LastRecalcBy *string
	// CostProductID is cost_product_master.cpm_product_sys_id owning this spin,
	// derived from the parent head's mbh_cost_product_id.
	//
	// NULLABLE PERMANENTLY (D23): a spin under a DRAFT head legitimately has no
	// cost product. ⛔ DILARANG dipakai sebagai jalur aliran cost (D18) — this is
	// traceability/ownership only, never a source of numbers.
	CostProductID *int64
}

// HydrateLineage applies the persisted 000484/000490 columns onto a
// Reconstruct-ed entity. Unvalidated on purpose — see Lineage.
func (e *Entity) HydrateLineage(l Lineage) {
	e.parentSpinID = l.ParentSpinID
	e.duplicatedAt = l.DuplicatedAt
	e.duplicatedBy = l.DuplicatedBy
	e.lastRecalcAt = l.LastRecalcAt
	e.lastRecalcBy = l.LastRecalcBy
	e.costProductID = l.CostProductID
}

// ParentSpinID returns the spin this row was duplicated from, or nil when this
// row is not a clone.
func (e *Entity) ParentSpinID() *uuid.UUID { return e.parentSpinID }

// DuplicatedAt returns when this row was produced by the duplicate action, or nil.
func (e *Entity) DuplicatedAt() *time.Time { return e.duplicatedAt }

// DuplicatedBy returns who ran the duplicate action that produced this row, or nil.
func (e *Entity) DuplicatedBy() *string { return e.duplicatedBy }

// LastRecalcAt returns when this spin was last recalculated, or nil when it has
// never been through the recalc path.
func (e *Entity) LastRecalcAt() *time.Time { return e.lastRecalcAt }

// LastRecalcBy returns the actor of the last recalculation, or nil.
func (e *Entity) LastRecalcBy() *string { return e.lastRecalcBy }

// CostProductID returns the owning cost_product_master.cpm_product_sys_id, or
// nil for a spin under a DRAFT head (a permanently legitimate state, D23).
// ⛔ Traceability only (D18) — never read this as a cost input.
func (e *Entity) CostProductID() *int64 { return e.costProductID }

// IsClone reports whether this spin was produced by the duplicate path.
func (e *Entity) IsClone() bool { return e.parentSpinID != nil }

// IsRnD reports whether this spin carries the R&D status, i.e. whether it is a
// candidate for automatic recalculation when its parent changes (A6). Spins with
// Spinning / Boughtout / NULL status are non-candidates (A7) and are skipped.
func (e *Entity) IsRnD() bool { return e.mbsStatus != nil && *e.mbsStatus == StatusRnD }
