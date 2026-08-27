// Package mbworkflowlog provides domain logic for MB (Master Batch) head workflow-transition audit logging.
package mbworkflowlog

// Entity is a single MB head workflow-transition audit log row.
type Entity struct {
	id          string
	mbhID       string
	fromState   string
	toState     string
	actorUserID string
	actorAt     string
	reason      string
	version     int32
	// meta is the mbwl_meta JSONB document as raw JSON text, empty when the row
	// carries none.
	//
	// ⚠ The column has existed since migration 000448:10 but carried NO Go path
	// at all until P8 — it was neither written nor read. It is a free-form
	// document on purpose (the DOZING_CHANGED payload is not the only shape it
	// will ever hold), so the domain keeps it as opaque JSON text and does NOT
	// validate its shape: the producer owns the schema.
	meta string
}

// NewEntity constructs a new workflow-log row, validating mbh_id and to_state are present.
func NewEntity(mbhID, fromState, toState, actorUserID string) (*Entity, error) {
	if mbhID == "" {
		return nil, ErrMbhIDRequired
	}
	if toState == "" {
		return nil, ErrToStateRequired
	}
	return &Entity{mbhID: mbhID, fromState: fromState, toState: toState, actorUserID: actorUserID}, nil
}

// WithMeta attaches the mbwl_meta JSONB document to this log row and returns the
// same entity, so it can be chained onto NewEntity.
//
// Passing "" leaves the column NULL. ⛔ No shape validation happens here — see
// the meta field comment.
func (e *Entity) WithMeta(meta string) *Entity {
	e.meta = meta
	return e
}

// WithReason attaches the free-form mbwl_reason text and returns the same entity.
func (e *Entity) WithReason(reason string) *Entity {
	e.reason = reason
	return e
}

// Reconstruct rebuilds a workflow-log Entity from persisted values, bypassing NewEntity's
// validation since the row already exists in storage.
//
//nolint:revive // positional params mirror the hydration DTO's column order
func Reconstruct(id, mbhID, fromState, toState, actorUserID, actorAt, reason string, version int32, meta string) *Entity {
	return &Entity{
		id:          id,
		mbhID:       mbhID,
		fromState:   fromState,
		toState:     toState,
		actorUserID: actorUserID,
		actorAt:     actorAt,
		reason:      reason,
		version:     version,
		meta:        meta,
	}
}

// ID returns the log row's UUID.
func (e *Entity) ID() string { return e.id }

// MbhID returns the MB head this transition belongs to.
func (e *Entity) MbhID() string { return e.mbhID }

// FromState returns the state transitioned out of, empty if this is the initial transition.
func (e *Entity) FromState() string { return e.fromState }

// ToState returns the state transitioned into.
func (e *Entity) ToState() string { return e.toState }

// ActorUserID returns the user ID who performed this transition.
func (e *Entity) ActorUserID() string { return e.actorUserID }

// ActorAt returns the timestamp this transition occurred at.
func (e *Entity) ActorAt() string { return e.actorAt }

// Reason returns the free-form reason attached to this transition, if any.
func (e *Entity) Reason() string { return e.reason }

// Version returns the mb_head composition version this transition was recorded against.
func (e *Entity) Version() int32 { return e.version }

// Meta returns the mbwl_meta JSONB document as raw JSON text, empty when the row
// has none.
func (e *Entity) Meta() string { return e.meta }
