// Package shift provides application-layer handlers for shift-master CRUD.
package shift

import (
	"context"

	shiftdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/shift"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all shift use cases over a single repository.
type Service struct {
	repo shiftdomain.Repository
}

// NewService creates a new shift application service.
func NewService(repo shiftdomain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a shift.
type CreateCommand struct {
	Code      string
	Name      string
	StartTime string
	EndTime   string
	CreatedBy string
}

// Create validates and persists a new shift.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*shiftdomain.Shift, error) {
	entity, err := shiftdomain.NewShift(cmd.Code, cmd.Name, cmd.StartTime, cmd.EndTime, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a shift by ID.
func (s *Service) Get(ctx context.Context, id int64) (*shiftdomain.Shift, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a shift.
type UpdateCommand struct {
	ID        int64
	Name      *string
	StartTime *string
	EndTime   *string
	IsActive  *bool
	UpdatedBy string
}

// Update mutates an existing shift.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*shiftdomain.Shift, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(cmd.Name, cmd.StartTime, cmd.EndTime, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a shift by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing shifts.
type ListQuery struct {
	Page     int
	PageSize int
	IsActive *bool
}

// ListResult holds a page of shifts plus pagination metadata.
type ListResult struct {
	Items       []*shiftdomain.Shift
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of shifts.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := shiftdomain.ListFilter{
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
