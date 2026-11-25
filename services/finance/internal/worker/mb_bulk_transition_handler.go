package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbheadbulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// MBBulkTransitionHandler processes one Bulk MB Head Regenerate child job: it runs
// the single mbhead.Entity transition named by msg.Subtype (force_unvalidate/
// submit/validate — see application/mbheadbulk's Action constants) against the
// single mbh_id carried in msg.MbhID, then reports the outcome to the parent batch
// job via job.Repository.IncrementChildProgress.
//
// 🔴 Registered SEQUENTIALLY in cmd/worker/main.go (rabbitmq.NewConsumer, not
// NewConcurrentConsumer): ValidateHandler's underlying repo call
// (TransitionWithAutoGen) has not been audited for concurrent-write safety, so
// concurrency here would risk racing writes to cost_product_master/cost_route_*
// across children of the same batch.
type MBBulkTransitionHandler struct {
	jobRepo          job.Repository
	forceUnvalidateH *appmbhead.ForceUnvalidateHandler
	submitH          *appmbhead.SubmitHandler
	validateH        *appmbhead.ValidateHandler
	recompute        appmbhead.RevalidateRecomputer
	logger           zerolog.Logger
}

// WithRevalidateRecompute attaches the MB cost snapshot recompute run ONCE when a bulk
// Validate batch finishes, over every successfully validated head (deduped), instead of
// once per child. nil disables it.
func (h *MBBulkTransitionHandler) WithRevalidateRecompute(r appmbhead.RevalidateRecomputer) *MBBulkTransitionHandler {
	h.recompute = r
	return h
}

// NewMBBulkTransitionHandler constructs the handler. submitH and validateH are the
// SAME handlers wired to the ordinary SubmitMBHead/ValidateMBHead RPCs — this
// worker only re-triggers them, it does not duplicate their logic.
func NewMBBulkTransitionHandler(
	jobRepo job.Repository,
	forceUnvalidateH *appmbhead.ForceUnvalidateHandler,
	submitH *appmbhead.SubmitHandler,
	validateH *appmbhead.ValidateHandler,
	logger zerolog.Logger,
) *MBBulkTransitionHandler {
	return &MBBulkTransitionHandler{
		jobRepo:          jobRepo,
		forceUnvalidateH: forceUnvalidateH,
		submitH:          submitH,
		validateH:        validateH,
		logger:           logger,
	}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: PROCESSING -> (success: COMPLETED) | (failure: FAILED), then in both
// cases IncrementChildProgress reports the outcome to the parent batch job. A
// per-child failure never fails the whole batch and never nacks the delivery
// (returns nil) — it is isolated to this one child, exactly like
// CostSheetExportHandler's per-child pattern.
func (h *MBBulkTransitionHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if err := exec.Start(); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: job state transition failed; continuing")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: persist PROCESSING failed")
	}

	runErr := h.runTransition(ctx, msg)
	if runErr != nil {
		h.markFailed(ctx, exec, msg, runErr)
		return nil
	}

	h.markCompleted(ctx, exec, msg)
	return nil
}

// runTransition dispatches msg.Subtype to the matching mbhead application handler
// against msg.MbhID. msg.CreatedBy (the Super Admin who triggered the bulk action)
// is threaded through as the actor on every transition, exactly as it would be for
// the equivalent single-head RPC call.
func (h *MBBulkTransitionHandler) runTransition(ctx context.Context, msg rabbitmq.JobMessage) error {
	mbhID, err := uuid.Parse(msg.MbhID)
	if err != nil {
		return fmt.Errorf("invalid mbh_id: %w", err)
	}

	switch msg.Subtype {
	case mbheadbulk.ActionForceUnvalidate:
		if h.forceUnvalidateH == nil {
			return fmt.Errorf("force-unvalidate handler unavailable")
		}
		_, err = h.forceUnvalidateH.Handle(ctx, appmbhead.ForceUnvalidateCommand{
			MbhID:       mbhID,
			Reason:      msg.Reason,
			ActorUserID: msg.CreatedBy,
		})
	case mbheadbulk.ActionSubmit:
		if h.submitH == nil {
			return fmt.Errorf("submit handler unavailable")
		}
		_, err = h.submitH.Handle(ctx, appmbhead.SubmitCommand{
			MbhID:       mbhID,
			ActorUserID: msg.CreatedBy,
		})
	case mbheadbulk.ActionValidate:
		if h.validateH == nil {
			return fmt.Errorf("validate handler unavailable")
		}
		_, err = h.validateH.Handle(ctx, appmbhead.ValidateCommand{
			MbhID:       mbhID,
			ActorUserID: msg.CreatedBy,
		})
	default:
		return fmt.Errorf("unknown bulk transition action %q", msg.Subtype)
	}
	return err
}

// markCompleted persists the COMPLETED status and reports success to the parent
// batch job's progress counters.
func (h *MBBulkTransitionHandler) markCompleted(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage) {
	if err := exec.Complete(nil); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: complete state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: persist COMPLETED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, true)
	h.logger.Info().Str("job_id", msg.JobID).Str("mbh_id", msg.MbhID).Str("action", msg.Subtype).
		Msg("mb bulk transition completed")
}

// markFailed persists the FAILED status (with runErr recorded as the job's
// error_message — this IS the per-child failure detail surfaced later by
// ListBulkMBHeadJobFailures, which reads it straight off the child job row) and
// reports the failure to the parent batch job's progress counters.
func (h *MBBulkTransitionHandler) markFailed(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, runErr error) {
	if err := exec.Fail(bulkFailureMessage(runErr)); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: fail state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("mb bulk transition: persist FAILED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, false)
	h.logger.Error().Err(runErr).Str("job_id", msg.JobID).Str("mbh_id", msg.MbhID).Str("action", msg.Subtype).
		Msg("mb bulk transition failed")
}

// handleChildCompletion atomically increments the parent batch job's
// completed/failed counter for one finished child and, when that increment
// reports the batch is now fully done, marks the parent job_execution row
// COMPLETED/FAILED so it stops showing as PROCESSING in the job list. Mirrors
// CostSheetExportHandler.handleChildCompletion, minus the notification fan-out —
// Bulk MB Head Regenerate has no IAM notification wired for this phase; progress
// is polled via GetBulkMBHeadJobStatus instead.
func (h *MBBulkTransitionHandler) handleChildCompletion(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, success bool) {
	parentID := exec.ParentJobID()
	if parentID == nil {
		return
	}
	batchComplete, err := h.jobRepo.IncrementChildProgress(ctx, *parentID, success)
	if err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Str("parent_job_id", parentID.String()).
			Msg("mb bulk transition: increment parent batch progress failed")
		return
	}
	if !batchComplete {
		return
	}
	h.completeParentJob(ctx, *parentID, msg)
}

// completeParentJob loads the now-fully-finished parent job and transitions it to
// a terminal status, recording the final child tallies as its result summary. A
// parent has no transition of its own to run — it exists purely to track the batch.
func (h *MBBulkTransitionHandler) completeParentJob(ctx context.Context, parentID uuid.UUID, msg rabbitmq.JobMessage) {
	parent, err := h.jobRepo.GetByID(ctx, parentID)
	if err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("mb bulk transition: load parent job for batch-complete failed")
		return
	}

	allFailed := parent.CompletedChildren() == 0 && parent.FailedChildren() > 0
	summary := fmt.Appendf(nil, `{"total_children":%d,"completed_children":%d,"failed_children":%d}`,
		parent.TotalChildren(), parent.CompletedChildren(), parent.FailedChildren())

	if allFailed {
		if err := parent.Fail(fmt.Sprintf("all %d child transitions failed", parent.FailedChildren())); err != nil {
			h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).
				Msg("mb bulk transition: parent fail state transition failed")
		}
	} else if err := parent.Complete(summary); err != nil {
		h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).
			Msg("mb bulk transition: parent complete state transition failed")
	}

	if err := h.jobRepo.UpdateStatus(ctx, parent); err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("mb bulk transition: persist parent batch completion status failed")
		return
	}
	h.triggerRecompute(ctx, parent)
	_ = msg // msg is retained on the signature for logging-context symmetry with markCompleted/markFailed.
}

// triggerRecompute hands the successfully validated heads of a finished bulk Validate batch
// to the snapshot recompute (detached; it never affects the batch outcome). Heads come from
// the persisted child rows, so a worker restart mid-batch loses none. First validates are
// harmless: they have no snapshot yet, so the recompute selects no period for them.
func (h *MBBulkTransitionHandler) triggerRecompute(ctx context.Context, parent *job.Execution) {
	if h.recompute == nil || parent.Subtype() != mbheadbulk.ActionValidate {
		return
	}
	children, err := h.jobRepo.ListChildren(ctx, parent.ID())
	if err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parent.ID().String()).
			Msg("mb bulk transition: list children for snapshot recompute failed; snapshots not refreshed")
		return
	}
	ids := validatedChildMBHIDs(children)
	if len(ids) > 0 {
		h.recompute.TriggerRevalidateRecompute(ctx, ids, parent.CreatedBy())
	}
}

// validatedChildMBHIDs returns the distinct mbh_id of every SUCCESS child, read from the
// child params JSON written by mbheadbulk.buildParams.
func validatedChildMBHIDs(children []*job.Execution) []string {
	seen := make(map[string]struct{}, len(children))
	out := make([]string, 0, len(children))
	for _, c := range children {
		if c.Status() != job.StatusSuccess {
			continue
		}
		var p struct {
			MBHID string `json:"mbh_id"`
		}
		if err := json.Unmarshal(c.Params(), &p); err != nil || p.MBHID == "" {
			continue
		}
		if _, dup := seen[p.MBHID]; dup {
			continue
		}
		seen[p.MBHID] = struct{}{}
		out = append(out, p.MBHID)
	}
	return out
}

// supersedeConfirmationPrefix marks a child failure caused by a shade+FG collision that
// needs an explicit supersede confirmation. Bulk Validate never confirms, so it never
// supersedes silently; the operator validates that head individually instead.
const supersedeConfirmationPrefix = "SUPERSEDE_CONFIRMATION_REQUIRED: "

// bulkFailureMessage renders the child job error_message (surfaced unchanged by
// ListBulkMBHeadJobFailures).
func bulkFailureMessage(err error) string {
	if errors.Is(err, mbspin.ErrSupersedeConfirmationRequired) {
		return supersedeConfirmationPrefix + err.Error()
	}
	return err.Error()
}
