package changeover

import (
	"context"
	"time"
)

// Filter constrains a changeover events list query.
type Filter struct {
	Page      int32
	PageSize  int32
	MachineID *int64
	DateFrom  *time.Time
	DateTo    *time.Time
	Status    string
}

// Repository is the persistence contract for changeover events and their
// components.
type Repository interface {
	Create(ctx context.Context, event *Event) error
	GetByID(ctx context.Context, id int64) (*Event, error)
	List(ctx context.Context, filter Filter) ([]*Event, int64, error)
	UpdateActual(ctx context.Context, event *Event) error
}

// WOSpecSource resolves a work order's changeover-relevant specification (from
// its spec snapshot or product parameters). Implemented in infrastructure.
type WOSpecSource interface {
	SpecForWO(ctx context.Context, woID int64) (Spec, bool, error)
	MachineForWO(ctx context.Context, woID int64) (machineID int64, ok bool, err error)
}
