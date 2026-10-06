// Package periodlock holds the lock / unlock / get use cases for the
// (period, ACTUAL) cost freeze (design Part 1 §5.5, Part 2 §9.2; plan-02
// P1-T2). Permission checks (finance.cost.erpintegration.lock / .unlock) are
// the delivery layer's job (P6).
package periodlock

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// Audit identifiers. cal_entity_id is the period as an integer (YYYYMM).
const (
	AuditEntityPeriodLock = "cst_period_lock"
)

// AuditSink is the subset of the cost audit emitter (auditapp.Emitter) used here.
type AuditSink interface {
	Emit(ctx context.Context, in auditdomain.NewInput) error
}

// lockSnapshot is the JSON written to cal_before_data / cal_after_data.
type lockSnapshot struct {
	Period        string     `json:"period"`
	CalcType      string     `json:"calc_type"`
	Locked        bool       `json:"locked"`
	LockedAt      time.Time  `json:"locked_at"`
	LockedBy      string     `json:"locked_by"`
	Reason        string     `json:"reason"`
	ErpBatchID    *int64     `json:"erp_batch_id,omitempty"`
	UnlockedAt    *time.Time `json:"unlocked_at,omitempty"`
	UnlockedBy    string     `json:"unlocked_by,omitempty"`
	UnlockReason  string     `json:"unlock_reason,omitempty"`
	RepushFlagged *int64     `json:"repush_flagged,omitempty"`
}

func snapshotOf(l *domain.PeriodLock) *lockSnapshot {
	if l == nil {
		return nil
	}
	return &lockSnapshot{
		Period: l.Period(), CalcType: l.CalcType(), Locked: l.IsLocked(),
		LockedAt: l.LockedAt(), LockedBy: l.LockedBy(), Reason: l.Reason(),
		ErpBatchID: l.ErpBatchID(),
		UnlockedAt: l.UnlockedAt(), UnlockedBy: l.UnlockedBy(), UnlockReason: l.UnlockReason(),
	}
}

func marshalSnapshot(s *lockSnapshot) string {
	if s == nil {
		return ""
	}
	b, err := json.Marshal(s)
	if err != nil {
		log.Warn().Err(err).Msg("period lock: marshal audit snapshot")
		return ""
	}
	return string(b)
}

// emitAudit appends one audit row. Best effort, like every other audit call
// in the service: the lock change has already committed, so an audit failure
// is logged and does not fail the request.
func emitAudit(ctx context.Context, sink AuditSink, op, period, user string, before, after *lockSnapshot) {
	if sink == nil {
		return
	}
	id, err := strconv.ParseInt(period, 10, 64)
	if err != nil {
		log.Warn().Err(err).Str("period", period).Msg("period lock: audit entity id")
		return
	}
	in := auditdomain.NewInput{
		EntityType: AuditEntityPeriodLock,
		EntityID:   id,
		Operation:  op,
		BeforeData: marshalSnapshot(before),
		AfterData:  marshalSnapshot(after),
		UserID:     user,
	}
	if err := sink.Emit(ctx, in); err != nil {
		log.Warn().Err(err).Str("operation", op).Str("period", period).Msg("period lock: audit emit failed")
	}
}
