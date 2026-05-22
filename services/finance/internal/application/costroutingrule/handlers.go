// Package costroutingrule holds rule admin use cases.
package costroutingrule

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroutingrule"
)

// CreateCommand input.
type CreateCommand struct {
	Priority     int32
	Condition    string
	ActionType   string
	ActionTarget string
	CreatedBy    string
}

// CreateHandler creates a new rule.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle creates the rule.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.Rule, error) {
	r, err := domain.New(domain.NewInput{
		Priority: cmd.Priority, Condition: cmd.Condition,
		ActionType: cmd.ActionType, ActionTarget: cmd.ActionTarget, CreatedBy: cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// GetQuery input.
type GetQuery struct{ RuleID int32 }

// GetHandler returns one.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler constructs.
func NewGetHandler(r domain.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle loads.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.Rule, error) {
	return h.repo.GetByID(ctx, q.RuleID)
}

// UpdateCommand input.
type UpdateCommand struct {
	RuleID       int32
	Priority     int32
	Condition    string
	ActionType   string
	ActionTarget string
	IsActive     bool
}

// UpdateHandler updates.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle updates.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.Rule, error) {
	r, err := h.repo.GetByID(ctx, cmd.RuleID)
	if err != nil {
		return nil, err
	}
	if err := r.Update(domain.UpdateInput{
		Priority: cmd.Priority, Condition: cmd.Condition,
		ActionType: cmd.ActionType, ActionTarget: cmd.ActionTarget, IsActive: cmd.IsActive,
	}); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// DeleteCommand input.
type DeleteCommand struct{ RuleID int32 }

// DeleteHandler removes.
type DeleteHandler struct{ repo domain.Repository }

// NewDeleteHandler constructs.
func NewDeleteHandler(r domain.Repository) *DeleteHandler { return &DeleteHandler{repo: r} }

// Handle deletes.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.RuleID)
}

// ListQuery input.
type ListQuery struct {
	ActiveFilter string
	Page         int
	PageSize     int
}

// ListResult bundles.
type ListResult struct {
	Items []*domain.Rule
	Total int64
}

// ListHandler lists.
type ListHandler struct{ repo domain.Repository }

// NewListHandler constructs.
func NewListHandler(r domain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle returns paginated rules.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, domain.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}
