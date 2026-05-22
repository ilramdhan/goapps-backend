package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// TriggerCommand carries inputs from the gRPC handler down to TriggerJobHandler.
// Scopes other than SINGLE_PRODUCT are reserved for the S8c orchestrator and
// currently return ErrScopeNotYetSupported.
type TriggerCommand struct {
	Period              string
	CalcType            costcalcdom.CalculationType
	Scope               costcalcdom.JobScope
	ProductSysID        int64
	RouteHeadID         int64
	ProductTypeIDFilter int32
	TriggeredBy         string
	Actor               string
	Filter              []byte
}

// TriggerJobHandler exposes the synchronous SINGLE_PRODUCT path of the calc
// engine. Other scopes will land in S8c when the orchestrator + RMQ worker
// machinery exists.
type TriggerJobHandler struct {
	svc *Service
}

// NewTriggerJobHandler constructs the handler.
func NewTriggerJobHandler(svc *Service) *TriggerJobHandler {
	return &TriggerJobHandler{svc: svc}
}

// auditEntityKindJob is the EntityKind value for all COST_CALC_JOB_* audit events.
const auditEntityKindJob = "COST_CALC_JOB"

// ErrScopeNotYetSupported is returned for batch / route scopes until the S8c
// orchestrator lands.
var ErrScopeNotYetSupported = errors.New("scope not yet supported in S8b foundation; orchestrator lands in S8c")

// ErrProductRequired is returned when SINGLE_PRODUCT was requested without a
// product id.
var ErrProductRequired = errors.New("product_sys_id required for SINGLE_PRODUCT scope")

// Handle creates a job + chunk + job_product row, runs ProcessChunk inline,
// finalizes the job, and returns the fully-resolved Job aggregate.
func (h *TriggerJobHandler) Handle(ctx context.Context, cmd TriggerCommand) (*costcalcdom.Job, error) {
	if cmd.Scope != costcalcdom.ScopeSingleProduct {
		return nil, fmt.Errorf("%w: scope=%s", ErrScopeNotYetSupported, cmd.Scope)
	}
	if cmd.ProductSysID == 0 {
		return nil, ErrProductRequired
	}

	job, err := costcalcdom.NewJob(cmd.Period, cmd.CalcType, cmd.Scope, cmd.Filter, cmd.TriggeredBy, cmd.Actor)
	if err != nil {
		return nil, fmt.Errorf("new job: %w", err)
	}
	if err := h.svc.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType: "COST_CALC_JOB_TRIGGERED", EntityKind: auditEntityKindJob,
		EntityID: fmt.Sprintf("%d", job.ID()), Actor: cmd.Actor,
		Message: fmt.Sprintf("triggered single-product job for product=%d period=%s calc=%s", cmd.ProductSysID, cmd.Period, cmd.CalcType),
	})

	routeHeadID, ok, err := h.resolveRouteHead(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return h.finalizeMissingRoute(ctx, job, cmd)
	}

	if err := h.planJob(ctx, job); err != nil {
		return nil, err
	}

	chunk, err := h.seedChunkAndProduct(ctx, job, cmd, routeHeadID)
	if err != nil {
		return nil, err
	}

	if err := job.MarkProcessing(); err != nil {
		return nil, fmt.Errorf("mark processing: %w", err)
	}
	if err := h.svc.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return nil, fmt.Errorf("update job status processing: %w", err)
	}

	out, err := h.svc.ProcessChunk(ctx, ProcessChunkInput{
		JobID:    job.ID(),
		ChunkID:  chunk.ID(),
		Period:   cmd.Period,
		CalcType: cmd.CalcType,
		Products: []int64{cmd.ProductSysID},
		Actor:    cmd.Actor,
	})
	if err != nil {
		return nil, fmt.Errorf("process chunk: %w", err)
	}

	if err := h.completeJob(ctx, job, out); err != nil {
		return nil, err
	}
	return job, nil
}

// resolveRouteHead returns the active route head_id for the product. ok=false
// means no active (COMPLETE/LOCKED) route exists.
func (h *TriggerJobHandler) resolveRouteHead(ctx context.Context, productSysID int64) (int64, bool, error) {
	routes, err := h.svc.loader.LoadRoutesByProducts(ctx, []int64{productSysID})
	if err != nil {
		return 0, false, fmt.Errorf("load route: %w", err)
	}
	route, ok := routes[productSysID]
	if !ok || route == nil || route.Head == nil {
		return 0, false, nil
	}
	return route.Head.HeadID, true, nil
}

// finalizeMissingRoute records the job as FAILED with 1 blocked product when
// the trigger came in for a product that has no active route. Mirrors what
// ProcessChunk would have done if it ran the chunk.
func (h *TriggerJobHandler) finalizeMissingRoute(ctx context.Context, job *costcalcdom.Job, cmd TriggerCommand) (*costcalcdom.Job, error) {
	if err := job.MarkPlanning(); err != nil {
		return nil, fmt.Errorf("mark planning: %w", err)
	}
	if err := h.svc.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return nil, fmt.Errorf("update job status planning: %w", err)
	}
	if err := job.MarkProcessing(); err != nil {
		return nil, fmt.Errorf("mark processing: %w", err)
	}
	if err := h.svc.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return nil, fmt.Errorf("update job status processing: %w", err)
	}
	if err := job.MarkComplete(0, 0, 1); err != nil {
		return nil, fmt.Errorf("mark complete: %w", err)
	}
	if err := h.svc.jobRepo.UpdateCompletion(ctx, job.ID(), job.Status(), 0, 0, 1, job.DurationMs(), nil); err != nil {
		return nil, fmt.Errorf("update job completion: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType: "COST_CALC_JOB_BLOCKED", EntityKind: auditEntityKindJob,
		EntityID: fmt.Sprintf("%d", job.ID()), Actor: cmd.Actor,
		Message: fmt.Sprintf("no active route for product %d", cmd.ProductSysID),
	})
	return job, nil
}

// planJob writes PLANNING + totals=(1,1,1) for the single-product case.
func (h *TriggerJobHandler) planJob(ctx context.Context, job *costcalcdom.Job) error {
	if err := job.MarkPlanning(); err != nil {
		return fmt.Errorf("mark planning: %w", err)
	}
	if err := h.svc.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return fmt.Errorf("update job status planning: %w", err)
	}
	job.SetTotals(1, 1, 1)
	if err := h.svc.jobRepo.UpdateTotals(ctx, job.ID(), 1, 1, 1); err != nil {
		return fmt.Errorf("update job totals: %w", err)
	}
	return nil
}

// seedChunkAndProduct creates the one-chunk + one-job-product rows and wires
// them together.
func (h *TriggerJobHandler) seedChunkAndProduct(ctx context.Context, job *costcalcdom.Job, cmd TriggerCommand, routeHeadID int64) (*costcalcdom.Chunk, error) {
	chunk := costcalcdom.NewChunk(job.ID(), 1, 0, []int64{cmd.ProductSysID})
	if err := h.svc.chunkRepo.Create(ctx, chunk); err != nil {
		return nil, fmt.Errorf("create chunk: %w", err)
	}
	jp := costcalcdom.NewJobProduct(job.ID(), cmd.ProductSysID, routeHeadID, 0)
	if err := h.svc.productRepo.BulkCreate(ctx, []*costcalcdom.JobProduct{jp}); err != nil {
		return nil, fmt.Errorf("create job_product: %w", err)
	}
	if err := h.svc.productRepo.AssignChunk(ctx, job.ID(), cmd.ProductSysID, chunk.ID()); err != nil {
		return nil, fmt.Errorf("assign chunk: %w", err)
	}
	return chunk, nil
}

// completeJob marks the in-memory aggregate complete and persists the final
// counters + duration.
func (h *TriggerJobHandler) completeJob(ctx context.Context, job *costcalcdom.Job, out *ProcessChunkOutput) error {
	if err := job.MarkComplete(out.Success, out.Failed, out.Blocked); err != nil {
		return fmt.Errorf("mark complete: %w", err)
	}
	if err := h.svc.jobRepo.UpdateCompletion(ctx, job.ID(), job.Status(), out.Success, out.Failed, out.Blocked, job.DurationMs(), nil); err != nil {
		return fmt.Errorf("update job completion: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType:  jobCompletionEvent(job.Status()),
		EntityKind: auditEntityKindJob,
		EntityID:   fmt.Sprintf("%d", job.ID()),
		Actor:      job.CreatedBy(),
		Message:    fmt.Sprintf("job %s: success=%d failed=%d blocked=%d", job.Status(), out.Success, out.Failed, out.Blocked),
	})
	return nil
}

func jobCompletionEvent(status costcalcdom.JobStatus) string {
	switch status {
	case costcalcdom.JobStatusSuccess:
		return "COST_CALC_JOB_SUCCESS"
	case costcalcdom.JobStatusPartialFailed:
		return "COST_CALC_JOB_PARTIAL"
	case costcalcdom.JobStatusFailed:
		return "COST_CALC_JOB_FAILED"
	default:
		return "COST_CALC_JOB_COMPLETED"
	}
}
