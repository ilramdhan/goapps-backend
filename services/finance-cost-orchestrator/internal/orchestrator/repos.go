package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/pkg/costcalc"
)

// Status string constants used by the orchestrator when writing cal_job rows.
const (
	statusQueued     = "QUEUED"
	statusPlanning   = "PLANNING"
	statusProcessing = "PROCESSING"
	statusSuccess    = "SUCCESS"
	statusFailed     = "FAILED"
	statusPartial    = "PARTIAL_FAILED"
	statusDispatched = "DISPATCHED"
	statusPending    = "PENDING"
	statusCancelled  = "CANCELLED"
)

// JobDTO mirrors the cal_job row data the orchestrator needs. Constructed on
// the orchestrator side (no domain entity dependency) so we can read jobs the
// finance service has already created.
type JobDTO struct {
	JobID               int64
	JobCode             string
	Period              string
	CalcType            costcalc.CalculationType
	Scope               costcalc.JobScope
	Status              string
	TriggeredBy         string
	CreatedBy           string
	ProductSysID        int64
	RouteHeadID         int64
	ProductTypeIDFilter int32
}

// ChunkRow is a minimal struct for inserting cal_job_chunk rows.
type ChunkRow struct {
	JobID       int64
	ChunkNumber int
	WaveNo      int
	ProductIDs  []int64
	ChunkID     int64 // populated after BulkInsert via RETURNING.
}

// JobProductRow is a minimal struct for inserting cal_job_product rows.
type JobProductRow struct {
	JobID        int64
	ProductSysID int64
	RouteHeadID  int64
	WaveNo       int
	ChunkID      int64
}

// JobRepo is the orchestrator-side facade over the cal_job table.
type JobRepo struct{ db *sql.DB }

// NewJobRepo constructs a JobRepo.
func NewJobRepo(db *sql.DB) *JobRepo { return &JobRepo{db: db} }

// DB exposes the underlying connection for callers that need to issue tiny
// read-only queries the repo doesn't expose (e.g. fetching total_waves +
// started_at during job finalization).
func (r *JobRepo) DB() *sql.DB { return r.db }

// GetByID loads a cal_job row by primary key and decodes the JSONB filter.
func (r *JobRepo) GetByID(ctx context.Context, id int64) (*JobDTO, error) {
	const q = `
		SELECT cj_job_id, cj_job_code, cj_period, cj_calculation_type, cj_scope,
		       cj_status, cj_triggered_by, cj_created_by, cj_product_filter
		FROM cal_job WHERE cj_job_id = $1
	`
	var j JobDTO
	var filter sql.NullString
	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&j.JobID, &j.JobCode, &j.Period, &j.CalcType, &j.Scope,
		&j.Status, &j.TriggeredBy, &j.CreatedBy, &filter,
	); err != nil {
		return nil, fmt.Errorf("get job %d: %w", id, err)
	}
	if filter.Valid && filter.String != "" {
		applyFilter(&j, filter.String)
	}
	return &j, nil
}

// applyFilter populates scope-specific fields from cj_product_filter JSONB.
func applyFilter(j *JobDTO, raw string) {
	var data struct {
		ProductSysID        int64 `json:"product_sys_id"`
		RouteHeadID         int64 `json:"route_head_id"`
		ProductTypeIDFilter int32 `json:"product_type_id_filter"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return
	}
	j.ProductSysID = data.ProductSysID
	j.RouteHeadID = data.RouteHeadID
	j.ProductTypeIDFilter = data.ProductTypeIDFilter
}

// UpdateStatus sets cj_status only.
func (r *JobRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE cal_job SET cj_status = $1 WHERE cj_job_id = $2`, status, id); err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

// UpdateTotals writes totals + chunks + waves once planning resolves them.
func (r *JobRepo) UpdateTotals(ctx context.Context, id int64, products, chunks, waves int) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cal_job SET cj_total_products = $1, cj_total_chunks = $2, cj_total_waves = $3 WHERE cj_job_id = $4`,
		products, chunks, waves, id,
	); err != nil {
		return fmt.Errorf("update job totals: %w", err)
	}
	return nil
}

// IncrementProgress is called once per chunk_completed event.
func (r *JobRepo) IncrementProgress(ctx context.Context, id int64, succ, fail, blocked int) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE cal_job SET
		  cj_processed_chunks = cj_processed_chunks + 1,
		  cj_success_count    = cj_success_count    + $1,
		  cj_failed_count     = cj_failed_count     + $2,
		  cj_blocked_count    = cj_blocked_count    + $3
		WHERE cj_job_id = $4
	`, succ, fail, blocked, id); err != nil {
		return fmt.Errorf("increment job progress: %w", err)
	}
	return nil
}

// CompleteJob writes the terminal status + duration + counts.
func (r *JobRepo) CompleteJob(ctx context.Context, id int64, status string, succ, fail, blocked int, durationMs int64) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE cal_job SET
		  cj_status        = $1,
		  cj_success_count = $2,
		  cj_failed_count  = $3,
		  cj_blocked_count = $4,
		  cj_completed_at  = now(),
		  cj_duration_ms   = $5
		WHERE cj_job_id = $6
	`, status, succ, fail, blocked, durationMs, id); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// MarkStarted records started_at = now() on first transition to PROCESSING.
func (r *JobRepo) MarkStarted(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cal_job SET cj_started_at = COALESCE(cj_started_at, now()) WHERE cj_job_id = $1`, id,
	); err != nil {
		return fmt.Errorf("mark started: %w", err)
	}
	return nil
}

// GetProgress reads current counts.
func (r *JobRepo) GetProgress(ctx context.Context, id int64) (processed, total, succ, fail, blocked int, err error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT cj_processed_chunks, cj_total_chunks, cj_success_count, cj_failed_count, cj_blocked_count
		FROM cal_job WHERE cj_job_id = $1
	`, id)
	if scanErr := row.Scan(&processed, &total, &succ, &fail, &blocked); scanErr != nil {
		err = fmt.Errorf("get job progress: %w", scanErr)
	}
	return
}

// CreateAutoJob inserts a QUEUED ALL-scope cal_job triggered by cron and
// returns its id. Used by the monthly cron auto-trigger (S8e.6).
func (r *JobRepo) CreateAutoJob(ctx context.Context, period, calcType, scope, triggeredBy, createdBy string) (int64, error) {
	const q = `
		WITH new_code AS (SELECT generate_cal_job_code() AS code)
		INSERT INTO cal_job (
			cj_job_code, cj_period, cj_calculation_type, cj_scope,
			cj_product_filter, cj_status, cj_priority, cj_triggered_by, cj_created_by
		)
		SELECT (SELECT code FROM new_code), $1, $2, $3, NULL::JSONB, 'QUEUED', 5, $4, $5
		RETURNING cj_job_id
	`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, period, calcType, scope, triggeredBy, createdBy).Scan(&id); err != nil {
		return 0, fmt.Errorf("create auto job: %w", err)
	}
	return id, nil
}

// ChunkRepo is the orchestrator-side facade over cal_job_chunk.
type ChunkRepo struct{ db *sql.DB }

// NewChunkRepo constructs a ChunkRepo.
func NewChunkRepo(db *sql.DB) *ChunkRepo { return &ChunkRepo{db: db} }

// BulkInsert inserts N chunk rows in one statement, populating each row's
// ChunkID via RETURNING in input order.
func (r *ChunkRepo) BulkInsert(ctx context.Context, rows []*ChunkRow) error {
	if len(rows) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`INSERT INTO cal_job_chunk (cjc_job_id, cjc_chunk_number, cjc_wave_no, cjc_product_ids, cjc_product_count, cjc_status) VALUES `)
	args := make([]any, 0, len(rows)*6)
	for i, row := range rows {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 6
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d::jsonb,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
		idsJSON, err := json.Marshal(row.ProductIDs)
		if err != nil {
			return fmt.Errorf("marshal product_ids: %w", err)
		}
		args = append(args, row.JobID, row.ChunkNumber, row.WaveNo, string(idsJSON), len(row.ProductIDs), statusQueued)
	}
	sb.WriteString(" RETURNING cjc_chunk_id")
	dbRows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("bulk insert chunks: %w", err)
	}
	defer func() {
		if e := dbRows.Close(); e != nil {
			_ = e
		}
	}()
	i := 0
	for dbRows.Next() {
		if err := dbRows.Scan(&rows[i].ChunkID); err != nil {
			return fmt.Errorf("scan returned chunk id: %w", err)
		}
		i++
	}
	if err := dbRows.Err(); err != nil {
		return fmt.Errorf("iterate returned chunk ids: %w", err)
	}
	return nil
}

// MarkRemainingChunksSkipped flips all QUEUED chunks at or after fromWave to
// FAILED with a cancellation reason. Used when a job is cancelled mid-flight
// and the orchestrator must stop dispatching subsequent waves. We reuse the
// FAILED status (not a new SKIPPED status) to avoid widening the
// chk_cjc_status CHECK constraint.
func (r *ChunkRepo) MarkRemainingChunksSkipped(ctx context.Context, jobID int64, fromWave int) error {
	const q = `
		UPDATE cal_job_chunk
		SET cjc_status='FAILED',
		    cjc_completed_at=now(),
		    cjc_error_message='job cancelled before dispatch'
		WHERE cjc_job_id=$1 AND cjc_wave_no >= $2 AND cjc_status=$3
	`
	if _, err := r.db.ExecContext(ctx, q, jobID, fromWave, statusQueued); err != nil {
		return fmt.Errorf("mark remaining chunks skipped: %w", err)
	}
	return nil
}

// UpdateDispatched flips a chunk to DISPATCHED + sets cjc_dispatched_at.
func (r *ChunkRepo) UpdateDispatched(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cal_job_chunk SET cjc_status=$1, cjc_dispatched_at=now() WHERE cjc_chunk_id=$2`,
		statusDispatched, id,
	); err != nil {
		return fmt.Errorf("update chunk dispatched: %w", err)
	}
	return nil
}

// CountByJobWave returns (totalChunks, completedChunks) for the given job +
// wave. A chunk is "completed" when it transitions to a terminal status.
func (r *ChunkRepo) CountByJobWave(ctx context.Context, jobID int64, wave int) (total, completed int, err error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE cjc_status IN ('SUCCESS','PARTIAL_FAILED','FAILED'))
		FROM cal_job_chunk WHERE cjc_job_id=$1 AND cjc_wave_no=$2
	`, jobID, wave)
	if scanErr := row.Scan(&total, &completed); scanErr != nil {
		err = fmt.Errorf("count chunks by wave: %w", scanErr)
	}
	return
}

// ListChunksOfWave returns the rows needed to re-publish ChunkSpec messages.
func (r *ChunkRepo) ListChunksOfWave(ctx context.Context, jobID int64, wave int) ([]*ChunkRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cjc_chunk_id, cjc_chunk_number, cjc_product_ids
		FROM cal_job_chunk WHERE cjc_job_id=$1 AND cjc_wave_no=$2
		ORDER BY cjc_chunk_number
	`, jobID, wave)
	if err != nil {
		return nil, fmt.Errorf("list chunks of wave: %w", err)
	}
	defer func() {
		if e := rows.Close(); e != nil {
			_ = e
		}
	}()
	var out []*ChunkRow
	for rows.Next() {
		var c ChunkRow
		var idsJSON []byte
		if err := rows.Scan(&c.ChunkID, &c.ChunkNumber, &idsJSON); err != nil {
			return nil, fmt.Errorf("scan chunk row: %w", err)
		}
		if err := json.Unmarshal(idsJSON, &c.ProductIDs); err != nil {
			return nil, fmt.Errorf("unmarshal product_ids: %w", err)
		}
		c.JobID = jobID
		c.WaveNo = wave
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunk rows: %w", err)
	}
	return out, nil
}

// JobProductRepo is the orchestrator-side facade over cal_job_product.
type JobProductRepo struct{ db *sql.DB }

// NewJobProductRepo constructs a JobProductRepo.
func NewJobProductRepo(db *sql.DB) *JobProductRepo { return &JobProductRepo{db: db} }

// BulkInsert inserts N cal_job_product rows in one statement.
func (r *JobProductRepo) BulkInsert(ctx context.Context, rows []*JobProductRow) error {
	if len(rows) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`INSERT INTO cal_job_product (cjp_job_id, cjp_chunk_id, cjp_product_sys_id, cjp_route_head_id, cjp_wave_no, cjp_status) VALUES `)
	args := make([]any, 0, len(rows)*6)
	for i, row := range rows {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 6
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
		var chunkID any
		if row.ChunkID > 0 {
			chunkID = row.ChunkID
		} else {
			chunkID = nil
		}
		var routeHeadID any
		if row.RouteHeadID > 0 {
			routeHeadID = row.RouteHeadID
		} else {
			routeHeadID = nil
		}
		args = append(args, row.JobID, chunkID, row.ProductSysID, routeHeadID, row.WaveNo, statusPending)
	}
	sb.WriteString(" ON CONFLICT (cjp_job_id, cjp_product_sys_id) DO NOTHING")
	if _, err := r.db.ExecContext(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("bulk insert job_products: %w", err)
	}
	return nil
}

// ResolveProductRouteMap returns map[productSysID]=routeHeadID for the active
// COMPLETE/LOCKED route head per product.
//
// Two cases (self-contained DAG model — see migration 000244):
//
//  1. The product is the head of its own route (FG-level products): match via
//     cost_route_head.crh_product_sys_id.
//  2. The product is an intermediate that exists only as a seq inside another
//     FG's route_head: match via cost_route_seq.crs_product_sys_id and return
//     the seq's owning head id.
//
// UNION ALL and DISTINCT ON (priority FG-as-head over seq-membership) ensures
// each product resolves to ONE route_head.
func (r *JobProductRepo) ResolveProductRouteMap(ctx context.Context, productSysIDs []int64) (map[int64]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (product_sys_id) product_sys_id, head_id
		FROM (
		  SELECT crh.crh_product_sys_id AS product_sys_id, crh.crh_head_id AS head_id, 0 AS rank
		  FROM cost_route_head crh
		  WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		    AND crh.crh_deleted_at IS NULL
		    AND crh.crh_product_sys_id = ANY($1)
		  UNION ALL
		  SELECT crs.crs_product_sys_id AS product_sys_id, crs.crs_head_id AS head_id, 1 AS rank
		  FROM cost_route_seq crs
		  JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
		  WHERE crh.crh_routing_status IN ('COMPLETE','LOCKED')
		    AND crh.crh_deleted_at IS NULL
		    AND crs.crs_product_sys_id = ANY($1)
		) t
		ORDER BY product_sys_id, rank ASC, head_id DESC
	`, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("resolve route map: %w", err)
	}
	defer func() {
		if e := rows.Close(); e != nil {
			_ = e
		}
	}()
	out := map[int64]int64{}
	for rows.Next() {
		var pid, hid int64
		if err := rows.Scan(&pid, &hid); err != nil {
			return nil, fmt.Errorf("scan route map row: %w", err)
		}
		out[pid] = hid
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route map: %w", err)
	}
	return out, nil
}
