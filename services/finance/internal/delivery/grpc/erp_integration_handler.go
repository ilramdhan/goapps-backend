package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	periodlockapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	domainperiodlock "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// Per-operation permission codes re-checked by the handler (the interceptor
// only enforces the base code from the permission map).
const (
	permErpValuate = "finance.cost.erpintegration.valuate"
	permErpApprove = "finance.cost.erpintegration.approve"
	permErpRestore = "finance.cost.erpintegration.restore"
	permErpPush    = "finance.cost.erpintegration.push"
	permErpLock    = "finance.cost.erpintegration.lock"
)

// Narrow seams over the application handlers so the delivery layer is
// unit-testable with fakes.
type (
	erpBatchCreator interface {
		Handle(ctx context.Context, cmd erpapp.CreateBatchCommand) (*domain.Batch, error)
	}
	erpStepTrigger interface {
		Handle(ctx context.Context, cmd erpapp.StepTriggerCommand) (*job.Execution, error)
		TriggerPush(ctx context.Context, cmd erpapp.PushTriggerCommand) (*job.Execution, error)
		TriggerLockBatch(ctx context.Context, cmd erpapp.LockBatchTriggerCommand) (*job.Execution, error)
		TriggerAdjExecute(ctx context.Context, cmd erpapp.AdjTriggerCommand) (*job.Execution, error)
		TriggerSubtype(ctx context.Context, subtype, period string, params any, actor string) (*job.Execution, error)
	}
	erpBatchReader interface {
		GetByID(ctx context.Context, id int64) (*domain.Batch, error)
		ListByPeriod(ctx context.Context, period string) ([]*domain.Batch, error)
	}
	erpAcker interface {
		Handle(ctx context.Context, cmd erpapp.AckWarningsCommand) (erpapp.AckWarningsResult, error)
	}
	erpPushPreviewer interface {
		Handle(ctx context.Context, q erpapp.PushPreviewQuery) (erpapp.PushPreview, error)
	}
	erpValuationPreviewer interface {
		Handle(ctx context.Context, cmd erpapp.ValuationPreviewCommand) (erpapp.ValuationPreviewResult, error)
	}
	erpPeriodLocker interface {
		Handle(ctx context.Context, cmd periodlockapp.LockCommand) (*domainperiodlock.PeriodLock, error)
	}
	erpPeriodUnlocker interface {
		Handle(ctx context.Context, cmd periodlockapp.UnlockCommand) (periodlockapp.UnlockResult, error)
	}
)

// ErpIntegrationDeps groups the collaborators of ErpIntegrationHandler.
type ErpIntegrationDeps struct {
	Create     erpBatchCreator
	Steps      erpStepTrigger
	Batches    erpBatchReader
	Ack        erpAcker
	PushPrev   erpPushPreviewer
	ValPreview erpValuationPreviewer
	Lock       erpPeriodLocker
	Unlock     erpPeriodUnlocker

	// T3b read/trigger collaborators.
	Previews   erpPreviewGetter
	Schedule   erpScheduler
	PeriodLock erpPeriodLockGetter
	Coverage   erpCoverageLister
	StdCost    erpStdCostLister
	Recon      erpReconLister
	OracleCall erpOracleCallLister
	Sanity     erpSanityReporter
	LinkReport erpLinkReporter
	Backtest   erpBacktestGetter
	Config     ErpConfigView
	Abandon    erpAbandoner

	// T3d collaborators.
	LinkErp       erpProductLinker
	CreateProduct erpProductFromDemand
	MasterSync    erpMasterSyncTrigger
}

// ErpIntegrationHandler implements financev1.ErpIntegrationServiceServer (T3a
// core subset; the remaining RPCs stay on the embedded Unimplemented server
// until T3b).
type ErpIntegrationHandler struct {
	financev1.UnimplementedErpIntegrationServiceServer
	d ErpIntegrationDeps
}

// NewErpIntegrationGRPCHandler builds the handler.
func NewErpIntegrationGRPCHandler(d ErpIntegrationDeps) *ErpIntegrationHandler {
	return &ErpIntegrationHandler{d: d}
}

// erpErrBase maps a domain/application error to a BaseResponse. Gate and
// permission errors keep their message so the operator sees which gate fired.
func erpErrBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrBatchNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, erpapp.ErrAckPermissionDenied), errors.Is(err, erpapp.ErrAdjPermissionDenied),
		errors.Is(err, erpapp.ErrValuatePermissionDenied), errors.Is(err, erpapp.ErrPushPermissionDenied):
		return ErrorResponse("403", err.Error())
	case errors.Is(err, domain.ErrInvalidBatchPeriod), errors.Is(err, domain.ErrInvalidBatchMode),
		errors.Is(err, domain.ErrInvalidBatchStatus), errors.Is(err, domain.ErrActorRequired), errors.Is(err, domain.ErrInvalidSchedule),
		errors.Is(err, erpapp.ErrInvalidPeriod), errors.Is(err, erpapp.ErrAckHashMismatch),
		errors.Is(err, domain.ErrConfirmMismatch), errors.Is(err, domain.ErrPreviewRequired):
		return BadRequestResponse(err.Error())
	case errors.Is(err, domainperiodlock.ErrPeriodLocked), errors.Is(err, erpapp.ErrStepNotAllowed),
		errors.Is(err, domain.ErrFeatureDisabled), errors.Is(err, domain.ErrShadowNotPushable),
		errors.Is(err, domain.ErrWriterNotConfigured), errors.Is(err, domain.ErrPeriodNotLocked),
		errors.Is(err, domain.ErrPeriodPosted), errors.Is(err, domain.ErrActiveBatchExists),
		errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrNeedsRepush):
		return ErrorResponse("412", err.Error()) // FailedPrecondition class
	}
	return InternalErrorResponse(err.Error())
}

func erpActor(ctx context.Context) string {
	u, _ := GetUsernameFromCtx(ctx)
	return u
}

func erpBatchMode(m financev1.ErpBatchMode) domain.BatchMode {
	if m == financev1.ErpBatchMode_ERP_BATCH_MODE_SHADOW {
		return domain.ModeShadow
	}
	return domain.ModeLive
}

func erpBatchToProto(b *domain.Batch) *financev1.ErpBatch {
	if b == nil {
		return nil
	}
	mode := financev1.ErpBatchMode_ERP_BATCH_MODE_LIVE
	if b.Mode() == domain.ModeShadow {
		mode = financev1.ErpBatchMode_ERP_BATCH_MODE_SHADOW
	}
	allowed := make([]string, 0, len(b.AllowedTransitions()))
	for _, s := range b.AllowedTransitions() {
		allowed = append(allowed, string(s))
	}
	return &financev1.ErpBatch{
		BatchId:        b.ID(),
		Period:         b.Period(),
		Mode:           mode,
		Status:         financev1.ErpBatchStatus(financev1.ErpBatchStatus_value["ERP_BATCH_STATUS_"+string(b.Status())]),
		Seq:            int32(b.Seq()), //nolint:gosec // seq is small
		CreatedBy:      b.CreatedBy(),
		CreatedAt:      b.CreatedAt().UTC().Format(time.RFC3339),
		UpdatedAt:      b.UpdatedAt().UTC().Format(time.RFC3339),
		SummaryJson:    string(b.Summary()),
		AllowedActions: allowed,
		FailedReason:   b.ErrorText(),
	}
}

// CreateErpBatch creates a DRAFT batch.
func (h *ErpIntegrationHandler) CreateErpBatch(ctx context.Context, req *financev1.CreateErpBatchRequest) (*financev1.CreateErpBatchResponse, error) {
	b, err := h.d.Create.Handle(ctx, erpapp.CreateBatchCommand{
		Period: req.GetPeriod(), Mode: erpBatchMode(req.GetMode()), Actor: erpActor(ctx),
	})
	if err != nil {
		return &financev1.CreateErpBatchResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.CreateErpBatchResponse{Base: erpOK("ERP batch created"), Data: erpBatchToProto(b)}, nil
}

// ListErpBatches lists the batches of a period (period is required for now;
// pagination is applied in memory).
func (h *ErpIntegrationHandler) ListErpBatches(ctx context.Context, req *financev1.ListErpBatchesRequest) (*financev1.ListErpBatchesResponse, error) {
	if req.GetPeriod() == "" {
		return &financev1.ListErpBatchesResponse{Base: BadRequestResponse("period is required")}, nil
	}
	all, err := h.d.Batches.ListByPeriod(ctx, req.GetPeriod())
	if err != nil {
		return &financev1.ListErpBatchesResponse{Base: erpErrBase(err)}, nil
	}
	rows := make([]*financev1.ErpBatch, 0, len(all))
	for _, b := range all {
		p := erpBatchToProto(b)
		if req.GetStatus() != financev1.ErpBatchStatus_ERP_BATCH_STATUS_UNSPECIFIED && p.Status != req.GetStatus() {
			continue
		}
		if req.GetMode() != financev1.ErpBatchMode_ERP_BATCH_MODE_UNSPECIFIED && p.Mode != req.GetMode() {
			continue
		}
		rows = append(rows, p)
	}
	page, size := int32(1), int32(20)
	if p := req.GetPagination(); p != nil {
		if p.GetPage() > 0 {
			page = p.GetPage()
		}
		if p.GetPageSize() > 0 {
			size = p.GetPageSize()
		}
	}
	total := int64(len(rows))
	lo := min(int64(page-1)*int64(size), total)
	hi := min(lo+int64(size), total)
	pages := int32((total + int64(size) - 1) / int64(size)) //nolint:gosec // bounded by page size
	return &financev1.ListErpBatchesResponse{
		Base: erpOK("OK"), Data: rows[lo:hi],
		Pagination: &commonv1.PaginationResponse{CurrentPage: page, PageSize: size, TotalItems: total, TotalPages: pages},
	}, nil
}

// GetErpBatch returns one batch.
func (h *ErpIntegrationHandler) GetErpBatch(ctx context.Context, req *financev1.GetErpBatchRequest) (*financev1.GetErpBatchResponse, error) {
	b, err := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.GetErpBatchResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.GetErpBatchResponse{Base: erpOK("OK"), Data: erpBatchToProto(b)}, nil
}

// trigger enqueues a plain batch step and returns (base, jobID).
func (h *ErpIntegrationHandler) trigger(ctx context.Context, batchID int64, step string) (*commonv1.BaseResponse, string) {
	ex, err := h.d.Steps.Handle(ctx, erpapp.StepTriggerCommand{BatchID: batchID, Step: step, Actor: erpActor(ctx)})
	if err != nil {
		return erpErrBase(err), ""
	}
	return erpOK("Step queued"), ex.ID().String()
}

// LoadErpDemand queues the load_demand step.
func (h *ErpIntegrationHandler) LoadErpDemand(ctx context.Context, req *financev1.LoadErpDemandRequest) (*financev1.LoadErpDemandResponse, error) {
	base, id := h.trigger(ctx, req.GetBatchId(), erpapp.StepLoadDemand)
	return &financev1.LoadErpDemandResponse{Base: base, JobId: id, BatchId: req.GetBatchId()}, nil
}

// RunErpCoverage queues the coverage step.
func (h *ErpIntegrationHandler) RunErpCoverage(ctx context.Context, req *financev1.RunErpCoverageRequest) (*financev1.RunErpCoverageResponse, error) {
	base, id := h.trigger(ctx, req.GetBatchId(), erpapp.StepCoverage)
	return &financev1.RunErpCoverageResponse{Base: base, JobId: id, BatchId: req.GetBatchId()}, nil
}

// RunErpDerive queues the derive step.
func (h *ErpIntegrationHandler) RunErpDerive(ctx context.Context, req *financev1.RunErpDeriveRequest) (*financev1.RunErpDeriveResponse, error) {
	base, id := h.trigger(ctx, req.GetBatchId(), erpapp.StepDerive)
	return &financev1.RunErpDeriveResponse{Base: base, JobId: id, BatchId: req.GetBatchId()}, nil
}

// ValidateErpBatch queues the validate step.
func (h *ErpIntegrationHandler) ValidateErpBatch(ctx context.Context, req *financev1.ValidateErpBatchRequest) (*financev1.ValidateErpBatchResponse, error) {
	base, id := h.trigger(ctx, req.GetBatchId(), erpapp.StepValidate)
	return &financev1.ValidateErpBatchResponse{Base: base, JobId: id, BatchId: req.GetBatchId()}, nil
}

// ReconErpBatch queues the recon step.
func (h *ErpIntegrationHandler) ReconErpBatch(ctx context.Context, req *financev1.ReconErpBatchRequest) (*financev1.ReconErpBatchResponse, error) {
	base, id := h.trigger(ctx, req.GetBatchId(), erpapp.StepRecon)
	return &financev1.ReconErpBatchResponse{Base: base, JobId: id, BatchId: req.GetBatchId()}, nil
}

// PushErpBatch queues the W1 push with the operator-confirmed totals.
func (h *ErpIntegrationHandler) PushErpBatch(ctx context.Context, req *financev1.PushErpBatchRequest) (*financev1.PushErpBatchResponse, error) {
	ex, err := h.d.Steps.TriggerPush(ctx, erpapp.PushTriggerCommand{
		BatchID: req.GetBatchId(), Actor: erpActor(ctx), HasPermission: HasPermission(ctx, permErpPush),
		ConfirmRowCount: req.GetConfirmRowCount(), ConfirmSumStd: req.GetConfirmSumStd(),
	})
	if err != nil {
		return &financev1.PushErpBatchResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.PushErpBatchResponse{Base: erpOK("Push queued"), JobId: ex.ID().String(), BatchId: req.GetBatchId()}, nil
}

// AckErpWarnings records the V-05/V-08w warnings acknowledgement.
func (h *ErpIntegrationHandler) AckErpWarnings(ctx context.Context, req *financev1.AckErpWarningsRequest) (*financev1.AckErpWarningsResponse, error) {
	_, err := h.d.Ack.Handle(ctx, erpapp.AckWarningsCommand{
		BatchID: req.GetBatchId(), Actor: erpActor(ctx), WarningSetHash: req.GetWarningSetHash(),
		HasPermission: HasPermission(ctx, permErpApprove),
	})
	if err != nil {
		return &financev1.AckErpWarningsResponse{Base: erpErrBase(err)}, nil
	}
	b, gerr := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if gerr != nil {
		return &financev1.AckErpWarningsResponse{Base: erpErrBase(gerr)}, nil
	}
	return &financev1.AckErpWarningsResponse{Base: erpOK("Warnings acknowledged"), Data: erpBatchToProto(b)}, nil
}

// PreviewErpPush returns the push control totals.
func (h *ErpIntegrationHandler) PreviewErpPush(ctx context.Context, req *financev1.PreviewErpPushRequest) (*financev1.PreviewErpPushResponse, error) {
	p, err := h.d.PushPrev.Handle(ctx, erpapp.PushPreviewQuery{BatchID: req.GetBatchId(), HasPermission: HasPermission(ctx, permErpPush)})
	if err != nil {
		return &financev1.PreviewErpPushResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.PreviewErpPushResponse{Base: erpOK("OK"), RowCount: p.RowCount, SumStd: p.SumStd, SetHash: p.RowsMD5}, nil
}

func erpAdjOp(o financev1.ErpAdjOperation) (domain.AdjOperation, string) {
	switch o {
	case financev1.ErpAdjOperation_ERP_ADJ_OPERATION_APPROVE:
		return domain.AdjOpApprove, permErpApprove
	case financev1.ErpAdjOperation_ERP_ADJ_OPERATION_RESTORE:
		return domain.AdjOpRestore, permErpRestore
	default:
		return domain.AdjOpValuate, permErpValuate
	}
}

// PreviewErpAdjOperation builds an ADJ preview. The design calls for a job;
// the application handler is synchronous, so the preview_id is returned
// directly and job_id stays empty.
func (h *ErpIntegrationHandler) PreviewErpAdjOperation(ctx context.Context, req *financev1.PreviewErpAdjOperationRequest) (*financev1.PreviewErpAdjOperationResponse, error) {
	op, perm := erpAdjOp(req.GetOperation())
	res, err := h.d.ValPreview.Handle(ctx, erpapp.ValuationPreviewCommand{
		BatchID: req.GetBatchId(), Actor: erpActor(ctx), Operation: op, HasPermission: HasPermission(ctx, perm),
	})
	if err != nil {
		return &financev1.PreviewErpAdjOperationResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.PreviewErpAdjOperationResponse{Base: erpOK("Preview built"), BatchId: req.GetBatchId(), PreviewId: res.Preview.ID}, nil
}

// LockErpBatch queues LOCK_BATCH.
func (h *ErpIntegrationHandler) LockErpBatch(ctx context.Context, req *financev1.LockErpBatchRequest) (*financev1.LockErpBatchResponse, error) {
	_, err := h.d.Steps.TriggerLockBatch(ctx, erpapp.LockBatchTriggerCommand{
		BatchID: req.GetBatchId(), Actor: erpActor(ctx), HasPermission: HasPermission(ctx, permErpLock),
	})
	if err != nil {
		return &financev1.LockErpBatchResponse{Base: erpErrBase(err)}, nil
	}
	b, gerr := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if gerr != nil {
		return &financev1.LockErpBatchResponse{Base: erpErrBase(gerr)}, nil
	}
	return &financev1.LockErpBatchResponse{Base: erpOK("Lock queued"), Data: erpBatchToProto(b)}, nil
}

func erpLockToProto(l *domainperiodlock.PeriodLock) *financev1.ErpPeriodLock {
	if l == nil {
		return nil
	}
	return &financev1.ErpPeriodLock{
		Period: l.Period(), Locked: l.IsLocked(), Reason: l.Reason(), LockedBy: l.LockedBy(),
		LockedAt: l.LockedAt().UTC().Format(time.RFC3339),
	}
}

// LockErpPeriod freezes ACTUAL costing for a period.
func (h *ErpIntegrationHandler) LockErpPeriod(ctx context.Context, req *financev1.LockErpPeriodRequest) (*financev1.LockErpPeriodResponse, error) {
	l, err := h.d.Lock.Handle(ctx, periodlockapp.LockCommand{Period: req.GetPeriod(), User: erpActor(ctx), Reason: req.GetReason()})
	if err != nil {
		return &financev1.LockErpPeriodResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.LockErpPeriodResponse{Base: erpOK("Period locked"), Data: erpLockToProto(l)}, nil
}

// UnlockErpPeriod unlocks a period (refused while ADJ heads are posted).
func (h *ErpIntegrationHandler) UnlockErpPeriod(ctx context.Context, req *financev1.UnlockErpPeriodRequest) (*financev1.UnlockErpPeriodResponse, error) {
	r, err := h.d.Unlock.Handle(ctx, periodlockapp.UnlockCommand{Period: req.GetPeriod(), User: erpActor(ctx), Reason: req.GetReason()})
	if err != nil {
		return &financev1.UnlockErpPeriodResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.UnlockErpPeriodResponse{Base: erpOK("Period unlocked"), Data: erpLockToProto(r.Lock)}, nil
}

func erpOK(msg string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{IsSuccess: true, StatusCode: "200", Message: msg}
}
