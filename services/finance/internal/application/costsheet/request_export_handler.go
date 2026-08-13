// Package costsheet implements the async product-cost-sheet export use cases
// (queue + download-URL) that back CostCalcService's export RPCs. Cloned in
// structure from internal/application/rmcost's export handlers, but the
// underlying artifact and job type differ, so it lives in its own package.
package costsheet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ErrPublisherUnavailable is returned when the finance service has no working
// RabbitMQ publisher, so no export job can be queued.
var ErrPublisherUnavailable = errors.New("message queue unavailable: RabbitMQ not connected " +
	"(finance service could not reach the broker at startup; check RabbitMQ health and restart the finance service)")

// maxExportProducts caps how many products a single export job renders.
// Filter resolutions above this no longer truncate — they fan out into a
// parent job plus N ≤maxExportProducts-sized child jobs (see Handle), each
// rendering its own workbook and notifying only once the whole batch is
// done. Explicit ProductSysIDs stay bounded by the proto's
// buf.validate.field.repeated.max_items = 200 rule, so that path never fans
// out; a defensive check still rejects it here as a second line of defense.
//
// Worker memory note: the export worker (internal/worker.CostSheetExportHandler)
// keeps the built *excelize.File in memory while rendering all sheets, then
// serializes it to a local temp file (not a second in-memory buffer) before
// uploading — see writeWorkbookToTemp. So peak memory per job is roughly one
// workbook's worth (excelize's internal representation of up to
// maxExportProducts sheets, one per product, sheet name = item code), not
// two. If this cap is ever raised significantly, re-check that per-sheet
// cost against available worker memory/limits.
const maxExportProducts = 200

// resolvePageSize is the page size used while paging through ListResults to
// build the full filtered product ID list. It uses the repository's own max
// (500) rather than maxExportProducts to minimize round-trips when a period
// resolves to tens of thousands of products.
const resolvePageSize = 500

// exportSubtype is the job.Execution subtype recorded for every product cost
// sheet export (all such jobs render an xlsx workbook).
const exportSubtype = "xlsx"

// ExportJobPublisher abstracts the RabbitMQ publisher dependency for testability.
type ExportJobPublisher interface {
	PublishProductCostSheetExport(ctx context.Context, jobID, period, requestingUserID, createdBy string, productSysIDs []int64) error
}

// RequestExportCommand carries the validated input for queueing a product
// cost sheet export. When ProductSysIDs is non-empty it overrides the filter
// fields entirely — the filter fields are then ignored.
type RequestExportCommand struct {
	Period           string
	CalcType         costcalcdom.CalculationType
	ProductTypeIDs   []int32
	Search           string
	Status           string
	ProductSysIDs    []int64
	RequestingUserID string // recipient for the EXPORT_READY notification
	CreatedBy        string // audit identity (typically "user:<uuid>" or username)
}

// RequestExportResult is the queue acknowledgement.
type RequestExportResult struct {
	Execution *job.Execution
}

// RequestExportHandler queues an asynchronous product cost sheet export job.
type RequestExportHandler struct {
	jobRepo    job.Repository
	resultRepo costcalcdom.ResultRepository
	publisher  ExportJobPublisher
}

// NewRequestExportHandler constructs the handler.
func NewRequestExportHandler(jobRepo job.Repository, resultRepo costcalcdom.ResultRepository, publisher ExportJobPublisher) *RequestExportHandler {
	return &RequestExportHandler{jobRepo: jobRepo, resultRepo: resultRepo, publisher: publisher}
}

// Handle resolves the target product list, then either queues a single
// job_execution (≤maxExportProducts products) or fans the request out into a
// parent job plus N child jobs (more than maxExportProducts products, only
// reachable via a filter — explicit ProductSysIDs are already bounded by
// proto validation). Returns the job the caller/UI should track: the single
// job in the unchanged path, or the parent job in the fan-out path.
func (h *RequestExportHandler) Handle(ctx context.Context, cmd RequestExportCommand) (*RequestExportResult, error) {
	if err := h.validate(cmd); err != nil {
		return nil, err
	}
	if cmd.CreatedBy == "" {
		cmd.CreatedBy = cmd.RequestingUserID
	}
	if len(cmd.ProductSysIDs) > maxExportProducts {
		return nil, fmt.Errorf("product_sys_ids exceeds the %d-item limit", maxExportProducts)
	}

	productSysIDs, err := h.resolveProducts(ctx, cmd)
	if err != nil {
		return nil, err
	}

	if len(productSysIDs) <= maxExportProducts {
		return h.handleSingle(ctx, cmd, productSysIDs)
	}
	return h.handleBatch(ctx, cmd, productSysIDs)
}

// handleSingle queues one job_execution for a product list within the cap —
// the original, unchanged behavior.
func (h *RequestExportHandler) handleSingle(ctx context.Context, cmd RequestExportCommand, productSysIDs []int64) (*RequestExportResult, error) {
	paramsJSON, err := buildParams(cmd, productSysIDs)
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}

	// Allow multiple concurrent export jobs for the same period — different
	// users may export with different filters. Skip the HasActiveJob check.
	exec, err := job.NewExecution(job.TypeProductCostSheetExport, exportSubtype, cmd.Period, cmd.CreatedBy, 5, paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist job: %w", err)
	}

	if err := h.publisher.PublishProductCostSheetExport(
		ctx, exec.ID().String(), cmd.Period, cmd.RequestingUserID, cmd.CreatedBy, productSysIDs,
	); err != nil {
		return nil, h.failJob(ctx, exec, err)
	}

	return &RequestExportResult{Execution: exec}, nil
}

// handleBatch fans a filter resolving to more than maxExportProducts
// products out into one parent job_execution (pure progress-tracking row, no
// product list or file of its own) plus N ≤maxExportProducts-sized child
// jobs, each persisted and published exactly like a standalone job so the
// worker needs no awareness of batching. The worker's per-child completion
// path (internal/worker.CostSheetExportHandler) atomically increments the
// parent's counters and fires exactly one notification once every child has
// finished.
func (h *RequestExportHandler) handleBatch(ctx context.Context, cmd RequestExportCommand, productSysIDs []int64) (*RequestExportResult, error) {
	chunks := chunkIDs(productSysIDs, maxExportProducts)

	parentParams, err := buildParams(cmd, nil)
	if err != nil {
		return nil, fmt.Errorf("encode parent params: %w", err)
	}
	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, exportSubtype, cmd.Period, cmd.CreatedBy, 5, parentParams, len(chunks))
	if err != nil {
		return nil, fmt.Errorf("create parent execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, parent); err != nil {
		return nil, fmt.Errorf("persist parent job: %w", err)
	}

	children := make([]*job.Execution, 0, len(chunks))
	for _, chunk := range chunks {
		childParams, paramsErr := buildParams(cmd, chunk)
		if paramsErr != nil {
			return nil, fmt.Errorf("encode child params: %w", paramsErr)
		}
		child, childErr := job.NewChildExecution(job.TypeProductCostSheetExport, exportSubtype, cmd.Period, cmd.CreatedBy, 5, childParams, parent.ID())
		if childErr != nil {
			return nil, fmt.Errorf("create child execution: %w", childErr)
		}
		children = append(children, child)
	}
	if err := h.jobRepo.CreateChildren(ctx, children); err != nil {
		return nil, fmt.Errorf("persist child jobs: %w", err)
	}

	// Publish each child independently. A publish failure fails only that one
	// child (never the whole batch) — the worker will never see an unpublished
	// child, so it can never drive the parent's counters for it; increment the
	// parent's failed-children counter here so the batch can still reach
	// completion and fire its one notification instead of hanging forever.
	var publishErrs error
	for i, child := range children {
		if err := h.publisher.PublishProductCostSheetExport(
			ctx, child.ID().String(), cmd.Period, cmd.RequestingUserID, cmd.CreatedBy, chunks[i],
		); err != nil {
			publishErrs = errors.Join(publishErrs, h.failJob(ctx, child, err))
			if _, incErr := h.jobRepo.IncrementChildProgress(ctx, parent.ID(), false); incErr != nil {
				publishErrs = errors.Join(publishErrs, fmt.Errorf("record child publish failure: %w", incErr))
			}
		}
	}
	if publishErrs != nil {
		return nil, publishErrs
	}

	return &RequestExportResult{Execution: parent}, nil
}

// chunkIDs splits ids into consecutive slices of at most size elements each.
// size must be positive.
func chunkIDs(ids []int64, size int) [][]int64 {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]int64, 0, (len(ids)+size-1)/size)
	for start := 0; start < len(ids); start += size {
		end := min(start+size, len(ids))
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

// validate checks the fields Handle needs before doing any work.
func (h *RequestExportHandler) validate(cmd RequestExportCommand) error {
	if h.publisher == nil {
		return ErrPublisherUnavailable
	}
	if cmd.Period == "" {
		return fmt.Errorf("period is required")
	}
	if cmd.RequestingUserID == "" {
		return fmt.Errorf("requesting user id is required")
	}
	return nil
}

// resolveProducts returns the full target product_sys_id list. Explicit
// ProductSysIDs bypass the filter query entirely; otherwise the filter is
// resolved via the existing cost-result listing path, paging through every
// matching row (not just the first page) so a large filter's fan-out sees
// every product, not merely the first maxExportProducts.
func (h *RequestExportHandler) resolveProducts(ctx context.Context, cmd RequestExportCommand) ([]int64, error) {
	if len(cmd.ProductSysIDs) > 0 {
		return cmd.ProductSysIDs, nil
	}

	var ids []int64
	for page := 1; ; page++ {
		filter := costcalcdom.ResultListFilter{
			Period:         cmd.Period,
			CalcType:       cmd.CalcType,
			Status:         cmd.Status,
			Search:         cmd.Search,
			ProductTypeIDs: cmd.ProductTypeIDs,
			Page:           page,
			PageSize:       resolvePageSize,
		}
		items, total, _, err := h.resultRepo.ListResults(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("resolve product filter: %w", err)
		}
		if total == 0 {
			return nil, fmt.Errorf("no products matched the given filter")
		}
		for _, it := range items {
			ids = append(ids, it.ProductSysID)
		}
		if len(ids) >= total || len(items) == 0 {
			break
		}
	}
	return ids, nil
}

// failJob marks the job failed so it doesn't sit forever in QUEUED after a
// publish error. Best-effort: when the followup persist also fails, both root
// causes are surfaced via errors.Join instead of the update error masking the
// original publish error.
func (h *RequestExportHandler) failJob(ctx context.Context, exec *job.Execution, publishErr error) error {
	if failErr := exec.Fail("failed to publish to queue: " + publishErr.Error()); failErr == nil {
		if updErr := h.jobRepo.UpdateStatus(ctx, exec); updErr != nil {
			return errors.Join(fmt.Errorf("publish job: %w", publishErr), fmt.Errorf("persist failed: %w", updErr))
		}
	}
	return fmt.Errorf("publish job: %w", publishErr)
}

// buildParams serializes the job params JSON persisted for traceability/debug.
// productSysIDs is nil for a parent (batch-tracking) job — it has no product
// list of its own, each child carries its own chunk.
func buildParams(cmd RequestExportCommand, productSysIDs []int64) (json.RawMessage, error) {
	params := map[string]any{
		"period":             cmd.Period,
		"calculation_type":   string(cmd.CalcType),
		"product_type_ids":   cmd.ProductTypeIDs,
		"search":             cmd.Search,
		"status":             cmd.Status,
		"product_sys_ids":    productSysIDs,
		"requesting_user_id": cmd.RequestingUserID,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return paramsJSON, nil
}
