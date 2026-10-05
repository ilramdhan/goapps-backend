package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// auditEntityKindCost is the EntityKind value for COST_RESULT_* audit events.
const auditEntityKindCost = "COST_RESULT"

// VerifyCostCommand carries inputs for verifying a calculated cost.
type VerifyCostCommand struct {
	CostID int64
	Actor  string
}

// VerifyCostHandler transitions a CALCULATED result to VERIFIED.
type VerifyCostHandler struct {
	svc        *Service
	mbGuard    MBCostRowChecker
	periodLock costcalcdom.PeriodLockChecker
}

// VerifyOption customizes the handler at construction.
type VerifyOption func(*VerifyCostHandler)

// WithVerifyMBGuard installs the MB rejection check. Omitting it leaves the check
// disabled; tests omit it.
func WithVerifyMBGuard(c MBCostRowChecker) VerifyOption {
	return func(h *VerifyCostHandler) { h.mbGuard = c }
}

// WithVerifyPeriodLock installs the period-lock guard (plan-02 P1-T3): the target row's
// (period, calc type) is refused with ErrPeriodLocked when ACTUAL and locked.
// Omitting it (nil) keeps the pre-lock behavior; tests omit it.
func WithVerifyPeriodLock(c costcalcdom.PeriodLockChecker) VerifyOption {
	return func(h *VerifyCostHandler) { h.periodLock = c }
}

// NewVerifyCostHandler constructs the handler.
func NewVerifyCostHandler(svc *Service, opts ...VerifyOption) *VerifyCostHandler {
	h := &VerifyCostHandler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the verification.
func (h *VerifyCostHandler) Handle(ctx context.Context, cmd VerifyCostCommand) error {
	if cmd.CostID <= 0 {
		return errors.New(errMsgCostIDPositive)
	}
	if cmd.Actor == "" {
		return errors.New(errMsgActorRequired)
	}
	if err := rejectMBCostRow(ctx, h.mbGuard, cmd.CostID); err != nil {
		return err
	}
	if err := rejectLockedCostRow(ctx, h.periodLock, h.svc.resultRepo, cmd.CostID); err != nil {
		return err
	}
	if err := h.svc.resultRepo.MarkVerified(ctx, cmd.CostID, cmd.Actor); err != nil {
		return fmt.Errorf("mark verified: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType:  "COST_RESULT_VERIFIED",
		EntityKind: auditEntityKindCost,
		EntityID:   fmt.Sprintf("%d", cmd.CostID),
		Actor:      cmd.Actor,
		Message:    fmt.Sprintf("cost %d verified", cmd.CostID),
	})
	return nil
}
