package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
)

// CostImportJobRepository implements costimportjob.Repository using PostgreSQL.
type CostImportJobRepository struct {
	db *DB
}

// NewCostImportJobRepository constructs the repository.
func NewCostImportJobRepository(db *DB) *CostImportJobRepository {
	return &CostImportJobRepository{db: db}
}

var _ costimportjob.Repository = (*CostImportJobRepository)(nil)

// Create persists a new CostImportJob and assigns the generated job ID.
func (r *CostImportJobRepository) Create(ctx context.Context, job *costimportjob.CostImportJob) error {
	const q = `
		INSERT INTO cost_import_job (
			cij_entity, cij_status, cij_total_rows, cij_processed,
			cij_success, cij_failed, cij_skipped,
			cij_file_key, cij_error_file, cij_error_detail,
			cij_created_at, cij_created_by,
			cij_started_at, cij_completed_at, cij_parent_job_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15
		)
		RETURNING cij_job_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, q,
		job.Entity(), job.Status(), job.TotalRows(), job.Processed(),
		job.Success(), job.Failed(), job.Skipped(),
		job.FileKey(), job.ErrorFile(), job.ErrorDetail(),
		job.CreatedAt(), job.CreatedBy(),
		job.StartedAt(), job.CompletedAt(), job.ParentJobID(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("create cost_import_job: %w", err)
	}
	job.SetJobID(id)
	return nil
}

// GetByID loads a CostImportJob by its primary key.
func (r *CostImportJobRepository) GetByID(ctx context.Context, id int64) (*costimportjob.CostImportJob, error) {
	const q = `
		SELECT
			cij_job_id, cij_entity, cij_status,
			cij_total_rows, cij_processed, cij_success, cij_failed, cij_skipped,
			cij_file_key, cij_error_file, cij_error_detail,
			cij_created_at, cij_created_by,
			cij_started_at, cij_completed_at, cij_parent_job_id
		FROM cost_import_job
		WHERE cij_job_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, q, id))
}

// Update persists all mutable fields of an existing CostImportJob.
func (r *CostImportJobRepository) Update(ctx context.Context, job *costimportjob.CostImportJob) error {
	const q = `
		UPDATE cost_import_job SET
			cij_status       = $2,
			cij_total_rows   = $3,
			cij_processed    = $4,
			cij_success      = $5,
			cij_failed       = $6,
			cij_skipped      = $7,
			cij_file_key     = $8,
			cij_error_file   = $9,
			cij_error_detail = $10,
			cij_started_at   = $11,
			cij_completed_at = $12
		WHERE cij_job_id = $1
	`
	res, err := r.db.ExecContext(ctx, q,
		job.JobID(),
		job.Status(),
		job.TotalRows(), job.Processed(),
		job.Success(), job.Failed(), job.Skipped(),
		job.FileKey(), job.ErrorFile(), job.ErrorDetail(),
		job.StartedAt(), job.CompletedAt(),
	)
	if err != nil {
		return fmt.Errorf("update cost_import_job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected cost_import_job: %w", err)
	}
	if n == 0 {
		return costimportjob.ErrNotFound
	}
	return nil
}

// List returns a paginated list of jobs ordered by created_at DESC.
// entity and status are optional; empty string means no filter.
func (r *CostImportJobRepository) List(
	ctx context.Context,
	entity, status string,
	page, pageSize int,
) ([]*costimportjob.CostImportJob, int64, error) {
	where := "FROM cost_import_job WHERE 1=1"
	args := []any{}
	idx := 1

	if entity != "" {
		where += fmt.Sprintf(" AND cij_entity = $%d", idx)
		args = append(args, entity)
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND cij_status = $%d", idx)
		args = append(args, strings.ToUpper(status))
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_import_job: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	selectQ := `
		SELECT
			cij_job_id, cij_entity, cij_status,
			cij_total_rows, cij_processed, cij_success, cij_failed, cij_skipped,
			cij_file_key, cij_error_file, cij_error_detail,
			cij_created_at, cij_created_by,
			cij_started_at, cij_completed_at, cij_parent_job_id
		` + where + fmt.Sprintf(" ORDER BY cij_created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, selectQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_import_job: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	items := []*costimportjob.CostImportJob{}
	for rows.Next() {
		job, sErr := r.scanRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		items = append(items, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_import_job: %w", err)
	}
	return items, total, nil
}

// =============================================================================
// scan helpers
// =============================================================================

func (r *CostImportJobRepository) scanRow(row *sql.Row) (*costimportjob.CostImportJob, error) {
	var (
		jobID                      int64
		entity, status             string
		totalRows, processed       int
		success, failed, skipped   int
		fileKey, errorFile         string
		errorDetail                string
		createdAt                  time.Time
		createdBy                  string
		startedAt, completedAt     *time.Time
		parentJobID                *int64
	)
	err := row.Scan(
		&jobID, &entity, &status,
		&totalRows, &processed, &success, &failed, &skipped,
		&fileKey, &errorFile, &errorDetail,
		&createdAt, &createdBy,
		&startedAt, &completedAt, &parentJobID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costimportjob.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_import_job: %w", err)
	}
	return costimportjob.Reconstruct(
		jobID, entity, status,
		totalRows, processed, success, failed, skipped,
		fileKey, errorFile, errorDetail,
		createdAt, createdBy,
		startedAt, completedAt,
		parentJobID,
	), nil
}

func (r *CostImportJobRepository) scanRows(rows *sql.Rows) (*costimportjob.CostImportJob, error) {
	var (
		jobID                      int64
		entity, status             string
		totalRows, processed       int
		success, failed, skipped   int
		fileKey, errorFile         string
		errorDetail                string
		createdAt                  time.Time
		createdBy                  string
		startedAt, completedAt     *time.Time
		parentJobID                *int64
	)
	err := rows.Scan(
		&jobID, &entity, &status,
		&totalRows, &processed, &success, &failed, &skipped,
		&fileKey, &errorFile, &errorDetail,
		&createdAt, &createdBy,
		&startedAt, &completedAt, &parentJobID,
	)
	if err != nil {
		return nil, fmt.Errorf("scan cost_import_job row: %w", err)
	}
	return costimportjob.Reconstruct(
		jobID, entity, status,
		totalRows, processed, success, failed, skipped,
		fileKey, errorFile, errorDetail,
		createdAt, createdBy,
		startedAt, completedAt,
		parentJobID,
	), nil
}
