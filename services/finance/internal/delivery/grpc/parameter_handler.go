// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	paramapp "github.com/mutugading/goapps-backend/services/finance/internal/application/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// ParameterHandler implements the ParameterServiceServer interface.
type ParameterHandler struct {
	financev1.UnimplementedParameterServiceServer
	createHandler    *paramapp.CreateHandler
	getHandler       *paramapp.GetHandler
	updateHandler    *paramapp.UpdateHandler
	deleteHandler    *paramapp.DeleteHandler
	listHandler      *paramapp.ListHandler
	exportHandler    *paramapp.ExportHandler
	importHandler    *paramapp.ImportHandler
	templateHandler  *paramapp.TemplateHandler
	validationHelper *ValidationHelper
}

// NewParameterHandler creates a new Parameter gRPC handler.
func NewParameterHandler(repo parameter.Repository) (*ParameterHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &ParameterHandler{
		createHandler:    paramapp.NewCreateHandler(repo),
		getHandler:       paramapp.NewGetHandler(repo),
		updateHandler:    paramapp.NewUpdateHandler(repo),
		deleteHandler:    paramapp.NewDeleteHandler(repo),
		listHandler:      paramapp.NewListHandler(repo),
		exportHandler:    paramapp.NewExportHandler(repo),
		importHandler:    paramapp.NewImportHandler(repo),
		templateHandler:  paramapp.NewTemplateHandler(),
		validationHelper: validationHelper,
	}, nil
}

// CreateParameter creates a new Parameter.
func (h *ParameterHandler) CreateParameter(ctx context.Context, req *financev1.CreateParameterRequest) (*financev1.CreateParameterResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordParameterOperation("create", false)
		return &financev1.CreateParameterResponse{Base: baseResp}, nil
	}

	cmd := paramapp.CreateCommand{
		ParamCode:      req.ParamCode,
		ParamName:      req.ParamName,
		ParamShortName: req.ParamShortName,
		DataType:       protoDataTypeToString(req.DataType),
		ParamCategory:  protoParamCategoryToString(req.ParamCategory),
		UOMID:          req.UomId,
		DefaultValue:   req.DefaultValue,
		MinValue:       req.MinValue,
		MaxValue:       req.MaxValue,
		CreatedBy:      getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordParameterOperation("create", false)
		return &financev1.CreateParameterResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("create", true)

	return &financev1.CreateParameterResponse{
		Base: paramSuccessResponse("Parameter created successfully"),
		Data: paramEntityToProto(entity),
	}, nil
}

// GetParameter retrieves a Parameter by ID.
func (h *ParameterHandler) GetParameter(ctx context.Context, req *financev1.GetParameterRequest) (*financev1.GetParameterResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordParameterOperation("get", false)
		return &financev1.GetParameterResponse{Base: baseResp}, nil
	}

	query := paramapp.GetQuery{ParamID: req.ParamId}
	entity, err := h.getHandler.Handle(ctx, query)
	if err != nil {
		RecordParameterOperation("get", false)
		return &financev1.GetParameterResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("get", true)

	return &financev1.GetParameterResponse{
		Base: paramSuccessResponse("Parameter retrieved successfully"),
		Data: paramEntityToProto(entity),
	}, nil
}

// UpdateParameter updates an existing Parameter.
func (h *ParameterHandler) UpdateParameter(ctx context.Context, req *financev1.UpdateParameterRequest) (*financev1.UpdateParameterResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordParameterOperation("update", false)
		return &financev1.UpdateParameterResponse{Base: baseResp}, nil
	}

	cmd := paramapp.UpdateCommand{
		ParamID:   req.ParamId,
		UpdatedBy: getUserFromContext(ctx),
	}

	if req.ParamName != nil {
		cmd.ParamName = req.ParamName
	}
	if req.ParamShortName != nil {
		cmd.ParamShortName = req.ParamShortName
	}
	if req.DataType != nil && *req.DataType != financev1.DataType_DATA_TYPE_UNSPECIFIED {
		dt := protoDataTypeToString(*req.DataType)
		cmd.DataType = &dt
	}
	if req.ParamCategory != nil && *req.ParamCategory != financev1.ParamCategory_PARAM_CATEGORY_UNSPECIFIED {
		cat := protoParamCategoryToString(*req.ParamCategory)
		cmd.ParamCategory = &cat
	}
	if req.UomId != nil {
		cmd.UOMID = req.UomId
	}
	if req.DefaultValue != nil {
		cmd.DefaultValue = req.DefaultValue
	}
	if req.MinValue != nil {
		cmd.MinValue = req.MinValue
	}
	if req.MaxValue != nil {
		cmd.MaxValue = req.MaxValue
	}
	if req.IsActive != nil {
		cmd.IsActive = req.IsActive
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordParameterOperation("update", false)
		return &financev1.UpdateParameterResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("update", true)

	return &financev1.UpdateParameterResponse{
		Base: paramSuccessResponse("Parameter updated successfully"),
		Data: paramEntityToProto(entity),
	}, nil
}

// DeleteParameter soft deletes a Parameter.
func (h *ParameterHandler) DeleteParameter(ctx context.Context, req *financev1.DeleteParameterRequest) (*financev1.DeleteParameterResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordParameterOperation("delete", false)
		return &financev1.DeleteParameterResponse{Base: baseResp}, nil
	}

	cmd := paramapp.DeleteCommand{
		ParamID:   req.ParamId,
		DeletedBy: getUserFromContext(ctx),
	}

	if err := h.deleteHandler.Handle(ctx, cmd); err != nil {
		RecordParameterOperation("delete", false)
		return &financev1.DeleteParameterResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("delete", true)

	return &financev1.DeleteParameterResponse{
		Base: paramSuccessResponse("Parameter deleted successfully"),
	}, nil
}

// ListParameters lists Parameters with search, filter, and pagination.
func (h *ParameterHandler) ListParameters(ctx context.Context, req *financev1.ListParametersRequest) (*financev1.ListParametersResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := paramapp.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.DataType != financev1.DataType_DATA_TYPE_UNSPECIFIED {
		dt := protoDataTypeToString(req.DataType)
		query.DataType = &dt
	}

	if req.ParamCategory != financev1.ParamCategory_PARAM_CATEGORY_UNSPECIFIED {
		cat := protoParamCategoryToString(req.ParamCategory)
		query.ParamCategory = &cat
	}

	// Handle ActiveFilter enum
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Show all - no filter
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordParameterOperation("list", false)
		return &financev1.ListParametersResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("list", true)

	items := make([]*financev1.Parameter, len(result.Parameters))
	for i, entity := range result.Parameters {
		items[i] = paramEntityToProto(entity)
	}

	return &financev1.ListParametersResponse{
		Base: paramSuccessResponse("Parameters retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportParameters exports Parameters to Excel file.
func (h *ParameterHandler) ExportParameters(ctx context.Context, req *financev1.ExportParametersRequest) (*financev1.ExportParametersResponse, error) {
	query := paramapp.ExportQuery{}

	if req.DataType != financev1.DataType_DATA_TYPE_UNSPECIFIED {
		dt := protoDataTypeToString(req.DataType)
		query.DataType = &dt
	}

	if req.ParamCategory != financev1.ParamCategory_PARAM_CATEGORY_UNSPECIFIED {
		cat := protoParamCategoryToString(req.ParamCategory)
		query.ParamCategory = &cat
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Export all
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordParameterOperation("export", false)
		return &financev1.ExportParametersResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("export", true)

	return &financev1.ExportParametersResponse{
		Base:        paramSuccessResponse("Parameters exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportParameters imports Parameters from Excel file.
func (h *ParameterHandler) ImportParameters(ctx context.Context, req *financev1.ImportParametersRequest) (*financev1.ImportParametersResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordParameterOperation("import", false)
		return &financev1.ImportParametersResponse{Base: baseResp}, nil
	}

	cmd := paramapp.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordParameterOperation("import", false)
		return &financev1.ImportParametersResponse{Base: paramDomainErrorToBaseResponse(err)}, nil
	}

	RecordParameterOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportParametersResponse{
		Base:         paramSuccessResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadParameterTemplate downloads the Excel import template.
func (h *ParameterHandler) DownloadParameterTemplate(_ context.Context, _ *financev1.DownloadParameterTemplateRequest) (*financev1.DownloadParameterTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadParameterTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadParameterTemplateResponse{
		Base:        paramSuccessResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func paramSuccessResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  true,
		StatusCode: "200",
		Message:    message,
	}
}

func paramDomainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return paramSuccessResponse("")
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "not found"):
		return NotFoundResponse(errMsg)
	case strings.Contains(errMsg, "already exists"):
		return ConflictResponse(errMsg)
	case strings.Contains(errMsg, "invalid"):
		return ErrorResponse("400", errMsg)
	default:
		return InternalErrorResponse(errMsg)
	}
}

func protoDataTypeToString(dt financev1.DataType) string {
	switch dt {
	case financev1.DataType_DATA_TYPE_NUMBER:
		return "NUMBER"
	case financev1.DataType_DATA_TYPE_TEXT:
		return "TEXT"
	case financev1.DataType_DATA_TYPE_BOOLEAN:
		return "BOOLEAN"
	default:
		return ""
	}
}

func stringToProtoDataType(dt string) financev1.DataType {
	switch dt {
	case "NUMBER":
		return financev1.DataType_DATA_TYPE_NUMBER
	case "TEXT":
		return financev1.DataType_DATA_TYPE_TEXT
	case "BOOLEAN":
		return financev1.DataType_DATA_TYPE_BOOLEAN
	default:
		return financev1.DataType_DATA_TYPE_UNSPECIFIED
	}
}

func protoParamCategoryToString(cat financev1.ParamCategory) string {
	switch cat {
	case financev1.ParamCategory_PARAM_CATEGORY_INPUT:
		return "INPUT"
	case financev1.ParamCategory_PARAM_CATEGORY_RATE:
		return "RATE"
	case financev1.ParamCategory_PARAM_CATEGORY_CALCULATED:
		return "CALCULATED"
	default:
		return ""
	}
}

func stringToProtoParamCategory(cat string) financev1.ParamCategory {
	switch cat {
	case "INPUT":
		return financev1.ParamCategory_PARAM_CATEGORY_INPUT
	case "RATE":
		return financev1.ParamCategory_PARAM_CATEGORY_RATE
	case "CALCULATED":
		return financev1.ParamCategory_PARAM_CATEGORY_CALCULATED
	default:
		return financev1.ParamCategory_PARAM_CATEGORY_UNSPECIFIED
	}
}

func paramEntityToProto(entity *parameter.Parameter) *financev1.Parameter {
	proto := &financev1.Parameter{
		ParamId:        entity.ID().String(),
		ParamCode:      entity.Code().String(),
		ParamName:      entity.Name(),
		ParamShortName: entity.ShortName(),
		DataType:       stringToProtoDataType(entity.DataType().String()),
		ParamCategory:  stringToProtoParamCategory(entity.ParamCategory().String()),
		IsActive:       entity.IsActive(),
		UomCode:        entity.UOMCode(),
		UomName:        entity.UOMName(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: entity.CreatedAt().Format(time.RFC3339),
			CreatedBy: entity.CreatedBy(),
		},
	}

	if entity.UOMID() != nil {
		proto.UomId = entity.UOMID().String()
	}

	if entity.DefaultValue() != nil {
		proto.DefaultValue = *entity.DefaultValue()
	}
	if entity.MinValue() != nil {
		proto.MinValue = *entity.MinValue()
	}
	if entity.MaxValue() != nil {
		proto.MaxValue = *entity.MaxValue()
	}

	if entity.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = entity.UpdatedAt().Format(time.RFC3339)
	}
	if entity.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *entity.UpdatedBy()
	}

	return proto
}

// RecordParameterOperation records a Parameter operation metric.
func RecordParameterOperation(operation string, success bool) {
	parameterOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// Ensure unused import warning doesn't appear.
func init() {
	_ = log.Debug
}
