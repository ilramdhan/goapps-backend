// Package prdrequest contains the ProductRequest aggregate (ticket) and supporting types.
package prdrequest

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter narrows the result set returned by Repository.List.
type ListFilter struct {
	// Search is a free-text query applied against the FTS index.
	Search string

	// Status filters by ticket lifecycle status. Empty means no filter.
	Status string

	// RequesterID filters by the UUID of the requester. Nil means no filter.
	RequesterID *uuid.UUID

	// AssignedTo filters by the UUID of the assignee. Nil means no filter.
	AssignedTo *uuid.UUID

	// RequesterDeptID filters by the requester's department UUID. Nil means no filter.
	RequesterDeptID *uuid.UUID

	// SortField is the field name to sort by; mapped via the repo's sortColumnMap.
	SortField string

	// SortDesc controls sort direction; true = descending.
	SortDesc bool

	// Page is the 1-based page number.
	Page int

	// PageSize is the number of results per page.
	PageSize int
}

// Repository is the persistence contract for the Request aggregate.
type Repository interface {
	// Create persists a new Request.
	Create(ctx context.Context, r *Request) error

	// GetByID retrieves a Request by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*Request, error)

	// GetByTicketNo retrieves a non-deleted Request by its ticket number string.
	GetByTicketNo(ctx context.Context, ticketNo string) (*Request, error)

	// List retrieves Requests matching the filter with pagination.
	// Returns the matching items, the total count across all pages, and any error.
	List(ctx context.Context, f ListFilter) (items []*Request, total int, err error)

	// Update persists mutations to an existing Request.
	Update(ctx context.Context, r *Request) error

	// Delete soft-deletes a Request by its UUID.
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
}
