package costcalc

import (
	"context"
	"errors"
	"fmt"
)

// ApproveCostCommand carries inputs for approving a verified cost.
type ApproveCostCommand struct {
	CostID int64
	Actor  string
}

// ApproveCostHandler transitions a VERIFIED result to APPROVED.
type ApproveCostHandler struct {
	svc     *Service
	mbGuard MBCostRowChecker
}

// ApproveOption customizes the handler at construction.
type ApproveOption func(*ApproveCostHandler)

// WithApproveMBGuard installs the MB rejection check. Omitting it leaves the check
// disabled; tests omit it.
func WithApproveMBGuard(c MBCostRowChecker) ApproveOption {
	return func(h *ApproveCostHandler) { h.mbGuard = c }
}

// NewApproveCostHandler constructs the handler.
func NewApproveCostHandler(svc *Service, opts ...ApproveOption) *ApproveCostHandler {
	h := &ApproveCostHandler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the approval.
func (h *ApproveCostHandler) Handle(ctx context.Context, cmd ApproveCostCommand) error {
	if cmd.CostID <= 0 {
		return errors.New(errMsgCostIDPositive)
	}
	if cmd.Actor == "" {
		return errors.New(errMsgActorRequired)
	}
	if err := rejectMBCostRow(ctx, h.mbGuard, cmd.CostID); err != nil {
		return err
	}
	if err := h.svc.resultRepo.MarkApproved(ctx, cmd.CostID, cmd.Actor); err != nil {
		return fmt.Errorf("mark approved: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType:  "COST_RESULT_APPROVED",
		EntityKind: auditEntityKindCost,
		EntityID:   fmt.Sprintf("%d", cmd.CostID),
		Actor:      cmd.Actor,
		Message:    fmt.Sprintf("cost %d approved", cmd.CostID),
	})
	return nil
}
