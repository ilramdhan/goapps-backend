package grpc

import (
	"context"
	"time"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

// CostAuditLogHandler implements financev1.CostAuditLogServiceServer.
type CostAuditLogHandler struct {
	financev1.UnimplementedCostAuditLogServiceServer
	listH      *app.ListHandler
	validation *ValidationHelper
}

// NewCostAuditLogHandler constructs the handler.
func NewCostAuditLogHandler(repo domain.Repository) (*CostAuditLogHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostAuditLogHandler{listH: app.NewListHandler(repo), validation: v}, nil
}

// ListCostAuditLogs returns a filtered paginated list.
func (h *CostAuditLogHandler) ListCostAuditLogs(ctx context.Context, req *financev1.ListCostAuditLogsRequest) (*financev1.ListCostAuditLogsResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.ListCostAuditLogsResponse{Base: b}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	res, err := h.listH.Handle(ctx, app.ListQuery{
		EntityType: req.GetEntityType(), EntityID: req.GetEntityId(),
		UserID: req.GetUserId(), Operation: req.GetOperation(),
		FromDate: req.GetFromDate(), ToDate: req.GetToDate(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostAuditLogsResponse{Base: InternalErrorResponse(err.Error())}, nil
	}
	data := make([]*financev1.CostAuditLog, 0, len(res.Items))
	for _, l := range res.Items {
		data = append(data, &financev1.CostAuditLog{
			LogId: l.LogID, EntityType: l.EntityType, EntityId: l.EntityID,
			Operation: l.Operation, BeforeData: l.BeforeData, AfterData: l.AfterData,
			UserId: l.UserID, PerformedAt: l.PerformedAt.Format(time.RFC3339),
		})
	}
	return &financev1.ListCostAuditLogsResponse{
		Base: successResponse("OK"), Data: data,
		Pagination: paginationResponse(page, pageSize, res.Total),
	}, nil
}
