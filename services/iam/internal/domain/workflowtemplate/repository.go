package workflowtemplate

import (
	"context"

	"github.com/google/uuid"
)

// Filter is the list query parameters.
type Filter struct {
	Search       string
	Kind         string
	ActiveFilter string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// Repository persists workflow templates.
type Repository interface {
	Create(ctx context.Context, t *Template) error
	GetByID(ctx context.Context, id uuid.UUID) (*Template, error)
	GetActiveByKind(ctx context.Context, kind string) (*Template, error)
	// Activate atomically activates id and deactivates every other (non-deleted)
	// template of the same kind.
	Activate(ctx context.Context, id uuid.UUID, by string) (*Template, error)
	SoftDelete(ctx context.Context, id uuid.UUID, by string) error
	List(ctx context.Context, f Filter) (items []*Template, total int64, err error)
}
