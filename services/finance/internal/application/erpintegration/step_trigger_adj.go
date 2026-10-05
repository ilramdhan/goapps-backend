package erpintegration

// step_trigger_adj.go enqueues the W2 jobs (plan-06 P5-T5): ADJ execute
// (VALUATE / APPROVE / RESTORE) and LOCK_BATCH. The trigger checks G1-G3 and
// the batch status up front; the worker step re-checks every gate (G4-G11,
// re-probe, V-07) under the G11 lock before any package call.

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// WithAdjGates sets the G1 flags and the G2 writer the W2 triggers check up
// front. Unset: every W2 trigger is refused (fail closed).
func (h *StepTriggerHandler) WithAdjGates(valuationEnabled, adjApproveEnabled bool, writer WriterGate) *StepTriggerHandler {
	h.valuationEnabled, h.adjApproveEnabled, h.adjWriter = valuationEnabled, adjApproveEnabled, writer
	return h
}

// AdjTriggerCommand requests one ADJ operation from a confirmed preview.
type AdjTriggerCommand struct {
	BatchID        int64
	PreviewID      string
	Operation      domain.AdjOperation
	ConfirmSetHash string
	ConfirmText    string
	Actor          string
	HasPermission  bool
}

// TriggerAdjExecute checks G1-G3 and the request, then enqueues the
// adj_execute job.
func (h *StepTriggerHandler) TriggerAdjExecute(ctx context.Context, cmd AdjTriggerCommand) (*job.Execution, error) {
	if _, err := adjSpecFor(cmd.Operation); err != nil {
		return nil, err
	}
	denied := ErrAdjPermissionDenied
	if cmd.Operation == domain.AdjOpValuate {
		denied = ErrValuatePermissionDenied
	}
	if err := checkPermission(cmd.HasPermission, denied); err != nil {
		return nil, err
	}
	if err := h.checkAdjGates(cmd.Operation == domain.AdjOpApprove); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cmd.PreviewID) == "" {
		return nil, fmt.Errorf("%w: preview_id is required", domain.ErrPreviewRequired)
	}
	if strings.TrimSpace(cmd.ConfirmSetHash) == "" || strings.TrimSpace(cmd.ConfirmText) == "" {
		return nil, fmt.Errorf("%w: confirm_set_hash and confirm_text are required", domain.ErrConfirmMismatch)
	}
	return h.enqueue(ctx, StepTriggerCommand{BatchID: cmd.BatchID, Step: StepAdjExecute, Actor: cmd.Actor}, JobParams{
		PreviewID: strings.TrimSpace(cmd.PreviewID), Operation: string(cmd.Operation),
		ConfirmSetHash: cmd.ConfirmSetHash, ConfirmText: cmd.ConfirmText, AdjPermitted: true,
	})
}

// LockBatchTriggerCommand requests LOCK_BATCH.
type LockBatchTriggerCommand struct {
	BatchID       int64
	Actor         string
	HasPermission bool
}

// TriggerLockBatch checks G1-G3, then enqueues the lock_batch job.
func (h *StepTriggerHandler) TriggerLockBatch(ctx context.Context, cmd LockBatchTriggerCommand) (*job.Execution, error) {
	if err := checkPermission(cmd.HasPermission, ErrAdjPermissionDenied); err != nil {
		return nil, err
	}
	if err := h.checkAdjGates(false); err != nil {
		return nil, err
	}
	return h.enqueue(ctx, StepTriggerCommand{BatchID: cmd.BatchID, Step: StepLockBatch, Actor: cmd.Actor},
		JobParams{AdjPermitted: true})
}

func (h *StepTriggerHandler) checkAdjGates(approve bool) error {
	if err := checkFlag(h.valuationEnabled, flagValuationEnabled); err != nil {
		return err
	}
	if approve {
		if err := checkFlag(h.adjApproveEnabled, flagAdjApproveEnabled); err != nil {
			return err
		}
	}
	return checkWriter(h.adjWriter)
}

// checkAdjStep is the trigger-time status check of the W2 steps (the exact
// per-operation status is re-checked by the step under the lock).
func checkAdjStep(b *domain.Batch, step string) error {
	switch step {
	case StepAdjExecute:
		if b.Mode() == domain.ModeShadow {
			return domain.ErrShadowNotPushable
		}
		return requireStatus(b, "adj_execute", domain.StatusPushed, domain.StatusValuated, domain.StatusReconciled)
	case StepLockBatch:
		if b.Mode() == domain.ModeShadow {
			return domain.ErrShadowNotPushable
		}
		return requireStatus(b, "lock_batch", domain.StatusReconciled)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownStep, step)
	}
}
