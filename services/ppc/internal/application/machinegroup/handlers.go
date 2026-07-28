// Package machinegroup provides application-layer handlers for machine-group CRUD.
package machinegroup

import (
	"context"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/machinegroup"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all machine-group use cases over a single repository.
type Service struct {
	repo machinegroup.Repository
}

// NewService creates a new machine-group application service.
func NewService(repo machinegroup.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a machine group.
type CreateCommand struct {
	Name      string
	Area      string
	CreatedBy string
}

// Create validates and persists a new machine group.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*machinegroup.MachineGroup, error) {
	a, err := area.New(cmd.Area)
	if err != nil {
		return nil, machinegroup.ErrInvalidArea
	}
	entity, err := machinegroup.NewMachineGroup(cmd.Name, a, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a machine group by ID.
func (s *Service) Get(ctx context.Context, id int64) (*machinegroup.MachineGroup, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a machine group.
type UpdateCommand struct {
	ID        int64
	Name      *string
	Area      *string
	UpdatedBy string
}

// Update mutates an existing machine group.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*machinegroup.MachineGroup, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	var areaVO *area.Area
	if cmd.Area != nil {
		a, aErr := area.New(*cmd.Area)
		if aErr != nil {
			return nil, machinegroup.ErrInvalidArea
		}
		areaVO = &a
	}

	if err := entity.Update(cmd.Name, areaVO, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a machine group by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing machine groups.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	Area      string
	SortBy    string
	SortOrder string
}

// ListResult holds a page of machine groups plus pagination metadata.
type ListResult struct {
	Items       []*machinegroup.MachineGroup
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of machine groups.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := machinegroup.ListFilter{
		Search:    query.Search,
		Area:      query.Area,
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
