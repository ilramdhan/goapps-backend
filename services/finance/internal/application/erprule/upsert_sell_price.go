package erprule

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// UpsertSellPriceCommand sets the reference selling price of a basis
// (UpsertSellPrice, PUT /sell-prices/{basis}). Price is a decimal string.
type UpsertSellPriceCommand struct {
	Basis string
	Price string
	User  string
}

// UpsertSellPriceHandler inserts or updates a sell price. Audit: RULE_CREATE
// for a new basis row, RULE_UPDATE for a change (an inactive row is
// re-activated by the update, per the domain upsert semantics).
type UpsertSellPriceHandler struct {
	repo  domain.SellPriceRepository
	audit AuditSink
	now   func() time.Time
}

// NewUpsertSellPriceHandler builds the handler; audit may be nil.
func NewUpsertSellPriceHandler(repo domain.SellPriceRepository, audit AuditSink) *UpsertSellPriceHandler {
	return &UpsertSellPriceHandler{repo: repo, audit: audit, now: time.Now}
}

// Handle upserts the price. An unchanged active price is a no-op: nothing is
// written and no audit row is emitted.
func (h *UpsertSellPriceHandler) Handle(ctx context.Context, cmd UpsertSellPriceCommand) (*domain.SellPrice, error) {
	basis, err := domain.ParseSellPriceBasis(cmd.Basis)
	if err != nil {
		return nil, err
	}
	price, err := parsePrice(cmd.Price)
	if err != nil {
		return nil, err
	}
	now := h.now().UTC()

	current, err := h.repo.Get(ctx, basis)
	switch {
	case errors.Is(err, domain.ErrSellPriceNotFound):
		return h.create(ctx, basis, price, cmd.User, now)
	case err != nil:
		return nil, err
	}

	if current.IsActive() && current.Price().Equal(price) {
		return current, nil
	}
	before := sellPriceSnapshotOf(current)
	if err := current.UpdatePrice(price, cmd.User, now); err != nil {
		return nil, err
	}
	if err := h.repo.Upsert(ctx, current); err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntitySellPrice,
		EntityID:   SellPriceAuditID(basis),
		Operation:  auditdomain.OpRuleUpdate,
		BeforeData: snapshotJSON(before),
		AfterData:  snapshotJSON(sellPriceSnapshotOf(current)),
		UserID:     current.UpdatedBy(),
	})
	return current, nil
}

func (h *UpsertSellPriceHandler) create(
	ctx context.Context, basis domain.Basis, price decimal.Decimal, user string, now time.Time,
) (*domain.SellPrice, error) {
	sp, err := domain.NewSellPrice(basis, price, user, now)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Upsert(ctx, sp); err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntitySellPrice,
		EntityID:   SellPriceAuditID(basis),
		Operation:  auditdomain.OpRuleCreate,
		AfterData:  snapshotJSON(sellPriceSnapshotOf(sp)),
		UserID:     sp.CreatedBy(),
	})
	return sp, nil
}
