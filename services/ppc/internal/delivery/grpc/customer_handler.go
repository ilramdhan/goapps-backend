// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	customerapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/customer"
	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// customerUnavailable is reported when the customer service was never wired.
const customerUnavailable = "customer service unavailable"

// customerHandler implements the customer RPCs of PPCService.
type customerHandler struct {
	svc *customerapp.Service
}

func newCustomerHandler(svc *customerapp.Service) *customerHandler {
	return &customerHandler{svc: svc}
}

// CreateCustomer hand-adds a customer. The row is marked MANUAL so the Oracle
// sync will not overwrite it.
func (h *customerHandler) CreateCustomer(
	ctx context.Context,
	req *ppcv1.CreateCustomerRequest,
) (*ppcv1.CreateCustomerResponse, error) {
	if h.svc == nil {
		return &ppcv1.CreateCustomerResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	entity, err := h.svc.Create(ctx, customerapp.CreateCommand{
		Code:       req.GetCustomerCode(),
		Name:       req.GetCustomerName(),
		ShortName:  optionalStringField(req.GetCustomerShortName()),
		TaxNo:      optionalStringField(req.GetCustomerTaxNo()),
		ParentCode: optionalStringField(req.GetCustomerParentCode()),
		CreatedBy:  getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateCustomerResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateCustomerResponse{
		Base: successResponse("Customer created successfully"),
		Data: customerToProto(entity),
	}, nil
}

// GetCustomer retrieves a customer by ID.
func (h *customerHandler) GetCustomer(
	ctx context.Context,
	req *ppcv1.GetCustomerRequest,
) (*ppcv1.GetCustomerResponse, error) {
	if h.svc == nil {
		return &ppcv1.GetCustomerResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	entity, err := h.svc.Get(ctx, req.GetCustomerId())
	if err != nil {
		return &ppcv1.GetCustomerResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetCustomerResponse{
		Base: successResponse("Customer retrieved successfully"),
		Data: customerToProto(entity),
	}, nil
}

// UpdateCustomer edits a customer. The code is immutable and is not accepted here.
func (h *customerHandler) UpdateCustomer(
	ctx context.Context,
	req *ppcv1.UpdateCustomerRequest,
) (*ppcv1.UpdateCustomerResponse, error) {
	if h.svc == nil {
		return &ppcv1.UpdateCustomerResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	entity, err := h.svc.Update(ctx, customerapp.UpdateCommand{
		ID:         req.GetCustomerId(),
		Name:       req.CustomerName,
		ShortName:  req.CustomerShortName,
		TaxNo:      req.CustomerTaxNo,
		ParentCode: req.CustomerParentCode,
		IsActive:   req.CustomerIsActive,
		UpdatedBy:  getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdateCustomerResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateCustomerResponse{
		Base: successResponse("Customer updated successfully"),
		Data: customerToProto(entity),
	}, nil
}

// ListCustomers lists customers with filtering and pagination.
func (h *customerHandler) ListCustomers(
	ctx context.Context,
	req *ppcv1.ListCustomersRequest,
) (*ppcv1.ListCustomersResponse, error) {
	if h.svc == nil {
		return &ppcv1.ListCustomersResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	result, err := h.svc.List(ctx, customerapp.ListQuery{
		Search:    req.GetSearch(),
		IsActive:  activeFilterToBool(req.GetActiveFilter()),
		Source:    req.GetCustomerSource(),
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListCustomersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.Customer, len(result.Items))
	for i, entity := range result.Items {
		items[i] = customerToProto(entity)
	}
	return &ppcv1.ListCustomersResponse{
		Base: successResponse("Customers retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// SyncCustomers triggers an on-demand read-only sync from Oracle OM_CUSTOMER.
func (h *customerHandler) SyncCustomers(
	ctx context.Context,
	_ *ppcv1.SyncCustomersRequest,
) (*ppcv1.SyncCustomersResponse, error) {
	if h.svc == nil {
		return &ppcv1.SyncCustomersResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	res, err := h.svc.SyncFromOracle(ctx)
	if err != nil {
		return &ppcv1.SyncCustomersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SyncCustomersResponse{
		Base:           successResponse("Customer sync completed"),
		InsertedCount:  safeconv.IntToInt32(res.Inserted),
		UpdatedCount:   safeconv.IntToInt32(res.Updated),
		UnchangedCount: safeconv.IntToInt32(res.Unchanged),
		SourceUsed:     res.SourceUsed,
	}, nil
}

// ExportCustomers exports the customer master to an .xlsx workbook.
func (h *customerHandler) ExportCustomers(
	ctx context.Context,
	req *ppcv1.ExportCustomersRequest,
) (*ppcv1.ExportCustomersResponse, error) {
	if h.svc == nil {
		return &ppcv1.ExportCustomersResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	result, err := h.svc.Export(ctx, customerapp.ListQuery{
		Search:   req.GetSearch(),
		IsActive: activeFilterToBool(req.GetActiveFilter()),
		Source:   req.GetCustomerSource(),
	})
	if err != nil {
		return &ppcv1.ExportCustomersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ExportCustomersResponse{
		Base:        successResponse("Customers exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportCustomers bulk-creates or updates customers from an uploaded workbook.
func (h *customerHandler) ImportCustomers(
	ctx context.Context,
	req *ppcv1.ImportCustomersRequest,
) (*ppcv1.ImportCustomersResponse, error) {
	if h.svc == nil {
		return &ppcv1.ImportCustomersResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	result, err := h.svc.Import(ctx, customerapp.ImportCommand{
		FileContent:     req.GetFileContent(),
		FileName:        req.GetFileName(),
		DuplicateAction: req.GetDuplicateAction(),
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.ImportCustomersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	errs := make([]*ppcv1.CustomerImportError, len(result.Errors))
	for i, e := range result.Errors {
		errs[i] = &ppcv1.CustomerImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}
	return &ppcv1.ImportCustomersResponse{
		Base:         successResponse("Customer import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       errs,
	}, nil
}

// DownloadCustomerTemplate returns the customer import template workbook.
func (h *customerHandler) DownloadCustomerTemplate(
	_ context.Context,
	_ *ppcv1.DownloadCustomerTemplateRequest,
) (*ppcv1.DownloadCustomerTemplateResponse, error) {
	if h.svc == nil {
		return &ppcv1.DownloadCustomerTemplateResponse{Base: errorResponse(nilServiceCode, customerUnavailable)}, nil
	}
	result, err := h.svc.Template()
	if err != nil {
		return &ppcv1.DownloadCustomerTemplateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DownloadCustomerTemplateResponse{
		Base:        successResponse("Customer template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// customerToProto maps a customer entity to its proto message.
func customerToProto(e *customerdomain.Customer) *ppcv1.Customer {
	proto := &ppcv1.Customer{
		CustomerId:       e.ID(),
		CustomerCode:     e.Code(),
		CustomerName:     e.Name(),
		CustomerIsActive: e.IsActive(),
		CustomerSource:   e.Source(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.ShortName() != nil {
		proto.CustomerShortName = *e.ShortName()
	}
	if e.TaxNo() != nil {
		proto.CustomerTaxNo = *e.TaxNo()
	}
	if e.ParentCode() != nil {
		proto.CustomerParentCode = *e.ParentCode()
	}
	if e.SourceCreatedAt() != nil {
		proto.SourceCreatedAt = e.SourceCreatedAt().Format(time.RFC3339)
	}
	if e.SourceUpdatedAt() != nil {
		proto.SourceUpdatedAt = e.SourceUpdatedAt().Format(time.RFC3339)
	}
	if e.SyncedAt() != nil {
		proto.SyncedAt = e.SyncedAt().Format(time.RFC3339)
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
