package grpc

import (
	"context"
	"fmt"

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

// UpdateLookupMaster updates mutable fields of an existing lookup master.
func (h *LookupMasterHandler) UpdateLookupMaster(ctx context.Context, req *financev1.UpdateLookupMasterRequest) (*financev1.UpdateLookupMasterResponse, error) { //nolint:nilerr // BaseResponse pattern
	u := lookupmaster.UpdateMaster{}
	if req.LmDisplayName != nil {
		u.DisplayName = req.LmDisplayName
	}
	if req.LmTableName != nil {
		u.TableName = req.LmTableName
	}
	if req.LmIsActive != nil {
		u.IsActive = req.LmIsActive
	}
	if err := h.repo.UpdateMaster(ctx, req.GetLmCode(), u); err != nil {
		return &financev1.UpdateLookupMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.UpdateLookupMasterResponse{Base: successResponse("Lookup master updated")}, nil
}

// ListTableColumns introspects a registered PostgreSQL table's columns via information_schema.
func (h *LookupMasterHandler) ListTableColumns(ctx context.Context, req *financev1.ListTableColumnsRequest) (*financev1.ListTableColumnsResponse, error) { //nolint:nilerr // BaseResponse pattern
	cols, err := h.repo.ListTableColumns(ctx, req.GetTableName())
	if err != nil {
		return &financev1.ListTableColumnsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.TableColumn, 0, len(cols))
	for _, c := range cols {
		items = append(items, &financev1.TableColumn{
			ColumnName:      c.ColumnName,
			DataType:        c.DataType,
			RawType:         c.RawType,
			OrdinalPosition: int32(c.OrdinalPosition), //nolint:gosec // ordinal_position is small (bounded by information_schema)
		})
	}
	return &financev1.ListTableColumnsResponse{Base: successResponse(""), Data: items}, nil
}

// ListMasterOptions returns combobox options (value+label) by querying the registered table.
func (h *LookupMasterHandler) ListMasterOptions(ctx context.Context, req *financev1.ListMasterOptionsRequest) (*financev1.ListMasterOptionsResponse, error) { //nolint:nilerr // BaseResponse pattern
	opts, err := h.repo.ListMasterOptions(ctx, req.GetMasterCode())
	if err != nil {
		return &financev1.ListMasterOptionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.MasterOption, 0, len(opts))
	for _, o := range opts {
		items = append(items, &financev1.MasterOption{Value: o.Value, Label: o.Label})
	}
	return &financev1.ListMasterOptionsResponse{Base: successResponse(""), Data: items}, nil
}

// ExportLookupMasters exports all masters and columns to an Excel workbook.
func (h *LookupMasterHandler) ExportLookupMasters(ctx context.Context, _ *financev1.ExportLookupMastersRequest) (*financev1.ExportLookupMastersResponse, error) { //nolint:nilerr // BaseResponse pattern
	data, filename, err := h.repo.ExportMasters(ctx)
	if err != nil {
		return &financev1.ExportLookupMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.ExportLookupMastersResponse{
		Base:        successResponse("Export ready"),
		FileContent: data,
		FileName:    filename,
	}, nil
}

// ImportLookupMasters imports masters and columns from an Excel workbook.
func (h *LookupMasterHandler) ImportLookupMasters(ctx context.Context, req *financev1.ImportLookupMastersRequest) (*financev1.ImportLookupMastersResponse, error) { //nolint:nilerr // BaseResponse pattern
	success, skipped, failed, errs, err := h.repo.ImportMasters(ctx, req.GetFileContent())
	if err != nil {
		return &financev1.ImportLookupMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.ImportLookupMastersResponse{
		Base:         successResponse(fmt.Sprintf("Imported %d masters", success)),
		SuccessCount: int32(success), //nolint:gosec // small count bounded by Excel row limit
		SkippedCount: int32(skipped), //nolint:gosec // small count bounded by Excel row limit
		FailedCount:  int32(failed),  //nolint:gosec // small count bounded by Excel row limit
		Errors:       errs,
	}, nil
}

var _ financev1.LookupMasterServiceServer = (*LookupMasterHandler)(nil)
