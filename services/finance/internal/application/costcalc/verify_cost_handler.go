package costcalc

import (
	"context"
	"errors"
	"fmt"
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
	svc *Service
}

// NewVerifyCostHandler constructs the handler.
func NewVerifyCostHandler(svc *Service) *VerifyCostHandler {
	return &VerifyCostHandler{svc: svc}
}

// Handle executes the verification.
func (h *VerifyCostHandler) Handle(ctx context.Context, cmd VerifyCostCommand) error {
	if cmd.CostID <= 0 {
		return errors.New(errMsgCostIDPositive)
	}
	if cmd.Actor == "" {
		return errors.New(errMsgActorRequired)
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
