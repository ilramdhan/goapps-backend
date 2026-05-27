package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appwfins "github.com/mutugading/goapps-backend/services/iam/internal/application/workflowinstance"
	domwfins "github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowinstance"
	domtpl "github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowtemplate"
)

// WorkflowInstanceHandler implements iamv1.WorkflowInstanceServiceServer.
type WorkflowInstanceHandler struct {
	iamv1.UnimplementedWorkflowInstanceServiceServer
	startHandler   *appwfins.StartHandler
	getHandler     *appwfins.GetHandler
	advanceHandler *appwfins.AdvanceHandler
	rejectHandler  *appwfins.RejectHandler
	listHandler    *appwfins.ListHandler
	validation     *ValidationHelper
}

// NewWorkflowInstanceHandler wires the handler.
func NewWorkflowInstanceHandler(
	tplRepo domtpl.Repository, insRepo domwfins.Repository,
) (*WorkflowInstanceHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &WorkflowInstanceHandler{
		startHandler:   appwfins.NewStartHandler(tplRepo, insRepo),
		getHandler:     appwfins.NewGetHandler(insRepo),
		advanceHandler: appwfins.NewAdvanceHandler(tplRepo, insRepo),
		rejectHandler:  appwfins.NewRejectHandler(insRepo),
		listHandler:    appwfins.NewListHandler(insRepo),
		validation:     v,
	}, nil
}

// StartWorkflowInstance handles Start.
func (h *WorkflowInstanceHandler) StartWorkflowInstance(ctx context.Context, req *iamv1.StartWorkflowInstanceRequest) (*iamv1.StartWorkflowInstanceResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.StartWorkflowInstanceResponse{Base: baseResp}, nil
	}
	tplID, err := uuid.Parse(req.TemplateId)
	if err != nil {
		return &iamv1.StartWorkflowInstanceResponse{Base: ErrorResponse("400", "invalid template_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	entID, err := uuid.Parse(req.EntityId)
	if err != nil {
		return &iamv1.StartWorkflowInstanceResponse{Base: ErrorResponse("400", "invalid entity_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	ins, err := h.startHandler.Handle(ctx, appwfins.StartCommand{
		TemplateID: tplID,
		EntityKind: req.EntityKind,
		EntityID:   entID,
		StartedBy:  GetUsernameFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.StartWorkflowInstanceResponse{Base: wfInstanceErrToBase(err)}, nil
	}
	return &iamv1.StartWorkflowInstanceResponse{
		Base: SuccessResponse("Workflow instance started"),
		Data: instanceToProto(ins),
	}, nil
}

// GetWorkflowInstance handles Get.
func (h *WorkflowInstanceHandler) GetWorkflowInstance(ctx context.Context, req *iamv1.GetWorkflowInstanceRequest) (*iamv1.GetWorkflowInstanceResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetWorkflowInstanceResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.InstanceId)
	if err != nil {
		return &iamv1.GetWorkflowInstanceResponse{Base: ErrorResponse("400", "invalid instance_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	ins, err := h.getHandler.Handle(ctx, appwfins.GetQuery{ID: id})
	if err != nil {
		return &iamv1.GetWorkflowInstanceResponse{Base: wfInstanceErrToBase(err)}, nil
	}
	return &iamv1.GetWorkflowInstanceResponse{
		Base: SuccessResponse("OK"),
		Data: instanceToProto(ins),
	}, nil
}

// AdvanceWorkflowInstance handles Advance.
func (h *WorkflowInstanceHandler) AdvanceWorkflowInstance(ctx context.Context, req *iamv1.AdvanceWorkflowInstanceRequest) (*iamv1.AdvanceWorkflowInstanceResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.AdvanceWorkflowInstanceResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.InstanceId)
	if err != nil {
		return &iamv1.AdvanceWorkflowInstanceResponse{Base: ErrorResponse("400", "invalid instance_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	actorStr, _ := GetUserIDFromCtx(ctx)
	actor, perr := uuid.Parse(actorStr)
	_ = perr // best-effort: uuid.Nil when unauthenticated (tests)

	ins, err := h.advanceHandler.Handle(ctx, appwfins.AdvanceCommand{
		InstanceID: id,
		Actor:      actor,
		Comment:    req.Comment,
	})
	if err != nil {
		return &iamv1.AdvanceWorkflowInstanceResponse{Base: wfInstanceErrToBase(err)}, nil
	}
	return &iamv1.AdvanceWorkflowInstanceResponse{
		Base: SuccessResponse("Workflow step approved"),
		Data: instanceToProto(ins),
	}, nil
}

// RejectWorkflowInstance handles Reject.
func (h *WorkflowInstanceHandler) RejectWorkflowInstance(ctx context.Context, req *iamv1.RejectWorkflowInstanceRequest) (*iamv1.RejectWorkflowInstanceResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.RejectWorkflowInstanceResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.InstanceId)
	if err != nil {
		return &iamv1.RejectWorkflowInstanceResponse{Base: ErrorResponse("400", "invalid instance_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	actorStr, _ := GetUserIDFromCtx(ctx)
	actor, perr := uuid.Parse(actorStr)
	_ = perr // best-effort: uuid.Nil when unauthenticated (tests)

	ins, err := h.rejectHandler.Handle(ctx, appwfins.RejectCommand{
		InstanceID: id,
		Actor:      actor,
		Comment:    req.Comment,
	})
	if err != nil {
		return &iamv1.RejectWorkflowInstanceResponse{Base: wfInstanceErrToBase(err)}, nil
	}
	return &iamv1.RejectWorkflowInstanceResponse{
		Base: SuccessResponse("Workflow instance rejected"),
		Data: instanceToProto(ins),
	}, nil
}

// ListWorkflowInstances handles List.
func (h *WorkflowInstanceHandler) ListWorkflowInstances(ctx context.Context, req *iamv1.ListWorkflowInstancesRequest) (*iamv1.ListWorkflowInstancesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListWorkflowInstancesResponse{Base: baseResp}, nil
	}
	page := int32(1)
	pageSize := int32(10)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	res, err := h.listHandler.Handle(ctx, appwfins.ListQuery{
		EntityKind: req.EntityKind,
		EntityID:   req.EntityId,
		Status:     req.Status,
		Page:       int(page),
		PageSize:   int(pageSize),
	})
	if err != nil {
		return &iamv1.ListWorkflowInstancesResponse{Base: wfInstanceErrToBase(err)}, nil
	}
	items := make([]*iamv1.WorkflowInstance, 0, len(res.Items))
	for _, ins := range res.Items {
		items = append(items, instanceToProto(ins))
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeIntToInt32WfTplH(int((res.Total + int64(pageSize) - 1) / int64(pageSize)))
	}
	return &iamv1.ListWorkflowInstancesResponse{
		Base: SuccessResponse("OK"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalItems:  res.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// =============================================================================
// mappers
// =============================================================================

func instanceToProto(ins *domwfins.Instance) *iamv1.WorkflowInstance {
	steps := make([]*iamv1.WorkflowInstanceStep, 0, len(ins.Steps()))
	for _, s := range ins.Steps() {
		ps := &iamv1.WorkflowInstanceStep{
			InstanceStepId:          s.ID().String(),
			InstanceId:              s.InstanceID().String(),
			StepNo:                  safeIntToInt32WfIns2(s.StepNo()),
			StepName:                s.StepName(),
			ApproverResolutionType:  s.ApproverResolutionType(),
			ApproverResolutionValue: s.ApproverResolutionValue(),
			SlaHours:                safeIntToInt32WfIns2(s.SLAHours()),
			AssignedAt:              s.AssignedAt().Format(time.RFC3339),
			Decision:                s.Decision(),
			Comment:                 s.Comment(),
		}
		if s.ActorUserID() != nil {
			ps.ActorUserId = s.ActorUserID().String()
		}
		if s.DecidedAt() != nil {
			ps.DecidedAt = s.DecidedAt().Format(time.RFC3339)
		}
		if s.StuckSince() != nil {
			ps.StuckSince = s.StuckSince().Format(time.RFC3339)
		}
		steps = append(steps, ps)
	}
	out := &iamv1.WorkflowInstance{
		InstanceId:      ins.ID().String(),
		TemplateId:      ins.TemplateID().String(),
		TemplateVersion: safeIntToInt32WfIns2(ins.TemplateVersion()),
		Kind:            ins.Kind(),
		EntityKind:      ins.EntityKind(),
		EntityId:        ins.EntityID().String(),
		CurrentStepNo:   safeIntToInt32WfIns2(ins.CurrentStepNo()),
		Status:          ins.Status(),
		StartedBy:       ins.StartedBy(),
		StartedAt:       ins.StartedAt().Format(time.RFC3339),
		Steps:           steps,
	}
	if ins.CompletedAt() != nil {
		out.CompletedAt = ins.CompletedAt().Format(time.RFC3339)
	}
	return out
}

func wfInstanceErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domwfins.ErrNotFound), errors.Is(err, domtpl.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domwfins.ErrNoActiveTemplate),
		errors.Is(err, domwfins.ErrInvalidEntityKind),
		errors.Is(err, domwfins.ErrInvalidStatus),
		errors.Is(err, domwfins.ErrNotInProgress),
		errors.Is(err, domwfins.ErrRejectNotAllowed),
		errors.Is(err, domwfins.ErrCurrentStepMissing),
		errors.Is(err, domwfins.ErrInvalidComment):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}

func safeIntToInt32WfIns2(v int) int32 {
	const maxInt32 = 1<<31 - 1
	if v > maxInt32 {
		return maxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v) //nolint:gosec // bounds checked above
}
