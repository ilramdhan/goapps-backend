// Package erprule holds the application use cases of the Finance-owned ERP
// rule master (design Part 2 §8.2, §9.2; plan-03 P2-T4): valloss rule CRUD
// (soft delete), sell price upsert, grade-group assignment, the listings
// (including the V-08 unassigned-grade worklist) and the xlsx export.
//
// Every mutation emits one cost_audit_log row with the before and after
// state (RULE_CREATE / RULE_UPDATE / RULE_DELETE). Permission checks
// (finance.cost.erprule.*) belong to the delivery layer (P6).
package erprule

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// Audit entity types (cal_entity_type).
//
// cal_entity_id is BIGINT, so the natural keys that are not integers are
// mapped: a sell price uses SellPriceAuditID(basis) (SPPTY=1, SPITY=2,
// SPBSD=3), and a grade uses 0 because the domain Grade carries no numeric
// id; the grade code is always in the before/after JSON.
const (
	AuditEntityVallossRule = "cst_erp_valloss_rule"
	AuditEntitySellPrice   = "cst_erp_sell_price"
	AuditEntityGradeGroup  = "cost_erp_grade"
)

// AuditSink is the subset of the cost audit emitter (costauditlog.Emitter)
// used by this package.
type AuditSink interface {
	Emit(ctx context.Context, in auditdomain.NewInput) error
}

// SellPriceAuditID returns the stable cal_entity_id of a sell-price basis.
func SellPriceAuditID(b domain.Basis) int64 {
	switch b {
	case domain.BasisSPPTY:
		return 1
	case domain.BasisSPITY:
		return 2
	case domain.BasisSPBSD:
		return 3
	default:
		return 0
	}
}

// vallossSnapshot is the audit JSON of one valloss rule.
type vallossSnapshot struct {
	ID         int64      `json:"id"`
	FgType     string     `json:"fg_type"`
	ProdType   string     `json:"prod_type"`
	GradeGroup string     `json:"grade_group"`
	Basis      string     `json:"basis"`
	ValLoss    string     `json:"val_loss"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	UpdatedBy  string     `json:"updated_by,omitempty"`
}

func vallossSnapshotOf(r *domain.VallossRule) *vallossSnapshot {
	if r == nil {
		return nil
	}
	k := r.Key()
	return &vallossSnapshot{
		ID: r.ID(), FgType: k.FgType.String(), ProdType: k.ProdType.String(),
		GradeGroup: k.GradeGroup.String(), Basis: r.Basis().String(),
		ValLoss:  r.ValLoss().StringFixed(domain.Scale),
		IsActive: r.IsActive(), CreatedAt: r.CreatedAt(), CreatedBy: r.CreatedBy(),
		UpdatedAt: r.UpdatedAt(), UpdatedBy: r.UpdatedBy(),
	}
}

// sellPriceSnapshot is the audit JSON of one sell price.
type sellPriceSnapshot struct {
	Basis     string     `json:"basis"`
	Price     string     `json:"price"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy string     `json:"created_by"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	UpdatedBy string     `json:"updated_by,omitempty"`
}

func sellPriceSnapshotOf(p *domain.SellPrice) *sellPriceSnapshot {
	if p == nil {
		return nil
	}
	return &sellPriceSnapshot{
		Basis: p.Basis().String(), Price: p.Price().StringFixed(domain.Scale),
		IsActive: p.IsActive(), CreatedAt: p.CreatedAt(), CreatedBy: p.CreatedBy(),
		UpdatedAt: p.UpdatedAt(), UpdatedBy: p.UpdatedBy(),
	}
}

// gradeGroupSnapshot is the audit JSON of one grade-group assignment.
// GradeGroup is null when the grade is unassigned.
type gradeGroupSnapshot struct {
	GradeCode  string  `json:"grade_code"`
	GradeName  string  `json:"grade_name"`
	GradeGroup *string `json:"grade_group"`
}

func gradeGroupSnapshotOf(code, name string, g *domain.GradeGroup) *gradeGroupSnapshot {
	s := &gradeGroupSnapshot{GradeCode: code, GradeName: name}
	if g != nil {
		v := g.String()
		s.GradeGroup = &v
	}
	return s
}

func marshalSnapshot(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Warn().Err(err).Msg("erp rule: marshal audit snapshot")
		return ""
	}
	return string(b)
}

// snapshotJSON returns "" (NULL) for a nil snapshot pointer.
func snapshotJSON[T any](s *T) string {
	if s == nil {
		return ""
	}
	return marshalSnapshot(s)
}

// emitAudit appends one audit row. Best effort, like every other audit call
// in the service: the change has already committed, so an audit failure is
// logged and does not fail the request.
func emitAudit(ctx context.Context, sink AuditSink, in auditdomain.NewInput) {
	if sink == nil {
		return
	}
	if err := sink.Emit(ctx, in); err != nil {
		log.Warn().Err(err).Str("operation", in.Operation).Str("entity", in.EntityType).
			Int64("entity_id", in.EntityID).Msg("erp rule: audit emit failed")
	}
}
