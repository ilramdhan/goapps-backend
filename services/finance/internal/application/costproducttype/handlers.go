// Package costproducttype contains application use cases for CostProductType.
package costproducttype

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// CreateCommand input.
type CreateCommand struct {
	TypeCode string
	TypeName string
}

// CreateHandler creates a new CostProductType.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.CostProductType, error) {
	t, err := domain.New(cmd.TypeCode, cmd.TypeName)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetQuery loads by id.
type GetQuery struct{ TypeID int32 }

// GetHandler loads by id.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r domain.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle executes the lookup.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.CostProductType, error) {
	return h.repo.GetByID(ctx, q.TypeID)
}

// UpdateCommand input.
type UpdateCommand struct {
	TypeID   int32
	TypeName string
	IsActive bool
}

// UpdateHandler updates a CostProductType.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.CostProductType, error) {
	t, err := h.repo.GetByID(ctx, cmd.TypeID)
	if err != nil {
		return nil, err
	}
	if err := t.Update(cmd.TypeName, cmd.IsActive); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// ListQuery input.
type ListQuery struct {
	Search       string
	ActiveFilter string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// ListResult is the list query result.
type ListResult struct {
	Items []*domain.CostProductType
	Total int64
}

// ListHandler returns a paginated list.
type ListHandler struct{ repo domain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r domain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle executes the list.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, domain.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}
