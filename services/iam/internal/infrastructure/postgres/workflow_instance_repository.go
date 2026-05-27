package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowinstance"
)

// WorkflowInstanceRepository persists wfl_workflow_instance + wfl_workflow_instance_step rows.
type WorkflowInstanceRepository struct {
	db *DB
}

// NewWorkflowInstanceRepository constructs the repository.
func NewWorkflowInstanceRepository(db *DB) *WorkflowInstanceRepository {
	return &WorkflowInstanceRepository{db: db}
}

var _ workflowinstance.Repository = (*WorkflowInstanceRepository)(nil)

// Create persists instance + first step atomically.
func (r *WorkflowInstanceRepository) Create(ctx context.Context, ins *workflowinstance.Instance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackOnError(tx)

	const insertIns = `
		INSERT INTO wfl_workflow_instance (
			instance_id, template_id, template_version, entity_kind, entity_id,
			current_step_no, status, started_at, started_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	if _, err := tx.ExecContext(ctx, insertIns,
		ins.ID(), ins.TemplateID(), ins.TemplateVersion(),
		ins.EntityKind(), ins.EntityID(),
		ins.CurrentStepNo(), ins.Status(), ins.StartedAt(), ins.StartedBy(),
	); err != nil {
		return fmt.Errorf("insert instance: %w", err)
	}

	for _, s := range ins.Steps() {
		if err := insertInstanceStep(ctx, tx, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetByID loads instance + steps.
func (r *WorkflowInstanceRepository) GetByID(ctx context.Context, id uuid.UUID) (*workflowinstance.Instance, error) {
	const q = `
		SELECT i.instance_id, i.template_id, i.template_version, t.kind,
		       i.entity_kind, i.entity_id, i.current_step_no, i.status,
		       i.started_at, i.started_by, i.completed_at,
		       (SELECT COUNT(*) FROM wfl_workflow_template_step s WHERE s.template_id = i.template_id)::int
		FROM wfl_workflow_instance i
		JOIN wfl_workflow_template t ON t.template_id = i.template_id
		WHERE i.instance_id = $1
	`
	row := r.db.QueryRowContext(ctx, q, id)
	var (
		insID, tplID, entID                        uuid.UUID
		tplVersion, currentStepNo, totalStepsInTpl int
		kind, entityKind, status, startedBy        string
		startedAt                                  time.Time
		completedAt                                sql.NullTime
	)
	if err := row.Scan(&insID, &tplID, &tplVersion, &kind,
		&entityKind, &entID, &currentStepNo, &status,
		&startedAt, &startedBy, &completedAt, &totalStepsInTpl,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workflowinstance.ErrNotFound
		}
		return nil, fmt.Errorf("scan instance: %w", err)
	}

	steps, err := r.loadInstanceSteps(ctx, id)
	if err != nil {
		return nil, err
	}

	var completedPtr *time.Time
	if completedAt.Valid {
		completedPtr = &completedAt.Time
	}
	return workflowinstance.Reconstruct(
		insID, tplID, tplVersion, kind, entityKind, entID,
		currentStepNo, status, startedAt, startedBy, completedPtr, steps, totalStepsInTpl,
	), nil
}

// SaveTransition writes instance state + step changes inside one tx.
// Strategy: UPDATE instance row, UPDATE every step row that has a decision set
// AND ON CONFLICT INSERT any newly-appended step rows.
func (r *WorkflowInstanceRepository) SaveTransition(ctx context.Context, ins *workflowinstance.Instance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackOnError(tx)

	if _, err := tx.ExecContext(ctx,
		`UPDATE wfl_workflow_instance
		 SET current_step_no = $2, status = $3, completed_at = $4
		 WHERE instance_id = $1`,
		ins.ID(), ins.CurrentStepNo(), ins.Status(), ins.CompletedAt(),
	); err != nil {
		return fmt.Errorf("update instance: %w", err)
	}

	for _, s := range ins.Steps() {
		if err := upsertInstanceStep(ctx, tx, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// List returns paginated instances (steps NOT preloaded).
func (r *WorkflowInstanceRepository) List(ctx context.Context, f workflowinstance.Filter) ([]*workflowinstance.Instance, int64, error) {
	where := `FROM wfl_workflow_instance i JOIN wfl_workflow_template t ON t.template_id = i.template_id WHERE 1=1`
	args := []any{}
	idx := 1
	if f.EntityKind != "" {
		where += fmt.Sprintf(" AND i.entity_kind = $%d", idx)
		args = append(args, f.EntityKind)
		idx++
	}
	if f.EntityID != "" {
		where += fmt.Sprintf(" AND i.entity_id = $%d", idx)
		args = append(args, f.EntityID)
		idx++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND i.status = $%d", idx)
		args = append(args, f.Status)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count instances: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	pageSize = min(pageSize, 100)
	offset := (page - 1) * pageSize

	q := `
		SELECT i.instance_id, i.template_id, i.template_version, t.kind,
		       i.entity_kind, i.entity_id, i.current_step_no, i.status,
		       i.started_at, i.started_by, i.completed_at,
		       (SELECT COUNT(*) FROM wfl_workflow_template_step s WHERE s.template_id = i.template_id)::int
		` + where + fmt.Sprintf(" ORDER BY i.started_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list instances: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("close instance rows")
		}
	}()

	out := []*workflowinstance.Instance{}
	for rows.Next() {
		var (
			insID, tplID, entID                        uuid.UUID
			tplVersion, currentStepNo, totalStepsInTpl int
			kind, entityKind, status, startedBy        string
			startedAt                                  time.Time
			completedAt                                sql.NullTime
		)
		if err := rows.Scan(&insID, &tplID, &tplVersion, &kind,
			&entityKind, &entID, &currentStepNo, &status,
			&startedAt, &startedBy, &completedAt, &totalStepsInTpl,
		); err != nil {
			return nil, 0, fmt.Errorf("scan instance row: %w", err)
		}
		var completedPtr *time.Time
		if completedAt.Valid {
			completedPtr = &completedAt.Time
		}
		out = append(out, workflowinstance.Reconstruct(
			insID, tplID, tplVersion, kind, entityKind, entID,
			currentStepNo, status, startedAt, startedBy, completedPtr, nil, totalStepsInTpl,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate instances: %w", err)
	}
	return out, total, nil
}

// =============================================================================
// helpers
// =============================================================================

func (r *WorkflowInstanceRepository) loadInstanceSteps(ctx context.Context, instanceID uuid.UUID) ([]workflowinstance.Step, error) {
	const q = `
		SELECT s.instance_step_id, s.instance_id, s.step_no, s.step_name,
		       s.approver_resolution_type, s.approver_resolution_value,
		       COALESCE(s.sla_hours, 0),
		       COALESCE(ts.allow_reject, TRUE),
		       COALESCE(ts.require_password_on_unlock, FALSE),
		       s.assigned_at, s.actor_user_id, COALESCE(s.decision, ''),
		       s.decided_at, COALESCE(s.comment, ''), s.stuck_since
		FROM wfl_workflow_instance_step s
		LEFT JOIN wfl_workflow_template_step ts
		  ON ts.template_id = (SELECT template_id FROM wfl_workflow_instance WHERE instance_id = s.instance_id)
		 AND ts.step_no = s.step_no
		WHERE s.instance_id = $1
		ORDER BY s.step_no ASC
	`
	rows, err := r.db.QueryContext(ctx, q, instanceID)
	if err != nil {
		return nil, fmt.Errorf("load instance steps: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("close step rows")
		}
	}()

	out := []workflowinstance.Step{}
	for rows.Next() {
		var (
			stepID, insID                uuid.UUID
			stepNo, slaHours             int
			stepName, resType, resValue  string
			allowReject, requirePassword bool
			assignedAt                   time.Time
			actorUserID                  uuid.NullUUID
			decision, comment            string
			decidedAt, stuckSince        sql.NullTime
		)
		if err := rows.Scan(&stepID, &insID, &stepNo, &stepName,
			&resType, &resValue, &slaHours,
			&allowReject, &requirePassword,
			&assignedAt, &actorUserID, &decision, &decidedAt, &comment, &stuckSince,
		); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		var actorPtr *uuid.UUID
		if actorUserID.Valid {
			actorPtr = &actorUserID.UUID
		}
		var decidedPtr *time.Time
		if decidedAt.Valid {
			decidedPtr = &decidedAt.Time
		}
		var stuckPtr *time.Time
		if stuckSince.Valid {
			stuckPtr = &stuckSince.Time
		}
		out = append(out, workflowinstance.ReconstructStep(
			stepID, insID, stepNo, stepName, resType, resValue,
			slaHours, allowReject, requirePassword,
			assignedAt, actorPtr, decision, decidedPtr, comment, stuckPtr,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate steps: %w", err)
	}
	return out, nil
}

func insertInstanceStep(ctx context.Context, tx *sql.Tx, s workflowinstance.Step) error {
	const q = `
		INSERT INTO wfl_workflow_instance_step (
			instance_step_id, instance_id, step_no, step_name,
			approver_resolution_type, approver_resolution_value,
			sla_hours, assigned_at, actor_user_id, decision, decided_at, comment, stuck_since
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	var slaArg sql.NullInt32
	if s.SLAHours() > 0 {
		slaArg = sql.NullInt32{Int32: safeIntToInt32WfIns(s.SLAHours()), Valid: true}
	}
	var decisionArg sql.NullString
	if s.Decision() != "" {
		decisionArg = sql.NullString{String: s.Decision(), Valid: true}
	}
	_, err := tx.ExecContext(ctx, q,
		s.ID(), s.InstanceID(), s.StepNo(), s.StepName(),
		s.ApproverResolutionType(), s.ApproverResolutionValue(),
		slaArg, s.AssignedAt(), nullableUUID(s.ActorUserID()),
		decisionArg, nullableTime(s.DecidedAt()), s.Comment(), nullableTime(s.StuckSince()),
	)
	if err != nil {
		return fmt.Errorf("insert instance step: %w", err)
	}
	return nil
}

// upsertInstanceStep inserts a fresh step row or updates the existing one.
// We rely on (instance_id, step_no) being unique via uk_wfl_instance_step_no.
func upsertInstanceStep(ctx context.Context, tx *sql.Tx, s workflowinstance.Step) error {
	const q = `
		INSERT INTO wfl_workflow_instance_step (
			instance_step_id, instance_id, step_no, step_name,
			approver_resolution_type, approver_resolution_value,
			sla_hours, assigned_at, actor_user_id, decision, decided_at, comment, stuck_since
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (instance_id, step_no)
		DO UPDATE SET
			actor_user_id = EXCLUDED.actor_user_id,
			decision      = EXCLUDED.decision,
			decided_at    = EXCLUDED.decided_at,
			comment       = EXCLUDED.comment,
			stuck_since   = EXCLUDED.stuck_since
	`
	var slaArg sql.NullInt32
	if s.SLAHours() > 0 {
		slaArg = sql.NullInt32{Int32: safeIntToInt32WfIns(s.SLAHours()), Valid: true}
	}
	var decisionArg sql.NullString
	if s.Decision() != "" {
		decisionArg = sql.NullString{String: s.Decision(), Valid: true}
	}
	_, err := tx.ExecContext(ctx, q,
		s.ID(), s.InstanceID(), s.StepNo(), s.StepName(),
		s.ApproverResolutionType(), s.ApproverResolutionValue(),
		slaArg, s.AssignedAt(), nullableUUID(s.ActorUserID()),
		decisionArg, nullableTime(s.DecidedAt()), s.Comment(), nullableTime(s.StuckSince()),
	)
	if err != nil {
		return fmt.Errorf("upsert instance step: %w", err)
	}
	return nil
}

func nullableUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func safeIntToInt32WfIns(v int) int32 {
	const maxInt32 = 1<<31 - 1
	if v > maxInt32 {
		return maxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v) //nolint:gosec // bounds checked above
}
