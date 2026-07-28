// Package wastecategory provides application-layer handlers for waste-category CRUD.
package wastecategory

import (
	"context"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	wastedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/wastecategory"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all waste-category use cases over a single repository.
type Service struct {
	repo wastedomain.Repository
}

// NewService creates a new waste-category application service.
func NewService(repo wastedomain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a waste category.
type CreateCommand struct {
	Area        string
	Type        string
	Code        string
	Name        string
	GradeTarget *string
	SortOrder   int32
	CreatedBy   string
}

// Create validates and persists a new waste category.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*wastedomain.Category, error) {
	a, err := area.New(cmd.Area)
	if err != nil {
		return nil, wastedomain.ErrInvalidArea
	}
	entity, err := wastedomain.NewCategory(a, cmd.Type, cmd.Code, cmd.Name, cmd.GradeTarget, cmd.SortOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a waste category by ID.
func (s *Service) Get(ctx context.Context, id int64) (*wastedomain.Category, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a waste category.
type UpdateCommand struct {
	ID          int64
	Name        *string
	GradeTarget *string
	IsActive    *bool
	SortOrder   *int32
	UpdatedBy   string
}

// Update mutates an existing waste category.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*wastedomain.Category, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Name, cmd.GradeTarget, cmd.IsActive, cmd.SortOrder, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a waste category by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing waste categories.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	Area      string
	Type      string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult holds a page of waste categories plus pagination metadata.
type ListResult struct {
	Items       []*wastedomain.Category
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of waste categories.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := wastedomain.ListFilter{
		Search:    query.Search,
		Area:      query.Area,
		Type:      query.Type,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
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
