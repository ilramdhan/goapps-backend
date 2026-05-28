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

// Create inserts a new ETL job row and returns the full persisted Job (with source_code resolved).
// The source_id FK is resolved from source_code via a sub-select on bi_data_source.
func (r *BiJobRepository) Create(ctx context.Context, p job.CreateJobParams) (*job.Job, error) {
	configJSON, err := marshalJobConfig(p.Config)
	if err != nil {
		return nil, err
	}
	var createdBy any
	if p.CreatedBy != uuid.Nil {
		createdBy = p.CreatedBy
	}
	const q = `
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active, created_at, updated_at, created_by, updated_by)
SELECT $1, ds.source_id, $3, $4, $5, $6, $7, now(), now(), $8, $8
FROM bi_data_source ds
WHERE ds.source_code = $2
RETURNING job_id, created_at, updated_at`
	var j job.Job
	j.Name = p.JobName
	j.SourceCode = p.SourceCode
	j.TargetType = p.TargetType
	j.ScheduleCron = p.ScheduleCron
	j.OracleProcedure = p.OracleProcedure
	j.Config = p.Config
	j.IsActive = p.IsActive
	err = r.db.QueryRowContext(ctx, q,
		p.JobName, p.SourceCode, p.TargetType,
		biNullableString(p.ScheduleCron), biNullableString(p.OracleProcedure),
		configJSON, p.IsActive, createdBy,
	).Scan(&j.ID, &j.CreatedAt, &j.UpdatedAt)
	if isUniqueViolation(err) {
		return nil, job.ErrAlreadyExists
	}
	if err != nil {
		return nil, fmt.Errorf("insert bi job: %w", err)
	}
	return &j, nil
}

// Update applies partial mutations to an existing ETL job.
func (r *BiJobRepository) Update(ctx context.Context, p job.UpdateJobParams) (*job.Job, error) {
	// Build SET clause dynamically from provided optional fields.
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	argN := 1

	if p.UpdatedBy != uuid.Nil {
		setClauses = append(setClauses, fmt.Sprintf("updated_by = $%d", argN))
		args = append(args, p.UpdatedBy)
		argN++
	}
	if p.ScheduleCron != nil {
		setClauses = append(setClauses, fmt.Sprintf("schedule_cron = $%d", argN))
		args = append(args, biNullableString(*p.ScheduleCron))
		argN++
	}
	if p.OracleProcedure != nil {
		setClauses = append(setClauses, fmt.Sprintf("oracle_procedure = $%d", argN))
		args = append(args, biNullableString(*p.OracleProcedure))
		argN++
	}
	if p.Config != nil {
		configJSON, err := marshalJobConfig(p.Config)
		if err != nil {
			return nil, err
		}
		setClauses = append(setClauses, fmt.Sprintf("config = $%d", argN))
		args = append(args, configJSON)
		argN++
	}
	if p.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argN))
		args = append(args, *p.IsActive)
		argN++
	}
	args = append(args, p.ID)
	q := fmt.Sprintf(`UPDATE bi_job SET %s WHERE job_id = $%d`,
		joinComma(setClauses), argN)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("update bi job: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update bi job rows affected: %w", err)
	}
	if affected == 0 {
		return nil, job.ErrNotFound
	}
	return r.GetByID(ctx, p.ID)
}

// Delete soft-disables a job by setting is_active=false and recording updated_by.
// The row and its logs are preserved for historical queries.
func (r *BiJobRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	var updatedByArg any
	if deletedBy != uuid.Nil {
		updatedByArg = deletedBy
	}
	const q = `UPDATE bi_job SET is_active = FALSE, updated_at = now(), updated_by = $2 WHERE job_id = $1`
	res, err := r.db.ExecContext(ctx, q, id, updatedByArg)
	if err != nil {
		return fmt.Errorf("delete bi job: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bi job rows affected: %w", err)
	}
	if affected == 0 {
		return job.ErrNotFound
	}
	return nil
}

// marshalJobConfig converts a map to JSON bytes for storage; nil maps produce nil bytes.
func marshalJobConfig(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil //nolint:nilnil // nil JSON is intentional for empty config
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal job config: %w", err)
	}
	return b, nil
}

// joinComma joins a slice of strings with ", ".
func joinComma(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
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
