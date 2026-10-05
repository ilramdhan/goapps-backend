package erprule

import (
	"context"
	"strings"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// AssignGradeGroupCommand sets or clears the group of an ERP grade
// (UpdateGradeGroup, PUT /grade-groups/{grade_code}). An empty GradeGroup
// clears it (the grade re-enters the V-08 worklist).
type AssignGradeGroupCommand struct {
	GradeCode  string
	GradeGroup string
	User       string
}

// AssignGradeGroupHandler writes cost_erp_grade.ceg_grade_group only,
// through Grade.AssignGroup / ClearGroup. Audit: RULE_UPDATE with the
// previous and new group (design §8.2: every rule write is RULE_*).
type AssignGradeGroupHandler struct {
	repo  domain.GradeGroupRepository
	audit AuditSink
}

// NewAssignGradeGroupHandler builds the handler; audit may be nil.
func NewAssignGradeGroupHandler(repo domain.GradeGroupRepository, audit AuditSink) *AssignGradeGroupHandler {
	return &AssignGradeGroupHandler{repo: repo, audit: audit}
}

// Handle assigns or clears the group. ErrGradeNotFound for an unknown code,
// ErrInvalidGradeGroup for an unknown group. An unchanged group is a no-op:
// nothing is written and no audit row is emitted.
func (h *AssignGradeGroupHandler) Handle(ctx context.Context, cmd AssignGradeGroupCommand) (*domain.Grade, error) {
	code, err := domain.ValidateGradeCode(cmd.GradeCode)
	if err != nil {
		return nil, err
	}
	var target *domain.GradeGroup
	if strings.TrimSpace(cmd.GradeGroup) != "" {
		g, perr := domain.ParseGradeGroup(cmd.GradeGroup)
		if perr != nil {
			return nil, perr
		}
		target = &g
	}

	grade, err := h.repo.GetGrade(ctx, code)
	if err != nil {
		return nil, err
	}
	if sameGroup(grade.Group(), target) {
		return grade, nil
	}

	var prev *domain.GradeGroup
	if target == nil {
		prev, err = grade.ClearGroup(cmd.User)
	} else {
		prev, err = grade.AssignGroup(*target, cmd.User)
	}
	if err != nil {
		return nil, err
	}
	if err := h.repo.SetGradeGroup(ctx, grade); err != nil {
		return nil, err
	}
	emitAudit(ctx, h.audit, auditdomain.NewInput{
		EntityType: AuditEntityGradeGroup,
		EntityID:   0,
		Operation:  auditdomain.OpRuleUpdate,
		BeforeData: snapshotJSON(gradeGroupSnapshotOf(grade.Code(), grade.Name(), prev)),
		AfterData:  snapshotJSON(gradeGroupSnapshotOf(grade.Code(), grade.Name(), grade.Group())),
		UserID:     grade.AssignedBy(),
	})
	return grade, nil
}

func sameGroup(a, b *domain.GradeGroup) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
