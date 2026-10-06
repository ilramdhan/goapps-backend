package erpintegration

// master_sync.go implements P0-T15: the read-only ERP master sync
// (OM_ITEM -> cost_erp_item, OM_GRADE_CODE_1 -> cost_erp_grade). Oracle is
// only ever read (SELECT via the read-only querier); the grade-group seed
// re-apply (P0-T15b) runs after the grade upsert has committed.

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// Master sync job type and subtypes (queue finance.jobs.erp_master_sync).
const (
	JobTypeErpMasterSync = string(job.TypeErpMasterSync)

	MasterSubtypeAll              = ""
	MasterSubtypeOMItem           = "om_item"
	MasterSubtypeOMGrade          = "om_grade"
	MasterSubtypeApplyGradeGroups = "apply_grade_groups"
)

// MasterSyncResult is the report of one master sync run.
type MasterSyncResult struct {
	Items       *domain.UpsertCounts
	Grades      *domain.UpsertCounts
	GradeGroups *domain.GradeGroupApplyReport
}

// MasterSyncHandler runs the ERP master sync.
type MasterSyncHandler struct {
	reader domain.MasterReader
	repo   domain.MasterRepository
	apply  *GradeGroupApplyHandler
}

// NewMasterSyncHandler builds the handler. A nil reader or repo makes every
// run fail with ErrMasterSyncNotConfigured. apply may be nil (no audit).
func NewMasterSyncHandler(reader domain.MasterReader, repo domain.MasterRepository, apply *GradeGroupApplyHandler) *MasterSyncHandler {
	if apply == nil && repo != nil {
		apply = NewGradeGroupApplyHandler(repo, nil)
	}
	return &MasterSyncHandler{reader: reader, repo: repo, apply: apply}
}

// Handle runs the requested subtype. The empty subtype runs items, grades
// and then the grade-group apply; om_grade also re-applies the seed.
func (h *MasterSyncHandler) Handle(ctx context.Context, subtype, actor string) (MasterSyncResult, error) {
	var res MasterSyncResult
	if h == nil || h.repo == nil {
		return res, domain.ErrMasterSyncNotConfigured
	}
	switch subtype {
	case MasterSubtypeAll:
		if err := h.syncItems(ctx, &res); err != nil {
			return res, err
		}
		return res, h.syncGradesAndApply(ctx, actor, &res)
	case MasterSubtypeOMItem:
		return res, h.syncItems(ctx, &res)
	case MasterSubtypeOMGrade:
		return res, h.syncGradesAndApply(ctx, actor, &res)
	case MasterSubtypeApplyGradeGroups:
		return res, h.applyGroups(ctx, actor, &res)
	default:
		return res, fmt.Errorf("%w: %q", domain.ErrInvalidMasterSubtype, subtype)
	}
}

func (h *MasterSyncHandler) syncItems(ctx context.Context, res *MasterSyncResult) error {
	if h.reader == nil {
		return domain.ErrMasterSyncNotConfigured
	}
	items, err := h.reader.ListItems(ctx)
	if err != nil {
		return fmt.Errorf("erp master sync: read items: %w", err)
	}
	counts, err := h.repo.UpsertItems(ctx, items)
	if err != nil {
		return fmt.Errorf("erp master sync: upsert items: %w", err)
	}
	res.Items = &counts
	return nil
}

// syncGradesAndApply upserts grades (commit) and then re-applies the seed as
// a separate statement.
func (h *MasterSyncHandler) syncGradesAndApply(ctx context.Context, actor string, res *MasterSyncResult) error {
	if h.reader == nil {
		return domain.ErrMasterSyncNotConfigured
	}
	grades, err := h.reader.ListGrades(ctx)
	if err != nil {
		return fmt.Errorf("erp master sync: read grades: %w", err)
	}
	counts, err := h.repo.UpsertGrades(ctx, grades)
	if err != nil {
		return fmt.Errorf("erp master sync: upsert grades: %w", err)
	}
	res.Grades = &counts
	return h.applyGroups(ctx, actor, res)
}

func (h *MasterSyncHandler) applyGroups(ctx context.Context, actor string, res *MasterSyncResult) error {
	rep, err := h.apply.Handle(ctx, actor)
	if err != nil {
		return err
	}
	res.GradeGroups = &rep
	return nil
}
