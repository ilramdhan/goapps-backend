// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// SessionHandler implements the SessionService gRPC service.
type SessionHandler struct {
	iamv1.UnimplementedSessionServiceServer
	sessionRepo      session.Repository
	validationHelper *ValidationHelper
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(sessionRepo session.Repository, validationHelper *ValidationHelper) *SessionHandler {
	return &SessionHandler{
		sessionRepo:      sessionRepo,
		validationHelper: validationHelper,
	}
}

// GetCurrentSession retrieves the current user's session.
func (h *SessionHandler) GetCurrentSession(ctx context.Context, req *iamv1.GetCurrentSessionRequest) (*iamv1.GetCurrentSessionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCurrentSessionResponse{Base: baseResp}, nil
	}

	// Extract session ID from context (set by auth interceptor)
	sessionID, err := getSessionIDFromContext(ctx)
	if err != nil {
		return &iamv1.GetCurrentSessionResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	s, err := h.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &iamv1.GetCurrentSessionResponse{Base: NotFoundResponse("session not found")}, nil //nolint:nilerr // error returned in response body
		}
		return &iamv1.GetCurrentSessionResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.GetCurrentSessionResponse{
		Base: SuccessResponse("Session retrieved successfully"),
		Data: sessionToProto(s),
	}, nil
}

// RevokeSession revokes a specific session.
func (h *SessionHandler) RevokeSession(ctx context.Context, req *iamv1.RevokeSessionRequest) (*iamv1.RevokeSessionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RevokeSessionResponse{Base: baseResp}, nil
	}

	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return &iamv1.RevokeSessionResponse{Base: ErrorResponse("400", "invalid session ID")}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.sessionRepo.Revoke(ctx, sessionID); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &iamv1.RevokeSessionResponse{Base: NotFoundResponse("session not found")}, nil //nolint:nilerr // error returned in response body
		}
		return &iamv1.RevokeSessionResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.RevokeSessionResponse{
		Base: SuccessResponse("Session revoked successfully"),
	}, nil
}

// ListActiveSessions lists active sessions.
func (h *SessionHandler) ListActiveSessions(ctx context.Context, req *iamv1.ListActiveSessionsRequest) (*iamv1.ListActiveSessionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListActiveSessionsResponse{Base: baseResp}, nil
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
	if req.UserId != nil && *req.UserId != "" {
		id, err := uuid.Parse(*req.UserId)
		if err != nil {
			return &iamv1.ListActiveSessionsResponse{Base: ErrorResponse("400", "invalid user ID")}, nil //nolint:nilerr // error returned in response body
		}
		userID = &id
	}

	params := session.ListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      req.GetSearch(),
		ServiceName: req.GetServiceName(),
		UserID:      userID,
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	}

	sessions, total, err := h.sessionRepo.ListActive(ctx, params)
	if err != nil {
		return &iamv1.ListActiveSessionsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	protoSessions := make([]*iamv1.Session, len(sessions))
	for i, s := range sessions {
		protoSessions[i] = sessionInfoToProto(s)
	}

	totalPages := safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))

	return &iamv1.ListActiveSessionsResponse{
		Base: SuccessResponse("Sessions retrieved successfully"),
		Data: protoSessions,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: safeconv.IntToInt32(page),
			PageSize:    safeconv.IntToInt32(pageSize),
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// Helper functions

func sessionToProto(s *session.Session) *iamv1.Session {
	proto := &iamv1.Session{
		SessionId:   s.ID().String(),
		UserId:      s.UserID().String(),
		DeviceInfo:  s.DeviceInfo(),
		IpAddress:   s.IPAddress(),
		ServiceName: s.ServiceName(),
		CreatedAt:   s.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:   s.ExpiresAt().Format("2006-01-02T15:04:05Z07:00"),
	}

	if s.RevokedAt() != nil {
		revokedAt := s.RevokedAt().Format("2006-01-02T15:04:05Z07:00")
		proto.RevokedAt = &revokedAt
	}

	return proto
}

func sessionInfoToProto(s *session.Info) *iamv1.Session {
	proto := &iamv1.Session{
		SessionId:   s.SessionID.String(),
		UserId:      s.UserID.String(),
		Username:    s.Username,
		FullName:    s.FullName,
		DeviceInfo:  s.DeviceInfo,
		IpAddress:   s.IPAddress,
		ServiceName: s.ServiceName,
		CreatedAt:   s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:   s.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if s.RevokedAt != nil {
		revokedAt := s.RevokedAt.Format("2006-01-02T15:04:05Z07:00")
		proto.RevokedAt = &revokedAt
	}

	return proto
}

func getSessionIDFromContext(ctx context.Context) (uuid.UUID, error) {
	sessionIDVal := ctx.Value("session_id")
	if sessionIDVal == nil {
		return uuid.Nil, shared.ErrUnauthorized
	}

	switch v := sessionIDVal.(type) {
	case string:
		return uuid.Parse(v)
	case uuid.UUID:
		return v, nil
	default:
		return uuid.Nil, shared.ErrUnauthorized
	}
}
