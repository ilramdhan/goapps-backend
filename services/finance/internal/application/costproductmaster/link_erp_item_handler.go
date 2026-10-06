package costproductmaster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// AuditEntityProductMaster is the cal_entity_type of product-master ERP link
// and attribute changes; cal_entity_id is cpm_product_sys_id.
const AuditEntityProductMaster = "cost_product_master"

// AuditSink is the subset of the cost audit emitter (costauditlog.Emitter)
// used by the ERP link workflow.
type AuditSink interface {
	Emit(ctx context.Context, in auditdomain.NewInput) error
}

// LinkErpItemCommand links (or, with an empty ErpItemCode, unlinks) one
// product to an ERP (item, shade) combo (P3-T5, D-LINK).
type LinkErpItemCommand struct {
	ProductSysID int64
	ErpItemCode  string // "" unlinks
	ErpShadeCode string // OM_GRADE_CODE_2; must equal cpm_shade_code
	ActorUserID  string
}

// LinkErpItemHandler is the D-LINK link workflow. It validates the link
// (active, AX grade, V-12 CMB↔MB, shade equality, V-04 no duplicate), writes
// cpm_erp_item_code only (never cpm_erp_grade_code_1/2) and audits every
// change as ERP_LINK. Permission checks belong to the delivery layer (P6).
type LinkErpItemHandler struct {
	repo     domain.Repository
	linkRepo domain.ErpLinkRepository
	typeRepo cptdomain.Repository
	audit    AuditSink
	now      func() time.Time
}

// NewLinkErpItemHandler constructs the handler. repo, linkRepo and typeRepo
// must not be nil; audit may be nil (no audit rows).
func NewLinkErpItemHandler(repo domain.Repository, linkRepo domain.ErpLinkRepository, typeRepo cptdomain.Repository, audit AuditSink) *LinkErpItemHandler {
	return &LinkErpItemHandler{repo: repo, linkRepo: linkRepo, typeRepo: typeRepo, audit: audit, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *LinkErpItemHandler) WithClock(now func() time.Time) *LinkErpItemHandler {
	h.now = now
	return h
}

// LinkErpItemResult is the outcome of a link command.
type LinkErpItemResult struct {
	Product *domain.CostProductMaster
	Changed bool
}

// Handle executes the link.
func (h *LinkErpItemHandler) Handle(ctx context.Context, cmd LinkErpItemCommand) (LinkErpItemResult, error) {
	item, err := domain.NormalizeErpItemCode(cmd.ErpItemCode)
	if err != nil {
		return LinkErpItemResult{}, err
	}
	p, err := h.repo.GetBySysID(ctx, cmd.ProductSysID)
	if err != nil {
		return LinkErpItemResult{}, err
	}
	if item != "" {
		if err := h.validateLink(ctx, p, item, cmd.ErpShadeCode); err != nil {
			return LinkErpItemResult{}, err
		}
	}
	before := linkSnapshotOf(p)
	if !p.LinkErpItem(item, cmd.ActorUserID, h.now()) {
		return LinkErpItemResult{Product: p}, nil
	}
	if err := h.linkRepo.SaveErpLink(ctx, domain.ErpLinkWrite{
		ProductSysID: p.ProductSysID(),
		PrevItemCode: before.ErpItemCode,
		ShadeKey:     domain.NormalizeShadeKey(p.ShadeCode()),
		NewItemCode:  p.ErpItemCode(),
		LinkedAt:     p.ErpLinkedAt(),
		LinkedBy:     p.ErpLinkedBy(),
		UpdatedAt:    p.UpdatedAt(),
		UpdatedBy:    p.UpdatedBy(),
	}); err != nil {
		return LinkErpItemResult{}, err
	}
	emitProductAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityProductMaster,
		EntityID:   p.ProductSysID(),
		Operation:  auditdomain.OpErpLink,
		BeforeData: marshalAudit(before),
		AfterData:  marshalAudit(linkSnapshotOf(p)),
		UserID:     cmd.ActorUserID,
	})
	return LinkErpItemResult{Product: p, Changed: true}, nil
}

// validateLink runs the pure checks plus the V-04 duplicate lookup.
func (h *LinkErpItemHandler) validateLink(ctx context.Context, p *domain.CostProductMaster, item, erpShade string) error {
	isMB, err := h.isMBProduct(ctx, p.ProductTypeID())
	if err != nil {
		return err
	}
	if err := p.CheckErpLink(item, erpShade, isMB); err != nil {
		return err
	}
	dups, err := h.linkRepo.ListActiveAxByErpKey(ctx, item, domain.NormalizeShadeKey(p.ShadeCode()), p.ProductSysID())
	if err != nil {
		return err
	}
	if len(dups) > 0 {
		return fmt.Errorf("%w: product_sys_id %v", domain.ErrLinkDuplicate, dups)
	}
	return nil
}

func (h *LinkErpItemHandler) isMBProduct(ctx context.Context, typeID int32) (bool, error) {
	pt, err := h.typeRepo.GetByID(ctx, typeID)
	if err != nil {
		if errors.Is(err, cptdomain.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("resolve product type %d: %w", typeID, err)
	}
	return pt.TypeCode() == mbTypeCode, nil
}

// linkSnapshot is the audit JSON of one product's ERP link.
type linkSnapshot struct {
	ProductCode string     `json:"product_code"`
	ErpItemCode string     `json:"erp_item_code"`
	ShadeCode   string     `json:"shade_code"`
	GradeCode   string     `json:"grade_code"`
	LinkedAt    *time.Time `json:"erp_linked_at,omitempty"`
	LinkedBy    string     `json:"erp_linked_by,omitempty"`
}

func linkSnapshotOf(p *domain.CostProductMaster) linkSnapshot {
	var at *time.Time
	if t := p.ErpLinkedAt(); t != nil {
		c := *t
		at = &c
	}
	return linkSnapshot{
		ProductCode: p.ProductCode(), ErpItemCode: p.ErpItemCode(), ShadeCode: p.ShadeCode(),
		GradeCode: p.GradeCode(), LinkedAt: at, LinkedBy: p.ErpLinkedBy(),
	}
}

func marshalAudit(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Warn().Err(err).Msg("cost product master: marshal audit snapshot")
		return ""
	}
	return string(b)
}

// emitProductAudit appends one audit row. Best effort, like every other audit
// call in the service: the change has committed, so a failure is logged only.
func emitProductAudit(ctx context.Context, sink AuditSink, in auditdomain.NewInput) {
	if sink == nil {
		return
	}
	if err := sink.Emit(ctx, in); err != nil {
		log.Warn().Err(err).Str("operation", in.Operation).Int64("entity_id", in.EntityID).
			Msg("cost product master: audit emit failed")
	}
}
