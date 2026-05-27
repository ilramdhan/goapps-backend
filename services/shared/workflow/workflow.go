// Package workflow provides a generic state-machine for master-data lifecycle.
// Transitions are validated against a predefined set of allowed edges.
// Each consumer maps their domain-specific enum to/from int32 State values.
package workflow

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// State is an opaque workflow state represented as int32 (matches proto enum values).
type State int32

// Action identifies the transition being performed.
type Action string

const (
	ActionSubmit        Action = "SUBMIT"
	ActionApprove       Action = "APPROVE"
	ActionRelease       Action = "RELEASE"
	ActionBypassRelease Action = "BYPASS_RELEASE"
)

// Errors.
var (
	ErrInvalidTransition = errors.New("invalid workflow transition")
	ErrAlreadyInState    = errors.New("entity is already in the target state")
)

// Transition defines a single allowed state change.
type Transition struct {
	From   State
	To     State
	Action Action
}

// Engine validates workflow transitions against a set of rules.
type Engine struct {
	transitions []Transition
}

// NewEngine creates an engine with the given transition rules.
func NewEngine(transitions []Transition) *Engine {
	return &Engine{transitions: transitions}
}

// Validate checks whether the given action is allowed for the current state.
// Returns the target state on success.
func (e *Engine) Validate(current State, action Action) (State, error) {
	for _, t := range e.transitions {
		if t.From == current && t.Action == action {
			return t.To, nil
		}
	}
	return current, fmt.Errorf("%w: cannot %s from state %d", ErrInvalidTransition, action, current)
}

// ValidateBypass validates a bypass transition (any pre-target state → target).
func (e *Engine) ValidateBypass(current, target State) error {
	if current == target {
		return ErrAlreadyInState
	}
	return nil
}

// HistoryEntry records a single workflow transition for audit.
type HistoryEntry struct {
	ID         uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	FromState  State
	ToState    State
	Action     Action
	UserID     string
	Notes      string
	OccurredAt time.Time
}

// NewHistoryEntry creates a new history entry.
func NewHistoryEntry(entityType string, entityID uuid.UUID, from, to State, action Action, userID, notes string) *HistoryEntry {
	return &HistoryEntry{
		ID:         uuid.New(),
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  from,
		ToState:    to,
		Action:     action,
		UserID:     userID,
		Notes:      notes,
		OccurredAt: time.Now(),
	}
}
