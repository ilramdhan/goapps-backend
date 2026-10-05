package costproductmaster

import (
	"context"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// UpdateErpAttributesCommand edits the optional 000551 ERP attributes of one
// product (P3-T5, design §4.4). A nil field is unchanged, a pointer to ""
// clears the column. The existing Create/Update commands do not carry these
// fields and behave exactly as before.
type UpdateErpAttributesCommand struct {
	ProductSysID int64
	Patch        domain.ErpAttributesPatch
	ActorUserID  string
}

// UpdateErpAttributesHandler writes the ERP attributes and audits the change
// (cal_operation UPDATE, entity cost_product_master).
type UpdateErpAttributesHandler struct {
	repo     domain.Repository
	linkRepo domain.ErpLinkRepository
	audit    AuditSink
	now      func() time.Time
}

// NewUpdateErpAttributesHandler constructs the handler; audit may be nil.
func NewUpdateErpAttributesHandler(repo domain.Repository, linkRepo domain.ErpLinkRepository, audit AuditSink) *UpdateErpAttributesHandler {
	return &UpdateErpAttributesHandler{repo: repo, linkRepo: linkRepo, audit: audit, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *UpdateErpAttributesHandler) WithClock(now func() time.Time) *UpdateErpAttributesHandler {
	h.now = now
	return h
}

// Handle executes the update. An empty or no-op patch writes nothing.
func (h *UpdateErpAttributesHandler) Handle(ctx context.Context, cmd UpdateErpAttributesCommand) (*domain.CostProductMaster, error) {
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	if cmd.Patch.IsEmpty() {
		return p, nil
	}
	before := attrSnapshotOf(p.ErpAttributes())
	changed, err := p.ApplyErpAttributes(cmd.Patch, cmd.ActorUserID, h.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		return p, nil
	}
	if err := h.linkRepo.SaveErpAttributes(ctx, domain.ErpAttributesWrite{
		ProductSysID: p.ProductSysID(),
		Attributes:   p.ErpAttributes(),
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
		AfterData:  marshalAudit(attrSnapshotOf(p.ErpAttributes())),
		UserID:     cmd.ActorUserID,
	})
	return p, nil
}

// attrSnapshot is the audit JSON of the ERP attributes (null = NULL).
type attrSnapshot struct {
	FgType      *string `json:"erp_fg_type"`
	ChpItemCode *string `json:"erp_chp_item_code"`
	MsBatchItem *string `json:"erp_ms_batch_item"`
	ItemType    *string `json:"erp_item_type"`
	PrdPerDay   *string `json:"erp_prd_per_day"`
}

func attrSnapshotOf(a domain.ErpAttributes) attrSnapshot {
	s := attrSnapshot{
		FgType: strPtrOrNil(a.FgType), ChpItemCode: strPtrOrNil(a.ChpItemCode),
		MsBatchItem: strPtrOrNil(a.MsBatchItem), ItemType: strPtrOrNil(a.ItemType),
	}
	if a.PrdPerDay.Valid {
		v := a.PrdPerDay.Decimal.String()
		s.PrdPerDay = &v
	}
	return s
}

func strPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
