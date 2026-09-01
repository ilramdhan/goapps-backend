// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the persistence interface for MB Head.
type Repository interface {
	// Create persists a new MB Head.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an MB Head by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByMBCosting retrieves an MB Head by its unique mb_costing value.
	GetByMBCosting(ctx context.Context, mbCosting string) (*Entity, error)

	// List retrieves MB Heads with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing MB Head.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks an MB Head as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByMBCosting checks if an MB Head with the given mb_costing exists.
	ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error)

	// ExistsByID checks if an MB Head with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted MB Heads matching filter, unpaginated (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*Entity, error)

	// Transition atomically persists a workflow-state change: updates mst_mb_head's
	// entry_status/current_version/state_reason (and, when params is non-nil, the frozen
	// mbh_param_* snapshot columns), inserts a mst_mb_workflow_log audit row, and — only when
	// toState is StatusValidated — snapshots the current composition into
	// mst_mb_composition_version. All writes commit or roll back together.
	Transition(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *ParamSnapshot) error

	// TransitionWithAutoGen performs the same work as Transition, then — only when toState is
	// StatusValidated and entity's cost product has not already been generated — auto-generates
	// the linked cost_product_master/cost_route_*/CAPP/CPP rows per PRD §8.2 and writes back
	// mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by onto mst_mb_head. All
	// writes (transition + auto-gen) commit or roll back together.
	TransitionWithAutoGen(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *ParamSnapshot, entity *Entity) error

	// ListShades returns the additional shade rows (mst_mb_head_shade) for one head,
	// ordered by mbhs_seq_no, excluding soft-deleted rows. An empty result is not an
	// error — most heads have no additional shades.
	//
	// ⚠ Unrelated to costerp's identically-named ListShades, which reads cost_erp_shade.
	ListShades(ctx context.Context, mbhID uuid.UUID) ([]Shade, error)

	// ReplaceShades replaces a head's additional shade rows wholesale, in ONE
	// transaction: existing live rows are soft-deleted and the supplied rows are
	// inserted. Passing an empty slice clears them. Validation of the slice shape is
	// the caller's job via Entity.SetAdditionalShades.
	ReplaceShades(ctx context.Context, mbhID uuid.UUID, shades []Shade, actorUserID string) error

	// ExistsByVSNumber reports whether any other live head already carries the given
	// mbh_vs_number, excluding excludeID (pass uuid.Nil on create).
	//
	// ⚠ This is a HELPER ONLY. The database index idx_mst_mb_head_vs_number is
	// deliberately NON-UNIQUE (000482): production holds 177 heads with '0' and two
	// with '16728'. Uniqueness is enforced in the application layer, only when the
	// value actually CHANGES, and must exclude '0' and the empty string — otherwise
	// every create collides with the legacy rows.
	ExistsByVSNumber(ctx context.Context, vsNumber string, excludeID uuid.UUID) (bool, error)

	// ExistsByDevCode reports whether any other live head already carries the given
	// mbh_dev_code, excluding excludeID (pass uuid.Nil on create).
	//
	// ⚠ HELPER ONLY, same contract as ExistsByVSNumber (U-D, 2026-08-22). There is
	// deliberately ⛔ NO UNIQUE constraint on mbh_dev_code: whatever duplicates
	// production already holds must stay readable and editable. Uniqueness is an
	// application-layer rule applied only to values that are actually being set or
	// changed, and the empty string is exempt.
	ExistsByDevCode(ctx context.Context, devCode string, excludeID uuid.UUID) (bool, error)

	// ForceUnvalidateTransition atomically forces a VALIDATED head back to DRAFT for the Bulk
	// MB Head Regenerate feature (Phase B): updates mst_mb_head's entry_status/state_reason,
	// clears the lock columns (is_locked/unlock_requested_at/unlock_requested_by), resets the
	// cost-product linkage (mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by so
	// a subsequent Bulk Validate takes the FULL auto-gen path instead of the lighter
	// regenerate-RMs-only path), and inserts a mst_mb_workflow_log audit row. All writes commit
	// or roll back together.
	ForceUnvalidateTransition(ctx context.Context, id uuid.UUID, currentVersion int, stateReason, actorUserID string) error

	// RefreezeCostParams updates the frozen mbh_param_* columns on mst_mb_head and re-runs the
	// CPP (cost_product_parameter) freeze from the entity's current in-memory param getters.
	// Unlike Validate, this does not change entry_status, does not bump the version, does not
	// create a workflow-log row, and does not attempt auto-gen — it assumes the cost product
	// already exists. Safe to run against already-VALIDATED heads whose frozen values were
	// incorrect (e.g. after ENG-MB-01's fix for the throughput/no_of_process default bug).
	RefreezeCostParams(ctx context.Context, id uuid.UUID, entity *Entity, params *ParamSnapshot) error
}

// ParamSnapshot carries the 8 frozen recipe-parameter values for a VALIDATE transition. Nil
// when the transition is not VALIDATE (Submit/Approve/UnApprove/Revoke pass nil).
type ParamSnapshot struct {
	Waste, QualityLoss, Efficiency, DevExpense, Packing, MBProdPerDay *string
	ThroughputPerHour, NoOfProcess                                    string
}

// ListFilter contains filtering options for listing MB Heads.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mbh_mb_costing", "mbh_mgt_name", "mbh_denier", "created_at"
	SortOrder string // "asc", "desc"
	// CostProductID filters by mst_mb_head.mbh_cost_product_id (R16). Nil means no
	// filter — this column is only populated once a head reaches VALIDATED
	// (see mb_autogen_repository.go), so a still-DRAFT head is never matched here.
	CostProductID *int64
}

// Validate normalizes filter values to safe defaults.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.SortBy == "" {
		f.SortBy = "mbh_mb_costing"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}

// ExportFilter contains filtering options for exporting MB Heads.
type ExportFilter struct {
	IsActive *bool
	// IncludeRejected, when false (the zero value), EXCLUDES heads whose workflow
	// status (mbh_entry_status) is StatusRejected from the export. This is the safe
	// default: a REJECTED head is not final, discarded output, and previously leaked
	// into every export unfiltered (§11 item 140).
	//
	// Set true only for an explicit "include rejected" export path (e.g. an audit
	// trail). ⚠ PENDING FOLLOW-UP: as of this change there is no proto field exposing
	// this flag, so no gRPC caller can currently set it to true — every request from
	// delivery gets the Go zero value (false). The plumbing exists so the option is
	// ready the moment a proto field is added; until then this can only be exercised
	// from Go code (e.g. tests) or a future internal caller.
	IncludeRejected bool
}

// RelockCandidate identifies one head whose granted unlock window has expired and
// carries the state it must be returned to.
type RelockCandidate struct {
	// ID is mst_mb_head.mbh_id.
	ID uuid.UUID
	// MBCosting is carried for log messages only — a UUID alone is unreadable in ops.
	MBCosting string
	// PreUnlockStatus is the locked state the head was unlocked FROM (APPROVED or
	// VALIDATED), read from mst_mb_workflow_log the same way selectCols reads it.
	// ⚠ It may be EMPTY when the workflow log has no matching row; the caller must
	// then SKIP the candidate and warn. ⛔ Never guess between APPROVED and VALIDATED
	// (consistent with K-52 option (a) and with RejectUnlock's refusal).
	PreUnlockStatus string
	// CurrentVersion is mbh_current_version, carried so the relock's workflow-log row
	// records the same version the head is on. ⛔ The relock does NOT bump it: no
	// content changed.
	CurrentVersion int32
	// GrantedAt is mbhl_actor_at of the UNLOCK_GRANT row that opened the window — the
	// moment the head became editable, on the DATABASE clock.
	//
	// 🔴 It is carried so ApplyRelock can RE-CHECK the untouched-since-grant condition
	// inside its own transaction. Between ListExpiredUnlocks and the UPDATE a user may
	// save; without the re-check the relock would overwrite that save. ⛔ Never compare
	// it against a Go-side time.Now(): both sides of the comparison must be the database
	// clock, which is what wrote both mbhl_actor_at and updated_at.
	GrantedAt time.Time
}

// RelockRepository is the persistence contract for the auto-relock job.
//
// 🔴 It is a SEPARATE interface, deliberately ⛔ NOT merged into Repository. Repository
// has 15 methods and at least one hand-written test double (MockRepository in
// internal/application/mbhead/handlers_test.go) plus a compile-time assertion in
// internal/infrastructure/postgres/mb_head_repository.go; adding two methods there
// would break every implementor for the benefit of one caller that needs neither
// CRUD nor the workflow transitions. The concrete PostgreSQL repository satisfies
// both interfaces, and the job depends only on this one.
type RelockRepository interface {
	// ListExpiredUnlocks returns every head whose granted unlock window has run out.
	//
	// A head qualifies when ALL of the following hold:
	//   - its MOST RECENT mst_mb_head_lock_log row is an UNLOCK_GRANT (a later
	//     RELOCK/LOCK/UNLOCK_REJECT row means the window is already closed, so the
	//     latest-row test is what makes this idempotent — a relocked head stops
	//     matching immediately),
	//   - that row's mbhl_auto_relock_at IS NOT NULL AND <= NOW() (the deadline
	//     GrantUnlock stamped has passed; measured against the DATABASE clock, the
	//     same clock that wrote it),
	//   - the head is still OPEN: COALESCE(mbh_is_locked, FALSE) = FALSE. ⛔ Never
	//     `mbh_is_locked = FALSE` — 4190 production rows hold NULL there, which means
	//     NOT locked and must still match,
	//   - mbh_entry_status = 'DRAFT' (where GrantUnlock parks a granted head; any
	//     other status means the head has since moved on under its own steam and the
	//     job must not yank it back),
	//   - 🔴 NOTHING WAS SAVED SINCE THE GRANT. The status test above CANNOT express
	//     this on its own: a head is DRAFT both when it was reopened and never touched
	//     AND when it is being actively edited (D7 keeps edits in DRAFT on purpose).
	//     Relocking the second case throws a user's in-progress work back to
	//     APPROVED/VALIDATED with no human assent — lost work, not an annoyance. The
	//     candidate therefore also requires every write timestamp of the recipe to be
	//     at or before the UNLOCK_GRANT instant. See ListExpiredUnlocks' SQL for the
	//     exact set of tables watched and why that set is the whole recipe,
	//   - deleted_at IS NULL.
	ListExpiredUnlocks(ctx context.Context) ([]RelockCandidate, error)

	// ApplyRelock closes one expired window in ONE transaction: it moves the head back
	// to toState, sets the lock columns, appends the mst_mb_workflow_log row, and
	// writes exactly ONE mst_mb_head_lock_log row with mbhl_event = 'RELOCK'.
	// ⛔ Never two transactions — a lock without its audit row is worse than a failure.
	//
	// toState MUST be a lockable state (APPROVED or VALIDATED); the caller supplies the
	// candidate's PreUnlockStatus and skips candidates whose origin is unknown.
	ApplyRelock(ctx context.Context, c RelockCandidate, toState string) error
}
