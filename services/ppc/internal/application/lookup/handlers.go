// Package lookup provides application-layer handlers for lookup-master CRUD.
package lookup

import (
	"context"

	lookupdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lookup"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all lookup use cases over a single repository.
type Service struct {
	repo lookupdomain.Repository
}

// NewService creates a new lookup application service.
func NewService(repo lookupdomain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a lookup.
type CreateCommand struct {
	Category  string
	Code      string
	Label     string
	SortOrder int32
	CreatedBy string
}

// Create validates and persists a new lookup.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*lookupdomain.Lookup, error) {
	entity, err := lookupdomain.NewLookup(cmd.Category, cmd.Code, cmd.Label, cmd.SortOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a lookup by ID.
func (s *Service) Get(ctx context.Context, id int64) (*lookupdomain.Lookup, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a lookup.
type UpdateCommand struct {
	ID        int64
	Label     *string
	SortOrder *int32
	IsActive  *bool
	UpdatedBy string
}

// Update mutates an existing lookup.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*lookupdomain.Lookup, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(cmd.Label, cmd.SortOrder, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a lookup by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing lookups.
type ListQuery struct {
	Page     int
	PageSize int
	Category string
	Search   string
	IsActive *bool
}

// ListResult holds a page of lookups plus pagination metadata.
type ListResult struct {
	Items       []*lookupdomain.Lookup
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of lookups.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := lookupdomain.ListFilter{
		Category: query.Category,
		Search:   query.Search,
		IsActive: query.IsActive,
		Page:     query.Page,
		PageSize: query.PageSize,
	}
	filter.Validate()

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
