package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	routeDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/requesthistory"
)

// CostProductRequestHandler implements financev1.CostProductRequestServiceServer.
type CostProductRequestHandler struct {
	financev1.UnimplementedCostProductRequestServiceServer
	createHandler      *app.CreateHandler
	getHandler         *app.GetHandler
	updateHandler      *app.UpdateHandler
	listHandler        *app.ListHandler
	transitionHandler  *app.TransitionHandler
	linkRouteHandler   *app.LinkRouteHandler
	unlinkRouteHandler *app.UnlinkRouteHandler
	validation         *ValidationHelper
	historyRepo        requesthistory.Repository   // optional; nil disables GetCostProductRequestHistory
	paramSummary       *app.GetParamSummaryHandler // optional; nil returns empty response
}

// NewCostProductRequestHandler constructs the handler. Pass auditEmitter=nil to
// disable audit log emission on state transitions. The routeRepo is used to
// validate route head existence on LinkExistingRoute.
func NewCostProductRequestHandler(repo domain.Repository, routeRepo routeDomain.Repository, auditEmitter app.AuditEmitter) (*CostProductRequestHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	transition := app.NewTransitionHandler(repo).WithRouteRepo(routeRepo)
	if auditEmitter != nil {
		transition = transition.WithAudit(auditEmitter)
	}
	return &CostProductRequestHandler{
		createHandler:      app.NewCreateHandler(repo),
		getHandler:         app.NewGetHandler(repo),
		updateHandler:      app.NewUpdateHandler(repo),
		listHandler:        app.NewListHandler(repo),
		transitionHandler:  transition,
		linkRouteHandler:   app.NewLinkRouteHandler(repo, routeRepo),
		unlinkRouteHandler: app.NewUnlinkRouteHandler(repo),
		validation:         v,
	}, nil
}

// WithFillCreator attaches a fill-task creator to the transition handler so that
// fill tasks are created when MarkParameterPending is called.
func (h *CostProductRequestHandler) WithFillCreator(c app.FillTaskCreator) *CostProductRequestHandler {
	h.transitionHandler = h.transitionHandler.WithFillCreator(c)
	return h
}

// WithFillChecker attaches a fill-completion checker so that MarkParameterComplete
// is blocked until all regular fill levels are approved.
func (h *CostProductRequestHandler) WithFillChecker(c app.FillCompletionChecker) *CostProductRequestHandler {
	h.transitionHandler = h.transitionHandler.WithFillChecker(c)
	return h
}

// WithParamCounter attaches an applicable-param counter so that cft_total_params is
// populated correctly when fill tasks are created during MarkParameterPending.
func (h *CostProductRequestHandler) WithParamCounter(c app.ApplicableParamCounter) *CostProductRequestHandler {
	h.transitionHandler = h.transitionHandler.WithParamCounter(c)
	return h
}

// WithNotifier attaches an in-app notification emitter to the transition handler.
func (h *CostProductRequestHandler) WithNotifier(n app.NotificationEmitter) *CostProductRequestHandler {
	h.transitionHandler = h.transitionHandler.WithNotifier(n)
	return h
}

// WithCPRNotifier attaches an IAM-backed CPRNotifier to both the create and
// transition handlers for rule-based multi-recipient fan-out notifications.
func (h *CostProductRequestHandler) WithCPRNotifier(n app.CPRNotifier) *CostProductRequestHandler {
	h.createHandler = h.createHandler.WithCPRNotifier(n)
	h.transitionHandler = h.transitionHandler.WithCPRNotifier(n)
	return h
}

// WithHistoryRepo attaches the approval trace repository and wires it into the
// transition handler so every state change is recorded, and enables the
// GetCostProductRequestHistory RPC.
func (h *CostProductRequestHandler) WithHistoryRepo(r requesthistory.Repository) *CostProductRequestHandler {
	h.historyRepo = r
	h.transitionHandler = h.transitionHandler.WithHistoryRepo(r)
	return h
}

// WithParamSummary attaches the param summary handler, enabling the GetParamSummary RPC.
func (h *CostProductRequestHandler) WithParamSummary(ps *app.GetParamSummaryHandler) *CostProductRequestHandler {
	h.paramSummary = ps
	return h
}

// WithRouteLockChecker attaches a route-lock checker so that Confirm is blocked
// until the linked route is locked.
func (h *CostProductRequestHandler) WithRouteLockChecker(c app.RouteLockChecker) *CostProductRequestHandler {
	h.transitionHandler = h.transitionHandler.WithRouteLockChecker(c)
	return h
}

// MarkParameterCompleteForGate advances the CPR aggregate to PARAMETER_COMPLETE and
// returns the requester user ID and request number needed for downstream notifications.
// It is called by the completion gate after L102 is approved — the caller ("system")
// is recorded as the actor since the transition is automated.
func (h *CostProductRequestHandler) MarkParameterCompleteForGate(ctx context.Context, requestID int64, actor string) (requesterUserID, requestNo string, err error) {
	req, tErr := h.transitionHandler.MarkParameterComplete(ctx, requestID, actor, actor)
	if tErr != nil {
		return "", "", tErr
	}
	return req.RequesterUserID(), req.RequestNo(), nil
}

// =============================================================================
// CRUD
// =============================================================================

// CreateCostProductRequest creates a draft request.
func (h *CostProductRequestHandler) CreateCostProductRequest(ctx context.Context, req *financev1.CreateCostProductRequestRequest) (*financev1.CreateCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	r, err := h.createHandler.Handle(ctx, app.CreateCommand{
		RequestTypeID:         req.GetRequestTypeId(),
		Title:                 req.GetTitle(),
		Description:           req.GetDescription(),
		CustomerName:          req.GetCustomerName(),
		CustomerCode:          req.GetCustomerCode(),
		ProductClassification: req.GetProductClassification(),
		TargetVolume:          req.GetTargetVolume(),
		TargetPriceRange:      req.GetTargetPriceRange(),
		UrgencyLevel:          req.GetUrgencyLevel(),
		NeededByDate:          req.GetNeededByDate(),
		RequesterUserID:       actor,
		Spec:                  specInputFromProto(req.GetSpec()),
	})
	if err != nil {
		return &financev1.CreateCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.CreateCostProductRequestResponse{
		Base: successResponse("Request created"),
		Data: requestToProto(r),
	}, nil
}

// GetCostProductRequest returns by id.
func (h *CostProductRequestHandler) GetCostProductRequest(ctx context.Context, req *financev1.GetCostProductRequestRequest) (*financev1.GetCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductRequestResponse{Base: baseResp}, nil
	}
	r, err := h.getHandler.Handle(ctx, app.GetQuery{RequestID: req.GetRequestId()})
	if err != nil {
		return &financev1.GetCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.GetCostProductRequestResponse{
		Base: successResponse("OK"),
		Data: requestToProto(r),
	}, nil
}

// GetCostProductRequestByNo returns by request_no.
func (h *CostProductRequestHandler) GetCostProductRequestByNo(ctx context.Context, req *financev1.GetCostProductRequestByNoRequest) (*financev1.GetCostProductRequestByNoResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductRequestByNoResponse{Base: baseResp}, nil
	}
	r, err := h.getHandler.Handle(ctx, app.GetQuery{RequestNo: req.GetRequestNo()})
	if err != nil {
		return &financev1.GetCostProductRequestByNoResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.GetCostProductRequestByNoResponse{
		Base: successResponse("OK"),
		Data: requestToProto(r),
	}, nil
}

// UpdateCostProductRequest mutates draft fields.
func (h *CostProductRequestHandler) UpdateCostProductRequest(ctx context.Context, req *financev1.UpdateCostProductRequestRequest) (*financev1.UpdateCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostProductRequestResponse{Base: baseResp}, nil
	}
	r, err := h.updateHandler.Handle(ctx, app.UpdateCommand{
		RequestID:             req.GetRequestId(),
		Title:                 req.GetTitle(),
		Description:           req.GetDescription(),
		CustomerName:          req.GetCustomerName(),
		CustomerCode:          req.GetCustomerCode(),
		ProductClassification: req.GetProductClassification(),
		TargetVolume:          req.GetTargetVolume(),
		TargetPriceRange:      req.GetTargetPriceRange(),
		UrgencyLevel:          req.GetUrgencyLevel(),
		NeededByDate:          req.GetNeededByDate(),
		Spec:                  specInputFromProto(req.GetSpec()),
	})
	if err != nil {
		return &financev1.UpdateCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.UpdateCostProductRequestResponse{
		Base: successResponse("Request updated"),
		Data: requestToProto(r),
	}, nil
}

// ListCostProductRequests paginates requests.
func (h *CostProductRequestHandler) ListCostProductRequests(ctx context.Context, req *financev1.ListCostProductRequestsRequest) (*financev1.ListCostProductRequestsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostProductRequestsResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	res, err := h.listHandler.Handle(ctx, app.ListQuery{
		Search:          req.GetSearch(),
		Status:          req.GetStatus(),
		RequestTypeID:   req.GetRequestTypeId(),
		RequesterUserID: req.GetRequesterUserId(),
		AssigneeUserID:  req.GetAssigneeUserId(),
		Page:            int(page),
		PageSize:        int(pageSize),
		SortBy:          req.GetSortBy(),
		SortOrder:       req.GetSortOrder(),
	})
	if err != nil {
		return &financev1.ListCostProductRequestsResponse{Base: requestErrToBase(err)}, nil
	}
	items := make([]*financev1.CostProductRequest, 0, len(res.Items))
	for _, r := range res.Items {
		items = append(items, requestToProto(r))
	}
	return &financev1.ListCostProductRequestsResponse{
		Base:       successResponse("OK"),
		Data:       items,
		Pagination: paginationResponse(page, pageSize, res.Total),
	}, nil
}

// =============================================================================
// State transitions
// =============================================================================

// SubmitCostProductRequest transitions DRAFT → SUBMITTED.
func (h *CostProductRequestHandler) SubmitCostProductRequest(ctx context.Context, req *financev1.SubmitCostProductRequestRequest) (*financev1.SubmitCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.SubmitCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Submit(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.SubmitCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.SubmitCostProductRequestResponse{Base: successResponse("Submitted"), Data: requestToProto(r)}, nil
}

// StartCostProductRequestReview transitions SUBMITTED → UNDER_REVIEW.
func (h *CostProductRequestHandler) StartCostProductRequestReview(ctx context.Context, req *financev1.StartCostProductRequestReviewRequest) (*financev1.StartCostProductRequestReviewResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.StartCostProductRequestReviewResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.StartReview(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.StartCostProductRequestReviewResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.StartCostProductRequestReviewResponse{Base: successResponse("Review started"), Data: requestToProto(r)}, nil
}

// VerifyCostProductRequestClassification sets verified_classification.
func (h *CostProductRequestHandler) VerifyCostProductRequestClassification(ctx context.Context, req *financev1.VerifyCostProductRequestClassificationRequest) (*financev1.VerifyCostProductRequestClassificationResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.VerifyCostProductRequestClassificationResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.VerifyClassification(ctx, req.GetRequestId(), req.GetVerifiedClassification(), req.GetOverrideReason(), actor, actorName)
	if err != nil {
		return &financev1.VerifyCostProductRequestClassificationResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.VerifyCostProductRequestClassificationResponse{Base: successResponse("Classification verified"), Data: requestToProto(r)}, nil
}

// DecideCostProductRequestFeasibility advances UNDER_REVIEW → ROUTING_DEFINED or REJECTED.
func (h *CostProductRequestHandler) DecideCostProductRequestFeasibility(ctx context.Context, req *financev1.DecideCostProductRequestFeasibilityRequest) (*financev1.DecideCostProductRequestFeasibilityResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DecideCostProductRequestFeasibilityResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.DecideFeasibility(ctx, req.GetRequestId(), req.GetDecision(), req.GetNote(), actor, actorName)
	if err != nil {
		return &financev1.DecideCostProductRequestFeasibilityResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.DecideCostProductRequestFeasibilityResponse{Base: successResponse("Feasibility decided"), Data: requestToProto(r)}, nil
}

// UseExistingCostingForCostProductRequest jumps UNDER_REVIEW → QUOTE_READY.
func (h *CostProductRequestHandler) UseExistingCostingForCostProductRequest(ctx context.Context, req *financev1.UseExistingCostingForCostProductRequestRequest) (*financev1.UseExistingCostingForCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UseExistingCostingForCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.UseExistingCosting(ctx, req.GetRequestId(), req.GetExistingProductSysId(), actor, actorName)
	if err != nil {
		return &financev1.UseExistingCostingForCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.UseExistingCostingForCostProductRequestResponse{Base: successResponse("Marked quote-ready"), Data: requestToProto(r)}, nil
}

// RejectCostProductRequest sends to REJECTED.
func (h *CostProductRequestHandler) RejectCostProductRequest(ctx context.Context, req *financev1.RejectCostProductRequestRequest) (*financev1.RejectCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.RejectCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Reject(ctx, req.GetRequestId(), req.GetReason(), actor, actorName)
	if err != nil {
		return &financev1.RejectCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.RejectCostProductRequestResponse{Base: successResponse("Rejected"), Data: requestToProto(r)}, nil
}

// MarkParameterPending advances ROUTING_DEFINED → PARAMETER_PENDING.
// Creates fill tasks for every route level linked to this request.
func (h *CostProductRequestHandler) MarkParameterPending(ctx context.Context, req *financev1.MarkParameterPendingRequest) (*financev1.MarkParameterPendingResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.MarkParameterPendingResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.MarkParameterPending(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.MarkParameterPendingResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.MarkParameterPendingResponse{
		Base: successResponse("Route promoted — fill tasks created"),
		Data: requestToProto(r),
	}, nil
}

// MarkParameterComplete advances PARAMETER_PENDING → PARAMETER_COMPLETE.
// The per-product missing-required-params validation happens client-side via
// CheckMissingRequiredParams (shown in the UI badge); this handler just
// performs the state transition.
//
// TODO(S8): once Phase C calc engine reaches the request handler, gate this
// here too by enumerating routing drafts → products → CPP MissingRequired.
func (h *CostProductRequestHandler) MarkParameterComplete(ctx context.Context, req *financev1.MarkParameterCompleteRequest) (*financev1.MarkParameterCompleteResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.MarkParameterCompleteResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.MarkParameterComplete(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.MarkParameterCompleteResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.MarkParameterCompleteResponse{
		Base: successResponse("Marked parameters complete"),
		Data: requestToProto(r),
	}, nil
}

// ReviseCostProductRequest re-submits a REJECTED request.
func (h *CostProductRequestHandler) ReviseCostProductRequest(ctx context.Context, req *financev1.ReviseCostProductRequestRequest) (*financev1.ReviseCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ReviseCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Revise(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.ReviseCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.ReviseCostProductRequestResponse{Base: successResponse("Revised; back to SUBMITTED"), Data: requestToProto(r)}, nil
}

// ReopenCostProductRequest moves a CLOSED request back to DRAFT.
func (h *CostProductRequestHandler) ReopenCostProductRequest(ctx context.Context, req *financev1.ReopenCostProductRequestRequest) (*financev1.ReopenCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ReopenCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Reopen(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.ReopenCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.ReopenCostProductRequestResponse{Base: successResponse("Reopened; back to DRAFT"), Data: requestToProto(r)}, nil
}

// CancelCostProductRequest closes with substatus=cancelled.
func (h *CostProductRequestHandler) CancelCostProductRequest(ctx context.Context, req *financev1.CancelCostProductRequestRequest) (*financev1.CancelCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CancelCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Cancel(ctx, req.GetRequestId(), req.GetReason(), actor, actorName)
	if err != nil {
		return &financev1.CancelCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.CancelCostProductRequestResponse{Base: successResponse("Cancelled"), Data: requestToProto(r)}, nil
}

// CloseCostProductRequest sets a closed substatus.
func (h *CostProductRequestHandler) CloseCostProductRequest(ctx context.Context, req *financev1.CloseCostProductRequestRequest) (*financev1.CloseCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CloseCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Close(ctx, req.GetRequestId(), req.GetClosedSubstatus(), actor, actorName)
	if err != nil {
		return &financev1.CloseCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.CloseCostProductRequestResponse{Base: successResponse("Closed"), Data: requestToProto(r)}, nil
}

// AssignCostProductRequest updates assignee.
func (h *CostProductRequestHandler) AssignCostProductRequest(ctx context.Context, req *financev1.AssignCostProductRequestRequest) (*financev1.AssignCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.AssignCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Assign(ctx, req.GetRequestId(), req.GetAssigneeUserId(), actor, actorName)
	if err != nil {
		return &financev1.AssignCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.AssignCostProductRequestResponse{Base: successResponse("Assigned"), Data: requestToProto(r)}, nil
}

// ConfirmCostProductRequest advances PARAMETER_COMPLETE → CONFIRMED.
func (h *CostProductRequestHandler) ConfirmCostProductRequest(ctx context.Context, req *financev1.ConfirmCostProductRequestRequest) (*financev1.ConfirmCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ConfirmCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Confirm(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.ConfirmCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.ConfirmCostProductRequestResponse{Base: successResponse("Confirmed"), Data: requestToProto(r)}, nil
}

// ApproveCostProductRequest advances CONFIRMED → APPROVED.
func (h *CostProductRequestHandler) ApproveCostProductRequest(ctx context.Context, req *financev1.ApproveCostProductRequestRequest) (*financev1.ApproveCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ApproveCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Approve(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.ApproveCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.ApproveCostProductRequestResponse{Base: successResponse("Approved"), Data: requestToProto(r)}, nil
}

// ReleaseCostProductRequest advances APPROVED → RELEASED.
func (h *CostProductRequestHandler) ReleaseCostProductRequest(ctx context.Context, req *financev1.ReleaseCostProductRequestRequest) (*financev1.ReleaseCostProductRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ReleaseCostProductRequestResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	actorName, _ := GetUsernameFromCtx(ctx)
	r, err := h.transitionHandler.Release(ctx, req.GetRequestId(), actor, actorName)
	if err != nil {
		return &financev1.ReleaseCostProductRequestResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.ReleaseCostProductRequestResponse{Base: successResponse("Released"), Data: requestToProto(r)}, nil
}

// =============================================================================
// mappers
// =============================================================================

func specInputFromProto(in *financev1.SpecInput) *domain.SpecInput {
	if in == nil {
		return nil
	}
	return &domain.SpecInput{
		RawMaterialType:    in.GetRawMaterialType(),
		ProductDescription: in.GetProductDescription(),
		ShadeID:            in.GetShadeId(),
		ShadeCustomText:    in.GetShadeCustomText(),
		PaperTubeTypeID:    in.GetPaperTubeTypeId(),
		WeightPerBobbinKg:  in.GetWeightPerBobbinKg(),
		BoxType:            in.GetBoxType(),
	}
}

func requestToProto(r *domain.Request) *financev1.CostProductRequest {
	out := &financev1.CostProductRequest{
		RequestId:                    r.RequestID(),
		RequestNo:                    r.RequestNo(),
		RequestTypeId:                r.RequestTypeID(),
		Title:                        r.Title(),
		Description:                  r.Description(),
		CustomerName:                 r.CustomerName(),
		CustomerCode:                 r.CustomerCode(),
		ProductClassification:        r.ProductClassification(),
		VerifiedClassification:       r.VerifiedClassification(),
		ClassificationOverrideReason: r.ClassificationOverrideReason(),
		TargetVolume:                 r.TargetVolume(),
		TargetPriceRange:             r.TargetPriceRange(),
		UrgencyLevel:                 r.UrgencyLevel(),
		NeededByDate:                 r.NeededByDate(),
		Status:                       r.Status(),
		ClosedSubstatus:              r.ClosedSubstatus(),
		FeasibilityDecision:          r.FeasibilityDecision(),
		FeasibilityNote:              r.FeasibilityNote(),
		FeasibilityBy:                r.FeasibilityBy(),
		RejectReason:                 r.RejectReason(),
		CancelReason:                 r.CancelReason(),
		AssignedToUserId:             r.AssignedToUserID(),
		RequesterUserId:              r.RequesterUserID(),
		ExistingProductSysId:         r.ExistingProductSysID(),
		LinkedRouteHeadId:            r.LinkedRouteHeadID(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: r.CreatedAt().Format(time.RFC3339),
			CreatedBy: r.RequesterUserID(),
			UpdatedAt: r.UpdatedAt().Format(time.RFC3339),
		},
	}
	if t := r.FeasibilityAt(); t != nil {
		out.FeasibilityAt = t.Format(time.RFC3339)
	}
	if s := r.Spec(); s != nil {
		out.Spec = &financev1.CostProductSpec{
			SpecId:             s.SpecID,
			RequestId:          r.RequestID(),
			RawMaterialType:    s.RawMaterialType,
			ProductDescription: s.ProductDescription,
			ShadeCustomText:    s.ShadeCustomText,
			PaperTubeTypeId:    s.PaperTubeTypeID,
			WeightPerBobbinKg:  s.WeightPerBobbinKg,
			BoxType:            s.BoxType,
			CreatedAt:          s.CreatedAt.Format(time.RFC3339),
			CreatedBy:          s.CreatedBy,
		}
		if s.ShadeID != nil {
			out.Spec.ShadeId = *s.ShadeID
		}
	}
	return out
}

// LinkExistingRoute attaches an existing route_head to the request.
func (h *CostProductRequestHandler) LinkExistingRoute(ctx context.Context, req *financev1.LinkExistingRouteRequest) (*financev1.LinkExistingRouteResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.LinkExistingRouteResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	res, err := h.linkRouteHandler.Handle(ctx, app.LinkRouteCommand{
		RequestID:   req.GetRequestId(),
		RouteHeadID: req.GetRouteHeadId(),
		ActorUserID: actor,
	})
	if err != nil {
		return &financev1.LinkExistingRouteResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.LinkExistingRouteResponse{
		Base: successResponse("Route linked"),
		Data: requestToProto(res),
	}, nil
}

// UnlinkRoute clears any linked route head from the request.
func (h *CostProductRequestHandler) UnlinkRoute(ctx context.Context, req *financev1.UnlinkRouteRequest) (*financev1.UnlinkRouteResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UnlinkRouteResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	res, err := h.unlinkRouteHandler.Handle(ctx, app.UnlinkRouteCommand{
		RequestID:   req.GetRequestId(),
		ActorUserID: actor,
	})
	if err != nil {
		return &financev1.UnlinkRouteResponse{Base: requestErrToBase(err)}, nil
	}
	return &financev1.UnlinkRouteResponse{
		Base: successResponse("Route unlinked"),
		Data: requestToProto(res),
	}, nil
}

// GetCostProductRequestHistory returns the full status-transition timeline for a CPR.
func (h *CostProductRequestHandler) GetCostProductRequestHistory(
	ctx context.Context,
	req *financev1.GetCostProductRequestHistoryRequest,
) (*financev1.GetCostProductRequestHistoryResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductRequestHistoryResponse{Base: baseResp}, nil
	}
	if h.historyRepo == nil {
		return &financev1.GetCostProductRequestHistoryResponse{
			Base:    successResponse("OK"),
			Entries: nil,
		}, nil
	}
	entries, err := h.historyRepo.ListByRequestID(ctx, req.GetRequestId())
	if err != nil {
		log.Error().Err(err).Int64("request_id", req.GetRequestId()).Msg("GetCostProductRequestHistory: list failed")
		return &financev1.GetCostProductRequestHistoryResponse{
			Base: InternalErrorResponse("internal server error"),
		}, nil //nolint:nilerr // BaseResponse pattern
	}
	result := make([]*financev1.StatusHistoryEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, &financev1.StatusHistoryEntry{
			Id:          e.ID,
			RequestId:   e.RequestID,
			FromStatus:  e.FromStatus,
			ToStatus:    e.ToStatus,
			ActorUserId: e.ActorUserID,
			ActorName:   e.ActorName,
			Note:        e.Note,
			CreatedAt:   e.CreatedAt.Format(time.RFC3339),
		})
	}
	return &financev1.GetCostProductRequestHistoryResponse{
		Base:    successResponse("OK"),
		Entries: result,
	}, nil
}

// GetParamSummary returns all param values for the request grouped by product and fill level.
func (h *CostProductRequestHandler) GetParamSummary(ctx context.Context, req *financev1.GetParamSummaryRequest) (*financev1.GetParamSummaryResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetParamSummaryResponse{Base: baseResp}, nil
	}
	if h.paramSummary == nil {
		return &financev1.GetParamSummaryResponse{
			Base:     successResponse("OK"),
			Products: nil,
		}, nil
	}
	products, total, filled, err := h.paramSummary.Handle(ctx, app.GetParamSummaryQuery{RequestID: req.GetRequestId()})
	if err != nil {
		log.Error().Err(err).Int64("request_id", req.GetRequestId()).Msg("GetParamSummary: failed")
		return &financev1.GetParamSummaryResponse{
			Base: InternalErrorResponse("internal server error"),
		}, nil //nolint:nilerr // BaseResponse pattern
	}
	protoProducts := make([]*financev1.ProductParamSummary, 0, len(products))
	for _, p := range products {
		levels := make([]*financev1.FillLevelSummary, 0, len(p.Levels))
		for _, l := range p.Levels {
			params := make([]*financev1.ParamValueEntry, 0, len(l.Params))
			for _, pv := range l.Params {
				params = append(params, &financev1.ParamValueEntry{
					ParamId:      pv.ParamID,
					ParamCode:    pv.ParamCode,
					ParamName:    pv.ParamName,
					DataType:     pv.DataType,
					HasValue:     pv.HasValue,
					ValueNumeric: pv.ValueNumeric,
					ValueText:    pv.ValueText,
					ValueFlag:    pv.ValueFlag,
					UomCode:      pv.UOMCode,
					IsRequired:   pv.IsRequired,
				})
			}
			levels = append(levels, &financev1.FillLevelSummary{
				RouteLevel:     l.RouteLevel,
				TaskStatus:     l.TaskStatus,
				FilledByUserId: l.FilledByUserID,
				FilledAt:       l.FilledAt,
				FilledParams:   l.FilledParams,
				TotalParams:    l.TotalParams,
				Params:         params,
				LastEditedBy:   l.LastEditedBy,
				LastEditedAt:   l.LastEditedAt,
			})
		}
		protoProducts = append(protoProducts, &financev1.ProductParamSummary{
			ProductSysId: p.ProductSysID,
			ProductCode:  p.ProductCode,
			ProductName:  p.ProductName,
			Levels:       levels,
		})
	}
	return &financev1.GetParamSummaryResponse{
		Base:         successResponse("OK"),
		Products:     protoProducts,
		TotalParams:  total,
		FilledParams: filled,
	}, nil
}

func requestErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidTitle),
		errors.Is(err, domain.ErrInvalidCustomerName),
		errors.Is(err, domain.ErrInvalidClassification),
		errors.Is(err, domain.ErrInvalidUrgency),
		errors.Is(err, domain.ErrInvalidVerified),
		errors.Is(err, domain.ErrOverrideReasonRequired),
		errors.Is(err, domain.ErrInvalidFeasibility),
		errors.Is(err, domain.ErrFeasibilityNoteMissing),
		errors.Is(err, domain.ErrInvalidSubstatus),
		errors.Is(err, domain.ErrSpecRequired),
		errors.Is(err, domain.ErrSpecNotAllowed),
		errors.Is(err, domain.ErrInvalidSpec),
		errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrExistingProductRequired):
		return ErrorResponse("400", err.Error())
	case errors.Is(err, app.ErrRouteNotLocked):
		return ErrorResponse("422", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
