package costproductmaster

import (
	"context"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// UpdateRMRatioModeCommand sets or clears the per-product RM ratio mode
// (000579). Mode "" clears it (follow the product type); INPUT or DENIER
// overrides the type mode for this product only.
type UpdateRMRatioModeCommand struct {
	ProductSysID int64
	Mode         string
	ActorUserID  string
}

// UpdateRMRatioModeHandler writes cpm_rm_ratio_mode and audits the change
// (cal_operation UPDATE, entity cost_product_master).
type UpdateRMRatioModeHandler struct {
	repo     domain.Repository
	modeRepo domain.RMRatioModeRepository
	audit    AuditSink
	now      func() time.Time
}

// NewUpdateRMRatioModeHandler constructs the handler; audit may be nil.
func NewUpdateRMRatioModeHandler(repo domain.Repository, modeRepo domain.RMRatioModeRepository, audit AuditSink) *UpdateRMRatioModeHandler {
	return &UpdateRMRatioModeHandler{repo: repo, modeRepo: modeRepo, audit: audit, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *UpdateRMRatioModeHandler) WithClock(now func() time.Time) *UpdateRMRatioModeHandler {
	h.now = now
	return h
}

// Handle executes the update. An unchanged mode writes nothing. On a change
// the product is reloaded so the effective mode reflects the product type
// after a clear.
func (h *UpdateRMRatioModeHandler) Handle(ctx context.Context, cmd UpdateRMRatioModeCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	before := rmRatioModeSnapshotOf(p)
	changed, err := p.SetRMRatioModeOverride(cmd.Mode, cmd.ActorUserID, h.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		return p, nil
	}
	if err := h.modeRepo.SaveRMRatioMode(ctx, domain.RMRatioModeWrite{
		ProductSysID: p.ProductSysID(),
		Override:     p.RMRatioModeOverride(),
		UpdatedAt:    p.UpdatedAt(),
		UpdatedBy:    p.UpdatedBy(),
	}); err != nil {
		return nil, err
	}
	if reloaded, rerr := h.repo.GetBySysID(ctx, p.ProductSysID()); rerr == nil && reloaded != nil {
		p = reloaded
	}
	emitProductAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityProductMaster,
		EntityID:   p.ProductSysID(),
		Operation:  auditdomain.OpUpdate,
		BeforeData: marshalAudit(before),
		AfterData:  marshalAudit(rmRatioModeSnapshotOf(p)),
		UserID:     cmd.ActorUserID,
	})
	return p, nil
}

// rmRatioModeSnapshot is the audit JSON of the RM ratio mode (null override = follow type).
type rmRatioModeSnapshot struct {
	Override  *string `json:"rm_ratio_mode_override"`
	Effective string  `json:"effective_rm_ratio_mode"`
}

func rmRatioModeSnapshotOf(p *domain.CostProductMaster) rmRatioModeSnapshot {
	return rmRatioModeSnapshot{Override: strPtrOrNil(p.RMRatioModeOverride()), Effective: p.EffectiveRMRatioMode()}
}
