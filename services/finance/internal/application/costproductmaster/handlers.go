// Package costproductmaster contains application use cases for CostProductMaster.
package costproductmaster

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// CreateCommand input.
type CreateCommand struct {
	ProductTypeID int32
	ProductName   string
	ShadeCode     string
	GradeCode     string
	Description   string
	ActorUserID   string
}

// CreateHandler creates a new product master.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.CostProductMaster, error) {
	p, err := domain.New(domain.NewInput{
		ProductTypeID: cmd.ProductTypeID,
		ProductName:   cmd.ProductName,
		ShadeCode:     cmd.ShadeCode,
		GradeCode:     cmd.GradeCode,
		Description:   cmd.Description,
		ActorUserID:   cmd.ActorUserID,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetQuery input.
type GetQuery struct {
	ProductSysID int64
	ProductCode  string
}

// GetHandler loads a product master.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r domain.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle resolves by sys_id (preferred) or by code.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.CostProductMaster, error) {
	if q.ProductSysID > 0 {
		return h.repo.GetBySysID(ctx, q.ProductSysID)
	}
	return h.repo.GetByCode(ctx, q.ProductCode)
}

// UpdateCommand input.
type UpdateCommand struct {
	ProductSysID int64
	ProductName  string
	ShadeCode    string
	GradeCode    string
	Description  string
	ActorUserID  string
}

// UpdateHandler updates editable fields.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	if err := p.Update(cmd.ProductName, cmd.ShadeCode, cmd.GradeCode, cmd.Description, cmd.ActorUserID); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// LinkErpCommand input.
type LinkErpCommand struct {
	ProductSysID   int64
	ErpItemCode    string
	ErpGradeCode1  string
	ErpGradeCode2  string
	ActorUserID    string
}

// LinkErpHandler sets/refreshes ERP linkage.
type LinkErpHandler struct{ repo domain.Repository }

// NewLinkErpHandler constructs a LinkErpHandler.
func NewLinkErpHandler(r domain.Repository) *LinkErpHandler { return &LinkErpHandler{repo: r} }

// Handle executes the linkage update.
func (h *LinkErpHandler) Handle(ctx context.Context, cmd LinkErpCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	p.LinkErp(cmd.ErpItemCode, cmd.ErpGradeCode1, cmd.ErpGradeCode2, cmd.ActorUserID)
	if err := h.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// DeactivateCommand input.
type DeactivateCommand struct {
	ProductSysID int64
	ActorUserID  string
}

// DeactivateHandler flips active=false.
type DeactivateHandler struct{ repo domain.Repository }

// NewDeactivateHandler constructs a DeactivateHandler.
func NewDeactivateHandler(r domain.Repository) *DeactivateHandler { return &DeactivateHandler{repo: r} }

// Handle executes deactivation.
func (h *DeactivateHandler) Handle(ctx context.Context, cmd DeactivateCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	p.Deactivate(cmd.ActorUserID)
	if err := h.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ListQuery input.
type ListQuery struct {
	Search        string
	ProductTypeID int32
	ShadeCode     string
	ActiveFilter  string
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string
}

// ListResult is the list query result.
type ListResult struct {
	Items []*domain.CostProductMaster
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
