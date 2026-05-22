// Package costrmtype contains application use cases for CostRmType.
package costrmtype

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrmtype"
)

// CreateCommand input.
type CreateCommand struct {
	TypeCode         string
	TypeName         string
	ReferenceTarget  string
	AllowSubSequence bool
}

// CreateHandler creates a new CostRmType.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.CostRmType, error) {
	t, err := domain.New(cmd.TypeCode, cmd.TypeName, cmd.ReferenceTarget, cmd.AllowSubSequence)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetQuery input.
type GetQuery struct{ TypeID int32 }

// GetHandler loads by id.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r domain.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle executes the lookup.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.CostRmType, error) {
	return h.repo.GetByID(ctx, q.TypeID)
}

// UpdateCommand input.
type UpdateCommand struct {
	TypeID   int32
	TypeName string
	IsActive bool
}

// UpdateHandler updates name + active flag.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.CostRmType, error) {
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
	Search          string
	ReferenceTarget string
	ActiveFilter    string
	Page            int
	PageSize        int
}

// ListResult is the list query result.
type ListResult struct {
	Items []*domain.CostRmType
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
