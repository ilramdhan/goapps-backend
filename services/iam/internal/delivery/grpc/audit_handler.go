// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// AuditHandler implements the AuditService gRPC service.
type AuditHandler struct {
	iamv1.UnimplementedAuditServiceServer
	auditRepo        audit.Repository
	validationHelper *ValidationHelper
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(auditRepo audit.Repository, validationHelper *ValidationHelper) *AuditHandler {
	return &AuditHandler{
		auditRepo:        auditRepo,
		validationHelper: validationHelper,
	}
}

// GetAuditLog retrieves a specific audit log entry.
func (h *AuditHandler) GetAuditLog(ctx context.Context, req *iamv1.GetAuditLogRequest) (*iamv1.GetAuditLogResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetAuditLogResponse{Base: baseResp}, nil
	}

	logID, err := uuid.Parse(req.LogId)
	if err != nil {
		return &iamv1.GetAuditLogResponse{Base: ErrorResponse("400", "invalid log ID")}, nil //nolint:nilerr // error returned in response body
	}

	log, err := h.auditRepo.GetByID(ctx, logID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &iamv1.GetAuditLogResponse{Base: NotFoundResponse("audit log not found")}, nil //nolint:nilerr // error returned in response body
		}
		return &iamv1.GetAuditLogResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.GetAuditLogResponse{
		Base: SuccessResponse("Audit log retrieved successfully"),
		Data: h.toAuditLogProto(log),
	}, nil
}

// ListAuditLogs lists audit logs with filtering and pagination.
func (h *AuditHandler) ListAuditLogs(ctx context.Context, req *iamv1.ListAuditLogsRequest) (*iamv1.ListAuditLogsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListAuditLogsResponse{Base: baseResp}, nil
	}

	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 {
		pageSize = 10
	}

	var userID *uuid.UUID
	if req.GetUserId() != "" {
		id, err := uuid.Parse(req.GetUserId())
		if err != nil {
			return &iamv1.ListAuditLogsResponse{Base: ErrorResponse("400", "invalid user ID")}, nil //nolint:nilerr // error returned in response body
		}
		userID = &id
	}

	params := audit.ListParams{
		Page:        page,
		PageSize:    pageSize,
		UserID:      userID,
		TableName:   req.GetTableName(),
		ServiceName: req.GetServiceName(),
		Search:      req.GetSearch(),
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	}

	// Convert event type if specified.
	if req.GetEventType() != iamv1.EventType_EVENT_TYPE_UNSPECIFIED {
		params.EventType = eventTypeToString(req.GetEventType())
	}

	logs, total, err := h.auditRepo.List(ctx, params)
	if err != nil {
		return &iamv1.ListAuditLogsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	protoLogs := make([]*iamv1.AuditLog, len(logs))
	for i, l := range logs {
		protoLogs[i] = h.toAuditLogProto(l)
	}

	totalPages := safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))

	return &iamv1.ListAuditLogsResponse{
		Base: SuccessResponse("Audit logs listed successfully"),
		Data: protoLogs,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: safeconv.IntToInt32(page),
			PageSize:    safeconv.IntToInt32(pageSize),
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// GetAuditSummary retrieves audit statistics.
func (h *AuditHandler) GetAuditSummary(ctx context.Context, req *iamv1.GetAuditSummaryRequest) (*iamv1.GetAuditSummaryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetAuditSummaryResponse{Base: baseResp}, nil
	}

	summary, err := h.auditRepo.GetSummary(ctx, req.GetTimeRange(), req.GetServiceName())
	if err != nil {
		return &iamv1.GetAuditSummaryResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	// Convert top users.
	topUsers := make([]*iamv1.UserActivityInfo, len(summary.TopUsers))
	for i, u := range summary.TopUsers {
		topUsers[i] = &iamv1.UserActivityInfo{
			UserId:     u.UserID.String(),
			Username:   u.Username,
			FullName:   u.FullName,
			EventCount: u.EventCount,
		}
	}

	// Convert events by hour.
	eventsByHour := make([]*iamv1.HourlyCount, len(summary.EventsByHour))
	for i, hc := range summary.EventsByHour {
		eventsByHour[i] = &iamv1.HourlyCount{
			Hour:  int32(hc.Hour), //nolint:gosec // hour value 0-23, safe for int32
			Count: hc.Count,
		}
	}

	return &iamv1.GetAuditSummaryResponse{
		Base: SuccessResponse("Audit summary retrieved successfully"),
		Data: &iamv1.AuditSummary{
			TotalEvents:      summary.TotalEvents,
			LoginCount:       summary.LoginCount,
			LoginFailedCount: summary.LoginFailedCount,
			LogoutCount:      summary.LogoutCount,
			CreateCount:      summary.CreateCount,
			UpdateCount:      summary.UpdateCount,
			DeleteCount:      summary.DeleteCount,
			ExportCount:      summary.ExportCount,
			ImportCount:      summary.ImportCount,
			TopUsers:         topUsers,
			EventsByHour:     eventsByHour,
		},
	}, nil
}

// ExportAuditLogs exports audit logs in specified format.
func (h *AuditHandler) ExportAuditLogs(_ context.Context, _ *iamv1.ExportAuditLogsRequest) (*iamv1.ExportAuditLogsResponse, error) {
	return &iamv1.ExportAuditLogsResponse{
		Base: ErrorResponse("501", "not implemented"),
	}, nil
}

// Helper methods

func eventTypeToString(et iamv1.EventType) audit.EventType {
	switch et {
	case iamv1.EventType_EVENT_TYPE_LOGIN:
		return audit.EventTypeLogin
	case iamv1.EventType_EVENT_TYPE_LOGOUT:
		return audit.EventTypeLogout
	case iamv1.EventType_EVENT_TYPE_LOGIN_FAILED:
		return audit.EventTypeLoginFailed
	case iamv1.EventType_EVENT_TYPE_CREATE:
		return audit.EventTypeCreate
	case iamv1.EventType_EVENT_TYPE_UPDATE:
		return audit.EventTypeUpdate
	case iamv1.EventType_EVENT_TYPE_DELETE:
		return audit.EventTypeDelete
	case iamv1.EventType_EVENT_TYPE_EXPORT:
		return audit.EventTypeExport
	case iamv1.EventType_EVENT_TYPE_IMPORT:
		return audit.EventTypeImport
	default:
		return ""
	}
}

func (h *AuditHandler) toAuditLogProto(l *audit.Log) *iamv1.AuditLog {
	proto := &iamv1.AuditLog{
		LogId:       l.ID().String(),
		EventType:   eventTypeToProto(l.EventType()),
		Username:    l.Username(),
		FullName:    l.FullName(),
		IpAddress:   l.IPAddress(),
		UserAgent:   l.UserAgent(),
		ServiceName: l.ServiceName(),
		PerformedAt: l.PerformedAt().Format("2006-01-02T15:04:05Z07:00"),
	}

	if l.RecordID() != nil {
		proto.RecordId = ptrString(l.RecordID().String())
	}
	if l.UserID() != nil {
		proto.UserId = l.UserID().String()
	}
	if l.TableName() != "" {
		proto.TableName = ptrString(l.TableName())
	}

	if len(l.OldData()) > 0 {
		proto.OldData = ptrString(string(l.OldData()))
	}
	if len(l.NewData()) > 0 {
		proto.NewData = ptrString(string(l.NewData()))
	}
	if len(l.Changes()) > 0 {
		proto.Changes = ptrString(string(l.Changes()))
	}

	return proto
}

func ptrString(s string) *string {
	return &s
}

func eventTypeToProto(et audit.EventType) iamv1.EventType {
	switch et {
	case audit.EventTypeLogin:
		return iamv1.EventType_EVENT_TYPE_LOGIN
	case audit.EventTypeLogout:
		return iamv1.EventType_EVENT_TYPE_LOGOUT
	case audit.EventTypeLoginFailed:
		return iamv1.EventType_EVENT_TYPE_LOGIN_FAILED
	case audit.EventTypeCreate:
		return iamv1.EventType_EVENT_TYPE_CREATE
	case audit.EventTypeUpdate:
		return iamv1.EventType_EVENT_TYPE_UPDATE
	case audit.EventTypeDelete:
		return iamv1.EventType_EVENT_TYPE_DELETE
	case audit.EventTypeExport:
		return iamv1.EventType_EVENT_TYPE_EXPORT
	case audit.EventTypeImport:
		return iamv1.EventType_EVENT_TYPE_IMPORT
	default:
		return iamv1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}
