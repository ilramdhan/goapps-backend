// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	formulaapp "github.com/mutugading/goapps-backend/services/finance/internal/application/formula"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// FormulaHandler implements the FormulaServiceServer interface.
type FormulaHandler struct {
	financev1.UnimplementedFormulaServiceServer
	createHandler    *formulaapp.CreateHandler
	getHandler       *formulaapp.GetHandler
	updateHandler    *formulaapp.UpdateHandler
	deleteHandler    *formulaapp.DeleteHandler
	listHandler      *formulaapp.ListHandler
	exportHandler    *formulaapp.ExportHandler
	importHandler    *formulaapp.ImportHandler
	templateHandler  *formulaapp.TemplateHandler
	validationHelper *ValidationHelper
}

// NewFormulaHandler creates a new Formula gRPC handler.
func NewFormulaHandler(repo formula.Repository) (*FormulaHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &FormulaHandler{
		createHandler:    formulaapp.NewCreateHandler(repo),
		getHandler:       formulaapp.NewGetHandler(repo),
		updateHandler:    formulaapp.NewUpdateHandler(repo),
		deleteHandler:    formulaapp.NewDeleteHandler(repo),
		listHandler:      formulaapp.NewListHandler(repo),
		exportHandler:    formulaapp.NewExportHandler(repo),
		importHandler:    formulaapp.NewImportHandler(repo),
		templateHandler:  formulaapp.NewTemplateHandler(),
		validationHelper: validationHelper,
	}, nil
}

// CreateFormula creates a new Formula.
func (h *FormulaHandler) CreateFormula(ctx context.Context, req *financev1.CreateFormulaRequest) (*financev1.CreateFormulaResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordFormulaOperation("create", false)
		return &financev1.CreateFormulaResponse{Base: baseResp}, nil
	}

	cmd := formulaapp.CreateCommand{
		FormulaCode:   req.FormulaCode,
		FormulaName:   req.FormulaName,
		FormulaType:   protoFormulaTypeToString(req.FormulaType),
		Expression:    req.Expression,
		ResultParamID: req.ResultParamId,
		InputParamIDs: req.InputParamIds,
		Description:   req.Description,
		CreatedBy:     getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordFormulaOperation("create", false)
		return &financev1.CreateFormulaResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("create", true)

	return &financev1.CreateFormulaResponse{
		Base: formulaSuccessResponse("Formula created successfully"),
		Data: formulaEntityToProto(entity),
	}, nil
}

// GetFormula retrieves a Formula by ID.
func (h *FormulaHandler) GetFormula(ctx context.Context, req *financev1.GetFormulaRequest) (*financev1.GetFormulaResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordFormulaOperation("get", false)
		return &financev1.GetFormulaResponse{Base: baseResp}, nil
	}

	query := formulaapp.GetQuery{FormulaID: req.FormulaId}
	entity, err := h.getHandler.Handle(ctx, query)
	if err != nil {
		RecordFormulaOperation("get", false)
		return &financev1.GetFormulaResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("get", true)

	return &financev1.GetFormulaResponse{
		Base: formulaSuccessResponse("Formula retrieved successfully"),
		Data: formulaEntityToProto(entity),
	}, nil
}

// UpdateFormula updates an existing Formula.
func (h *FormulaHandler) UpdateFormula(ctx context.Context, req *financev1.UpdateFormulaRequest) (*financev1.UpdateFormulaResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordFormulaOperation("update", false)
		return &financev1.UpdateFormulaResponse{Base: baseResp}, nil
	}

	cmd := formulaapp.UpdateCommand{
		FormulaID: req.FormulaId,
		UpdatedBy: getUserFromContext(ctx),
	}

	if req.FormulaName != nil {
		cmd.FormulaName = req.FormulaName
	}
	if req.FormulaType != nil && *req.FormulaType != financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED {
		ft := protoFormulaTypeToString(*req.FormulaType)
		cmd.FormulaType = &ft
	}
	if req.Expression != nil {
		cmd.Expression = req.Expression
	}
	if req.ResultParamId != nil {
		cmd.ResultParamID = req.ResultParamId
	}
	if req.InputParamIds != nil {
		cmd.InputParamIDs = req.InputParamIds
	}
	if req.Description != nil {
		cmd.Description = req.Description
	}
	if req.IsActive != nil {
		cmd.IsActive = req.IsActive
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordFormulaOperation("update", false)
		return &financev1.UpdateFormulaResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("update", true)

	return &financev1.UpdateFormulaResponse{
		Base: formulaSuccessResponse("Formula updated successfully"),
		Data: formulaEntityToProto(entity),
	}, nil
}

// DeleteFormula soft deletes a Formula.
func (h *FormulaHandler) DeleteFormula(ctx context.Context, req *financev1.DeleteFormulaRequest) (*financev1.DeleteFormulaResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordFormulaOperation("delete", false)
		return &financev1.DeleteFormulaResponse{Base: baseResp}, nil
	}

	cmd := formulaapp.DeleteCommand{
		FormulaID: req.FormulaId,
		DeletedBy: getUserFromContext(ctx),
	}

	if err := h.deleteHandler.Handle(ctx, cmd); err != nil {
		RecordFormulaOperation("delete", false)
		return &financev1.DeleteFormulaResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("delete", true)

	return &financev1.DeleteFormulaResponse{
		Base: formulaSuccessResponse("Formula deleted successfully"),
	}, nil
}

// ListFormulas lists Formulas with search, filter, and pagination.
func (h *FormulaHandler) ListFormulas(ctx context.Context, req *financev1.ListFormulasRequest) (*financev1.ListFormulasResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := formulaapp.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.FormulaType != financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED {
		ft := protoFormulaTypeToString(req.FormulaType)
		query.FormulaType = &ft
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Show all
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordFormulaOperation("list", false)
		return &financev1.ListFormulasResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("list", true)

	items := make([]*financev1.Formula, len(result.Formulas))
	for i, entity := range result.Formulas {
		items[i] = formulaEntityToProto(entity)
	}

	return &financev1.ListFormulasResponse{
		Base: formulaSuccessResponse("Formulas retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportFormulas exports Formulas to Excel file.
func (h *FormulaHandler) ExportFormulas(ctx context.Context, req *financev1.ExportFormulasRequest) (*financev1.ExportFormulasResponse, error) {
	query := formulaapp.ExportQuery{}

	if req.FormulaType != financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED {
		ft := protoFormulaTypeToString(req.FormulaType)
		query.FormulaType = &ft
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
		RecordFormulaOperation("export", false)
		return &financev1.ExportFormulasResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("export", true)

	return &financev1.ExportFormulasResponse{
		Base:        formulaSuccessResponse("Formulas exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportFormulas imports Formulas from Excel file.
func (h *FormulaHandler) ImportFormulas(ctx context.Context, req *financev1.ImportFormulasRequest) (*financev1.ImportFormulasResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordFormulaOperation("import", false)
		return &financev1.ImportFormulasResponse{Base: baseResp}, nil
	}

	cmd := formulaapp.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordFormulaOperation("import", false)
		return &financev1.ImportFormulasResponse{Base: formulaDomainErrorToBaseResponse(err)}, nil
	}

	RecordFormulaOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportFormulasResponse{
		Base:         formulaSuccessResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadFormulaTemplate downloads the Excel import template.
func (h *FormulaHandler) DownloadFormulaTemplate(_ context.Context, _ *financev1.DownloadFormulaTemplateRequest) (*financev1.DownloadFormulaTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadFormulaTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadFormulaTemplateResponse{
		Base:        formulaSuccessResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// =============================================================================
// Helper functions
// =============================================================================

func formulaSuccessResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  true,
		StatusCode: "200",
		Message:    message,
	}
}

func formulaDomainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return formulaSuccessResponse("")
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "not found"):
		return NotFoundResponse(errMsg)
	case strings.Contains(errMsg, "already exists"),
		strings.Contains(errMsg, "already used"):
		return ConflictResponse(errMsg)
	case strings.Contains(errMsg, "invalid"),
		strings.Contains(errMsg, "empty"),
		strings.Contains(errMsg, "cannot"),
		strings.Contains(errMsg, "must"),
		strings.Contains(errMsg, "too long"),
		strings.Contains(errMsg, "duplicate"),
		strings.Contains(errMsg, "circular"):
		return ErrorResponse("400", errMsg)
	default:
		return InternalErrorResponse(errMsg)
	}
}

func protoFormulaTypeToString(ft financev1.FormulaType) string {
	switch ft {
	case financev1.FormulaType_FORMULA_TYPE_CALCULATION:
		return "CALCULATION"
	case financev1.FormulaType_FORMULA_TYPE_SQL_QUERY:
		return "SQL_QUERY"
	case financev1.FormulaType_FORMULA_TYPE_CONSTANT:
		return "CONSTANT"
	default:
		return ""
	}
}

func stringToProtoFormulaType(ft string) financev1.FormulaType {
	switch ft {
	case "CALCULATION":
		return financev1.FormulaType_FORMULA_TYPE_CALCULATION
	case "SQL_QUERY":
		return financev1.FormulaType_FORMULA_TYPE_SQL_QUERY
	case "CONSTANT":
		return financev1.FormulaType_FORMULA_TYPE_CONSTANT
	default:
		return financev1.FormulaType_FORMULA_TYPE_UNSPECIFIED
	}
}

func formulaEntityToProto(entity *formula.Formula) *financev1.Formula {
	proto := &financev1.Formula{
		FormulaId:       entity.ID().String(),
		FormulaCode:     entity.Code().String(),
		FormulaName:     entity.Name(),
		FormulaType:     stringToProtoFormulaType(entity.FormulaType().String()),
		Expression:      entity.Expression(),
		ResultParamId:   entity.ResultParamID().String(),
		ResultParamCode: entity.ResultParamCode(),
		ResultParamName: entity.ResultParamName(),
		Description:     entity.Description(),
		Version:         safeIntToInt32(entity.Version()),
		IsActive:        entity.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: entity.CreatedAt().Format(time.RFC3339),
			CreatedBy: entity.CreatedBy(),
		},
	}

	if entity.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = entity.UpdatedAt().Format(time.RFC3339)
	}
	if entity.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *entity.UpdatedBy()
	}

	for _, fp := range entity.InputParams() {
		proto.InputParams = append(proto.InputParams, &financev1.FormulaParam{
			FormulaParamId: fp.ID().String(),
			ParamId:        fp.ParamID().String(),
			ParamCode:      fp.ParamCode(),
			ParamName:      fp.ParamName(),
			SortOrder:      safeIntToInt32(fp.SortOrder()),
		})
	}

	return proto
}

// RecordFormulaOperation records a Formula operation metric.
func RecordFormulaOperation(operation string, success bool) {
	formulaOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// safeIntToInt32 converts int to int32 with bounds clamping to prevent overflow.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) //nolint:gosec // bounds checked above
}

// Ensure unused import warning doesn't appear.
func init() {
	_ = log.Debug
}
