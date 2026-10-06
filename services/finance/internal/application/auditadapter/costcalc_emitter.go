package auditadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	costcalcapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

// costCalcSystemActor is recorded when a calc-engine event carries no actor
// (cal_user_id is NOT NULL).
const costCalcSystemActor = "system"

// costCalcOperations maps calc-engine event types to the cal_operation
// whitelist (chk_cal_operation, migration 000215). Unknown event types fall
// back to STATUS_CHANGE so a new event never fails the DB CHECK.
var costCalcOperations = map[string]string{
	"COST_CALC_JOB_TRIGGERED":   auditdomain.OpInsert,
	"COST_CALC_JOB_BLOCKED":     auditdomain.OpStatusChange,
	"COST_CALC_JOB_CANCELLED":   auditdomain.OpStatusChange,
	"COST_CALC_PRODUCT_BLOCKED": auditdomain.OpStatusChange,
	"COST_RESULT_VERIFIED":      auditdomain.OpStatusChange,
	"COST_RESULT_APPROVED":      auditdomain.OpStatusChange,
}

// auditSink is the subset of *auditapp.Emitter the adapter needs (test seam).
type auditSink interface {
	Emit(ctx context.Context, in auditdomain.NewInput) error
}

// CostCalcEmitter adapts auditapp.Emitter to costcalc.AuditEmitter, so the calc
// engine's COST_CALC_* / COST_RESULT_* events land in cost_audit_log. The
// costcalc.AuditEvent shape is unchanged; this adapter only maps it:
//
//   - EntityKind -> cal_entity_type, EntityID (decimal) -> cal_entity_id
//   - EventType  -> cal_operation via costCalcOperations
//   - Actor      -> cal_user_id ("system" when empty)
//   - {event_type, message, payload} -> cal_after_data (JSON)
type CostCalcEmitter struct{ sink auditSink }

var _ costcalcapp.AuditEmitter = (*CostCalcEmitter)(nil)

// NewCostCalcEmitter constructs the adapter.
func NewCostCalcEmitter(emitter *auditapp.Emitter) *CostCalcEmitter {
	return &CostCalcEmitter{sink: emitter}
}

// costCalcAfterData is the JSON written to cal_after_data.
type costCalcAfterData struct {
	EventType string          `json:"event_type"`
	Message   string          `json:"message,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Emit forwards one calc-engine event to cost_audit_log. Errors are returned to
// the caller (costcalc.Service.emitAudit treats them as best-effort).
func (a *CostCalcEmitter) Emit(ctx context.Context, e costcalcapp.AuditEvent) error {
	entityID, err := strconv.ParseInt(e.EntityID, 10, 64)
	if err != nil {
		return fmt.Errorf("costcalc audit: entity id %q is not an integer: %w", e.EntityID, err)
	}
	op, ok := costCalcOperations[e.EventType]
	if !ok {
		op = auditdomain.OpStatusChange
	}
	actor := e.Actor
	if actor == "" {
		actor = costCalcSystemActor
	}
	after := costCalcAfterData{EventType: e.EventType, Message: e.Message}
	if len(e.Payload) > 0 && json.Valid(e.Payload) {
		after.Payload = json.RawMessage(e.Payload)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("costcalc audit: marshal after data: %w", err)
	}
	return a.sink.Emit(ctx, auditdomain.NewInput{
		EntityType: e.EntityKind,
		EntityID:   entityID,
		Operation:  op,
		AfterData:  string(afterJSON),
		UserID:     actor,
	})
}
