package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// FactorCreateCommand represents the create MB cross-section factor command.
type FactorCreateCommand struct {
	FromCode  string
	ToCode    string
	Factor    float64
	Operation string
	Note      string
	IsActive  bool
	CreatedBy string
}

// FactorCreateHandler handles the CreateMbCrossSectionFactor command.
type FactorCreateHandler struct {
	repo mbcrosssection.FactorRepository
}

// NewFactorCreateHandler creates a new FactorCreateHandler.
func NewFactorCreateHandler(repo mbcrosssection.FactorRepository) *FactorCreateHandler {
	return &FactorCreateHandler{repo: repo}
}

// Handle executes the create MB cross-section factor command.
func (h *FactorCreateHandler) Handle(ctx context.Context, cmd FactorCreateCommand) (*mbcrosssection.FactorEntity, error) {
	entity, err := mbcrosssection.NewFactorEntity(
		cmd.FromCode, cmd.ToCode, cmd.Factor, cmd.Operation, cmd.Note, cmd.IsActive, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}

// FactorGetQuery represents the get MB cross-section factor query.
type FactorGetQuery struct {
	ID string
}

// FactorGetHandler handles the GetMbCrossSectionFactor query.
type FactorGetHandler struct {
	repo mbcrosssection.FactorRepository
}

// NewFactorGetHandler creates a new FactorGetHandler.
func NewFactorGetHandler(repo mbcrosssection.FactorRepository) *FactorGetHandler {
	return &FactorGetHandler{repo: repo}
}

// Handle executes the get MB cross-section factor query.
func (h *FactorGetHandler) Handle(ctx context.Context, query FactorGetQuery) (*mbcrosssection.FactorEntity, error) {
	return h.repo.GetByID(ctx, query.ID)
}

// FactorListQuery represents the list MB cross-section factor query.
type FactorListQuery struct {
	Page      int32
	PageSize  int32
	Search    string
	FromCode  string
	ToCode    string
	SortBy    string
	SortOrder string
	IsActive  *bool
}

// FactorListResult represents the list MB cross-section factor result.
type FactorListResult struct {
	Items       []*mbcrosssection.FactorEntity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// FactorListHandler handles the ListMbCrossSectionFactor query.
type FactorListHandler struct {
	repo mbcrosssection.FactorRepository
}

// NewFactorListHandler creates a new FactorListHandler.
func NewFactorListHandler(repo mbcrosssection.FactorRepository) *FactorListHandler {
	return &FactorListHandler{repo: repo}
}

// Handle executes the list MB cross-section factor query.
func (h *FactorListHandler) Handle(ctx context.Context, query FactorListQuery) (*FactorListResult, error) {
	filter := mbcrosssection.FactorListFilter{
		Search:    query.Search,
		FromCode:  query.FromCode,
		ToCode:    query.ToCode,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}
	filter.Validate()

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &FactorListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages(total, filter.PageSize),
		CurrentPage: filter.Page,
		PageSize:    filter.PageSize,
	}, nil
}

// FactorUpdateCommand represents the update MB cross-section factor command.
// The (from_code, to_code) pair is immutable and therefore absent here.
type FactorUpdateCommand struct {
	ID        string
	Factor    float64
	Operation string
	Note      string
	IsActive  bool
	UpdatedBy string
}

// FactorUpdateHandler handles the UpdateMbCrossSectionFactor command.
type FactorUpdateHandler struct {
	repo mbcrosssection.FactorRepository
}

// NewFactorUpdateHandler creates a new FactorUpdateHandler.
func NewFactorUpdateHandler(repo mbcrosssection.FactorRepository) *FactorUpdateHandler {
	return &FactorUpdateHandler{repo: repo}
}

// Handle executes the update MB cross-section factor command.
func (h *FactorUpdateHandler) Handle(ctx context.Context, cmd FactorUpdateCommand) (*mbcrosssection.FactorEntity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Factor, cmd.Operation, cmd.Note, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}

// FactorDeleteCommand represents the delete MB cross-section factor command.
type FactorDeleteCommand struct {
	ID        string
	DeletedBy string
}

// FactorDeleteHandler handles the DeleteMbCrossSectionFactor command.
type FactorDeleteHandler struct {
	repo mbcrosssection.FactorRepository
}

// NewFactorDeleteHandler creates a new FactorDeleteHandler.
func NewFactorDeleteHandler(repo mbcrosssection.FactorRepository) *FactorDeleteHandler {
	return &FactorDeleteHandler{repo: repo}
}

// Handle executes the delete MB cross-section factor command (soft delete).
func (h *FactorDeleteHandler) Handle(ctx context.Context, cmd FactorDeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.DeletedBy)
}
