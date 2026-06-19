package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// LookupMasterHandler implements financev1.LookupMasterServiceServer.
type LookupMasterHandler struct {
	financev1.UnimplementedLookupMasterServiceServer
	repo lookupmaster.Repository
}

// NewLookupMasterHandler creates a new LookupMasterHandler.
func NewLookupMasterHandler(repo lookupmaster.Repository) (*LookupMasterHandler, error) {
	return &LookupMasterHandler{repo: repo}, nil
}

// ListLookupMasters returns all registered master lookup codes.
func (h *LookupMasterHandler) ListLookupMasters(ctx context.Context, req *financev1.ListLookupMastersRequest) (*financev1.ListLookupMastersResponse, error) { //nolint:nilerr // BaseResponse pattern
	masters, err := h.repo.ListMasters(ctx, req.GetActiveOnly())
	if err != nil {
		return &financev1.ListLookupMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.LookupMaster, 0, len(masters))
	for _, m := range masters {
		items = append(items, &financev1.LookupMaster{
			LmCode:        m.Code,
			LmDisplayName: m.DisplayName,
			LmApiPath:     m.APIPath,
			LmCodeField:   m.CodeField,
			LmLabelField:  m.LabelField,
			LmIsActive:    m.IsActive,
		})
	}
	return &financev1.ListLookupMastersResponse{Base: successResponse(""), Data: items}, nil
}

// ListLookupMasterColumns returns fillable columns for a given master code.
func (h *LookupMasterHandler) ListLookupMasterColumns(ctx context.Context, req *financev1.ListLookupMasterColumnsRequest) (*financev1.ListLookupMasterColumnsResponse, error) { //nolint:nilerr // BaseResponse pattern
	cols, err := h.repo.ListColumns(ctx, req.GetMasterCode())
	if err != nil {
		return &financev1.ListLookupMasterColumnsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.LookupMasterColumn, 0, len(cols))
	for _, c := range cols {
		items = append(items, &financev1.LookupMasterColumn{
			LmcId:          c.ID,
			LmcMasterCode:  c.MasterCode,
			LmcColumnName:  c.ColumnName,
			LmcDisplayName: c.DisplayName,
			LmcDataType:    c.DataType,
			LmcSortOrder:   int32(c.SortOrder), //nolint:gosec // sort_order is bounded (seeded data, max ~100)
		})
	}
	return &financev1.ListLookupMasterColumnsResponse{Base: successResponse(""), Data: items}, nil
}

// CreateLookupMaster adds a new master to the registry.
func (h *LookupMasterHandler) CreateLookupMaster(ctx context.Context, req *financev1.CreateLookupMasterRequest) (*financev1.CreateLookupMasterResponse, error) { //nolint:nilerr // BaseResponse pattern
	actor := getUserFromContext(ctx)
	m := &lookupmaster.LookupMaster{
		Code:        req.GetLmCode(),
		DisplayName: req.GetLmDisplayName(),
		APIPath:     req.GetLmApiPath(),
		CodeField:   req.GetLmCodeField(),
		LabelField:  req.GetLmLabelField(),
		IsActive:    true,
	}
	if err := h.repo.CreateMaster(ctx, m, actor); err != nil {
		return &financev1.CreateLookupMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.CreateLookupMasterResponse{
		Base: successResponse("Lookup master created"),
		Data: &financev1.LookupMaster{
			LmCode:        m.Code,
			LmDisplayName: m.DisplayName,
			LmApiPath:     m.APIPath,
			LmCodeField:   m.CodeField,
			LmLabelField:  m.LabelField,
			LmIsActive:    true,
		},
	}, nil
}

// DeleteLookupMaster removes a master from the registry.
func (h *LookupMasterHandler) DeleteLookupMaster(ctx context.Context, req *financev1.DeleteLookupMasterRequest) (*financev1.DeleteLookupMasterResponse, error) { //nolint:nilerr // BaseResponse pattern
	if err := h.repo.DeleteMaster(ctx, req.GetLmCode()); err != nil {
		return &financev1.DeleteLookupMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.DeleteLookupMasterResponse{Base: successResponse("Lookup master deleted")}, nil
}

// CreateLookupMasterColumn adds a fillable column to a master.
func (h *LookupMasterHandler) CreateLookupMasterColumn(ctx context.Context, req *financev1.CreateLookupMasterColumnRequest) (*financev1.CreateLookupMasterColumnResponse, error) { //nolint:nilerr // BaseResponse pattern
	c := &lookupmaster.Column{
		MasterCode:  req.GetLmcMasterCode(),
		ColumnName:  req.GetLmcColumnName(),
		DisplayName: req.GetLmcDisplayName(),
		DataType:    req.GetLmcDataType(),
		SortOrder:   int(req.GetLmcSortOrder()), //nolint:gosec // sort_order is bounded input
	}
	id, err := h.repo.CreateColumn(ctx, c, getUserFromContext(ctx))
	if err != nil {
		return &financev1.CreateLookupMasterColumnResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.CreateLookupMasterColumnResponse{
		Base: successResponse("Column created"),
		Data: &financev1.LookupMasterColumn{
			LmcId:          id,
			LmcMasterCode:  c.MasterCode,
			LmcColumnName:  c.ColumnName,
			LmcDisplayName: c.DisplayName,
			LmcDataType:    c.DataType,
			LmcSortOrder:   req.GetLmcSortOrder(),
		},
	}, nil
}

// DeleteLookupMasterColumn removes a column from a master.
func (h *LookupMasterHandler) DeleteLookupMasterColumn(ctx context.Context, req *financev1.DeleteLookupMasterColumnRequest) (*financev1.DeleteLookupMasterColumnResponse, error) { //nolint:nilerr // BaseResponse pattern
	if err := h.repo.DeleteColumn(ctx, req.GetLmcId()); err != nil {
		return &financev1.DeleteLookupMasterColumnResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.DeleteLookupMasterColumnResponse{Base: successResponse("Column deleted")}, nil
}

var _ financev1.LookupMasterServiceServer = (*LookupMasterHandler)(nil)
