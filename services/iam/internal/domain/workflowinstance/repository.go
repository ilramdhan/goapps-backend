package workflowinstance

import (
	"context"

	"github.com/google/uuid"
)

// Filter is the list query params.
type Filter struct {
	EntityKind string
	EntityID   string
	Status     string
	Page       int
	PageSize   int
}

// Repository persists workflow instances.
type Repository interface {
	// Create persists a new instance + its first step in one tx.
	Create(ctx context.Context, ins *Instance) error
	// GetByID loads an instance + all its step rows.
	GetByID(ctx context.Context, id uuid.UUID) (*Instance, error)
	// SaveTransition updates the instance row and persists step changes (current
	// step decision recorded, optional new pending step row appended).
	SaveTransition(ctx context.Context, ins *Instance) error
	// List returns paginated instances (steps NOT preloaded — call GetByID for full detail).
	List(ctx context.Context, f Filter) (items []*Instance, total int64, err error)
}
