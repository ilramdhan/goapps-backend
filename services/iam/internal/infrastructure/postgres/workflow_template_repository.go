package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowtemplate"
)

// WorkflowTemplateRepository persists workflow_template + workflow_template_step rows.
type WorkflowTemplateRepository struct {
	db *DB
}

// NewWorkflowTemplateRepository constructs a repository instance.
func NewWorkflowTemplateRepository(db *DB) *WorkflowTemplateRepository {
	return &WorkflowTemplateRepository{db: db}
}

var _ workflowtemplate.Repository = (*WorkflowTemplateRepository)(nil)

// Create persists a template + its steps inside a single transaction.
func (r *WorkflowTemplateRepository) Create(ctx context.Context, t *workflowtemplate.Template) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackOnError(tx)

	const insertTpl = `
		INSERT INTO wfl_workflow_template (
			template_id, kind, name, version, is_active, description, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	if _, err := tx.ExecContext(ctx, insertTpl,
		t.ID(), t.Kind().String(), t.Name().String(), t.Version(),
		t.IsActive(), t.Description().String(), t.CreatedAt(), t.CreatedBy(),
	); err != nil {
		return fmt.Errorf("insert template: %w", err)
	}

	if err := insertSteps(ctx, tx, t.ID(), t.Steps(), t.CreatedBy()); err != nil {
		return err
	}
	return tx.Commit()
}

// GetByID loads a template + its steps.
func (r *WorkflowTemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*workflowtemplate.Template, error) {
	t, err := r.loadTemplate(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	steps, err := r.loadSteps(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	return rebuildWithSteps(t, steps)
}

// GetActiveByKind returns the active template for a kind (one row only by uk_wfl_template_kind_active).
func (r *WorkflowTemplateRepository) GetActiveByKind(ctx context.Context, kind string) (*workflowtemplate.Template, error) {
	const q = `
		SELECT template_id, kind, name, version, is_active, description,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM wfl_workflow_template
		WHERE kind = $1 AND is_active = TRUE AND deleted_at IS NULL
		LIMIT 1
	`
	t, err := r.scanTemplate(r.db.QueryRowContext(ctx, q, kind))
	if err != nil {
		return nil, err
	}
	steps, err := r.loadSteps(ctx, r.db, t.ID())
	if err != nil {
		return nil, err
	}
	return rebuildWithSteps(t, steps)
}

// Activate flips is_active TRUE for id and FALSE for sibling kinds in one tx.
func (r *WorkflowTemplateRepository) Activate(ctx context.Context, id uuid.UUID, by string) (*workflowtemplate.Template, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackOnError(tx)

	var kind string
	if err := tx.QueryRowContext(ctx,
		`SELECT kind FROM wfl_workflow_template WHERE template_id = $1 AND deleted_at IS NULL`, id,
	).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workflowtemplate.ErrNotFound
		}
		return nil, fmt.Errorf("lookup kind: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE wfl_workflow_template
		 SET is_active = FALSE, updated_at = NOW(), updated_by = $2
		 WHERE kind = $1 AND is_active = TRUE AND deleted_at IS NULL`,
		kind, by,
	); err != nil {
		return nil, fmt.Errorf("deactivate siblings: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE wfl_workflow_template
		 SET is_active = TRUE, updated_at = NOW(), updated_by = $2
		 WHERE template_id = $1 AND deleted_at IS NULL`,
		id, by,
	); err != nil {
		return nil, fmt.Errorf("activate template: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return r.GetByID(ctx, id)
}

// SoftDelete marks the template deleted.
func (r *WorkflowTemplateRepository) SoftDelete(ctx context.Context, id uuid.UUID, by string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE wfl_workflow_template
		 SET deleted_at = NOW(), deleted_by = $2, is_active = FALSE
		 WHERE template_id = $1 AND deleted_at IS NULL`,
		id, by,
	)
	if err != nil {
		return fmt.Errorf("soft delete template: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return workflowtemplate.ErrNotFound
	}
	return nil
}

// List returns a paginated set of templates (each with their steps preloaded).
func (r *WorkflowTemplateRepository) List(ctx context.Context, f workflowtemplate.Filter) ([]*workflowtemplate.Template, int64, error) { //nolint:gocyclo // filter + sort + pagination builder
	where := "FROM wfl_workflow_template WHERE deleted_at IS NULL"
	args := []any{}
	idx := 1

	if f.Search != "" {
		where += fmt.Sprintf(" AND (LOWER(name) LIKE LOWER($%d) OR LOWER(kind) LIKE LOWER($%d))", idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.Kind != "" {
		where += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, f.Kind)
		idx++
	}
	switch f.ActiveFilter {
	case "active":
		where += " AND is_active = TRUE"
	case "inactive":
		where += " AND is_active = FALSE"
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count templates: %w", err)
	}

	col := "kind"
	switch f.SortBy {
	case "name", "version", "created_at":
		col = f.SortBy
	}
	dir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		dir = "DESC"
	}
	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	pageSize = min(pageSize, 100)
	offset := (page - 1) * pageSize

	q := `
		SELECT template_id, kind, name, version, is_active, description,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		` + where + fmt.Sprintf(" ORDER BY %s %s, version DESC LIMIT $%d OFFSET $%d", col, dir, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list templates: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("close rows")
		}
	}()

	out := []*workflowtemplate.Template{}
	for rows.Next() {
		t, sErr := r.scanTemplateFromRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		steps, lErr := r.loadSteps(ctx, r.db, t.ID())
		if lErr != nil {
			return nil, 0, lErr
		}
		full, rErr := rebuildWithSteps(t, steps)
		if rErr != nil {
			return nil, 0, rErr
		}
		out = append(out, full)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate templates: %w", err)
	}
	return out, total, nil
}

// =============================================================================
// helpers
// =============================================================================

type runner interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func (r *WorkflowTemplateRepository) loadTemplate(ctx context.Context, run runner, id uuid.UUID) (*workflowtemplate.Template, error) {
	const q = `
		SELECT template_id, kind, name, version, is_active, description,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM wfl_workflow_template
		WHERE template_id = $1 AND deleted_at IS NULL
	`
	return r.scanTemplate(run.QueryRowContext(ctx, q, id))
}

func (r *WorkflowTemplateRepository) loadSteps(ctx context.Context, run runner, templateID uuid.UUID) ([]workflowtemplate.Step, error) {
	const q = `
		SELECT template_step_id, template_id, step_no, step_name,
		       approver_resolution_type, approver_resolution_value,
		       COALESCE(sla_hours, 0), allow_reject, allow_reassign,
		       require_password_on_unlock, COALESCE(reject_to_step_no, 0)
		FROM wfl_workflow_template_step
		WHERE template_id = $1
		ORDER BY step_no ASC
	`
	rows, err := run.QueryContext(ctx, q, templateID)
	if err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("close steps rows")
		}
	}()

	out := []workflowtemplate.Step{}
	for rows.Next() {
		var (
			stepID, tplID                                   uuid.UUID
			stepNo, slaHours, rejectToStepNo                int
			stepName, resolutionType, resolutionValue       string
			allowReject, allowReassign, requirePasswordFlag bool
		)
		if err := rows.Scan(&stepID, &tplID, &stepNo, &stepName,
			&resolutionType, &resolutionValue, &slaHours,
			&allowReject, &allowReassign, &requirePasswordFlag, &rejectToStepNo,
		); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		s, rErr := workflowtemplate.ReconstructStep(
			stepID, tplID, stepNo, stepName, resolutionType, resolutionValue,
			slaHours, allowReject, allowReassign, requirePasswordFlag, rejectToStepNo,
		)
		if rErr != nil {
			return nil, fmt.Errorf("rebuild step: %w", rErr)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate steps: %w", err)
	}
	return out, nil
}

func insertSteps(ctx context.Context, tx *sql.Tx, templateID uuid.UUID, steps []workflowtemplate.Step, createdBy string) error {
	const q = `
		INSERT INTO wfl_workflow_template_step (
			template_step_id, template_id, step_no, step_name,
			approver_resolution_type, approver_resolution_value,
			sla_hours, allow_reject, allow_reassign, require_password_on_unlock,
			reject_to_step_no, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), $12)
	`
	for _, s := range steps {
		var sla, rejectTo sql.NullInt32
		if s.SLAHours() > 0 {
			sla = sql.NullInt32{Int32: safeIntToInt32WfTpl(s.SLAHours()), Valid: true}
		}
		if s.RejectToStepNo() > 0 {
			rejectTo = sql.NullInt32{Int32: safeIntToInt32WfTpl(s.RejectToStepNo()), Valid: true}
		}
		if _, err := tx.ExecContext(ctx, q,
			s.ID(), templateID, s.StepNo(), s.StepName(),
			s.ApproverResolutionType().String(), s.ApproverResolutionValue(),
			sla, s.AllowReject(), s.AllowReassign(), s.RequirePasswordOnUnlock(),
			rejectTo, createdBy,
		); err != nil {
			return fmt.Errorf("insert step %d: %w", s.StepNo(), err)
		}
	}
	return nil
}

func (r *WorkflowTemplateRepository) scanTemplate(row *sql.Row) (*workflowtemplate.Template, error) {
	var (
		id                    uuid.UUID
		kind, name, createdBy string
		version               int
		isActive              bool
		description           sql.NullString
		createdAt             time.Time
		updatedAt, deletedAt  sql.NullTime
		updatedBy, deletedBy  sql.NullString
	)
	err := row.Scan(&id, &kind, &name, &version, &isActive, &description,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, workflowtemplate.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}
	return assembleTemplate(id, kind, name, version, isActive, description, createdAt, createdBy,
		updatedAt, updatedBy, deletedAt, deletedBy, nil)
}

func (r *WorkflowTemplateRepository) scanTemplateFromRows(rows *sql.Rows) (*workflowtemplate.Template, error) {
	var (
		id                    uuid.UUID
		kind, name, createdBy string
		version               int
		isActive              bool
		description           sql.NullString
		createdAt             time.Time
		updatedAt, deletedAt  sql.NullTime
		updatedBy, deletedBy  sql.NullString
	)
	err := rows.Scan(&id, &kind, &name, &version, &isActive, &description,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy)
	if err != nil {
		return nil, fmt.Errorf("scan template row: %w", err)
	}
	return assembleTemplate(id, kind, name, version, isActive, description, createdAt, createdBy,
		updatedAt, updatedBy, deletedAt, deletedBy, nil)
}

func assembleTemplate(
	id uuid.UUID, kind, name string, version int, isActive bool,
	description sql.NullString,
	createdAt time.Time, createdBy string,
	updatedAt sql.NullTime, updatedBy sql.NullString,
	deletedAt sql.NullTime, deletedBy sql.NullString,
	steps []workflowtemplate.Step,
) (*workflowtemplate.Template, error) {
	desc := ""
	if description.Valid {
		desc = description.String
	}
	var updAt *time.Time
	if updatedAt.Valid {
		updAt = &updatedAt.Time
	}
	updBy := ""
	if updatedBy.Valid {
		updBy = updatedBy.String
	}
	var delAt *time.Time
	if deletedAt.Valid {
		delAt = &deletedAt.Time
	}
	delBy := ""
	if deletedBy.Valid {
		delBy = deletedBy.String
	}
	return workflowtemplate.Reconstruct(
		id, kind, name, version, isActive, desc, steps,
		createdAt, createdBy, updAt, updBy, delAt, delBy,
	)
}

func rebuildWithSteps(t *workflowtemplate.Template, steps []workflowtemplate.Step) (*workflowtemplate.Template, error) {
	return workflowtemplate.Reconstruct(
		t.ID(), t.Kind().String(), t.Name().String(), t.Version(), t.IsActive(),
		t.Description().String(), steps,
		t.CreatedAt(), t.CreatedBy(), t.UpdatedAt(), t.UpdatedBy(), t.DeletedAt(), t.DeletedBy(),
	)
}

func rollbackOnError(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Warn().Err(err).Msg("rollback tx")
	}
}

// safeIntToInt32WfTpl clamps int to int32 bounds; sla_hours and step_no values
// are tiny in practice, but we still bounds-check to satisfy gosec G115.
func safeIntToInt32WfTpl(v int) int32 {
	const maxInt32 = 1<<31 - 1
	if v > maxInt32 {
		return maxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v) //nolint:gosec // bounds checked above
}
