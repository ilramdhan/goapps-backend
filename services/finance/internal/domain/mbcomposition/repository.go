package mbcomposition

import "context"

// SumGuard validates a pending composition write against the parent head's current
// non-carrier percentage total, as read inside the same transaction that will
// perform the write. currentSum is the raw decimal string returned by the database.
//
// Returning a non-nil error aborts and rolls back the write. A nil SumGuard means
// "no check" — the caller (the MB_COMPOSITION_SUM_ENFORCED flag) decided the rule
// is off, and the repository then skips both the lock and the sum query entirely.
type SumGuard func(currentSum string) error

// Repository defines the persistence contract for MB composition rows.
type Repository interface {
	Create(ctx context.Context, e *Entity) error
	Update(ctx context.Context, e *Entity) error

	// CreateWithSumGuard inserts e atomically with the composition-sum check (G24).
	//
	// When guard is non-nil the implementation MUST, in ONE transaction: take a row
	// lock on the parent mst_mb_head row, read the current non-carrier percentage
	// sum, call guard with it, and only then insert. That serializes concurrent
	// writers on the same mbh_id — without the lock two requests can each read a
	// total of 90, each add 10, and both pass while the stored total reaches 110.
	//
	// A nil guard is equivalent to Create.
	CreateWithSumGuard(ctx context.Context, e *Entity, guard SumGuard) error

	// UpdateWithSumGuard updates e under the same atomicity contract as
	// CreateWithSumGuard. A nil guard is equivalent to Update.
	UpdateWithSumGuard(ctx context.Context, e *Entity, guard SumGuard) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*Entity, error)
	ListByMbhID(ctx context.Context, mbhID string) ([]*Entity, error)
	SumPercentageByMbhID(ctx context.Context, mbhID string) (string, error)

	// ListVersionsByMbhID returns the frozen composition snapshot rows for mbhID at the given
	// version. version == 0 resolves to the latest version available for mbhID.
	ListVersionsByMbhID(ctx context.Context, mbhID string, version int32) ([]VersionRow, error)

	// ParentEntryStatus returns the mbh_entry_status of the parent mst_mb_head row
	// identified by mbhID, considering only rows that are not soft-deleted. It is the
	// single read the DRAFT gate needs: composition rows may be created, updated or
	// deleted ONLY while their parent head is still DRAFT.
	//
	// When no live parent row exists (missing or soft-deleted) it returns
	// ErrParentHeadNotFound, the same sentinel the guarded write paths report — a
	// composition write with no parent is the same failure however it is detected.
	//
	// ⚠ The status is a plain string on purpose, NOT a type from the mbhead packages:
	// application/mbhead already imports application/mbcomposition (its submit and
	// validate gates call EnforceHeadSum), so importing mbhead from here would create
	// an import cycle.
	ParentEntryStatus(ctx context.Context, mbhID string) (string, error)

	// ListMBRefEdgesForBatch returns the within-batch MB-to-MB composition
	// references among mbhIDs: one BatchRefEdge per live (non-deleted) MB
	// composition row whose SourceType is MB and whose OWN mbh_id and
	// referenced mb_ref_mbh_id are BOTH members of mbhIDs. References to heads
	// outside mbhIDs are deliberately excluded — this is used only to detect
	// same-batch ordering dependencies (see mbheadbulk.RequestBulkTransitionHandler),
	// not to resolve every recipe reference.
	ListMBRefEdgesForBatch(ctx context.Context, mbhIDs []string) ([]BatchRefEdge, error)
}

// BatchRefEdge is one within-batch MB-to-MB composition reference: the
// composition row belonging to MbhID references RefMbhID as a nested MB RM
// input, i.e. MbhID depends on RefMbhID.
type BatchRefEdge struct {
	MbhID    string
	RefMbhID string
}

// VersionRow is one frozen composition line from a VALIDATED snapshot.
type VersionRow struct {
	ID             string
	MbhID          string
	Version        int32
	ValidatedAt    string
	ValidatedBy    string
	SeqNo          int32
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
}
