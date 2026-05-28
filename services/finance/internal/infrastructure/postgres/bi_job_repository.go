package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// BiJobRepository implements job.Repository.
type BiJobRepository struct {
	db *DB
}

// NewBiJobRepository constructs a BiJobRepository.
func NewBiJobRepository(db *DB) *BiJobRepository {
	return &BiJobRepository{db: db}
}

var _ job.Repository = (*BiJobRepository)(nil)

// List returns jobs with their most recent run summary joined from bi_job_log.
func (r *BiJobRepository) List(ctx context.Context, includeInactive bool) ([]*job.Job, error) {
	q := `
SELECT j.job_id, j.job_name, j.source_id, ds.source_code,
       j.target_type, j.schedule_cron, j.oracle_procedure, j.config, j.is_active,
       j.created_at, j.updated_at,
       l.status, l.started_at, l.duration_ms
FROM bi_job j
LEFT JOIN bi_data_source ds ON ds.source_id = j.source_id
LEFT JOIN LATERAL (
    SELECT status, started_at, duration_ms FROM bi_job_log
    WHERE job_id = j.job_id ORDER BY started_at DESC LIMIT 1
) l ON TRUE`
	if !includeInactive {
		q += " WHERE j.is_active = TRUE"
	}
	q += " ORDER BY j.job_name"

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*job.Job
	for rows.Next() {
		j, err := r.scanJob(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// GetByID returns a single job by primary key.
func (r *BiJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*job.Job, error) {
	q := `
SELECT j.job_id, j.job_name, j.source_id, ds.source_code,
       j.target_type, j.schedule_cron, j.oracle_procedure, j.config, j.is_active,
       j.created_at, j.updated_at,
       l.status, l.started_at, l.duration_ms
FROM bi_job j
LEFT JOIN bi_data_source ds ON ds.source_id = j.source_id
LEFT JOIN LATERAL (
    SELECT status, started_at, duration_ms FROM bi_job_log
    WHERE job_id = j.job_id ORDER BY started_at DESC LIMIT 1
) l ON TRUE
WHERE j.job_id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	return r.scanJob(row.Scan)
}

// ListLogs returns paginated logs for a job.
func (r *BiJobRepository) ListLogs(ctx context.Context, jobID uuid.UUID, page, pageSize int) ([]*job.Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM bi_job_log WHERE job_id = $1", jobID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count logs: %w", err)
	}

	const q = `
SELECT l.log_id, l.job_id, j.job_name, l.started_at, l.ended_at, l.status,
       l.rows_affected, l.error_message, l.triggered_by, l.duration_ms
FROM bi_job_log l
JOIN bi_job j ON j.job_id = l.job_id
WHERE l.job_id = $1
ORDER BY l.started_at DESC
LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, jobID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("query logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*job.Log
	for rows.Next() {
		l, err := r.scanLog(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// InsertLog records a new job-run entry.
func (r *BiJobRepository) InsertLog(ctx context.Context, l *job.Log) error {
	const q = `
INSERT INTO bi_job_log (
    job_id, started_at, ended_at, status, rows_affected, error_message, triggered_by, duration_ms
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING log_id`
	row := r.db.QueryRowContext(ctx, q,
		l.JobID, l.StartedAt, nullableTime(l.EndedAt), l.Status, nullableInt(l.RowsAffected),
		biNullableString(l.ErrorMessage), biNullableString(l.TriggeredBy), nullableInt(l.DurationMs))
	if err := row.Scan(&l.LogID); err != nil {
		return fmt.Errorf("insert job log: %w", err)
	}
	return nil
}

// UpdateLog mutates the ended_at/status/duration on an existing log row.
func (r *BiJobRepository) UpdateLog(ctx context.Context, l *job.Log) error {
	const q = `
UPDATE bi_job_log SET
    ended_at = $2, status = $3, rows_affected = $4, error_message = $5, duration_ms = $6
WHERE log_id = $1`
	res, err := r.db.ExecContext(ctx, q,
		l.LogID, nullableTime(l.EndedAt), l.Status, nullableInt(l.RowsAffected),
		biNullableString(l.ErrorMessage), nullableInt(l.DurationMs))
	if err != nil {
		return fmt.Errorf("update job log: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return job.ErrNotFound
	}
	return nil
}

// scanJob handles the row produced by List/GetByID (with LATERAL join columns).
func (r *BiJobRepository) scanJob(scan scanFunc) (*job.Job, error) {
	var (
		j             job.Job
		sourceCode    sql.NullString
		targetType    sql.NullString
		cron          sql.NullString
		oracleProc    sql.NullString
		config        []byte
		createdAt     sql.NullTime
		updatedAt     sql.NullTime
		lastStatus    sql.NullString
		lastStartedAt sql.NullTime
		lastDuration  sql.NullInt64
	)
	err := scan(
		&j.ID, &j.Name, &j.SourceID, &sourceCode,
		&targetType, &cron, &oracleProc, &config, &j.IsActive,
		&createdAt, &updatedAt,
		&lastStatus, &lastStartedAt, &lastDuration,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, job.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.SourceCode = nullToString(sourceCode)
	j.TargetType = nullToString(targetType)
	j.ScheduleCron = nullToString(cron)
	j.OracleProcedure = nullToString(oracleProc)
	if len(config) > 0 {
		if err := json.Unmarshal(config, &j.Config); err != nil {
			return nil, fmt.Errorf("unmarshal job config: %w", err)
		}
	}
	j.CreatedAt = nullTimeOrZero(createdAt)
	j.UpdatedAt = nullTimeOrZero(updatedAt)
	j.LastStatus = nullToString(lastStatus)
	j.LastRunAt = nullTimeOrZero(lastStartedAt)
	if lastDuration.Valid {
		j.LastDurationMs = int(lastDuration.Int64)
	}
	return &j, nil
}

// scanLog handles bi_job_log rows.
func (r *BiJobRepository) scanLog(scan scanFunc) (*job.Log, error) {
	var (
		l            job.Log
		endedAt      sql.NullTime
		rowsAffected sql.NullInt64
		errMsg       sql.NullString
		triggeredBy  sql.NullString
		durationMs   sql.NullInt64
	)
	err := scan(&l.LogID, &l.JobID, &l.JobName, &l.StartedAt, &endedAt, &l.Status,
		&rowsAffected, &errMsg, &triggeredBy, &durationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, job.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan job log: %w", err)
	}
	l.EndedAt = nullTimeOrZero(endedAt)
	if rowsAffected.Valid {
		l.RowsAffected = int(rowsAffected.Int64)
	}
	l.ErrorMessage = nullToString(errMsg)
	l.TriggeredBy = nullToString(triggeredBy)
	if durationMs.Valid {
		l.DurationMs = int(durationMs.Int64)
	}
	return &l, nil
}

// nullableTime returns nil for zero time values.
func nullableTime(t any) any {
	if v, ok := t.(interface{ IsZero() bool }); ok && v.IsZero() {
		return nil
	}
	return t
}
