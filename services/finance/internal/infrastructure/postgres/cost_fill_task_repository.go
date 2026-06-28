package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CostFillTaskRepository implements domain.TaskRepository.
type CostFillTaskRepository struct{ db *DB }

// NewCostFillTaskRepository constructs the repo.
func NewCostFillTaskRepository(db *DB) *CostFillTaskRepository {
	return &CostFillTaskRepository{db: db}
}

var _ domain.TaskRepository = (*CostFillTaskRepository)(nil)

// cftCols is used in RETURNING clauses (correlated subqueries are not allowed there).
const cftCols = `
	cft_task_id, cft_request_id, cft_route_head_id, cft_route_level,
	cft_filler_type, cft_filler_value,
	COALESCE(cft_approver_type,''), COALESCE(cft_approver_value,''),
	cft_status, COALESCE(cft_claimed_by,''),
	cft_reapprove_on_change, cft_sla_fill_hours, cft_sla_approve_hours,
	cft_total_params, cft_filled_params, cft_activated_at, cft_filled_at`

// cftSelectCols is used in SELECT queries. It computes cft_filled_params dynamically
// by counting cost_product_parameter rows for non-CALCULATED applicable params at this
// task's route level. CALCULATED params are excluded because they are computed by the
// calc engine and must not block fill submission.
// Queries using this constant MUST alias the table as "t".
const cftSelectCols = `
	t.cft_task_id, t.cft_request_id, t.cft_route_head_id, t.cft_route_level,
	t.cft_filler_type, t.cft_filler_value,
	COALESCE(t.cft_approver_type,''), COALESCE(t.cft_approver_value,''),
	t.cft_status, COALESCE(t.cft_claimed_by,''),
	t.cft_reapprove_on_change, t.cft_sla_fill_hours, t.cft_sla_approve_hours,
	t.cft_total_params,
	(
	    SELECT COUNT(*)::int4
	      FROM cost_product_applicable_param ca
	      JOIN cost_route_seq crs
	        ON crs.crs_product_sys_id = ca.capp_product_sys_id
	       AND crs.crs_head_id = t.cft_route_head_id
	       AND crs.crs_route_level = t.cft_route_level
	      JOIN mst_parameter mp ON mp.id = ca.capp_param_id
	                            AND mp.param_category != 'CALCULATED'
	     WHERE EXISTS (
	           SELECT 1 FROM cost_product_parameter cpp
	           WHERE cpp.cpp_product_sys_id = ca.capp_product_sys_id
	             AND cpp.cpp_param_id = ca.capp_param_id
	     )
	) AS cft_filled_params,
	t.cft_activated_at, t.cft_filled_at`

// BulkInsert creates all fill tasks for a request in a single transaction.
func (r *CostFillTaskRepository) BulkInsert(ctx context.Context, tasks []*domain.Task) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bulk insert tasks: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("rollback bulk insert tasks")
		}
	}()
	for _, t := range tasks {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO cost_fill_task
			   (cft_request_id, cft_route_head_id, cft_route_level,
			    cft_filler_type, cft_filler_value,
			    cft_approver_type, cft_approver_value,
			    cft_reapprove_on_change, cft_sla_fill_hours, cft_sla_approve_hours,
			    cft_status, cft_total_params)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			t.RequestID, t.RouteHeadID, t.RouteLevel,
			t.FillerType, t.FillerValue,
			nullableStr(t.ApproverType), nullableStr(t.ApproverValue),
			t.ReapproveOnChange, t.SLAFillHours, t.SLAApproveHours,
			t.Status(), t.TotalParams,
		); err != nil {
			return fmt.Errorf("insert task level %d: %w", t.RouteLevel, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit bulk insert tasks: %w", err)
	}
	return nil
}

// GetByID loads a single fill task.
func (r *CostFillTaskRepository) GetByID(ctx context.Context, taskID int64) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_task_id=$1`, taskID)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	return t, err
}

// GetByRequestLevel loads the task for (request, level).
func (r *CostFillTaskRepository) GetByRequestLevel(ctx context.Context, requestID int64, routeLevel int32) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_request_id=$1 AND t.cft_route_level=$2`,
		requestID, routeLevel)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	return t, err
}

// ListByRequest returns all tasks for a request ordered by route level desc.
func (r *CostFillTaskRepository) ListByRequest(ctx context.Context, requestID int64) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_request_id=$1
		  ORDER BY t.cft_route_level DESC`, requestID)
	if err != nil {
		return nil, fmt.Errorf("list tasks by request: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	return scanTaskRows(rows)
}

// ListForUser returns tasks assigned to a user (by user ID or dept codes).
func (r *CostFillTaskRepository) ListForUser(ctx context.Context, userID string, deptCodes []string) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE (
		    (t.cft_filler_type='USER' AND t.cft_filler_value=$1)
		    OR (t.cft_filler_type='DEPT' AND t.cft_filler_value = ANY($2))
		    OR (t.cft_approver_type='USER' AND t.cft_approver_value=$1)
		    OR (t.cft_approver_type='DEPT' AND t.cft_approver_value = ANY($2))
		  )
		  AND t.cft_status <> 'APPROVED'
		  ORDER BY t.cft_activated_at DESC`,
		userID, pq.Array(deptCodes))
	if err != nil {
		return nil, fmt.Errorf("list tasks for user: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	return scanTaskRows(rows)
}

// Claim atomically claims an ACTIVE task. Returns false if already claimed.
func (r *CostFillTaskRepository) Claim(ctx context.Context, taskID int64, userID string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE cost_fill_task
		    SET cft_claimed_by=$1, cft_claimed_at=NOW(), cft_status='FILLING'
		  WHERE cft_task_id=$2
		    AND cft_claimed_by IS NULL
		    AND cft_status='ACTIVE'`,
		userID, taskID)
	if err != nil {
		return false, fmt.Errorf("claim task: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected claim: %w", err)
	}
	return n == 1, nil
}

// Save persists status + counters of a task.
func (r *CostFillTaskRepository) Save(ctx context.Context, t *domain.Task) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cost_fill_task
		    SET cft_status=$1,
		        cft_filled_params=$2,
		        cft_filled_at=$3
		  WHERE cft_task_id=$4`,
		t.Status(), t.FilledParams, t.FilledAt, t.TaskID)
	if err != nil {
		return fmt.Errorf("save task %d: %w", t.TaskID, err)
	}
	return nil
}

// IncrementFilled bumps filled_params and returns the updated task row.
func (r *CostFillTaskRepository) IncrementFilled(ctx context.Context, requestID int64, routeLevel, delta int32) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx,
		`UPDATE cost_fill_task
		    SET cft_filled_params = LEAST(cft_total_params, cft_filled_params + $3)
		  WHERE cft_request_id=$1 AND cft_route_level=$2
		  RETURNING `+cftCols,
		requestID, routeLevel, delta)
	t, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("increment filled: %w", err)
	}
	return t, nil
}

// CountNonApproved returns tasks for a request that are not APPROVED.
func (r *CostFillTaskRepository) CountNonApproved(ctx context.Context, requestID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cost_fill_task
		  WHERE cft_request_id=$1 AND cft_status <> 'APPROVED'`,
		requestID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count non-approved: %w", err)
	}
	return n, nil
}

// CountNonApprovedBelow returns tasks with route_level < maxLevel that are not APPROVED.
func (r *CostFillTaskRepository) CountNonApprovedBelow(ctx context.Context, requestID int64, maxLevel int32) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cost_fill_task
		  WHERE cft_request_id=$1
		    AND cft_route_level < $2
		    AND cft_status <> 'APPROVED'`,
		requestID, maxLevel).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count non-approved below %d: %w", maxLevel, err)
	}
	return n, nil
}

// CountUnapprovedFillTasksForHead returns the count of fill tasks for all
// requests linked to the given route head that (a) have an approver configured
// and (b) have not yet reached APPROVED status.
// Levels with no approver are excluded — they can be FILLED/ACTIVE and still
// allow the route to lock.
func (r *CostFillTaskRepository) CountUnapprovedFillTasksForHead(ctx context.Context, headID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM cost_fill_task cft
		 JOIN cost_product_request cpr ON cpr.cpr_request_id = cft.cft_request_id
		 WHERE cpr.cpr_linked_route_head_id = $1
		   AND cft.cft_approver_type IS NOT NULL
		   AND cft.cft_status <> 'APPROVED'`,
		headID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unapproved fill tasks for head %d: %w", headID, err)
	}
	return n, nil
}

// ActivateTask sets the status to ACTIVE and stamps activated_at to now.
// Only transitions tasks that are currently INACTIVE (status='INACTIVE').
func (r *CostFillTaskRepository) ActivateTask(ctx context.Context, taskID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cost_fill_task
		    SET cft_status='ACTIVE', cft_activated_at=NOW()
		  WHERE cft_task_id=$1`,
		taskID)
	if err != nil {
		return fmt.Errorf("activate task %d: %w", taskID, err)
	}
	return nil
}

// MarkNotified stamps last_notified_at for a task.
func (r *CostFillTaskRepository) MarkNotified(ctx context.Context, taskID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cost_fill_task SET cft_last_notified_at=NOW() WHERE cft_task_id=$1`,
		taskID)
	if err != nil {
		return fmt.Errorf("mark notified task %d: %w", taskID, err)
	}
	return nil
}

// ListOverdue returns unfinished tasks past SLA whose last reminder is stale.
func (r *CostFillTaskRepository) ListOverdue(ctx context.Context, reminderGapHours int) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_status IN ('ACTIVE','FILLING','APPROVAL_PENDING')
		    AND t.cft_activated_at + (t.cft_sla_fill_hours || ' hours')::interval < NOW()
		    AND (t.cft_last_notified_at IS NULL
		         OR t.cft_last_notified_at < NOW() - ($1 || ' hours')::interval)`,
		reminderGapHours)
	if err != nil {
		return nil, fmt.Errorf("list overdue tasks: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	return scanTaskRows(rows)
}

// ListPendingFill returns ACTIVE or FILLING tasks whose last notify is stale.
// These are tasks waiting for the filler to submit parameter values.
func (r *CostFillTaskRepository) ListPendingFill(ctx context.Context, reminderGapHours int) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_status IN ('ACTIVE','FILLING')
		    AND (t.cft_last_notified_at IS NULL
		         OR t.cft_last_notified_at < NOW() - make_interval(hours => $1))`,
		reminderGapHours)
	if err != nil {
		return nil, fmt.Errorf("list pending fill tasks: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	return scanTaskRows(rows)
}

// ListPendingApproval returns APPROVAL_PENDING tasks whose last notify is stale.
// These are tasks waiting for the approver to approve or reject.
func (r *CostFillTaskRepository) ListPendingApproval(ctx context.Context, reminderGapHours int) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cftSelectCols+`
		   FROM cost_fill_task t
		  WHERE t.cft_status = 'APPROVAL_PENDING'
		    AND (t.cft_last_notified_at IS NULL
		         OR t.cft_last_notified_at < NOW() - make_interval(hours => $1))`,
		reminderGapHours)
	if err != nil {
		return nil, fmt.Errorf("list pending approval tasks: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	return scanTaskRows(rows)
}

// AddApproval records an approval/rejection event.
func (r *CostFillTaskRepository) AddApproval(ctx context.Context, a *domain.Approval) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cost_fill_approval
		   (cfa_task_id, cfa_decision, cfa_decided_by, cfa_note, cfa_trigger)
		 VALUES ($1,$2,$3,$4,$5)`,
		a.TaskID, a.Decision, a.DecidedBy,
		nullableStr(a.Note), a.Trigger)
	if err != nil {
		return fmt.Errorf("add approval: %w", err)
	}
	return nil
}

// ListApprovals returns a task's approval history newest-first.
func (r *CostFillTaskRepository) ListApprovals(ctx context.Context, taskID int64) ([]*domain.Approval, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cfa_approval_id, cfa_task_id, cfa_decision,
		        cfa_decided_by, cfa_decided_at, COALESCE(cfa_note,''), cfa_trigger
		   FROM cost_fill_approval
		  WHERE cfa_task_id=$1
		  ORDER BY cfa_created_at DESC`,
		taskID)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	var out []*domain.Approval
	for rows.Next() {
		a := &domain.Approval{}
		if scanErr := rows.Scan(
			&a.ApprovalID, &a.TaskID, &a.Decision,
			&a.DecidedBy, &a.DecidedAt, &a.Note, &a.Trigger,
		); scanErr != nil {
			return nil, fmt.Errorf("scan approval: %w", scanErr)
		}
		out = append(out, a)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err approvals: %w", err)
	}
	return out, nil
}

// --- helpers ---

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(s taskScanner) (*domain.Task, error) {
	var (
		taskID, requestID, routeHeadID int64
		routeLevel                     int32
		fillerType, fillerValue        string
		approverType, approverValue    string
		status, claimedBy              string
		reapprove                      bool
		slaFill, slaApprove            int32
		total, filled                  int32
		activatedAt                    time.Time
		filledAt                       *time.Time
	)
	if err := s.Scan(
		&taskID, &requestID, &routeHeadID, &routeLevel,
		&fillerType, &fillerValue,
		&approverType, &approverValue,
		&status, &claimedBy,
		&reapprove, &slaFill, &slaApprove,
		&total, &filled, &activatedAt, &filledAt,
	); err != nil {
		return nil, err
	}
	return domain.Hydrate(
		taskID, requestID, routeHeadID, routeLevel,
		fillerType, fillerValue, approverType, approverValue,
		status, claimedBy,
		reapprove, slaFill, slaApprove, total, filled, activatedAt, filledAt,
	), nil
}

func scanTaskRows(rows *sql.Rows) ([]*domain.Task, error) {
	var out []*domain.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err scan tasks: %w", err)
	}
	return out, nil
}

func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// ResetFillTaskApprovalIfNeeded implements cppapp.FillTaskApprovalResetter.
// It resets the fill task status for the given (requestID, routeLevel) from APPROVED back to
// APPROVAL_PENDING, but ONLY when the task has an approver configured (cft_approver_type IS NOT NULL).
// When no approver is configured, the edit is allowed without re-approval, so no change is made.
func (r *CostFillTaskRepository) ResetFillTaskApprovalIfNeeded(ctx context.Context, requestID int64, routeLevel int) error {
	const q = `
UPDATE cost_fill_task
SET    cft_status     = 'APPROVAL_PENDING',
       cft_updated_at = NOW()
WHERE  cft_request_id  = $1
  AND  cft_route_level = $2
  AND  cft_approver_type IS NOT NULL
  AND  cft_status = 'APPROVED'`

	if _, err := r.db.ExecContext(ctx, q, requestID, routeLevel); err != nil {
		return fmt.Errorf("reset fill task approval: %w", err)
	}
	return nil
}
