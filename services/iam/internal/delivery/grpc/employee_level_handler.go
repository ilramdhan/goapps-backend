// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	elapp "github.com/mutugading/goapps-backend/services/iam/internal/application/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
)

// EmployeeLevelHandler implements the EmployeeLevelService gRPC service.
type EmployeeLevelHandler struct {
	iamv1.UnimplementedEmployeeLevelServiceServer
	createHandler    *elapp.CreateHandler
	getHandler       *elapp.GetHandler
	updateHandler    *elapp.UpdateHandler
	deleteHandler    *elapp.DeleteHandler
	listHandler      *elapp.ListHandler
	exportHandler    *elapp.ExportHandler
	importHandler    *elapp.ImportHandler
	templateHandler  *elapp.TemplateHandler
	workflowHandler  *elapp.WorkflowHandler
	validationHelper *ValidationHelper
}

// NewEmployeeLevelHandler creates a new EmployeeLevelHandler.
func NewEmployeeLevelHandler(repo employeelevel.Repository, historyRepo employeelevel.WorkflowHistoryRepository, validationHelper *ValidationHelper) *EmployeeLevelHandler {
	return &EmployeeLevelHandler{
		createHandler:    elapp.NewCreateHandler(repo),
		getHandler:       elapp.NewGetHandler(repo),
		updateHandler:    elapp.NewUpdateHandler(repo),
		deleteHandler:    elapp.NewDeleteHandler(repo),
		listHandler:      elapp.NewListHandler(repo),
		exportHandler:    elapp.NewExportHandler(repo),
		importHandler:    elapp.NewImportHandler(repo),
		templateHandler:  elapp.NewTemplateHandler(),
		workflowHandler:  elapp.NewWorkflowHandler(repo, historyRepo),
		validationHelper: validationHelper,
	}
}

// CreateEmployeeLevel creates a new employee level.
func (h *EmployeeLevelHandler) CreateEmployeeLevel(ctx context.Context, req *iamv1.CreateEmployeeLevelRequest) (*iamv1.CreateEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateEmployeeLevelResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.createHandler.Handle(ctx, elapp.CreateCommand{
		Code:      req.GetCode(),
		Name:      req.GetName(),
		Grade:     req.GetGrade(),
		Type:      employeelevel.Type(req.GetType()),
		Sequence:  req.GetSequence(),
		Workflow:  employeelevel.Workflow(req.GetWorkflow()),
		CreatedBy: userID,
	})
	if err != nil {
		return &iamv1.CreateEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreateEmployeeLevelResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Employee level created successfully",
		},
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// GetEmployeeLevel retrieves an employee level by ID.
func (h *EmployeeLevelHandler) GetEmployeeLevel(ctx context.Context, req *iamv1.GetEmployeeLevelRequest) (*iamv1.GetEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetEmployeeLevelResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, elapp.GetQuery{
		EmployeeLevelID: req.GetEmployeeLevelId(),
	})
	if err != nil {
		return &iamv1.GetEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetEmployeeLevelResponse{
		Base: SuccessResponse("Employee level retrieved successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// UpdateEmployeeLevel updates an employee level.
func (h *EmployeeLevelHandler) UpdateEmployeeLevel(ctx context.Context, req *iamv1.UpdateEmployeeLevelRequest) (*iamv1.UpdateEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateEmployeeLevelResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	var typ *employeelevel.Type
	if req.Type != nil {
		v := employeelevel.Type(*req.Type)
		typ = &v
	}
	var wf *employeelevel.Workflow
	if req.Workflow != nil {
		v := employeelevel.Workflow(*req.Workflow)
		wf = &v
	}

	entity, err := h.updateHandler.Handle(ctx, elapp.UpdateCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		Name:            req.Name,
		Grade:           req.Grade,
		Type:            typ,
		Sequence:        req.Sequence,
		Workflow:        wf,
		IsActive:        req.IsActive,
		UpdatedBy:       userID,
	})
	if err != nil {
		return &iamv1.UpdateEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdateEmployeeLevelResponse{
		Base: SuccessResponse("Employee level updated successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// DeleteEmployeeLevel soft-deletes an employee level.
func (h *EmployeeLevelHandler) DeleteEmployeeLevel(ctx context.Context, req *iamv1.DeleteEmployeeLevelRequest) (*iamv1.DeleteEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteEmployeeLevelResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	if err := h.deleteHandler.Handle(ctx, elapp.DeleteCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		DeletedBy:       userID,
	}); err != nil {
		return &iamv1.DeleteEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DeleteEmployeeLevelResponse{
		Base: SuccessResponse("Employee level deleted successfully"),
	}, nil
}

// ListEmployeeLevels lists employee levels with pagination and filters.
func (h *EmployeeLevelHandler) ListEmployeeLevels(ctx context.Context, req *iamv1.ListEmployeeLevelsRequest) (*iamv1.ListEmployeeLevelsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListEmployeeLevelsResponse{Base: baseResp}, nil
	}

	isActive := activeFilterToBool(req.GetActiveFilter())

	var typ *employeelevel.Type
	if req.GetType() != iamv1.EmployeeLevelType_EMPLOYEE_LEVEL_TYPE_UNSPECIFIED {
		v := employeelevel.Type(req.GetType())
		typ = &v
	}
	var wf *employeelevel.Workflow
	if req.GetWorkflow() != iamv1.EmployeeLevelWorkflow_EMPLOYEE_LEVEL_WORKFLOW_UNSPECIFIED {
		v := employeelevel.Workflow(req.GetWorkflow())
		wf = &v
	}

	result, err := h.listHandler.Handle(ctx, elapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		IsActive:  isActive,
		Type:      typ,
		Workflow:  wf,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &iamv1.ListEmployeeLevelsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoItems := make([]*iamv1.EmployeeLevel, len(result.Items))
	for i, e := range result.Items {
		protoItems[i] = toEmployeeLevelProto(e)
	}

	return &iamv1.ListEmployeeLevelsResponse{
		Base: SuccessResponse("Employee levels retrieved successfully"),
		Data: protoItems,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportEmployeeLevels exports employee levels to Excel.
func (h *EmployeeLevelHandler) ExportEmployeeLevels(ctx context.Context, req *iamv1.ExportEmployeeLevelsRequest) (*iamv1.ExportEmployeeLevelsResponse, error) {
	isActive := activeFilterToBool(req.GetActiveFilter())

	var typ *employeelevel.Type
	if req.GetType() != iamv1.EmployeeLevelType_EMPLOYEE_LEVEL_TYPE_UNSPECIFIED {
		v := employeelevel.Type(req.GetType())
		typ = &v
	}
	var wf *employeelevel.Workflow
	if req.GetWorkflow() != iamv1.EmployeeLevelWorkflow_EMPLOYEE_LEVEL_WORKFLOW_UNSPECIFIED {
		v := employeelevel.Workflow(req.GetWorkflow())
		wf = &v
	}

	result, err := h.exportHandler.Handle(ctx, elapp.ExportQuery{
		IsActive: isActive,
		Type:     typ,
		Workflow: wf,
	})
	if err != nil {
		return &iamv1.ExportEmployeeLevelsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.ExportEmployeeLevelsResponse{
		Base:        SuccessResponse("Employee levels exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportEmployeeLevels imports employee levels from Excel.
func (h *EmployeeLevelHandler) ImportEmployeeLevels(ctx context.Context, req *iamv1.ImportEmployeeLevelsRequest) (*iamv1.ImportEmployeeLevelsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ImportEmployeeLevelsResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	result, err := h.importHandler.Handle(ctx, elapp.ImportCommand{
		FileContent:     req.GetFileContent(),
		FileName:        req.GetFileName(),
		DuplicateAction: req.GetDuplicateAction(),
		CreatedBy:       userID,
	})
	if err != nil {
		return &iamv1.ImportEmployeeLevelsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	importErrors := make([]*iamv1.ImportError, len(result.Errors))
	for i, ie := range result.Errors {
		importErrors[i] = &iamv1.ImportError{
			RowNumber: ie.RowNumber,
			Field:     ie.Field,
			Message:   ie.Message,
		}
	}

	return &iamv1.ImportEmployeeLevelsResponse{
		Base:         SuccessResponse("Employee levels import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadEmployeeLevelTemplate downloads an Excel import template.
func (h *EmployeeLevelHandler) DownloadEmployeeLevelTemplate(_ context.Context, _ *iamv1.DownloadEmployeeLevelTemplateRequest) (*iamv1.DownloadEmployeeLevelTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &iamv1.DownloadEmployeeLevelTemplateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DownloadEmployeeLevelTemplateResponse{
		Base:        SuccessResponse("Import template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// SubmitEmployeeLevel transitions workflow DRAFT → SUBMITTED.
func (h *EmployeeLevelHandler) SubmitEmployeeLevel(ctx context.Context, req *iamv1.SubmitEmployeeLevelRequest) (*iamv1.SubmitEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.SubmitEmployeeLevelResponse{Base: baseResp}, nil
	}
	entity, err := h.workflowHandler.Submit(ctx, elapp.WorkflowCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		Notes:           req.GetNotes(),
		UserID:          getUserFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.SubmitEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.SubmitEmployeeLevelResponse{
		Base: SuccessResponse("Employee level submitted successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// ApproveEmployeeLevel transitions workflow SUBMITTED → APPROVED.
func (h *EmployeeLevelHandler) ApproveEmployeeLevel(ctx context.Context, req *iamv1.ApproveEmployeeLevelRequest) (*iamv1.ApproveEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ApproveEmployeeLevelResponse{Base: baseResp}, nil
	}
	entity, err := h.workflowHandler.Approve(ctx, elapp.WorkflowCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		Notes:           req.GetNotes(),
		UserID:          getUserFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.ApproveEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.ApproveEmployeeLevelResponse{
		Base: SuccessResponse("Employee level approved successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// ReleaseEmployeeLevel transitions workflow APPROVED → RELEASED.
func (h *EmployeeLevelHandler) ReleaseEmployeeLevel(ctx context.Context, req *iamv1.ReleaseEmployeeLevelRequest) (*iamv1.ReleaseEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ReleaseEmployeeLevelResponse{Base: baseResp}, nil
	}
	entity, err := h.workflowHandler.Release(ctx, elapp.WorkflowCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		Notes:           req.GetNotes(),
		UserID:          getUserFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.ReleaseEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.ReleaseEmployeeLevelResponse{
		Base: SuccessResponse("Employee level released successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// BypassReleaseEmployeeLevel transitions workflow directly to RELEASED.
func (h *EmployeeLevelHandler) BypassReleaseEmployeeLevel(ctx context.Context, req *iamv1.BypassReleaseEmployeeLevelRequest) (*iamv1.BypassReleaseEmployeeLevelResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.BypassReleaseEmployeeLevelResponse{Base: baseResp}, nil
	}
	entity, err := h.workflowHandler.BypassRelease(ctx, elapp.WorkflowCommand{
		EmployeeLevelID: req.GetEmployeeLevelId(),
		Notes:           req.GetNotes(),
		UserID:          getUserFromCtx(ctx),
	})
	if err != nil {
		return &iamv1.BypassReleaseEmployeeLevelResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.BypassReleaseEmployeeLevelResponse{
		Base: SuccessResponse("Employee level bypass released successfully"),
		Data: toEmployeeLevelProto(entity),
	}, nil
}

// activeFilterToBool converts proto ActiveFilter to *bool (nil = no filter).
func activeFilterToBool(f iamv1.ActiveFilter) *bool {
	switch f {
	case iamv1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		v := true
		return &v
	case iamv1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		v := false
		return &v
	case iamv1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		return nil
	default:
		return nil
	}
}

// toEmployeeLevelProto converts a domain EmployeeLevel to the proto message.
func toEmployeeLevelProto(e *employeelevel.EmployeeLevel) *iamv1.EmployeeLevel {
	return &iamv1.EmployeeLevel{
		EmployeeLevelId: e.ID().String(),
		Code:            e.Code().String(),
		Name:            e.Name(),
		Grade:           e.Grade(),
		Type:            iamv1.EmployeeLevelType(e.Type()),
		Sequence:        e.Sequence(),
		Workflow:        iamv1.EmployeeLevelWorkflow(e.Workflow()),
		IsActive:        e.IsActive(),
		Audit:           toAuditProto(e.Audit()),
	}
}
