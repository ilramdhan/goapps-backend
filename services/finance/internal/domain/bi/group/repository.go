package group

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the contract the infrastructure layer must implement for the
// DashboardGroup aggregate.
type Repository interface {
	Create(ctx context.Context, g *Group) error
	GetByID(ctx context.Context, id uuid.UUID) (*Group, error)
	GetByCode(ctx context.Context, code string) (*Group, error)
	List(ctx context.Context, includeInactive bool) ([]*Group, error)
	Update(ctx context.Context, g *Group) error
	Delete(ctx context.Context, id uuid.UUID) error
}
