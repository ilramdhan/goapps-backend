package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// CostCalcChunkRepository persists Chunk aggregates against `cal_job_chunk`.
type CostCalcChunkRepository struct {
	db *DB
}

// NewCostCalcChunkRepository constructs a CostCalcChunkRepository.
func NewCostCalcChunkRepository(db *DB) *CostCalcChunkRepository {
	return &CostCalcChunkRepository{db: db}
}

var _ costcalc.ChunkRepository = (*CostCalcChunkRepository)(nil)

// Create inserts a fresh QUEUED chunk row.
func (r *CostCalcChunkRepository) Create(ctx context.Context, c *costcalc.Chunk) error {
	if c == nil {
		return fmt.Errorf("create chunk: nil chunk")
	}
	idsJSON, err := json.Marshal(c.ProductIDs())
	if err != nil {
		return fmt.Errorf("marshal product ids: %w", err)
	}
	const q = `
		INSERT INTO cal_job_chunk (
			cjc_job_id, cjc_chunk_number, cjc_wave_no, cjc_product_ids, cjc_product_count,
			cjc_status, cjc_max_retries
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING cjc_chunk_id`
	var id int64
	if err := r.db.QueryRowContext(ctx, q,
		c.JobID(), safeconv.IntToInt32(c.ChunkNumber()), safeconv.IntToInt32(c.WaveNo()),
		idsJSON, safeconv.IntToInt32(c.ProductCount()), string(c.Status()),
		safeconv.IntToInt32(c.MaxRetries()),
	).Scan(&id); err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	c.AssignID(id)
	return nil
}

// GetByID returns a hydrated chunk or ErrJobNotFound (reusing the not-found sentinel).
func (r *CostCalcChunkRepository) GetByID(ctx context.Context, id int64) (*costcalc.Chunk, error) {
	const q = `
		SELECT cjc_chunk_id, cjc_job_id, cjc_chunk_number, cjc_wave_no, cjc_product_ids,
		       cjc_status, COALESCE(cjc_worker_id, ''), cjc_queued_at,
		       cjc_dispatched_at, cjc_started_at, cjc_completed_at,
		       COALESCE(cjc_duration_ms, 0), cjc_success_count, cjc_failed_count,
		       COALESCE(cjc_error_message, ''), cjc_retry_count, cjc_max_retries
		FROM cal_job_chunk
		WHERE cjc_chunk_id = $1`
	chunk, err := scanChunk(r.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costcalc.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	return chunk, nil
}

// ListByJob returns paginated chunks for a job, optionally filtered by wave and status.
func (r *CostCalcChunkRepository) ListByJob(
	ctx context.Context, jobID int64, wave *int, status *costcalc.ChunkStatus, page, pageSize int,
) ([]*costcalc.Chunk, int, error) {
	where := []string{"cjc_job_id = $1"}
	args := []any{jobID}
	idx := 2
	if wave != nil {
		where = append(where, fmt.Sprintf("cjc_wave_no = $%d", idx))
		args = append(args, *wave)
		idx++
	}
	if status != nil {
		where = append(where, fmt.Sprintf("cjc_status = $%d", idx))
		args = append(args, string(*status))
		idx++
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM cal_job_chunk`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chunks: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	offset := (page - 1) * pageSize

	listSQL := `
		SELECT cjc_chunk_id, cjc_job_id, cjc_chunk_number, cjc_wave_no, cjc_product_ids,
		       cjc_status, COALESCE(cjc_worker_id, ''), cjc_queued_at,
		       cjc_dispatched_at, cjc_started_at, cjc_completed_at,
		       COALESCE(cjc_duration_ms, 0), cjc_success_count, cjc_failed_count,
		       COALESCE(cjc_error_message, ''), cjc_retry_count, cjc_max_retries
		FROM cal_job_chunk` + whereSQL + ` ORDER BY cjc_wave_no ASC, cjc_chunk_number ASC` +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list chunks: %w", err)
	}
	defer closeRows(rows)

	out := []*costcalc.Chunk{}
	for rows.Next() {
		c, scanErr := scanChunk(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan chunk row: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate chunk rows: %w", err)
	}
	return out, total, nil
}

// UpdateStatus rewrites the status + worker id (worker id empty string -> NULL).
func (r *CostCalcChunkRepository) UpdateStatus(ctx context.Context, id int64, status costcalc.ChunkStatus, workerID string) error {
	const q = `
		UPDATE cal_job_chunk
		   SET cjc_status = $1::VARCHAR,
		       cjc_worker_id = NULLIF($2::VARCHAR, ''),
		       cjc_dispatched_at = CASE WHEN $1::VARCHAR = 'DISPATCHED' THEN now() ELSE cjc_dispatched_at END,
		       cjc_started_at    = CASE WHEN $1::VARCHAR = 'PROCESSING' THEN now() ELSE cjc_started_at END
		 WHERE cjc_chunk_id = $3`
	res, err := r.db.ExecContext(ctx, q, string(status), workerID, id)
	if err != nil {
		return fmt.Errorf("update chunk status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update chunk status rows: %w", err)
	}
	if n == 0 {
		return costcalc.ErrJobNotFound
	}
	return nil
}

// UpdateResult records the final outcome of a chunk.
func (r *CostCalcChunkRepository) UpdateResult(
	ctx context.Context, id int64, status costcalc.ChunkStatus, succ, fail, durationMs int, errMsg string,
) error {
	const q = `
		UPDATE cal_job_chunk
		   SET cjc_status = $1, cjc_success_count = $2, cjc_failed_count = $3,
		       cjc_duration_ms = $4, cjc_error_message = NULLIF($5, ''),
		       cjc_completed_at = now()
		 WHERE cjc_chunk_id = $6`
	res, err := r.db.ExecContext(ctx, q, string(status),
		safeconv.IntToInt32(succ), safeconv.IntToInt32(fail), safeconv.IntToInt32(durationMs),
		errMsg, id)
	if err != nil {
		return fmt.Errorf("update chunk result: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update chunk result rows: %w", err)
	}
	if n == 0 {
		return costcalc.ErrJobNotFound
	}
	return nil
}

// IncrementRetry bumps cjc_retry_count and returns the new value.
func (r *CostCalcChunkRepository) IncrementRetry(ctx context.Context, id int64) (int, error) {
	const q = `
		UPDATE cal_job_chunk
		   SET cjc_retry_count = cjc_retry_count + 1
		 WHERE cjc_chunk_id = $1
		 RETURNING cjc_retry_count`
	var n int
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, costcalc.ErrJobNotFound
		}
		return 0, fmt.Errorf("increment chunk retry: %w", err)
	}
	return n, nil
}

// scanChunk reads one chunk row, unmarshals the JSONB product ids, and hydrates.
func scanChunk(s rowScanner) (*costcalc.Chunk, error) {
	var (
		id, jobID         int64
		chunkNo, wave     int32
		idsJSON           []byte
		status, workerID  string
		queuedAt          time.Time
		dispatchedAt      sql.NullTime
		startedAt         sql.NullTime
		completedAt       sql.NullTime
		durationMs        int32
		successC, failedC int32
		errMsg            string
		retry, maxRetries int32
	)
	if err := s.Scan(
		&id, &jobID, &chunkNo, &wave, &idsJSON, &status, &workerID, &queuedAt,
		&dispatchedAt, &startedAt, &completedAt, &durationMs, &successC, &failedC,
		&errMsg, &retry, &maxRetries,
	); err != nil {
		return nil, err
	}
	productIDs := []int64{}
	if len(idsJSON) > 0 {
		if err := json.Unmarshal(idsJSON, &productIDs); err != nil {
			return nil, fmt.Errorf("unmarshal product ids: %w", err)
		}
	}
	var dispatchedPtr, startedPtr, completedPtr *time.Time
	if dispatchedAt.Valid {
		t := dispatchedAt.Time
		dispatchedPtr = &t
	}
	if startedAt.Valid {
		t := startedAt.Time
		startedPtr = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		completedPtr = &t
	}
	return costcalc.HydrateChunk(
		id, jobID, int(chunkNo), int(wave), productIDs, costcalc.ChunkStatus(status),
		workerID, queuedAt, dispatchedPtr, startedPtr, completedPtr,
		int(durationMs), int(successC), int(failedC), errMsg, int(retry), int(maxRetries),
	), nil
}
