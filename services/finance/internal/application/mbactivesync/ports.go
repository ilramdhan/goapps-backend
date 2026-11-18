package mbactivesync

import (
	"context"

	"github.com/google/uuid"
)

// SnapshotReader loads a consistent, read-only view of the MB master data the
// planner needs. Implemented in internal/infrastructure/postgres; all reads must
// happen inside a single read-only repeatable-read transaction.
type SnapshotReader interface {
	Load(ctx context.Context) (*Snapshot, error)
}

// Item result statuses (result.csv column `result`).
const (
	ResultApplied = "APPLIED"
	ResultPartial = "PARTIAL"
	ResultSkipped = "SKIPPED"
	ResultFailed  = "FAILED"
)

// Result reasons produced by the writer.
const (
	ReasonAlreadyInTarget = "already_in_target"
	ReasonStalePlan       = "stale_plan"
	ReasonFGMismatch      = "fg_mismatch"
	ReasonFGFollowsRecipe = "fg_follows_recipe"
	ReasonPOYInvalid      = "poy_invalid"
	ReasonShadeMismatch   = "shade_mismatch"
	ReasonInvalidLDR      = "invalid_ldr"
	ReasonDBError         = "db_error"
	// ReasonChangedSinceApply: revert found a key column that no longer holds the
	// after-image written by the apply (a later change) — the item is left alone.
	ReasonChangedSinceApply = "changed_since_apply"
	// ReasonAlreadyReverted: every column the item changed already holds its before-image.
	ReasonAlreadyReverted = "already_reverted"
	// ReasonNotRevertable: the tool does not revert this action (SUPERSEDE: use Un-supersede).
	ReasonNotRevertable = "not_revertable"
	// ReasonNewSpinNotReady: the superseding spin is missing, not live, on a non-live
	// head, or has no shade/FG yet (its phase row has not run) — rerun after it.
	ReasonNewSpinNotReady = "new_spin_not_ready"
	// ReasonNewHeadNotReady: SUPERSEDE_HEAD target head is deleted, inactive or superseded.
	ReasonNewHeadNotReady = "new_head_not_ready"
	// ReasonPartialClone: the clone exists (tx 1) but its target state (tx 2) failed;
	// a rerun finishes tx 2 on the same clone (design part 2 §5.4).
	ReasonPartialClone = "partial_clone"
	// ReasonPartialCloneInactiveHead (manual-items): a partial clone on an inactive head
	// was deactivated again (Q1 guard); a rerun reactivates it through tx 2.
	ReasonPartialCloneInactiveHead = "partial_clone_inactive_head"
	// ReasonValidateGate: the head could not be moved to VALIDATED through the existing
	// Submit/Validate use cases (state cannot transition, composition-sum gate, missing
	// param) — handled manually in the UI (design part 2 §5.5).
	ReasonValidateGate = "validate_gate"
	// ReasonSupersedeConfirmRequired: Validate would supersede live spins of other heads
	// (same shade + FG) but the plan has no SUPERSEDE pair for the head (confirm=false).
	ReasonSupersedeConfirmRequired = "supersede_confirmation_required"
)

// SyncSpinInput drives Writer.SyncExistingSpin: one existing spin to its target
// state in ONE transaction. SubActions (SET_FG, SET_LDR, SET_SHADE, ACT_SPIN,
// ACT_HEAD, SET_HEAD_SHADE) select what the writer may change; each one is
// applied only when the live row differs, so a rerun is idempotent.
type SyncSpinInput struct {
	SpinID          uuid.UUID
	Actor           string
	SubActions      []string
	TargetShadeCode string   // exact POY shade code
	TargetShadeName string   // file shade name ("" keeps the current name)
	TargetCS        string   // cross-section ("" keeps the current one)
	TargetLDR       *float64 // doz -> ACTUAL run LDR
	TargetFG        *int64   // POY product sys id
	// TargetDen / TargetFil are set only for a GENERATE_SPIN role=ROOT row (P2h-4c): the
	// generated root carries the head spec, so the writer also moves denier, filament
	// and (with TargetCS) cross-section to the row spec when they differ. Spin only,
	// never cascaded to the head. nil = leave the spin spec alone.
	TargetDen *float64
	TargetFil *int
}

// SyncSpec reports whether the input asks the writer to sync the spin spec (ROOT rows).
func (in SyncSpinInput) SyncSpec() bool { return in.TargetDen != nil || in.TargetFil != nil }

// Has reports whether sub is among the input sub-actions.
func (in SyncSpinInput) Has(sub string) bool {
	for _, s := range in.SubActions {
		if s == sub {
			return true
		}
	}
	return false
}

// ItemResult is the outcome of one writer call.
type ItemResult struct {
	Result      string // APPLIED | PARTIAL | SKIPPED | FAILED
	Reason      string
	SpinID      string
	CloneSpinID string
	HeadID      string
	BeforeJSON  string
	AfterJSON   string
	Message     string
	// ManualReason, when set, lists the item in manual-items.csv (operator follow-up).
	ManualReason string
}

// SupersedeInput drives Writer.SupersedeExisting (SUPERSEDE, design part 2 §5.6):
// the live old spin OldSpinID of OldHeadID is superseded by the existing spin
// NewSpinID (the applier resolves new_spin_ref spin:<id> / ln:<n> beforehand;
// uuid.Nil means "not created yet" → SKIPPED new_spin_not_ready).
type SupersedeInput struct {
	OldHeadID uuid.UUID // optional cross-check (uuid.Nil skips it)
	OldSpinID uuid.UUID
	NewSpinID uuid.UUID
	Actor     string
}

// SupersedeHeadInput drives Writer.SupersedeHeadOnly (SUPERSEDE_HEAD, Q15, §5.6a).
type SupersedeHeadInput struct {
	OldHeadID uuid.UUID
	NewHeadID uuid.UUID
	Actor     string
}

// DeactivateSpinInput drives Writer.DeactivateSpin (DEACTIVATE_SPIN, §5.7).
type DeactivateSpinInput struct {
	SpinID uuid.UUID
	Actor  string
}

// DeactivateHeadInput drives Writer.DeactivateHead (DEACTIVATE_HEAD, §5.7).
type DeactivateHeadInput struct {
	HeadID uuid.UUID
	Actor  string
}

// DuplicateTargetInput drives Writer.DuplicateToTarget (DUPLICATE_SPIN, GENERATE
// role=DUP — design part 2 §5.4): clone SourceSpinID through the existing duplicate
// use case (tx 1, name = source name per Q10), then move the clone to the target
// state with SyncExistingSpin semantics (tx 2).
type DuplicateTargetInput struct {
	Ln           int
	SourceSpinID uuid.UUID
	// CloneSpinID is the clone recorded for this ln by an earlier run (--prev-results);
	// uuid.Nil = claim an existing clone (deterministic fallback) or create one.
	CloneSpinID uuid.UUID
	// ExcludeClones are clones already claimed by other rows (prev-results + this run).
	ExcludeClones []uuid.UUID
	TargetDen     *float64 // nil = keep the source denier
	TargetFil     *int     // nil = keep the source filament
	// Target is the tx 2 target (SpinID is ignored; the writer sets the clone id).
	Target SyncSpinInput
	// OnCloned is called right after tx 1 with the PARTIAL result (clone_spin_id set)
	// so it is flushed before tx 2. An error stops the item before tx 2.
	OnCloned func(ItemResult) error
}

// DuplicateSpinRequest is one call of the existing duplicate-spin use case.
type DuplicateSpinRequest struct {
	SourceSpinID uuid.UUID
	MgtName      *string
	Denier       *float64
	Filament     *int
	FgProductID  *int64
	Actor        string
}

// SpinDuplicator runs the existing duplicate-spin use case (application/mbspin
// DuplicateHandler, its own transaction) and returns the clone id. Wired in cmd.
type SpinDuplicator interface {
	Duplicate(ctx context.Context, req DuplicateSpinRequest) (uuid.UUID, error)
}

// GenerateRootInput drives Writer.GenerateRootSpin (GENERATE_SPIN role=ROOT, K7 —
// design part 2 §5.5): the first (root) spin of a head that already has its MB cost
// product but no spin, built exactly like the Validate auto-gen root spin.
type GenerateRootInput struct {
	HeadID uuid.UUID
	Actor  string
}

// HeadWorkflow moves a head that never got its cost product (cost_product_id = 0) to
// VALIDATED through the existing UI use cases (Submit → Validate, boughtout: Validate
// from DRAFT), which auto-generates the cost product and the root spin (§5.5).
// confirmSupersede is true only when the plan pairs the head with a phase C SUPERSEDE.
// On APPLIED/SKIPPED already_in_target the result SpinID is the head's root spin.
type HeadWorkflow interface {
	EnsureValidated(ctx context.Context, headID uuid.UUID, actor string, confirmSupersede bool) (ItemResult, error)
}

// RevertInput drives Writer.Revert: one APPLIED result.csv record (design part 2 §8).
// Action/Phase are the original item's; BeforeJSON/AfterJSON are its images.
type RevertInput struct {
	Key         string
	Action      string
	Phase       Phase
	SpinID      uuid.UUID
	CloneSpinID uuid.UUID
	HeadID      uuid.UUID
	BeforeJSON  string
	AfterJSON   string
	Actor       string
}

// Writer applies reviewed plan items. One transaction per item; a non-nil error
// is a database failure (the item's transaction is rolled back).
type Writer interface {
	// SyncExistingSpin moves an existing spin (and optionally its head) to the target state.
	SyncExistingSpin(ctx context.Context, in SyncSpinInput) (ItemResult, error)
	// DuplicateToTarget clones a root spin (tx 1) and moves the clone to its target (tx 2).
	DuplicateToTarget(ctx context.Context, in DuplicateTargetInput) (ItemResult, error)
	// GenerateRootSpin creates the root spin of a head with cost product and no spin (K7).
	GenerateRootSpin(ctx context.Context, in GenerateRootInput) (ItemResult, error)
	// SupersedeExisting supersedes a live old spin by an existing new spin (phase B/C/D).
	SupersedeExisting(ctx context.Context, in SupersedeInput) (ItemResult, error)
	// SupersedeHeadOnly marks an old head without live spin superseded by the new head (Q15).
	SupersedeHeadOnly(ctx context.Context, in SupersedeHeadInput) (ItemResult, error)
	// DeactivateSpin deactivates one spin no product references (guard has_refs).
	DeactivateSpin(ctx context.Context, in DeactivateSpinInput) (ItemResult, error)
	// DeactivateHead deactivates a head and its live spins when none is referenced.
	DeactivateHead(ctx context.Context, in DeactivateHeadInput) (ItemResult, error)
	// Revert restores the before-image of one APPLIED item when its columns still hold
	// the after-image (optimistic, 1 tx); clones/generated spins are deactivated.
	Revert(ctx context.Context, in RevertInput) (ItemResult, error)
}
