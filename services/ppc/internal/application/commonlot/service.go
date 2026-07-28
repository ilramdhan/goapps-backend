// Package commonlot provides application usecases for common lots: create (fold
// leftover bobbins from several original lots into a new ERP identity), get, and
// list.
package commonlot

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/commonlot"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service orchestrates common-lot usecases over the domain repository.
type Service struct {
	repo domain.Repository
}

// NewService builds the common-lot service.
func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// ComponentInput is one original-lot line supplied when creating a common lot.
type ComponentInput struct {
	OriginalLotNo     string
	OriginalShadeCode string
	BobbinCount       int32
	QtyKg             float64
}

// CreateCommand carries the inputs to create a common lot.
type CreateCommand struct {
	LotNo        string
	ItemCode     string
	ShadeCode    string
	ErpGradeCode string
	Components   []ComponentInput
}

// Create builds and persists a common lot from its component lines.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.CommonLot, error) {
	comps := make([]domain.Component, 0, len(cmd.Components))
	for _, in := range cmd.Components {
		comp, err := domain.NewComponent(in.OriginalLotNo, in.OriginalShadeCode, in.BobbinCount, in.QtyKg)
		if err != nil {
			return nil, err
		}
		comps = append(comps, comp)
	}
	lot, err := domain.NewCommonLot(cmd.LotNo, cmd.ItemCode, cmd.ShadeCode, cmd.ErpGradeCode, comps)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, lot); err != nil {
		return nil, err
	}
	return lot, nil
}

// Get returns a common lot with its components.
func (s *Service) Get(ctx context.Context, id int64) (*domain.CommonLot, error) {
	return s.repo.GetByID(ctx, id)
}

// ListResult is a paginated common-lot list.
type ListResult struct {
	Items       []*domain.CommonLot
	CurrentPage int32
	PageSize    int32
	TotalItems  int64
	TotalPages  int32
}

// List returns common lots matching the filter, with pagination metadata.
func (s *Service) List(ctx context.Context, filter domain.Filter) (ListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return ListResult{}, err
	}
	totalPages := int32(0)
	if filter.PageSize > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return ListResult{
		Items:       items,
		CurrentPage: filter.Page,
		PageSize:    filter.PageSize,
		TotalItems:  total,
		TotalPages:  totalPages,
	}, nil
}
