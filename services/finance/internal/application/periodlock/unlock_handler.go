package periodlock

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// ErrAdjProbeNotConfigured is returned when no AdjPostedProbe is wired:
// unlock fails closed rather than assume "not posted".
var ErrAdjProbeNotConfigured = errors.New("period unlock: ADJ posted probe not configured")

// UnlockCommand unlocks (period, ACTUAL).
type UnlockCommand struct {
	Period   string
	CalcType string // "" defaults to ACTUAL
	User     string
	Reason   string
}

// UnlockResult is the unlocked lock plus the number of ERP batches that were
// flagged ceib_needs_repush.
type UnlockResult struct {
	Lock          *domain.PeriodLock
	RepushFlagged int64
}

// UnlockHandler releases the cost freeze. It is refused when a batch for the
// period is LOCKED or the ADJ probe reports the period as posted (design
// §5.5). Audit: ERP_PERIOD_UNLOCK with the before-snapshot.
type UnlockHandler struct {
	repo  domain.Repository
	probe domain.AdjPostedProbe
	audit AuditSink
	now   func() time.Time
}

// NewUnlockHandler builds the handler; audit may be nil. probe must be set
// (use StaticAdjPostedProbe until the read-only Oracle probe lands, P3-T3).
func NewUnlockHandler(repo domain.Repository, probe domain.AdjPostedProbe, audit AuditSink) *UnlockHandler {
	return &UnlockHandler{repo: repo, probe: probe, audit: audit, now: time.Now}
}

// Handle unlocks the period.
func (h *UnlockHandler) Handle(ctx context.Context, cmd UnlockCommand) (UnlockResult, error) {
	calcType := cmd.CalcType
	if calcType == "" {
		calcType = domain.CalcTypeActual
	}
	if err := domain.ValidatePeriod(cmd.Period); err != nil {
		return UnlockResult{}, err
	}
	if err := domain.ValidateCalcType(calcType); err != nil {
		return UnlockResult{}, err
	}

	lock, err := h.repo.Get(ctx, cmd.Period, calcType)
	if errors.Is(err, domain.ErrLockNotFound) {
		return UnlockResult{}, domain.ErrNotLocked
	}
	if err != nil {
		return UnlockResult{}, fmt.Errorf("period unlock: read current: %w", err)
	}
	if !lock.IsLocked() {
		return UnlockResult{}, domain.ErrNotLocked
	}
	before := snapshotOf(lock)

	batchLocked, err := h.repo.HasLockedErpBatch(ctx, cmd.Period)
	if err != nil {
		return UnlockResult{}, fmt.Errorf("period unlock: batch probe: %w", err)
	}
	if batchLocked {
		return UnlockResult{}, domain.ErrUnlockBatchLocked
	}
	if h.probe == nil {
		return UnlockResult{}, ErrAdjProbeNotConfigured
	}
	posted, err := h.probe.IsAdjPosted(ctx, cmd.Period)
	if err != nil {
		return UnlockResult{}, fmt.Errorf("period unlock: ADJ posted probe: %w", err)
	}
	if err := lock.Unlock(cmd.User, cmd.Reason, posted, h.now().UTC()); err != nil {
		return UnlockResult{}, err
	}

	flagged, err := h.repo.Unlock(ctx, lock)
	if err != nil {
		return UnlockResult{}, err
	}
	after := snapshotOf(lock)
	after.RepushFlagged = &flagged
	emitAudit(ctx, h.audit, auditdomain.OpErpPeriodUnlock, lock.Period(), lock.UnlockedBy(), before, after)
	return UnlockResult{Lock: lock, RepushFlagged: flagged}, nil
}

// StaticAdjPostedProbe is the stand-in AdjPostedProbe used until the
// read-only Oracle probe exists (P3-T3). It answers Posted for every period.
type StaticAdjPostedProbe struct{ Posted bool }

// IsAdjPosted returns the static answer.
func (p StaticAdjPostedProbe) IsAdjPosted(context.Context, string) (bool, error) {
	return p.Posted, nil
}

var _ domain.AdjPostedProbe = StaticAdjPostedProbe{}
