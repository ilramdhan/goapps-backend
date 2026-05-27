package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var errNoDBConn = errors.New("db connection nil")

// chunkRepo is a tiny direct-SQL adapter for the four cal_job_chunk lifecycle
// operations the worker performs. We intentionally avoid pulling in finance's
// domain repo because the worker module is a separate go.mod and finance keeps
// its repos under internal/.
type chunkRepo struct{ db *sql.DB }

func newChunkRepo(db *sql.DB) *chunkRepo { return &chunkRepo{db: db} }

// GetStatus returns the current cjc_status for the chunk row.
func (r *chunkRepo) GetStatus(ctx context.Context, id int64) (string, error) {
	if r.db == nil {
		return "", errNoDBConn
	}
	var s string
	if err := r.db.QueryRowContext(ctx,
		`SELECT cjc_status FROM cal_job_chunk WHERE cjc_chunk_id=$1`,
		id,
	).Scan(&s); err != nil {
		return "", fmt.Errorf("get chunk status %d: %w", id, err)
	}
	return s, nil
}

// MarkProcessing transitions the chunk to PROCESSING + records started_at +
// worker id. Called immediately after the message is picked up.
func (r *chunkRepo) MarkProcessing(ctx context.Context, id int64, workerID string) error {
	if r.db == nil {
		return errNoDBConn
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cal_job_chunk
		    SET cjc_status='PROCESSING',
		        cjc_started_at=now(),
		        cjc_worker_id=$2
		  WHERE cjc_chunk_id=$1`,
		id, workerID,
	); err != nil {
		return fmt.Errorf("mark chunk %d processing: %w", id, err)
	}
	return nil
}

// MarkCompleted writes the terminal status + counts + duration.
func (r *chunkRepo) MarkCompleted(ctx context.Context, id int64, status string, succ, fail, durationMs int) error {
	if r.db == nil {
		return errNoDBConn
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cal_job_chunk
		    SET cjc_status=$2,
		        cjc_completed_at=now(),
		        cjc_success_count=$3,
		        cjc_failed_count=$4,
		        cjc_duration_ms=$5
		  WHERE cjc_chunk_id=$1`,
		id, status, succ, fail, durationMs,
	); err != nil {
		return fmt.Errorf("mark chunk %d completed: %w", id, err)
	}
	return nil
}

// IncrementRetry bumps cjc_retry_count and returns the new value.
func (r *chunkRepo) IncrementRetry(ctx context.Context, id int64) (int, error) {
	if r.db == nil {
		return 0, errNoDBConn
	}
	var n int
	if err := r.db.QueryRowContext(ctx,
		`UPDATE cal_job_chunk
		    SET cjc_retry_count = cjc_retry_count + 1
		  WHERE cjc_chunk_id=$1
		RETURNING cjc_retry_count`,
		id,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("increment retry %d: %w", id, err)
	}
	return n, nil
}
