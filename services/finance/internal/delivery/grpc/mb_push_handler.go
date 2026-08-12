// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"fmt"
	"strings"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbpush "github.com/mutugading/goapps-backend/services/finance/internal/application/mbpush"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbpushlog"
)

// MBPushHandler implements financev1.MbPushServiceServer.
type MBPushHandler struct {
	financev1.UnimplementedMbPushServiceServer
	previewHandler *appmbpush.PreviewHandler
	executeHandler *appmbpush.ExecuteHandler
	listLogHandler *appmbpush.ListLogsHandler
	validation     *ValidationHelper
}

// NewMBPushHandler constructs an MBPushHandler.
func NewMBPushHandler(previewHandler *appmbpush.PreviewHandler, executeHandler *appmbpush.ExecuteHandler, pushLogRepo mbpushlog.Repository) (*MBPushHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBPushHandler{
		previewHandler: previewHandler,
		executeHandler: executeHandler,
		listLogHandler: appmbpush.NewListLogsHandler(pushLogRepo),
		validation:     v,
	}, nil
}

// PreviewPushToHead previews which VALIDATED MB Heads are ready for a push-to-head execution.
func (h *MBPushHandler) PreviewPushToHead(ctx context.Context, req *financev1.PreviewPushToHeadRequest) (*financev1.PreviewPushToHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBPushOperation("preview", false)
		return &financev1.PreviewPushToHeadResponse{Base: baseResp}, nil
	}

	result, err := h.previewHandler.Preview(ctx, req.Period)
	if err != nil {
		RecordMBPushOperation("preview", false)
		return &financev1.PreviewPushToHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBPushOperation("preview", true)

	pushableItems := make([]*financev1.PushableMbHead, len(result.Pushable))
	for i, p := range result.Pushable {
		pushableItems[i] = pushableMBHeadToProto(p)
	}
	skippedItems := make([]*financev1.SkippedMbHead, len(result.Skipped))
	for i, s := range result.Skipped {
		skippedItems[i] = skippedMBHeadToProto(s)
	}

	return &financev1.PreviewPushToHeadResponse{
		Base:             successResponse("MB push preview retrieved successfully"),
		Pushable:         pushableItems,
		Skipped:          skippedItems,
		NeedsRepushCount: result.NeedsRepushCount,
	}, nil
}

// ExecutePushToHead executes a push-to-head batch for the requested MB Heads.
func (h *MBPushHandler) ExecutePushToHead(ctx context.Context, req *financev1.ExecutePushToHeadRequest) (*financev1.ExecutePushToHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBPushOperation("execute", false)
		return &financev1.ExecutePushToHeadResponse{Base: baseResp}, nil
	}

	result, err := h.executeHandler.Execute(ctx, req.Period, req.MbHeadIds, getUserFromContext(ctx))
	if err != nil {
		RecordMBPushOperation("execute", false)
		return &financev1.ExecutePushToHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBPushOperation("execute", true)

	return &financev1.ExecutePushToHeadResponse{
		Base: successResponse(executePushMessage(result)),
		Data: executeResultToProto(result),
	}, nil
}

// ListMbPushLogs lists paginated push-execution audit log rows.
func (h *MBPushHandler) ListMbPushLogs(ctx context.Context, req *financev1.ListMbPushLogsRequest) (*financev1.ListMbPushLogsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBPushOperation("list_logs", false)
		return &financev1.ListMbPushLogsResponse{Base: baseResp}, nil
	}

	result, err := h.listLogHandler.Handle(ctx, appmbpush.ListLogsQuery{
		Page:     req.Page,
		PageSize: req.PageSize,
		Period:   req.Period,
	})
	if err != nil {
		RecordMBPushOperation("list_logs", false)
		return &financev1.ListMbPushLogsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBPushOperation("list_logs", true)

	items := make([]*financev1.MbPushLog, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbPushLogEntityToProto(e)
	}

	return &financev1.ListMbPushLogsResponse{
		Base: successResponse("MB push logs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// executePushMessage summarizes a push execution outcome, folding any per-MB partial failures
// into the base message since ExecutePushToHeadResponse has no dedicated error-list field.
func executePushMessage(result *appmbpush.ExecuteResult) string {
	if len(result.Errors) == 0 {
		return "MB push executed successfully"
	}
	failures := make([]string, len(result.Errors))
	for i, e := range result.Errors {
		failures[i] = fmt.Sprintf("%s: %s", e.MBHID, e.Error)
	}
	return fmt.Sprintf("MB push completed with %d succeeded, %d failed (%s)",
		result.MBCount, len(result.Errors), strings.Join(failures, "; "))
}

// pushableMBHeadToProto converts an application-layer PushableMBHead to its proto representation.
func pushableMBHeadToProto(p appmbpush.PushableMBHead) *financev1.PushableMbHead {
	return &financev1.PushableMbHead{
		MbhId:       p.MBHID,
		Code:        p.Code,
		Name:        p.Name,
		HasActual:   p.HasActual,
		HasSelling:  p.HasSelling,
		HasForecast: p.HasForecast,
		NeedsRepush: p.NeedsRepush,
	}
}

// skippedMBHeadToProto converts an application-layer SkippedMBHead to its proto representation.
func skippedMBHeadToProto(s appmbpush.SkippedMBHead) *financev1.SkippedMbHead {
	return &financev1.SkippedMbHead{
		MbhId:  s.MBHID,
		Code:   s.Code,
		Name:   s.Name,
		Reason: s.Reason,
	}
}

// executeResultToProto converts an application-layer ExecuteResult to its proto MbPushLog summary.
func executeResultToProto(result *appmbpush.ExecuteResult) *financev1.MbPushLog {
	return &financev1.MbPushLog{
		MbplId:    result.PushLogID,
		Period:    result.Period,
		MbCount:   result.MBCount,
		RowCount:  result.RowCount,
		CostTypes: "ACTUAL,SELLING,FORECAST",
	}
}

// mbPushLogEntityToProto converts a domain mbpushlog Entity to its proto representation.
func mbPushLogEntityToProto(e *mbpushlog.Entity) *financev1.MbPushLog {
	return &financev1.MbPushLog{
		MbplId:         e.ID(),
		Period:         e.Period(),
		PushedAt:       e.PushedAt(),
		PushedBy:       e.PushedBy(),
		MbCount:        e.MBCount(),
		RowCount:       e.RowCount(),
		CostTypes:      e.CostTypes(),
		PreviousPeriod: e.PreviousPeriod(),
		Notes:          e.Notes(),
	}
}
