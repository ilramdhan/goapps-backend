package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// CostCalcJobProductRepository persists JobProduct rows against `cal_job_product`.
type CostCalcJobProductRepository struct {
	db *DB
}

// NewCostCalcJobProductRepository constructs a CostCalcJobProductRepository.
func NewCostCalcJobProductRepository(db *DB) *CostCalcJobProductRepository {
	return &CostCalcJobProductRepository{db: db}
}

var _ costcalc.JobProductRepository = (*CostCalcJobProductRepository)(nil)

const jobProductColumns = `cjp_job_product_id, cjp_job_id, COALESCE(cjp_chunk_id, 0),
		       cjp_product_sys_id, cjp_route_head_id, cjp_wave_no, cjp_status,
		       COALESCE(cjp_block_reason, ''), cjp_started_at, cjp_completed_at,
		       COALESCE(cjp_duration_ms, 0), COALESCE(cjp_cost_id, 0),
		       COALESCE(cjp_error_message, ''), cjp_calculation_log`

// BulkCreate inserts the provided JobProducts in a single multi-row INSERT
// inside a transaction, then assigns the returned IDs back onto the items.
func (r *CostCalcJobProductRepository) BulkCreate(ctx context.Context, items []*costcalc.JobProduct) error {
	if len(items) == 0 {
		return nil
	}
	query, args, err := buildJobProductBulkInsert(items)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bulk-create tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("bulk insert job products: %w", err)
	}
	defer closeRows(rows)

	idx := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan inserted job product id: %w", err)
		}
		if idx < len(items) {
			items[idx].AssignID(id)
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate inserted job product ids: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bulk-create tx: %w", err)
	}
	committed = true
	return nil
}

// buildJobProductBulkInsert assembles a parameterized multi-row INSERT.
func buildJobProductBulkInsert(items []*costcalc.JobProduct) (string, []any, error) {
	placeholders := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*5)
	idx := 1
	for _, p := range items {
		if p == nil {
			continue
		}
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", idx, idx+1, idx+2, idx+3, idx+4))
		args = append(args, p.JobID(), p.ProductSysID(), p.RouteHeadID(),
			safeconv.IntToInt32(p.WaveNo()), string(p.Status()))
		idx += 5
	}
	if len(placeholders) == 0 {
		return "", nil, fmt.Errorf("build bulk insert: no items to insert")
	}
	q := `
		INSERT INTO cal_job_product (
			cjp_job_id, cjp_product_sys_id, cjp_route_head_id, cjp_wave_no, cjp_status
		) VALUES ` + strings.Join(placeholders, ", ") + `
		RETURNING cjp_job_product_id`
	return q, args, nil
}

// GetByJobAndProduct returns a hydrated JobProduct or ErrJobNotFound.
func (r *CostCalcJobProductRepository) GetByJobAndProduct(ctx context.Context, jobID, productSysID int64) (*costcalc.JobProduct, error) {
	q := `SELECT ` + jobProductColumns + ` FROM cal_job_product
		   WHERE cjp_job_id = $1 AND cjp_product_sys_id = $2`
	jp, err := scanJobProduct(r.db.QueryRowContext(ctx, q, jobID, productSysID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costcalc.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job product: %w", err)
	}
	return jp, nil
}

// ListByJob returns paginated job products.
func (r *CostCalcJobProductRepository) ListByJob(ctx context.Context, jobID int64, f costcalc.JobProductFilter) ([]*costcalc.JobProduct, int, error) {
	where := []string{"cjp_job_id = $1"}
	args := []any{jobID}
	idx := 2
	if f.Status != "" {
		where = append(where, fmt.Sprintf("cjp_status = $%d", idx))
		args = append(args, string(f.Status))
		idx++
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM cal_job_product`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count job products: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	offset := (page - 1) * pageSize

	listSQL := `SELECT ` + jobProductColumns + ` FROM cal_job_product` + whereSQL +
		` ORDER BY cjp_wave_no ASC, cjp_job_product_id ASC` +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list job products: %w", err)
	}
	defer closeRows(rows)

	out := []*costcalc.JobProduct{}
	for rows.Next() {
		jp, scanErr := scanJobProduct(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan job product: %w", scanErr)
		}
		out = append(out, jp)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate job products: %w", err)
	}
	return out, total, nil
}

// AssignChunk sets the chunk id for a (job, product) pair.
func (r *CostCalcJobProductRepository) AssignChunk(ctx context.Context, jobID, productSysID, chunkID int64) error {
	return r.execJobProductUpdate(ctx,
		`UPDATE cal_job_product SET cjp_chunk_id = $3
		   WHERE cjp_job_id = $1 AND cjp_product_sys_id = $2`,
		jobID, productSysID, chunkID,
	)
}

// MarkSuccess records a successful per-product calc result.
func (r *CostCalcJobProductRepository) MarkSuccess(ctx context.Context, jobID, productSysID, costID int64, durationMs int, log []byte) error {
	return r.execJobProductUpdate(ctx,
		`UPDATE cal_job_product
		    SET cjp_status = 'SUCCESS', cjp_cost_id = NULLIF($3, 0)::BIGINT,
		        cjp_duration_ms = $4, cjp_calculation_log = $5,
		        cjp_completed_at = now()
		  WHERE cjp_job_id = $1 AND cjp_product_sys_id = $2`,
		jobID, productSysID, costID, safeconv.IntToInt32(durationMs), nullableJSON(log),
	)
}

// MarkFailed records a non-recoverable error.
func (r *CostCalcJobProductRepository) MarkFailed(ctx context.Context, jobID, productSysID int64, errMsg string, log []byte) error {
	return r.execJobProductUpdate(ctx,
		`UPDATE cal_job_product
		    SET cjp_status = 'FAILED',
		        cjp_error_message = NULLIF($3, ''), cjp_calculation_log = $4,
		        cjp_completed_at = now()
		  WHERE cjp_job_id = $1 AND cjp_product_sys_id = $2`,
		jobID, productSysID, errMsg, nullableJSON(log),
	)
}

// MarkBlocked records a dependency / data-gap block.
func (r *CostCalcJobProductRepository) MarkBlocked(ctx context.Context, jobID, productSysID int64, reason string, log []byte) error {
	return r.execJobProductUpdate(ctx,
		`UPDATE cal_job_product
		    SET cjp_status = 'BLOCKED',
		        cjp_block_reason = NULLIF($3, ''), cjp_calculation_log = $4,
		        cjp_completed_at = now()
		  WHERE cjp_job_id = $1 AND cjp_product_sys_id = $2`,
		jobID, productSysID, reason, nullableJSON(log),
	)
}

// MarkSkippedForJob bulk-skips any rows still in PENDING/READY for the job.
func (r *CostCalcJobProductRepository) MarkSkippedForJob(ctx context.Context, jobID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cal_job_product
		    SET cjp_status = 'SKIPPED', cjp_completed_at = now()
		  WHERE cjp_job_id = $1 AND cjp_status IN ('PENDING', 'READY')`,
		jobID)
	if err != nil {
		return fmt.Errorf("mark skipped for job: %w", err)
	}
	return nil
}

// execJobProductUpdate runs an UPDATE keyed on (job_id, product_sys_id).
func (r *CostCalcJobProductRepository) execJobProductUpdate(ctx context.Context, q string, args ...any) error {
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update job product: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update job product rows: %w", err)
	}
	if n == 0 {
		return costcalc.ErrJobNotFound
	}
	return nil
}

// scanJobProduct reads one cal_job_product row.
func scanJobProduct(s rowScanner) (*costcalc.JobProduct, error) {
	var (
		id, jobID, chunkID        int64
		productSysID, routeHeadID int64
		waveNo                    int32
		status, blockReason       string
		startedAt, completedAt    sql.NullTime
		durationMs                int32
		costID                    int64
		errMsg                    string
		calcLog                   []byte
	)
	if err := s.Scan(
		&id, &jobID, &chunkID, &productSysID, &routeHeadID, &waveNo, &status,
		&blockReason, &startedAt, &completedAt, &durationMs, &costID, &errMsg, &calcLog,
	); err != nil {
		return nil, err
	}
	var startedPtr, completedPtr *time.Time
	if startedAt.Valid {
		t := startedAt.Time
		startedPtr = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		completedPtr = &t
	}
	return costcalc.HydrateJobProduct(
		id, jobID, chunkID, productSysID, routeHeadID, int(waveNo),
		costcalc.JobProductStatus(status), blockReason,
		startedPtr, completedPtr, int(durationMs), costID, errMsg, calcLog,
	), nil
}
