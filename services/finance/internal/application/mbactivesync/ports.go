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
	// ReasonNewSpinNotReady: the superseding spin is missing, not live, on a non-live
	// head, or has no shade/FG yet (its phase row has not run) — rerun after it.
	ReasonNewSpinNotReady = "new_spin_not_ready"
	// ReasonNewHeadNotReady: SUPERSEDE_HEAD target head is deleted, inactive or superseded.
	ReasonNewHeadNotReady = "new_head_not_ready"
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
}

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

// Writer applies reviewed plan items. One transaction per item; a non-nil error
// is a database failure (the item's transaction is rolled back).
type Writer interface {
	// SyncExistingSpin moves an existing spin (and optionally its head) to the target state.
	SyncExistingSpin(ctx context.Context, in SyncSpinInput) (ItemResult, error)
	// SupersedeExisting supersedes a live old spin by an existing new spin (phase B/C/D).
	SupersedeExisting(ctx context.Context, in SupersedeInput) (ItemResult, error)
	// SupersedeHeadOnly marks an old head without live spin superseded by the new head (Q15).
	SupersedeHeadOnly(ctx context.Context, in SupersedeHeadInput) (ItemResult, error)
	// DeactivateSpin deactivates one spin no product references (guard has_refs).
	DeactivateSpin(ctx context.Context, in DeactivateSpinInput) (ItemResult, error)
	// DeactivateHead deactivates a head and its live spins when none is referenced.
	DeactivateHead(ctx context.Context, in DeactivateHeadInput) (ItemResult, error)
}
