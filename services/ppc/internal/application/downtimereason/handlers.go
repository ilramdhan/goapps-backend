// Package downtimereason provides application-layer handlers for downtime-reason CRUD.
package downtimereason

import (
	"context"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	downtimedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/downtimereason"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all downtime-reason use cases over a single repository.
type Service struct {
	repo downtimedomain.Repository
}

// NewService creates a new downtime-reason application service.
func NewService(repo downtimedomain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a downtime reason.
type CreateCommand struct {
	Area             string
	Code             string
	Name             string
	Category         string
	IsExcludeFromEff bool
	SortOrder        int32
	CreatedBy        string
}

// Create validates and persists a new downtime reason.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*downtimedomain.Reason, error) {
	a, err := area.New(cmd.Area)
	if err != nil {
		return nil, downtimedomain.ErrInvalidArea
	}
	entity, err := downtimedomain.NewReason(a, cmd.Code, cmd.Name, cmd.Category, cmd.IsExcludeFromEff, cmd.SortOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a downtime reason by ID.
func (s *Service) Get(ctx context.Context, id int64) (*downtimedomain.Reason, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a downtime reason.
type UpdateCommand struct {
	ID               int64
	Name             *string
	Category         *string
	IsExcludeFromEff *bool
	IsActive         *bool
	SortOrder        *int32
	UpdatedBy        string
}

// Update mutates an existing downtime reason.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*downtimedomain.Reason, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Name, cmd.Category, cmd.IsExcludeFromEff, cmd.IsActive, cmd.SortOrder, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a downtime reason by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing downtime reasons.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	Area      string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult holds a page of downtime reasons plus pagination metadata.
type ListResult struct {
	Items       []*downtimedomain.Reason
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of downtime reasons.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := downtimedomain.ListFilter{
		Search:    query.Search,
		Area:      query.Area,
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
