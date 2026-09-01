// Package mbheadbulk implements the Bulk MB Head Regenerate use case: fanning a
// Super-Admin-triggered force-unvalidate/submit/validate action out across many MB
// Heads as one parent job.Execution plus one child job.Execution per mbh_id, each
// published to RabbitMQ independently. Cloned in shape from
// internal/application/costsheet's export fan-out (see
// request_export_handler.go's handleBatch), minus chunking: Bulk MB Head Regenerate
// always creates exactly one child per mbh_id, however many there are — the whole
// point is that production currently has every MB Head stuck in VALIDATED, so a
// single run's mbh_ids list can be large but each child does trivial, independent
// work.
package mbheadbulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// Action discriminators, recorded as each job.Execution's subtype (parent and
// children alike) — they tell Phase C's worker which mbhead.Entity transition to run
// for a given child.
const (
	ActionForceUnvalidate = "force_unvalidate"
	ActionSubmit          = "submit"
	ActionValidate        = "validate"
)

// ErrPublisherUnavailable is returned when the finance service has no working
// RabbitMQ publisher, so no bulk transition job can be queued.
var ErrPublisherUnavailable = errors.New("message queue unavailable: RabbitMQ not connected " +
	"(finance service could not reach the broker at startup; check RabbitMQ health and restart the finance service)")

// BulkTransitionJobPublisher abstracts the RabbitMQ publisher dependency for testability.
type BulkTransitionJobPublisher interface {
	PublishMBBulkTransition(ctx context.Context, jobID, mbhID, action, reason, createdBy string) error
}

// CompositionRefLookup abstracts the within-batch MB-to-MB composition
// reference lookup Handle uses to order children by dependency, mirroring
// BulkTransitionJobPublisher's narrow-interface-for-testability pattern above
// instead of depending on the full mbcomposition.Repository interface.
type CompositionRefLookup interface {
	ListMBRefEdgesForBatch(ctx context.Context, mbhIDs []string) ([]mbcomposition.BatchRefEdge, error)
}

// RequestBulkTransitionCommand carries the validated input for queueing a bulk MB
// Head transition. Reason is only meaningful for ActionForceUnvalidate — Submit and
// Validate ignore it, mirroring mbhead.Entity.ForceUnvalidate's own optional-reason
// contract.
type RequestBulkTransitionCommand struct {
	MBHIDs    []string
	Action    string
	Reason    string
	CreatedBy string
}

// RequestBulkTransitionResult is the queue acknowledgement.
type RequestBulkTransitionResult struct {
	Execution *job.Execution
}

// RequestBulkTransitionHandler queues an asynchronous bulk MB Head transition job.
type RequestBulkTransitionHandler struct {
	jobRepo         job.Repository
	publisher       BulkTransitionJobPublisher
	compositionRepo CompositionRefLookup
}

// NewRequestBulkTransitionHandler constructs the handler. compositionRepo may be
// nil — Handle then skips dependency ordering entirely and publishes children in
// their original request order, exactly as before this was added.
func NewRequestBulkTransitionHandler(
	jobRepo job.Repository, publisher BulkTransitionJobPublisher, compositionRepo CompositionRefLookup,
) *RequestBulkTransitionHandler {
	return &RequestBulkTransitionHandler{jobRepo: jobRepo, publisher: publisher, compositionRepo: compositionRepo}
}

// Handle creates one parent job.Execution (total_children = len(cmd.MBHIDs), no
// chunking — one child per mbh_id directly) plus N child job.Execution rows, then
// publishes each child to RabbitMQ independently. A per-child publish failure fails
// only that child (recorded via failJob + IncrementChildProgress) — it never aborts
// the rest of the batch or the parent job, matching costsheet's handleBatch pattern.
//
// Before creating any child, cmd.MBHIDs is reordered (see orderByDependency) so
// that, if this batch contains both an MB head and another MB head it references
// as a nested RM input in its own recipe, the referenced head's job is created and
// published first. This fixes a real production incident: the mb_bulk_transition
// worker consumer processes strictly sequentially (concurrency 1, see
// cmd/worker/main.go), and RabbitMQ delivers a single consumer's messages in the
// order they were published — so publish order fully determines processing order
// here (see orderByDependency's doc comment for the caveats that would break this).
// Without the reorder, a dependent head validated before its dependency finished
// would hit mbResolveRefProductSysID with mbh_cost_product_id still NULL. Applies
// uniformly to all three actions (force_unvalidate/submit/validate) since Handle is
// one shared code path — ordering only matters for Validate's cost generation, but
// reordering the other two is harmless and keeps this simple.
//
// Handle's returned error is reserved for genuine Handle-level failures: the parent
// job.Execution (or its children) could not be created/persisted at all. A per-child
// publish failure is a normal, expected, partially-successful outcome — by the time
// it happens the parent job and every child are already durably persisted, so Handle
// still returns (*RequestBulkTransitionResult, nil) with the parent job.Execution
// (refreshed so its FailedChildren()/CompletedChildren() counters are accurate). The
// caller discovers per-child failure details later via GetBulkMBHeadJobStatus /
// ListBulkMBHeadJobFailures, not via Handle's returned error.
func (h *RequestBulkTransitionHandler) Handle(ctx context.Context, cmd RequestBulkTransitionCommand) (*RequestBulkTransitionResult, error) {
	if err := h.validate(cmd); err != nil {
		return nil, err
	}
	cmd.MBHIDs = h.orderByDependency(ctx, cmd.MBHIDs)

	parentParams, err := buildParams(cmd, "")
	if err != nil {
		return nil, fmt.Errorf("encode parent params: %w", err)
	}
	parent, err := job.NewParentExecution(job.TypeMBBulkTransition, cmd.Action, "", cmd.CreatedBy, 5, parentParams, len(cmd.MBHIDs))
	if err != nil {
		return nil, fmt.Errorf("create parent execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, parent); err != nil {
		return nil, fmt.Errorf("persist parent job: %w", err)
	}

	children := make([]*job.Execution, 0, len(cmd.MBHIDs))
	for _, mbhID := range cmd.MBHIDs {
		childParams, paramsErr := buildParams(cmd, mbhID)
		if paramsErr != nil {
			return nil, fmt.Errorf("encode child params: %w", paramsErr)
		}
		child, childErr := job.NewChildExecution(job.TypeMBBulkTransition, cmd.Action, "", cmd.CreatedBy, 5, childParams, parent.ID())
		if childErr != nil {
			return nil, fmt.Errorf("create child execution: %w", childErr)
		}
		children = append(children, child)
	}
	if err := h.jobRepo.CreateChildren(ctx, children); err != nil {
		return nil, fmt.Errorf("persist child jobs: %w", err)
	}

	parent = h.publishChildren(ctx, cmd, parent, children)

	return &RequestBulkTransitionResult{Execution: parent}, nil
}

// publishChildren publishes each child independently to RabbitMQ. A publish
// failure fails only that one child (never the whole batch, and never Handle
// itself) — the worker will never see an unpublished child, so it can never
// drive the parent's counters for it; failJob + IncrementChildProgress record
// the failure so the batch can still reach completion instead of hanging
// forever waiting on a child that was never queued. Both are best-effort: the
// parent job and every child are already durably persisted by this point, so a
// failure to record a publish failure is logged, not escalated into a
// Handle-level error. Returns the parent to use in the result: refreshed from
// the repository (so FailedChildren()/CompletedChildren() are accurate)
// whenever at least one child failed to publish, otherwise the original
// in-memory parent unchanged.
func (h *RequestBulkTransitionHandler) publishChildren(
	ctx context.Context, cmd RequestBulkTransitionCommand, parent *job.Execution, children []*job.Execution,
) *job.Execution {
	anyPublishFailed := false
	for i, child := range children {
		if err := h.publisher.PublishMBBulkTransition(ctx, child.ID().String(), cmd.MBHIDs[i], cmd.Action, cmd.Reason, cmd.CreatedBy); err != nil {
			anyPublishFailed = true
			if failErr := h.failJob(ctx, child, err); failErr != nil {
				log.Warn().Err(failErr).Str("child_job_id", child.ID().String()).
					Msg("mb bulk transition: failed to record child publish failure")
			}
			if _, incErr := h.jobRepo.IncrementChildProgress(ctx, parent.ID(), false); incErr != nil {
				log.Warn().Err(incErr).Str("parent_job_id", parent.ID().String()).
					Msg("mb bulk transition: failed to increment parent failed-children counter")
			}
		}
	}
	if !anyPublishFailed {
		return parent
	}

	// IncrementChildProgress above updates the failed/completed counters at the
	// repository level, not on this in-memory parent — refresh it so the returned
	// job info (see bulkMBHeadJobInfoFromExecution) reports accurate counters
	// instead of stale zeros. Best-effort: if the refresh itself fails, fall back to
	// the original in-memory parent — the job still exists and is queryable via
	// GetBulkMBHeadJobStatus regardless.
	refreshed, getErr := h.jobRepo.GetByID(ctx, parent.ID())
	if getErr != nil {
		log.Warn().Err(getErr).Str("job_id", parent.ID().String()).
			Msg("mb bulk transition: failed to refresh parent job after publish failures")
		return parent
	}
	return refreshed
}

// orderByDependency returns mbhIDs reordered so that, within THIS batch only, any
// head another head in the batch references as a nested MB RM input
// (mst_mb_composition.mbcm_mb_ref_mbh_id, source_type MB) is placed before its
// dependent. References to heads outside this batch are irrelevant here — they
// are either already VALIDATED or will legitimately fail later with
// mbResolveRefProductSysID's clear error message naming the missing dependency,
// which is an acceptable, expected outcome, not something this ordering needs to
// prevent.
//
// Best-effort by design: h.compositionRepo == nil (not wired, e.g. in tests that
// don't care about ordering) or a lookup failure both fall back to the original
// order unchanged rather than failing the whole bulk request — this ordering is a
// correctness improvement for the common case, not a hard precondition for
// queuing the batch at all. A dependency cycle among referencing heads (should
// not normally happen, but recipes are not validated against it elsewhere) also
// falls back to original order for the cyclic subset only; see kahnTopoSort.
//
// This relies on the mb_bulk_transition worker consumer being both (a) strictly
// sequential (concurrency 1) and (b) a single consumer processing one queue in
// publish order — both true as of cmd/worker/main.go (NewConsumer, not
// NewConcurrentConsumer) and the finance-worker deployment (replicas: 1). A nacked
// delivery goes straight to the DLQ (Nack(false, false), no requeue), so a retry
// never re-enters the queue out of order either. If mb_bulk_transition ever moves
// to a concurrent or multi-replica consumer, or gains a requeue-on-retry path,
// publish-order sorting alone would stop being sufficient and this would need a
// real cross-child coordination mechanism instead.
func (h *RequestBulkTransitionHandler) orderByDependency(ctx context.Context, mbhIDs []string) []string {
	if h.compositionRepo == nil || len(mbhIDs) < 2 {
		return mbhIDs
	}
	edges, err := h.compositionRepo.ListMBRefEdgesForBatch(ctx, mbhIDs)
	if err != nil {
		log.Warn().Err(err).Msg("mb bulk transition: failed to look up within-batch composition dependencies; using original order")
		return mbhIDs
	}
	if len(edges) == 0 {
		return mbhIDs
	}
	return kahnTopoSort(mbhIDs, edges)
}

// validate checks the fields Handle needs before doing any work.
func (h *RequestBulkTransitionHandler) validate(cmd RequestBulkTransitionCommand) error {
	if h.publisher == nil {
		return ErrPublisherUnavailable
	}
	if len(cmd.MBHIDs) == 0 {
		return fmt.Errorf("mbh_ids is required")
	}
	switch cmd.Action {
	case ActionForceUnvalidate, ActionSubmit, ActionValidate:
	default:
		return fmt.Errorf("unknown action %q", cmd.Action)
	}
	if cmd.CreatedBy == "" {
		return fmt.Errorf("created by is required")
	}
	return nil
}

// failJob marks the job failed so it doesn't sit forever in QUEUED after a publish
// error. Best-effort: when the followup persist also fails, both root causes are
// surfaced via errors.Join instead of the update error masking the original publish
// error.
func (h *RequestBulkTransitionHandler) failJob(ctx context.Context, exec *job.Execution, publishErr error) error {
	if failErr := exec.Fail("failed to publish to queue: " + publishErr.Error()); failErr == nil {
		if updErr := h.jobRepo.UpdateStatus(ctx, exec); updErr != nil {
			return errors.Join(fmt.Errorf("publish job: %w", publishErr), fmt.Errorf("persist failed: %w", updErr))
		}
	}
	return fmt.Errorf("publish job: %w", publishErr)
}

// buildParams serializes the job params JSON persisted for traceability/debug. mbhID
// is empty for the parent (batch-tracking) job — it has no single target of its own,
// each child carries its own.
func buildParams(cmd RequestBulkTransitionCommand, mbhID string) (json.RawMessage, error) {
	params := map[string]any{
		"action":    cmd.Action,
		"reason":    cmd.Reason,
		"mbh_id":    mbhID,
		"mbh_count": len(cmd.MBHIDs),
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return paramsJSON, nil
}
