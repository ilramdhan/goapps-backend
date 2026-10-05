package periodlock

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// LockCommand locks (period, ACTUAL).
type LockCommand struct {
	Period   string
	CalcType string // "" defaults to ACTUAL
	User     string
	Reason   string
}

// LockHandler freezes ACTUAL costing for a period. Audit: ERP_PERIOD_LOCK.
type LockHandler struct {
	repo  domain.Repository
	audit AuditSink
	now   func() time.Time
}

// NewLockHandler builds the handler; audit may be nil.
func NewLockHandler(repo domain.Repository, audit AuditSink) *LockHandler {
	return &LockHandler{repo: repo, audit: audit, now: time.Now}
}

// Handle locks the period. A soft-unlocked period is re-locked in place.
func (h *LockHandler) Handle(ctx context.Context, cmd LockCommand) (*domain.PeriodLock, error) {
	calcType := cmd.CalcType
	if calcType == "" {
		calcType = domain.CalcTypeActual
	}
	lock, err := domain.NewPeriodLock(cmd.Period, calcType, cmd.User, cmd.Reason, h.now().UTC())
	if err != nil {
		return nil, err
	}

	before, err := h.repo.Get(ctx, lock.Period(), lock.CalcType())
	switch {
	case errors.Is(err, domain.ErrLockNotFound):
		before = nil
	case err != nil:
		return nil, fmt.Errorf("period lock: read current: %w", err)
	case before.IsLocked():
		return nil, domain.ErrAlreadyLocked
	}

	if err := h.repo.Lock(ctx, lock); err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.OpErpPeriodLock, lock.Period(), lock.LockedBy(),
		snapshotOf(before), snapshotOf(lock))
	return lock, nil
}
