// Package machine provides application-layer handlers for machine read/update.
package machine

import (
	"context"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	machinedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all machine use cases over a single repository.
type Service struct {
	repo machinedomain.Repository
}

// NewService creates a new machine application service.
func NewService(repo machinedomain.Repository) *Service {
	return &Service{repo: repo}
}

// Get retrieves a machine by ID.
func (s *Service) Get(ctx context.Context, id int64) (*machinedomain.Machine, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a machine's PPC-local fields.
type UpdateCommand struct {
	ID           int64
	Area         *string
	Line         *string
	GroupID      *int64
	DoffWeightKg *float64
	IsActive     *bool
	OrionCode    *string
	UpdatedBy    string
}

// Update mutates PPC-local fields on an existing machine.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*machinedomain.Machine, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	var areaVO *area.Area
	if cmd.Area != nil {
		a, aErr := area.New(*cmd.Area)
		if aErr != nil {
			return nil, machinedomain.ErrInvalidArea
		}
		areaVO = &a
	}

	if err := entity.Update(areaVO, cmd.Line, cmd.GroupID, cmd.DoffWeightKg, cmd.IsActive, cmd.OrionCode, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// ListQuery carries inputs for listing machines.
type ListQuery struct {
	Page           int
	PageSize       int
	Search         string
	Area           string
	MachineGroupID *int64
	IsActive       *bool
	SortBy         string
	SortOrder      string
}

// ListResult holds a page of machines plus pagination metadata.
type ListResult struct {
	Items       []*machinedomain.Machine
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of machines.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := machinedomain.ListFilter{
		Search:         query.Search,
		Area:           query.Area,
		MachineGroupID: query.MachineGroupID,
		IsActive:       query.IsActive,
		Page:           query.Page,
		PageSize:       query.PageSize,
		SortBy:         query.SortBy,
		SortOrder:      query.SortOrder,
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
