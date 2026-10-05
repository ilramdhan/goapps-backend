package erprule

import (
	"context"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// UpdateVallossRuleCommand changes the basis and value loss of an active
// rule. The key is immutable (a key change is a delete plus a create).
type UpdateVallossRuleCommand struct {
	ID      int64
	Basis   string
	ValLoss string
	User    string
}

// UpdateVallossRuleHandler updates a rule. Audit: RULE_UPDATE. The change
// applies to the next derive; batches already derived keep their snapshot
// (design §8.2), so no in-use check is made here.
type UpdateVallossRuleHandler struct {
	repo  domain.VallossRuleRepository
	audit AuditSink
	now   func() time.Time
}

// NewUpdateVallossRuleHandler builds the handler; audit may be nil.
func NewUpdateVallossRuleHandler(repo domain.VallossRuleRepository, audit AuditSink) *UpdateVallossRuleHandler {
	return &UpdateVallossRuleHandler{repo: repo, audit: audit, now: time.Now}
}

// Handle updates the rule. ErrRuleNotFound for an unknown id,
// ErrRuleInactive for a soft-deleted rule. An update that changes nothing
// is a no-op: nothing is written and no audit row is emitted.
func (h *UpdateVallossRuleHandler) Handle(ctx context.Context, cmd UpdateVallossRuleCommand) (*domain.VallossRule, error) {
	basis, err := domain.ParseBasis(cmd.Basis)
	if err != nil {
		return nil, err
	}
	valLoss, err := parseValLoss(cmd.ValLoss)
	if err != nil {
		return nil, err
	}
	rule, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if !rule.IsActive() {
		return nil, domain.ErrRuleInactive
	}
	before := vallossSnapshotOf(rule)
	if rule.Basis() == basis && rule.ValLoss().Equal(valLoss) {
		return rule, nil
	}
	if err := rule.Update(basis, valLoss, cmd.User, h.now().UTC()); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityVallossRule,
		EntityID:   rule.ID(),
		Operation:  auditdomain.OpRuleUpdate,
		BeforeData: snapshotJSON(before),
		AfterData:  snapshotJSON(vallossSnapshotOf(rule)),
		UserID:     rule.UpdatedBy(),
	})
	return rule, nil
}
