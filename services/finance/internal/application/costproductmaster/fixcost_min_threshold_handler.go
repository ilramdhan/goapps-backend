package costproductmaster

import (
	"context"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// UpdateFixCostMinThresholdCommand sets the per-product fixed-cost minimum
// threshold (000615). Threshold 0 turns the floor off.
type UpdateFixCostMinThresholdCommand struct {
	ProductSysID int64
	Threshold    float64
	ActorUserID  string
}

// UpdateFixCostMinThresholdHandler writes cpm_fixcost_min_threshold and audits
// the change (cal_operation UPDATE, entity cost_product_master).
type UpdateFixCostMinThresholdHandler struct {
	repo    domain.Repository
	thrRepo domain.FixCostMinThresholdRepository
	audit   AuditSink
	now     func() time.Time
}

// NewUpdateFixCostMinThresholdHandler constructs the handler; audit may be nil.
func NewUpdateFixCostMinThresholdHandler(repo domain.Repository, thrRepo domain.FixCostMinThresholdRepository, audit AuditSink) *UpdateFixCostMinThresholdHandler {
	return &UpdateFixCostMinThresholdHandler{repo: repo, thrRepo: thrRepo, audit: audit, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *UpdateFixCostMinThresholdHandler) WithClock(now func() time.Time) *UpdateFixCostMinThresholdHandler {
	h.now = now
	return h
}

// Handle executes the update. An unchanged threshold writes nothing.
func (h *UpdateFixCostMinThresholdHandler) Handle(ctx context.Context, cmd UpdateFixCostMinThresholdCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	before := fixCostMinThresholdSnapshot{Threshold: copyFloatPtr(p.FixCostMinThreshold())}
	changed, err := p.SetFixCostMinThreshold(cmd.Threshold, cmd.ActorUserID, h.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		return p, nil
	}
	if err := h.thrRepo.SaveFixCostMinThreshold(ctx, domain.FixCostMinThresholdWrite{
		ProductSysID: p.ProductSysID(),
		Threshold:    cmd.Threshold,
		UpdatedAt:    p.UpdatedAt(),
		UpdatedBy:    p.UpdatedBy(),
	}); err != nil {
		return nil, err
	}
	emitProductAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityProductMaster,
		EntityID:   p.ProductSysID(),
		Operation:  auditdomain.OpUpdate,
		BeforeData: marshalAudit(before),
		AfterData:  marshalAudit(fixCostMinThresholdSnapshot{Threshold: p.FixCostMinThreshold()}),
		UserID:     cmd.ActorUserID,
	})
	return p, nil
}

// fixCostMinThresholdSnapshot is the audit JSON (null = auto by machine).
type fixCostMinThresholdSnapshot struct {
	Threshold *float64 `json:"fixcost_min_threshold"`
}

// copyFloatPtr detaches the snapshot from the entity's pointer.
func copyFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
