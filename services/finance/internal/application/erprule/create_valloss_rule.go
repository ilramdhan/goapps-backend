package erprule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// CreateVallossRuleCommand creates an active valloss rule. ValLoss is a
// decimal string (the proto carries decimals as strings, design §8.3).
type CreateVallossRuleCommand struct {
	FgType     string
	ProdType   string
	GradeGroup string
	Basis      string
	ValLoss    string
	User       string
}

// CreateVallossRuleHandler creates a rule. Audit: RULE_CREATE.
type CreateVallossRuleHandler struct {
	repo  domain.VallossRuleRepository
	audit AuditSink
	now   func() time.Time
}

// NewCreateVallossRuleHandler builds the handler; audit may be nil.
func NewCreateVallossRuleHandler(repo domain.VallossRuleRepository, audit AuditSink) *CreateVallossRuleHandler {
	return &CreateVallossRuleHandler{repo: repo, audit: audit, now: time.Now}
}

// Handle validates and inserts the rule. A clash with an active rule of the
// same key returns domain.ErrDuplicateRule (from the repository).
func (h *CreateVallossRuleHandler) Handle(ctx context.Context, cmd CreateVallossRuleCommand) (*domain.VallossRule, error) {
	key, err := domain.NewRuleKey(cmd.FgType, cmd.ProdType, cmd.GradeGroup)
	if err != nil {
		return nil, err
	}
	basis, err := domain.ParseBasis(cmd.Basis)
	if err != nil {
		return nil, err
	}
	valLoss, err := parseValLoss(cmd.ValLoss)
	if err != nil {
		return nil, err
	}
	rule, err := domain.NewVallossRule(key, basis, valLoss, cmd.User, h.now().UTC())
	if err != nil {
		return nil, err
	}
	created, err := h.repo.Create(ctx, rule)
	if err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityVallossRule,
		EntityID:   created.ID(),
		Operation:  auditdomain.OpRuleCreate,
		AfterData:  snapshotJSON(vallossSnapshotOf(created)),
		UserID:     created.CreatedBy(),
	})
	return created, nil
}

// parseValLoss parses a decimal string. An unparsable value is reported as
// ErrInvalidPercent; the range and scale are checked by the domain.
func parseValLoss(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%w: %q", domain.ErrInvalidPercent, s)
	}
	return d, nil
}

// parsePrice parses a decimal string. An unparsable value is reported as
// ErrInvalidPrice; the range and scale are checked by the domain.
func parsePrice(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%w: %q", domain.ErrInvalidPrice, s)
	}
	return d, nil
}
