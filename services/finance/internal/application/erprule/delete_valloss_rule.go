package erprule

import (
	"context"
	"fmt"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// DeleteVallossRuleCommand soft-deletes a rule (is_active=false).
type DeleteVallossRuleCommand struct {
	ID   int64
	User string
}

// DeleteVallossRuleHandler soft-deletes a rule. Audit: RULE_DELETE.
//
// Before deleting it computes the hash of the current RuleSet and refuses
// with domain.ErrRuleInUse while a non-terminal batch (DERIVED..VALUATED)
// was derived with it (plan-03 P2-T4 step 2). The usage port is
// domain.RuleHashUsage; until P3-T2 the Postgres RuleSet loader answers it.
type DeleteVallossRuleHandler struct {
	repo   domain.VallossRuleRepository
	loader domain.RuleSetLoader
	usage  domain.RuleHashUsage
	audit  AuditSink
	now    func() time.Time
}

// NewDeleteVallossRuleHandler builds the handler; audit may be nil. loader
// and usage are required: the delete fails closed without them.
func NewDeleteVallossRuleHandler(
	repo domain.VallossRuleRepository, loader domain.RuleSetLoader, usage domain.RuleHashUsage, audit AuditSink,
) *DeleteVallossRuleHandler {
	return &DeleteVallossRuleHandler{repo: repo, loader: loader, usage: usage, audit: audit, now: time.Now}
}

// Handle soft-deletes the rule. ErrRuleNotFound for an unknown id,
// ErrRuleInactive for an already deleted rule, ErrRuleInUse while the
// current rule hash is used by a non-terminal batch.
func (h *DeleteVallossRuleHandler) Handle(ctx context.Context, cmd DeleteVallossRuleCommand) error {
	rule, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if !rule.IsActive() {
		return domain.ErrRuleInactive
	}
	if err := h.checkNotInUse(ctx); err != nil {
		return err
	}
	before := vallossSnapshotOf(rule)
	if err := rule.Deactivate(cmd.User, h.now().UTC()); err != nil {
		return err
	}
	if err := h.repo.Update(ctx, rule); err != nil {
		return err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityVallossRule,
		EntityID:   rule.ID(),
		Operation:  auditdomain.OpRuleDelete,
		BeforeData: snapshotJSON(before),
		AfterData:  snapshotJSON(vallossSnapshotOf(rule)),
		UserID:     rule.UpdatedBy(),
	})
	return nil
}

// checkNotInUse fails closed: a missing port or any read error refuses the
// delete.
func (h *DeleteVallossRuleHandler) checkNotInUse(ctx context.Context) error {
	if h.loader == nil || h.usage == nil {
		return fmt.Errorf("erp rule delete: rule usage check is not configured: %w", domain.ErrRuleInUse)
	}
	rs, err := h.loader.LoadRuleSet(ctx)
	if err != nil {
		return fmt.Errorf("erp rule delete: load current rule set: %w", err)
	}
	inUse, err := h.usage.IsRuleHashInUse(ctx, rs.Hash())
	if err != nil {
		return fmt.Errorf("erp rule delete: rule hash usage: %w", err)
	}
	if inUse {
		return domain.ErrRuleInUse
	}
	return nil
}
