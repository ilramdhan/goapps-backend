package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appwftpl "github.com/mutugading/goapps-backend/services/iam/internal/application/workflowtemplate"
	domwftpl "github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowtemplate"
)

// WorkflowTemplateHandler implements iamv1.WorkflowTemplateServiceServer.
type WorkflowTemplateHandler struct {
	iamv1.UnimplementedWorkflowTemplateServiceServer
	createHandler   *appwftpl.CreateHandler
	getHandler      *appwftpl.GetHandler
	updateHandler   *appwftpl.UpdateHandler
	activateHandler *appwftpl.ActivateHandler
	deleteHandler   *appwftpl.DeleteHandler
	listHandler     *appwftpl.ListHandler
	validation      *ValidationHelper
}

// NewWorkflowTemplateHandler constructs the handler.
func NewWorkflowTemplateHandler(repo domwftpl.Repository) (*WorkflowTemplateHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &WorkflowTemplateHandler{
		createHandler:   appwftpl.NewCreateHandler(repo),
		getHandler:      appwftpl.NewGetHandler(repo),
		updateHandler:   appwftpl.NewUpdateHandler(repo),
		activateHandler: appwftpl.NewActivateHandler(repo),
		deleteHandler:   appwftpl.NewDeleteHandler(repo),
		listHandler:     appwftpl.NewListHandler(repo),
		validation:      v,
	}, nil
}

// CreateWorkflowTemplate handles the create RPC.
func (h *WorkflowTemplateHandler) CreateWorkflowTemplate(ctx context.Context, req *iamv1.CreateWorkflowTemplateRequest) (*iamv1.CreateWorkflowTemplateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateWorkflowTemplateResponse{Base: baseResp}, nil
	}
	t, err := h.createHandler.Handle(ctx, appwftpl.CreateCommand{
		Kind:        req.Kind,
		Name:        req.Name,
		Description: req.Description,
		Steps:       toStepInputs(req.Steps),
		CreatedBy:   GetUsernameFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.CreateWorkflowTemplateResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	return &iamv1.CreateWorkflowTemplateResponse{
		Base: SuccessResponse("Workflow template created"),
		Data: templateToProto(t),
	}, nil
}

// GetWorkflowTemplate handles the get RPC.
func (h *WorkflowTemplateHandler) GetWorkflowTemplate(ctx context.Context, req *iamv1.GetWorkflowTemplateRequest) (*iamv1.GetWorkflowTemplateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetWorkflowTemplateResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.TemplateId)
	if err != nil {
		return &iamv1.GetWorkflowTemplateResponse{Base: ErrorResponse("400", "invalid template_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	t, err := h.getHandler.Handle(ctx, appwftpl.GetQuery{ID: id})
	if err != nil {
		return &iamv1.GetWorkflowTemplateResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	return &iamv1.GetWorkflowTemplateResponse{
		Base: SuccessResponse("OK"),
		Data: templateToProto(t),
	}, nil
}

// UpdateWorkflowTemplate creates a new version of the template.
func (h *WorkflowTemplateHandler) UpdateWorkflowTemplate(ctx context.Context, req *iamv1.UpdateWorkflowTemplateRequest) (*iamv1.UpdateWorkflowTemplateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateWorkflowTemplateResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.TemplateId)
	if err != nil {
		return &iamv1.UpdateWorkflowTemplateResponse{Base: ErrorResponse("400", "invalid template_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	t, err := h.updateHandler.Handle(ctx, appwftpl.UpdateCommand{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Steps:       toStepInputs(req.Steps),
		UpdatedBy:   GetUsernameFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.UpdateWorkflowTemplateResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	return &iamv1.UpdateWorkflowTemplateResponse{
		Base: SuccessResponse("New workflow template version created"),
		Data: templateToProto(t),
	}, nil
}

// ActivateWorkflowTemplate activates the given version.
func (h *WorkflowTemplateHandler) ActivateWorkflowTemplate(ctx context.Context, req *iamv1.ActivateWorkflowTemplateRequest) (*iamv1.ActivateWorkflowTemplateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.ActivateWorkflowTemplateResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.TemplateId)
	if err != nil {
		return &iamv1.ActivateWorkflowTemplateResponse{Base: ErrorResponse("400", "invalid template_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	t, err := h.activateHandler.Handle(ctx, appwftpl.ActivateCommand{ID: id, By: GetUsernameFromCtx(ctx)})
	if err != nil {
		return &iamv1.ActivateWorkflowTemplateResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	return &iamv1.ActivateWorkflowTemplateResponse{
		Base: SuccessResponse("Workflow template activated"),
		Data: templateToProto(t),
	}, nil
}

// DeleteWorkflowTemplate soft-deletes a template version.
func (h *WorkflowTemplateHandler) DeleteWorkflowTemplate(ctx context.Context, req *iamv1.DeleteWorkflowTemplateRequest) (*iamv1.DeleteWorkflowTemplateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteWorkflowTemplateResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.TemplateId)
	if err != nil {
		return &iamv1.DeleteWorkflowTemplateResponse{Base: ErrorResponse("400", "invalid template_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	if err := h.deleteHandler.Handle(ctx, appwftpl.DeleteCommand{ID: id, DeletedBy: GetUsernameFromCtx(ctx)}); err != nil {
		return &iamv1.DeleteWorkflowTemplateResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	return &iamv1.DeleteWorkflowTemplateResponse{Base: SuccessResponse("Workflow template deleted")}, nil
}

// ListWorkflowTemplates returns a paginated list.
func (h *WorkflowTemplateHandler) ListWorkflowTemplates(ctx context.Context, req *iamv1.ListWorkflowTemplatesRequest) (*iamv1.ListWorkflowTemplatesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListWorkflowTemplatesResponse{Base: baseResp}, nil
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
	res, err := h.listHandler.Handle(ctx, appwftpl.ListQuery{
		Search:       req.Search,
		Kind:         req.Kind,
		ActiveFilter: req.ActiveFilter,
		Page:         int(page),
		PageSize:     int(pageSize),
		SortBy:       req.SortBy,
		SortOrder:    req.SortOrder,
	})
	if err != nil {
		return &iamv1.ListWorkflowTemplatesResponse{Base: wfTemplateErrToBase(err)}, nil
	}
	items := make([]*iamv1.WorkflowTemplate, 0, len(res.Items))
	for _, t := range res.Items {
		items = append(items, templateToProto(t))
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeIntToInt32WfTplH(int((res.Total + int64(pageSize) - 1) / int64(pageSize)))
	}
	return &iamv1.ListWorkflowTemplatesResponse{
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

func toStepInputs(in []*iamv1.WorkflowTemplateStepInput) []appwftpl.StepInput {
	out := make([]appwftpl.StepInput, 0, len(in))
	for _, s := range in {
		out = append(out, appwftpl.StepInput{
			StepNo:                  int(s.StepNo),
			StepName:                s.StepName,
			ResolutionType:          s.ApproverResolutionType,
			ResolutionValue:         s.ApproverResolutionValue,
			SLAHours:                int(s.SlaHours),
			AllowReject:             s.AllowReject,
			AllowReassign:           s.AllowReassign,
			RequirePasswordOnUnlock: s.RequirePasswordOnUnlock,
			RejectToStepNo:          int(s.RejectToStepNo),
		})
	}
	return out
}

func templateToProto(t *domwftpl.Template) *iamv1.WorkflowTemplate {
	steps := make([]*iamv1.WorkflowTemplateStep, 0, len(t.Steps()))
	for _, s := range t.Steps() {
		steps = append(steps, &iamv1.WorkflowTemplateStep{
			TemplateStepId:          s.ID().String(),
			TemplateId:              s.TemplateID().String(),
			StepNo:                  safeIntToInt32WfTplH(s.StepNo()),
			StepName:                s.StepName(),
			ApproverResolutionType:  s.ApproverResolutionType().String(),
			ApproverResolutionValue: s.ApproverResolutionValue(),
			SlaHours:                safeIntToInt32WfTplH(s.SLAHours()),
			AllowReject:             s.AllowReject(),
			AllowReassign:           s.AllowReassign(),
			RequirePasswordOnUnlock: s.RequirePasswordOnUnlock(),
			RejectToStepNo:          safeIntToInt32WfTplH(s.RejectToStepNo()),
		})
	}
	out := &iamv1.WorkflowTemplate{
		TemplateId:  t.ID().String(),
		Kind:        t.Kind().String(),
		Name:        t.Name().String(),
		Version:     safeIntToInt32WfTplH(t.Version()),
		IsActive:    t.IsActive(),
		Description: t.Description().String(),
		Steps:       steps,
		Audit: &commonv1.AuditInfo{
			CreatedAt: t.CreatedAt().Format(time.RFC3339),
			CreatedBy: t.CreatedBy(),
		},
	}
	if t.UpdatedAt() != nil {
		out.Audit.UpdatedAt = t.UpdatedAt().Format(time.RFC3339)
	}
	if t.UpdatedBy() != "" {
		out.Audit.UpdatedBy = t.UpdatedBy()
	}
	return out
}

func wfTemplateErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domwftpl.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domwftpl.ErrInvalidKind),
		errors.Is(err, domwftpl.ErrInvalidName),
		errors.Is(err, domwftpl.ErrInvalidDesc),
		errors.Is(err, domwftpl.ErrNoSteps),
		errors.Is(err, domwftpl.ErrInvalidStep),
		errors.Is(err, domwftpl.ErrInvalidResolution),
		errors.Is(err, domwftpl.ErrStepOrder):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}

func safeIntToInt32WfTplH(v int) int32 {
	const maxInt32 = 1<<31 - 1
	if v > maxInt32 {
		return maxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v) //nolint:gosec // bounds checked above
}
