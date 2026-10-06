package erpintegration

// grade_group_apply.go implements P0-T15b: re-apply the grade-group seed
// after the master sync. It fills NULL ceg_grade_group values only and is
// idempotent; the audit event is ERP_GRADE_GROUP_APPLY.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Audit identifiers for the grade-group apply.
const (
	AuditEventGradeGroupApply = "ERP_GRADE_GROUP_APPLY"
	auditEntityErpGrade       = "cost_erp_grade"
	systemActor               = "system"
)

// AuditSink is the subset of the cost audit emitter used here.
type AuditSink interface {
	Emit(ctx context.Context, in auditdomain.NewInput) error
}

// GradeGroupApplyHandler re-applies cst_erp_grade_group_seed.
type GradeGroupApplyHandler struct {
	repo  domain.MasterRepository
	audit AuditSink
}

// NewGradeGroupApplyHandler builds the handler; audit may be nil.
func NewGradeGroupApplyHandler(repo domain.MasterRepository, audit AuditSink) *GradeGroupApplyHandler {
	return &GradeGroupApplyHandler{repo: repo, audit: audit}
}

// gradeGroupAuditData is written to cal_after_data.
type gradeGroupAuditData struct {
	EventType      string   `json:"event_type"`
	SeedRows       int64    `json:"seed_rows"`
	AppliedNow     int64    `json:"applied_now"`
	AlreadyGrouped int64    `json:"already_grouped"`
	MissingCodes   []string `json:"missing_codes,omitempty"`
}

// Handle applies the seed and emits the audit row (best effort: an audit
// failure does not undo or fail the apply).
func (h *GradeGroupApplyHandler) Handle(ctx context.Context, actor string) (domain.GradeGroupApplyReport, error) {
	if h == nil || h.repo == nil {
		return domain.GradeGroupApplyReport{}, domain.ErrMasterSyncNotConfigured
	}
	rep, err := h.repo.ApplyGradeGroupSeed(ctx)
	if err != nil {
		return rep, fmt.Errorf("erp grade-group apply: %w", err)
	}
	h.emitAudit(ctx, actor, rep)
	return rep, nil
}

func (h *GradeGroupApplyHandler) emitAudit(ctx context.Context, actor string, rep domain.GradeGroupApplyReport) {
	if h.audit == nil {
		return
	}
	if actor == "" {
		actor = systemActor
	}
	data, err := json.Marshal(gradeGroupAuditData{
		EventType:      AuditEventGradeGroupApply,
		SeedRows:       rep.SeedRows,
		AppliedNow:     rep.AppliedNow,
		AlreadyGrouped: rep.AlreadyGrouped,
		MissingCodes:   rep.MissingCodes,
	})
	if err != nil {
		log.Warn().Err(err).Msg("erp grade-group apply: marshal audit payload")
		return
	}
	in := auditdomain.NewInput{
		EntityType: auditEntityErpGrade,
		Operation:  auditdomain.OpUpdate,
		AfterData:  string(data),
		UserID:     actor,
	}
	if err := h.audit.Emit(ctx, in); err != nil {
		// Best effort: the apply has already committed.
		log.Warn().Err(err).Str("event", AuditEventGradeGroupApply).Msg("erp grade-group apply: audit emit failed")
	}
}
