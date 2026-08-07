package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// JobRepository implements job.Repository interface using PostgreSQL.
type JobRepository struct {
	db *DB
}

// NewJobRepository creates a new JobRepository instance.
func NewJobRepository(db *DB) *JobRepository {
	return &JobRepository{db: db}
}

// Verify interface implementation at compile time.
var _ job.Repository = (*JobRepository)(nil)

// Create persists a new job execution and assigns a sequential code.
func (r *JobRepository) Create(ctx context.Context, exec *job.Execution) error {
	seq, err := r.GetNextSequence(ctx, exec.JobType(), exec.Period())
	if err != nil {
		return fmt.Errorf("get next sequence: %w", err)
	}

	code := job.GenerateCode(exec.JobType(), exec.Period(), seq)
	exec.SetCode(code)

	query := `
		INSERT INTO job_execution (
			job_id, job_code, job_type, job_subtype, period, status, priority,
			params, progress, retry_count, max_retries, queued_at, created_by,
			jex_parent_job_id, jex_total_children
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err = r.db.ExecContext(ctx, query,
		exec.ID(),
		exec.Code().String(),
		exec.JobType().String(),
		nullString(exec.Subtype()),
		nullString(exec.Period()),
		exec.Status().String(),
		exec.Priority(),
		nullJSON(exec.Params()),
		exec.Progress(),
		exec.RetryCount(),
		exec.MaxRetries(),
		exec.QueuedAt(),
		exec.CreatedBy(),
		nullUUID(exec.ParentJobID()),
		exec.TotalChildren(),
	)
	if err != nil {
		if isDuplicateActiveJob(err) {
			return job.ErrDuplicateActiveJob
		}
		return fmt.Errorf("create job execution: %w", err)
	}

	return nil
}

// CreateChildren persists N child job executions in a single transaction,
// each referencing parentJobID and each assigned its own sequential code.
// Mirrors Create per row so children behave identically to standalone jobs
// once persisted (same status lifecycle, same worker handling).
func (r *JobRepository) CreateChildren(ctx context.Context, execs []*job.Execution) (err error) {
	if len(execs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create children tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback create children: %w", rbErr))
			}
			return
		}
		err = tx.Commit()
	}()

	const insertQuery = `
		INSERT INTO job_execution (
			job_id, job_code, job_type, job_subtype, period, status, priority,
			params, progress, retry_count, max_retries, queued_at, created_by,
			jex_parent_job_id, jex_total_children
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	for _, exec := range execs {
		seq, seqErr := r.nextSequenceTx(ctx, tx, exec.JobType(), exec.Period())
		if seqErr != nil {
			return fmt.Errorf("get next sequence: %w", seqErr)
		}
		code := job.GenerateCode(exec.JobType(), exec.Period(), seq)
		exec.SetCode(code)

		if _, execErr := tx.ExecContext(ctx, insertQuery,
			exec.ID(),
			exec.Code().String(),
			exec.JobType().String(),
			nullString(exec.Subtype()),
			nullString(exec.Period()),
			exec.Status().String(),
			exec.Priority(),
			nullJSON(exec.Params()),
			exec.Progress(),
			exec.RetryCount(),
			exec.MaxRetries(),
			exec.QueuedAt(),
			exec.CreatedBy(),
			nullUUID(exec.ParentJobID()),
			exec.TotalChildren(),
		); execErr != nil {
			return fmt.Errorf("create child job execution: %w", execErr)
		}
	}

	return nil
}

// IncrementChildProgress atomically increments a parent job's completed- or
// failed-children counter and reports whether the batch is now fully done.
// The single UPDATE ... RETURNING avoids a read-then-write race: concurrent
// children finishing at the same moment each get their own atomic increment
// applied by Postgres's row-level locking, and each caller sees the
// post-increment totals it itself produced.
func (r *JobRepository) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	column := "jex_failed_children"
	if success {
		column = "jex_completed_children"
	}
	query := fmt.Sprintf(`
		UPDATE job_execution
		SET %s = %s + 1
		WHERE job_id = $1
		RETURNING jex_completed_children, jex_failed_children, jex_total_children
	`, column, column)

	var completed, failed, total int
	if err := r.db.QueryRowContext(ctx, query, parentJobID).Scan(&completed, &failed, &total); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, job.ErrNotFound
		}
		return false, fmt.Errorf("increment child progress: %w", err)
	}

	return total > 0 && completed+failed >= total, nil
}

// nextSequenceTx mirrors GetNextSequence but runs inside an existing
// transaction so a batch of child inserts sees a consistent view and cannot
// race itself into duplicate sequence numbers within the same tx.
func (r *JobRepository) nextSequenceTx(ctx context.Context, tx *sql.Tx, jobType job.Type, period string) (int, error) {
	query := `
		SELECT COALESCE(MAX(
			CAST(NULLIF(SPLIT_PART(job_code, '-', 3), '') AS INT)
		), 0) + 1
		FROM job_execution
		WHERE job_type = $1 AND ($2 = '' OR period = $2)
	`

	periodArg := ""
	if period != "" {
		periodArg = period
	}

	var seq int
	if err := tx.QueryRowContext(ctx, query, jobType.String(), periodArg).Scan(&seq); err != nil {
		return 1, fmt.Errorf("get next sequence: %w", err)
	}
	return seq, nil
}

// GetByID retrieves a job execution by its ID, including logs.
func (r *JobRepository) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	exec, err := r.scanExecution(ctx, "WHERE je.job_id = $1", id)
	if err != nil {
		return nil, err
	}

	logs, err := r.getLogsByJobID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job logs: %w", err)
	}

	return job.Reconstitute(
		exec.ID(), exec.Code(), exec.JobType(), exec.Subtype(),
		exec.Period(), exec.Status(), exec.Priority(),
		exec.Params(), exec.ResultSummary(), exec.ErrorMessage(),
		exec.Progress(), exec.RetryCount(), exec.MaxRetries(),
		exec.QueuedAt(), exec.StartedAt(), exec.CompletedAt(),
		exec.CreatedBy(), exec.CancelledBy(), exec.CancelledAt(),
		logs,
		exec.ParentJobID(), exec.TotalChildren(), exec.CompletedChildren(), exec.FailedChildren(),
	), nil
}

// GetByCode retrieves a job execution by its code.
func (r *JobRepository) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	return r.scanExecution(ctx, "WHERE je.job_code = $1", code)
}

// List retrieves a paginated list of job executions.
//
//nolint:misspell // SQL column names cancelled_by/cancelled_at match migration schema
func (r *JobRepository) List(ctx context.Context, filter job.ListFilter) (_ []*job.Execution, _ int64, err error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.JobType != "" {
		conditions = append(conditions, fmt.Sprintf("je.job_type = $%d", argIdx))
		args = append(args, filter.JobType)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("je.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Period != "" {
		conditions = append(conditions, fmt.Sprintf("je.period = $%d", argIdx))
		args = append(args, filter.Period)
		argIdx++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(je.job_code ILIKE $%d OR je.error_message ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.ExcludeChildren {
		conditions = append(conditions, "je.jex_parent_job_id IS NULL")
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	countQuery := "SELECT COUNT(*) FROM job_execution je " + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count job executions: %w", err)
	}

	// Fetch page.
	page := max(filter.Page, 1)
	pageSize := min(max(filter.PageSize, 1), 100)
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`
		SELECT je.job_id, je.job_code, je.job_type, je.job_subtype, je.period,
			   je.status, je.priority, je.params, je.result_summary, je.error_message,
			   je.progress, je.retry_count, je.max_retries, je.queued_at, je.started_at,
			   je.completed_at, je.created_by, je.cancelled_by, je.cancelled_at,
			   je.jex_parent_job_id, je.jex_total_children, je.jex_completed_children, je.jex_failed_children
		FROM job_execution je %s
		ORDER BY je.queued_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)

	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list job executions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var executions []*job.Execution
	for rows.Next() {
		exec, scanErr := r.scanExecutionRow(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan job execution: %w", scanErr)
		}
		executions = append(executions, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rows: %w", err)
	}

	return executions, total, nil
}

// ListChildren returns every child job execution belonging to the given
// parent, ordered by creation time ascending.
func (r *JobRepository) ListChildren(ctx context.Context, parentJobID uuid.UUID) (_ []*job.Execution, err error) {
	query := `
		SELECT je.job_id, je.job_code, je.job_type, je.job_subtype, je.period,
			   je.status, je.priority, je.params, je.result_summary, je.error_message,
			   je.progress, je.retry_count, je.max_retries, je.queued_at, je.started_at,
			   je.completed_at, je.created_by, je.cancelled_by, je.cancelled_at,
			   je.jex_parent_job_id, je.jex_total_children, je.jex_completed_children, je.jex_failed_children
		FROM job_execution je
		WHERE je.jex_parent_job_id = $1
		ORDER BY je.queued_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, parentJobID)
	if err != nil {
		return nil, fmt.Errorf("list child job executions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var executions []*job.Execution
	for rows.Next() {
		exec, scanErr := r.scanExecutionRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan child job execution: %w", scanErr)
		}
		executions = append(executions, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return executions, nil
}

// UpdateStatus atomically updates a job execution's status fields.
//
//nolint:misspell // SQL column names cancelled_by/cancelled_at match migration schema
func (r *JobRepository) UpdateStatus(ctx context.Context, exec *job.Execution) error {
	query := `
		UPDATE job_execution SET
			status = $2, result_summary = $3, error_message = $4,
			progress = $5, retry_count = $6,
			started_at = $7, completed_at = $8,
			cancelled_by = $9, cancelled_at = $10
		WHERE job_id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		exec.ID(),
		exec.Status().String(),
		nullJSON(exec.ResultSummary()),
		nullString(exec.ErrorMessage()),
		exec.Progress(),
		exec.RetryCount(),
		exec.StartedAt(),
		exec.CompletedAt(),
		nullString(exec.CancelledBy()),
		exec.CancelledAt(),
	)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return job.ErrNotFound
	}

	return nil
}

// UpdateProgress atomically updates a job execution's progress.
func (r *JobRepository) UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	query := `UPDATE job_execution SET progress = $2 WHERE job_id = $1`
	_, err := r.db.ExecContext(ctx, query, id, progress)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	return nil
}

// AddLog persists a new log entry for a job execution.
func (r *JobRepository) AddLog(ctx context.Context, log *job.ExecutionLog) error {
	query := `
		INSERT INTO job_execution_log (
			log_id, job_id, step, status, message, metadata, started_at, completed_at, duration_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(ctx, query,
		log.ID(),
		log.JobID(),
		log.Step(),
		log.Status().String(),
		nullString(log.Message()),
		nullJSON(log.Metadata()),
		log.StartedAt(),
		log.CompletedAt(),
		log.DurationMs(),
	)
	if err != nil {
		return fmt.Errorf("add job execution log: %w", err)
	}
	return nil
}

// UpdateLog updates an existing log entry.
func (r *JobRepository) UpdateLog(ctx context.Context, log *job.ExecutionLog) error {
	query := `
		UPDATE job_execution_log SET
			status = $2, message = $3, metadata = $4, completed_at = $5, duration_ms = $6
		WHERE log_id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		log.ID(),
		log.Status().String(),
		nullString(log.Message()),
		nullJSON(log.Metadata()),
		log.CompletedAt(),
		log.DurationMs(),
	)
	if err != nil {
		return fmt.Errorf("update job execution log: %w", err)
	}
	return nil
}

// HasActiveJob checks if an active job exists for the given type and period.
func (r *JobRepository) HasActiveJob(ctx context.Context, jobType job.Type, period string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM job_execution
			WHERE job_type = $1 AND period = $2 AND status IN ('QUEUED', 'PROCESSING')
		)
	`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, jobType.String(), period).Scan(&exists); err != nil {
		return false, fmt.Errorf("check active job: %w", err)
	}
	return exists, nil
}

// GetNextSequence returns the next sequential number for job code generation.
func (r *JobRepository) GetNextSequence(ctx context.Context, jobType job.Type, period string) (int, error) {
	query := `
		SELECT COALESCE(MAX(
			CAST(NULLIF(SPLIT_PART(job_code, '-', 3), '') AS INT)
		), 0) + 1
		FROM job_execution
		WHERE job_type = $1 AND ($2 = '' OR period = $2)
	`

	periodArg := ""
	if period != "" {
		periodArg = period
	}

	var seq int
	if err := r.db.QueryRowContext(ctx, query, jobType.String(), periodArg).Scan(&seq); err != nil {
		return 1, fmt.Errorf("get next sequence: %w", err)
	}
	return seq, nil
}

// scanExecution retrieves a single execution by a WHERE clause.
//
//nolint:misspell // SQL column names and Go vars match domain field names (cancelledBy/cancelledAt)
func (r *JobRepository) scanExecution(ctx context.Context, whereClause string, args ...any) (*job.Execution, error) {
	query := fmt.Sprintf(`
		SELECT je.job_id, je.job_code, je.job_type, je.job_subtype, je.period,
			   je.status, je.priority, je.params, je.result_summary, je.error_message,
			   je.progress, je.retry_count, je.max_retries, je.queued_at, je.started_at,
			   je.completed_at, je.created_by, je.cancelled_by, je.cancelled_at,
			   je.jex_parent_job_id, je.jex_total_children, je.jex_completed_children, je.jex_failed_children
		FROM job_execution je %s
	`, whereClause)

	row := r.db.QueryRowContext(ctx, query, args...)

	var (
		id                uuid.UUID
		codeStr           string
		jobType           string
		subtype           sql.NullString
		period            sql.NullString
		status            string
		priority          int
		params            sql.NullString
		resultSummary     sql.NullString
		errorMessage      sql.NullString
		progress          int
		retryCount        int
		maxRetries        int
		queuedAt          time.Time
		startedAt         sql.NullTime
		completedAt       sql.NullTime
		createdBy         string
		cancelledBy       sql.NullString
		cancelledAt       sql.NullTime
		parentJobID       uuid.NullUUID
		totalChildren     int
		completedChildren int
		failedChildren    int
	)

	err := row.Scan(
		&id, &codeStr, &jobType, &subtype, &period,
		&status, &priority, &params, &resultSummary, &errorMessage,
		&progress, &retryCount, &maxRetries, &queuedAt, &startedAt,
		&completedAt, &createdBy, &cancelledBy, &cancelledAt,
		&parentJobID, &totalChildren, &completedChildren, &failedChildren,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, job.ErrNotFound
		}
		return nil, fmt.Errorf("scan job execution: %w", err)
	}

	code, err := job.NewCode(codeStr)
	if err != nil {
		return nil, fmt.Errorf("parse job code: %w", err)
	}

	return job.Reconstitute(
		id, code, job.Type(jobType), subtype.String,
		period.String, job.Status(status), priority,
		parseJSON(params), parseJSON(resultSummary), errorMessage.String,
		progress, retryCount, maxRetries,
		queuedAt, nullTimePtr(startedAt), nullTimePtr(completedAt),
		createdBy, cancelledBy.String, nullTimePtr(cancelledAt),
		nil,
		nullUUIDPtr(parentJobID), totalChildren, completedChildren, failedChildren,
	), nil
}

// scanExecutionRow scans a job execution from a rows iterator.
//
//nolint:misspell // Go vars match domain field names (cancelledBy/cancelledAt)
func (r *JobRepository) scanExecutionRow(rows *sql.Rows) (*job.Execution, error) {
	var (
		id                uuid.UUID
		codeStr           string
		jobType           string
		subtype           sql.NullString
		period            sql.NullString
		status            string
		priority          int
		params            sql.NullString
		resultSummary     sql.NullString
		errorMessage      sql.NullString
		progress          int
		retryCount        int
		maxRetries        int
		queuedAt          time.Time
		startedAt         sql.NullTime
		completedAt       sql.NullTime
		createdBy         string
		cancelledBy       sql.NullString
		cancelledAt       sql.NullTime
		parentJobID       uuid.NullUUID
		totalChildren     int
		completedChildren int
		failedChildren    int
	)

	err := rows.Scan(
		&id, &codeStr, &jobType, &subtype, &period,
		&status, &priority, &params, &resultSummary, &errorMessage,
		&progress, &retryCount, &maxRetries, &queuedAt, &startedAt,
		&completedAt, &createdBy, &cancelledBy, &cancelledAt,
		&parentJobID, &totalChildren, &completedChildren, &failedChildren,
	)
	if err != nil {
		return nil, fmt.Errorf("scan row: %w", err)
	}

	code, err := job.NewCode(codeStr)
	if err != nil {
		return nil, fmt.Errorf("parse job code: %w", err)
	}

	return job.Reconstitute(
		id, code, job.Type(jobType), subtype.String,
		period.String, job.Status(status), priority,
		parseJSON(params), parseJSON(resultSummary), errorMessage.String,
		progress, retryCount, maxRetries,
		queuedAt, nullTimePtr(startedAt), nullTimePtr(completedAt),
		createdBy, cancelledBy.String, nullTimePtr(cancelledAt),
		nil,
		nullUUIDPtr(parentJobID), totalChildren, completedChildren, failedChildren,
	), nil
}

// getLogsByJobID retrieves all log entries for a given job.
func (r *JobRepository) getLogsByJobID(ctx context.Context, jobID uuid.UUID) (_ []*job.ExecutionLog, err error) {
	query := `
		SELECT log_id, job_id, step, status, message, metadata, started_at, completed_at, duration_ms
		FROM job_execution_log
		WHERE job_id = $1
		ORDER BY started_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("query job logs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close log rows: %w", closeErr))
		}
	}()

	var logs []*job.ExecutionLog
	for rows.Next() {
		var (
			logID       uuid.UUID
			logJobID    uuid.UUID
			step        string
			status      string
			message     sql.NullString
			metadata    sql.NullString
			startedAt   time.Time
			completedAt sql.NullTime
			durationMs  sql.NullInt32
		)

		if scanErr := rows.Scan(
			&logID, &logJobID, &step, &status, &message, &metadata,
			&startedAt, &completedAt, &durationMs,
		); scanErr != nil {
			return nil, fmt.Errorf("scan log row: %w", scanErr)
		}

		var durPtr *int
		if durationMs.Valid {
			d := int(durationMs.Int32)
			durPtr = &d
		}

		logs = append(logs, job.ReconstituteLog(
			logID, logJobID, step, job.LogStatus(status),
			message.String, parseJSON(metadata),
			startedAt, nullTimePtr(completedAt), durPtr,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate log rows: %w", err)
	}

	return logs, nil
}

// Helper functions.

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullJSON(data json.RawMessage) sql.NullString {
	if len(data) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func parseJSON(ns sql.NullString) json.RawMessage {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return json.RawMessage(ns.String)
}

func nullTimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func nullUUIDPtr(nu uuid.NullUUID) *uuid.UUID {
	if !nu.Valid {
		return nil
	}
	id := nu.UUID
	return &id
}

func isDuplicateActiveJob(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		// Check for unique violation on the partial unique index.
		return pqErr.Code == "23505" && strings.Contains(pqErr.Constraint, "active_unique")
	}
	return false
}
