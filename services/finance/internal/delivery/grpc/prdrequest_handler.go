// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/application/prdrequest"
	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// PrdRequestGRPCHandler implements financev1.ProductRequestServiceServer.
type PrdRequestGRPCHandler struct {
	financev1.UnimplementedProductRequestServiceServer
	createHandler    *appprdrequest.CreateHandler
	getHandler       *appprdrequest.GetHandler
	listHandler      *appprdrequest.ListHandler
	updateHandler    *appprdrequest.UpdateHandler
	deleteHandler    *appprdrequest.DeleteHandler
	assignHandler    *appprdrequest.AssignHandler
	resolveHandler   *appprdrequest.ResolveHandler
	rejectHandler    *appprdrequest.RejectHandler
	searchHandler    *appprdrequest.SearchExistingHandler
	validationHelper *ValidationHelper
}

// NewPrdRequestGRPCHandler constructs a PrdRequestGRPCHandler.
func NewPrdRequestGRPCHandler(
	create *appprdrequest.CreateHandler,
	get *appprdrequest.GetHandler,
	list *appprdrequest.ListHandler,
	update *appprdrequest.UpdateHandler,
	deleteH *appprdrequest.DeleteHandler,
	assign *appprdrequest.AssignHandler,
	resolve *appprdrequest.ResolveHandler,
	reject *appprdrequest.RejectHandler,
	search *appprdrequest.SearchExistingHandler,
) (*PrdRequestGRPCHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &PrdRequestGRPCHandler{
		createHandler:    create,
		getHandler:       get,
		listHandler:      list,
		updateHandler:    update,
		deleteHandler:    deleteH,
		assignHandler:    assign,
		resolveHandler:   resolve,
		rejectHandler:    reject,
		searchHandler:    search,
		validationHelper: v,
	}, nil
}

// CreateRequest creates a new product request ticket.
// Requester identity is extracted from the JWT claims set by AuthInterceptor.
func (h *PrdRequestGRPCHandler) CreateRequest(ctx context.Context, req *financev1.CreateRequestRequest) (*financev1.CreateRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateRequestResponse{Base: baseResp}, nil
	}

	// Requester identity comes from auth context.
	requesterIDStr, ok := GetUserIDFromCtx(ctx)
	if !ok || requesterIDStr == "" {
		return &financev1.CreateRequestResponse{Base: UnauthorizedResponse("missing user context")}, nil
	}

	requesterID, err := uuid.Parse(requesterIDStr)
	if err != nil {
		return &financev1.CreateRequestResponse{Base: UnauthorizedResponse("invalid user context")}, nil
	}

	dueDate, err := parseDueDate(req.GetDueDate())
	if err != nil {
		return &financev1.CreateRequestResponse{Base: BadRequestResponse("invalid due_date: "+err.Error())}, nil
	}

	// RequesterDeptID: Phase 1 stub — dept enrichment not yet available.
	// Use the requester's own UUID as a placeholder so the domain constraint is satisfied.
	r, err := h.createHandler.Handle(ctx, appprdrequest.CreateCommand{
		RequesterID:       requesterID,
		RequesterUsername: getUserFromContext(ctx),
		RequesterDeptID:   requesterID,
		RequesterDeptCode: "",
		Title:             req.GetTitle(),
		Description:       req.GetDescription(),
		TargetSpecsJSON:   req.GetTargetSpecsJson(),
		DueDate:           dueDate,
		CreatedBy:         getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.CreateRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.CreateRequestResponse{
		Base: successResponse("product request created successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// GetRequest retrieves a product request by ID.
func (h *PrdRequestGRPCHandler) GetRequest(ctx context.Context, req *financev1.GetRequestRequest) (*financev1.GetRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.GetRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	r, err := h.getHandler.Handle(ctx, appprdrequest.GetCommand{ID: requestID})
	if err != nil {
		return &financev1.GetRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.GetRequestResponse{
		Base: successResponse("product request retrieved successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// ListRequests lists product requests with filtering and pagination.
func (h *PrdRequestGRPCHandler) ListRequests(ctx context.Context, req *financev1.ListRequestsRequest) (*financev1.ListRequestsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListRequestsResponse{Base: baseResp}, nil
	}

	page, pageSize := paginationFromProto(req.GetPagination())

	query := appprdrequest.ListQuery{
		Search:   req.GetSearch(),
		Status:   req.GetStatus(),
		Page:     page,
		PageSize: pageSize,
	}

	if requesterID := req.GetRequesterId(); requesterID != "" {
		id, err := uuid.Parse(requesterID)
		if err != nil {
			return &financev1.ListRequestsResponse{Base: BadRequestResponse("invalid requester_id: "+err.Error())}, nil
		}
		query.RequesterID = &id
	}

	if assignedTo := req.GetAssignedTo(); assignedTo != "" {
		id, err := uuid.Parse(assignedTo)
		if err != nil {
			return &financev1.ListRequestsResponse{Base: BadRequestResponse("invalid assigned_to: "+err.Error())}, nil
		}
		query.AssignedTo = &id
	}

	if deptID := req.GetRequesterDeptId(); deptID != "" {
		id, err := uuid.Parse(deptID)
		if err != nil {
			return &financev1.ListRequestsResponse{Base: BadRequestResponse("invalid requester_dept_id: "+err.Error())}, nil
		}
		query.RequesterDeptID = &id
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		return &financev1.ListRequestsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.ProductRequest, len(result.Requests))
	for i, r := range result.Requests {
		items[i] = prdRequestToProto(r)
	}

	return &financev1.ListRequestsResponse{
		Base: successResponse("product requests retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  int64(result.TotalItems),
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// UpdateRequest updates editable header fields on an existing product request.
func (h *PrdRequestGRPCHandler) UpdateRequest(ctx context.Context, req *financev1.UpdateRequestRequest) (*financev1.UpdateRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.UpdateRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	dueDate, err := parseDueDate(req.GetDueDate())
	if err != nil {
		return &financev1.UpdateRequestResponse{Base: BadRequestResponse("invalid due_date: "+err.Error())}, nil
	}

	r, err := h.updateHandler.Handle(ctx, appprdrequest.UpdateCommand{
		ID:              requestID,
		Title:           req.GetTitle(),
		Description:     req.GetDescription(),
		TargetSpecsJSON: req.GetTargetSpecsJson(),
		DueDate:         dueDate,
		UpdatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.UpdateRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.UpdateRequestResponse{
		Base: successResponse("product request updated successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// DeleteRequest soft-deletes a product request.
func (h *PrdRequestGRPCHandler) DeleteRequest(ctx context.Context, req *financev1.DeleteRequestRequest) (*financev1.DeleteRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.DeleteRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appprdrequest.DeleteCommand{
		ID:        requestID,
		DeletedBy: getUserFromContext(ctx),
	}); err != nil {
		return &financev1.DeleteRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.DeleteRequestResponse{
		Base: successResponse("product request deleted successfully"),
	}, nil
}

// AssignRequest assigns a user to a product request, transitioning OPEN → IN_REVIEW.
func (h *PrdRequestGRPCHandler) AssignRequest(ctx context.Context, req *financev1.AssignRequestRequest) (*financev1.AssignRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.AssignRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.AssignRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	assigneeID, err := uuid.Parse(req.GetAssigneeId())
	if err != nil {
		return &financev1.AssignRequestResponse{Base: BadRequestResponse("invalid assignee_id: "+err.Error())}, nil
	}

	r, err := h.assignHandler.Handle(ctx, appprdrequest.AssignCommand{
		ID:         requestID,
		AssigneeID: assigneeID,
		UpdatedBy:  getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.AssignRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.AssignRequestResponse{
		Base: successResponse("product request assigned successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// ResolveRequest closes a product request as RESOLVED by linking an existing product.
func (h *PrdRequestGRPCHandler) ResolveRequest(ctx context.Context, req *financev1.ResolveRequestRequest) (*financev1.ResolveRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ResolveRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.ResolveRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	productID, err := uuid.Parse(req.GetProductId())
	if err != nil {
		return &financev1.ResolveRequestResponse{Base: BadRequestResponse("invalid product_id: "+err.Error())}, nil
	}

	r, err := h.resolveHandler.Handle(ctx, appprdrequest.ResolveCommand{
		ID:             requestID,
		ProductID:      productID,
		ResolutionNote: req.GetResolutionNote(),
		UpdatedBy:      getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.ResolveRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.ResolveRequestResponse{
		Base: successResponse("product request resolved successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// RejectRequest closes a product request as REJECTED with a mandatory reason.
func (h *PrdRequestGRPCHandler) RejectRequest(ctx context.Context, req *financev1.RejectRequestRequest) (*financev1.RejectRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.RejectRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.RejectRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	r, err := h.rejectHandler.Handle(ctx, appprdrequest.RejectCommand{
		ID:        requestID,
		Reason:    req.GetReason(),
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.RejectRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.RejectRequestResponse{
		Base: successResponse("product request rejected successfully"),
		Data: prdRequestToProto(r),
	}, nil
}

// SearchExistingProducts performs a full-text product search to help requesters
// identify whether an existing product already satisfies their requirement.
func (h *PrdRequestGRPCHandler) SearchExistingProducts(ctx context.Context, req *financev1.SearchExistingProductsRequest) (*financev1.SearchExistingProductsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.SearchExistingProductsResponse{Base: baseResp}, nil
	}

	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}

	products, err := h.searchHandler.Handle(ctx, appprdrequest.SearchExistingCommand{
		Query:     req.GetQuery(),
		ShadeCode: req.GetShadeCode(),
		Limit:     limit,
	})
	if err != nil {
		return &financev1.SearchExistingProductsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.Product, len(products))
	for i, p := range products {
		items[i] = productToProto(p)
	}

	return &financev1.SearchExistingProductsResponse{
		Base: successResponse("products found"),
		Data: items,
	}, nil
}

// =============================================================================
// Mapping helpers
// =============================================================================

// prdRequestToProto converts a domain Request entity to its proto representation.
func prdRequestToProto(r *domainprdrequest.Request) *financev1.ProductRequest {
	proto := &financev1.ProductRequest{
		RequestId:         r.ID().String(),
		TicketNo:          r.TicketNo().String(),
		RequesterId:       r.RequesterID().String(),
		RequesterUsername: r.RequesterUsername(),
		RequesterDeptId:   r.RequesterDeptID().String(),
		Title:             r.Title().String(),
		Description:       r.Description().String(),
		TargetSpecsJson:   r.TargetSpecs().String(),
		Status:            r.Status().String(),
		ResolutionNote:    r.ResolutionNote().String(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: r.CreatedAt().Format(time.RFC3339),
			CreatedBy: r.CreatedBy(),
		},
	}

	if r.RequesterDeptID() == uuid.Nil {
		proto.RequesterDeptId = ""
	}

	if r.ResolvedProductID() != uuid.Nil {
		proto.ResolvedProductId = r.ResolvedProductID().String()
	}

	if r.AssignedTo() != uuid.Nil {
		proto.AssignedTo = r.AssignedTo().String()
	}

	if r.DueDate() != nil {
		proto.DueDate = r.DueDate().Format("2006-01-02")
	}

	if r.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = r.UpdatedAt().Format(time.RFC3339)
	}
	if r.UpdatedBy() != "" {
		proto.Audit.UpdatedBy = r.UpdatedBy()
	}

	return proto
}

// parseDueDate parses a YYYY-MM-DD date string into a *time.Time.
// Returns nil, nil for an empty string (due date is optional).
func parseDueDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
