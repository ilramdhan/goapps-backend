package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErpIntBatchRepository implements erpintegration.BatchRepository over
// cst_erp_int_batch (migration 000549; design §4.2; plan-04 P3-T2).
//
//   - Create allocates ceib_seq = max+1 per period under a per-period
//     transaction advisory lock; a clash with uix_ceib_inflight or
//     uix_ceib_active maps to erpintegration.ErrActiveBatchExists.
//   - Save is optimistic: UPDATE … WHERE ceib_status = $expected. A miss is
//     ErrStaleBatchStatus (or ErrBatchNotFound). The transition's
//     Invalidation (demand / coverage / std rows) is applied in the same tx.
//
// ceib_valuation_progress is not part of the aggregate state and is never
// written here.
type ErpIntBatchRepository struct{ db *DB }

// NewErpIntBatchRepository constructs the repository.
func NewErpIntBatchRepository(db *DB) *ErpIntBatchRepository {
	return &ErpIntBatchRepository{db: db}
}

var _ erpintegration.BatchRepository = (*ErpIntBatchRepository)(nil)

// erpQuerier is the subset of *sql.DB / *sql.Tx the ERP batch helpers need.
type erpQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Constraint names of migration 000549 that the repository maps.
const (
	constraintCeibActive   = "uix_ceib_active"
	constraintCeibInflight = "uix_ceib_inflight"
	constraintCeibShadow   = "chk_ceib_shadow_status"
	sqlStateCheckViolation = "23514"
)

// erpBatchColumns is the select list shared by every batch read; scanBatch
// reads it in this exact order.
const erpBatchColumns = `
	ceib_batch_id, ceib_period, ceib_seq, ceib_mode, ceib_status,
	COALESCE(ceib_job_id::text, ''), ceib_demand_loaded_at,
	ceib_rule_snapshot::text, COALESCE(ceib_rule_hash, ''),
	ceib_row_count, ceib_sum_std, ceib_sum_conv, ceib_sum_pvl,
	ceib_summary::text,
	ceib_warnings_ack_at, ceib_warnings_ack_by,
	ceib_pushed_at, ceib_pushed_by,
	ceib_valuated_at, ceib_valuated_by,
	ceib_reconciled_at,
	ceib_adj_approved_at, ceib_adj_approved_by,
	ceib_locked_at, ceib_locked_by,
	ceib_needs_repush, COALESCE(ceib_error, ''),
	created_at, created_by, updated_at, COALESCE(updated_by, '')`

// erpBatchPeriodLockSQL serializes seq allocation per period. It is a
// different key space ('erp:period:') from the G11 per-batch lock ('erp:').
const erpBatchPeriodLockSQL = `SELECT pg_advisory_xact_lock(hashtext('erp:period:' || $1::text))`

const erpBatchInsertSQL = `
	INSERT INTO cst_erp_int_batch (
		ceib_period, ceib_seq, ceib_status, ceib_mode, ceib_summary,
		created_at, created_by, updated_at, updated_by)
	SELECT $1::text, COALESCE(MAX(ceib_seq), 0) + 1, $2, $3, $4::jsonb, $5::timestamptz, $6::text, $5::timestamptz, $6::text
	  FROM cst_erp_int_batch
	 WHERE ceib_period = $1::text
	RETURNING ceib_batch_id, ceib_seq`

// Create inserts a new DRAFT batch and returns it with id and seq set.
func (r *ErpIntBatchRepository) Create(ctx context.Context, b *erpintegration.Batch) (*erpintegration.Batch, error) {
	if b == nil {
		return nil, fmt.Errorf("erp batch create: nil batch")
	}
	st := b.State()
	if st.Status != erpintegration.StatusDraft {
		return nil, fmt.Errorf("erp batch create: %w: new batch must be DRAFT, got %s", erpintegration.ErrInvalidBatchStatus, st.Status)
	}
	var out *erpintegration.Batch
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, erpBatchPeriodLockSQL, st.Period); err != nil {
			return fmt.Errorf("erp batch create period lock: %w", err)
		}
		var (
			id  int64
			seq int
		)
		err := tx.QueryRowContext(ctx, erpBatchInsertSQL,
			st.Period, string(st.Status), string(st.Mode), jsonOrEmptyObject(st.Summary),
			st.CreatedAt, st.CreatedBy).Scan(&id, &seq)
		if err != nil {
			return mapBatchWriteError("erp batch create", err)
		}
		got, err := getBatch(ctx, tx, id, false)
		if err != nil {
			return err
		}
		out = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetByID returns the batch, or ErrBatchNotFound.
func (r *ErpIntBatchRepository) GetByID(ctx context.Context, id int64) (*erpintegration.Batch, error) {
	return getBatch(ctx, r.db, id, false)
}

// ListByPeriod returns every batch of the period, newest seq first.
func (r *ErpIntBatchRepository) ListByPeriod(ctx context.Context, period string) (out []*erpintegration.Batch, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+erpBatchColumns+` FROM cst_erp_int_batch WHERE ceib_period = $1 ORDER BY ceib_seq DESC`, period)
	if err != nil {
		return nil, fmt.Errorf("erp batch list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp batch list")
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp batch list rows: %w", err)
	}
	return out, nil
}

// FindActive returns the LIVE VALUATED/RECONCILED/LOCKED batch of the period.
func (r *ErpIntBatchRepository) FindActive(ctx context.Context, period string) (*erpintegration.Batch, error) {
	return findOneBatch(ctx, r.db, `SELECT `+erpBatchColumns+` FROM cst_erp_int_batch
		 WHERE ceib_period = $1 AND ceib_mode = 'LIVE'
		   AND ceib_status IN ('VALUATED', 'RECONCILED', 'LOCKED')`, period)
}

// FindInFlight returns the LIVE DRAFT..PUSHED batch of the period.
func (r *ErpIntBatchRepository) FindInFlight(ctx context.Context, period string) (*erpintegration.Batch, error) {
	return findOneBatch(ctx, r.db, `SELECT `+erpBatchColumns+` FROM cst_erp_int_batch
		 WHERE ceib_period = $1 AND ceib_mode = 'LIVE'
		   AND ceib_status IN ('DRAFT', 'DEMAND_LOADED', 'COVERED', 'DERIVED', 'VALIDATED', 'PUSHED')`, period)
}

// Save persists b if its stored status still equals expected, applying inv in
// the same transaction.
func (r *ErpIntBatchRepository) Save(ctx context.Context, b *erpintegration.Batch, expected erpintegration.BatchStatus, inv erpintegration.Invalidation) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		return saveBatch(ctx, tx, b, expected, inv)
	})
}

func findOneBatch(ctx context.Context, q erpQuerier, query, period string) (*erpintegration.Batch, error) {
	b, err := scanBatch(q.QueryRowContext(ctx, query, period))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erpintegration.ErrBatchNotFound
	}
	return b, err
}

func getBatch(ctx context.Context, q erpQuerier, id int64, forUpdate bool) (*erpintegration.Batch, error) {
	query := `SELECT ` + erpBatchColumns + ` FROM cst_erp_int_batch WHERE ceib_batch_id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	b, err := scanBatch(q.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erpintegration.ErrBatchNotFound
	}
	return b, err
}

const erpBatchUpdateSQL = `
	UPDATE cst_erp_int_batch SET
		ceib_status           = $3,
		ceib_job_id           = NULLIF($4::text, '')::uuid,
		ceib_demand_loaded_at = $5,
		ceib_rule_snapshot    = $6::jsonb,
		ceib_rule_hash        = NULLIF($7::text, ''),
		ceib_row_count        = $8,
		ceib_sum_std          = $9,
		ceib_sum_conv         = $10,
		ceib_sum_pvl          = $11,
		ceib_summary          = $12::jsonb,
		ceib_warnings_ack_at  = $13,
		ceib_warnings_ack_by  = $14,
		ceib_pushed_at        = $15,
		ceib_pushed_by        = $16,
		ceib_valuated_at      = $17,
		ceib_valuated_by      = $18,
		ceib_reconciled_at    = $19,
		ceib_adj_approved_at  = $20,
		ceib_adj_approved_by  = $21,
		ceib_locked_at        = $22,
		ceib_locked_by        = $23,
		ceib_needs_repush     = $24,
		ceib_error            = NULLIF($25::text, ''),
		updated_at            = $26,
		updated_by            = NULLIF($27::text, '')
	 WHERE ceib_batch_id = $1 AND ceib_status = $2`

// saveBatch runs the optimistic UPDATE and applies inv. The invalidation is
// applied first so a failed status update rolls it back with the tx.
func saveBatch(ctx context.Context, q erpQuerier, b *erpintegration.Batch, expected erpintegration.BatchStatus, inv erpintegration.Invalidation) error {
	if b == nil || b.ID() <= 0 {
		return fmt.Errorf("erp batch save: batch is not persisted")
	}
	if !expected.IsValid() {
		return fmt.Errorf("erp batch save: %w: expected %q", erpintegration.ErrInvalidBatchStatus, expected)
	}
	if err := applyInvalidation(ctx, q, b.ID(), inv); err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, erpBatchUpdateSQL, batchUpdateArgs(b, expected)...)
	if err != nil {
		return mapBatchWriteError("erp batch save", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp batch save rows affected: %w", err)
	}
	if n == 1 {
		return nil
	}
	var exists bool
	if err := q.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM cst_erp_int_batch WHERE ceib_batch_id = $1)`, b.ID()).Scan(&exists); err != nil {
		return fmt.Errorf("erp batch save existence: %w", err)
	}
	if !exists {
		return erpintegration.ErrBatchNotFound
	}
	return fmt.Errorf("%w: batch %d is no longer %s", erpintegration.ErrStaleBatchStatus, b.ID(), expected)
}

func batchUpdateArgs(b *erpintegration.Batch, expected erpintegration.BatchStatus) []any {
	st := b.State()
	var (
		rowCount               sql.NullInt64
		sumStd, sumConv, sumPv decimal.NullDecimal
	)
	if st.Totals != nil {
		rowCount = sql.NullInt64{Int64: st.Totals.RowCount(), Valid: true}
		sumStd = decimal.NullDecimal{Decimal: st.Totals.SumStd(), Valid: true}
		sumConv = decimal.NullDecimal{Decimal: st.Totals.SumConv(), Valid: true}
		sumPv = decimal.NullDecimal{Decimal: st.Totals.SumPvl(), Valid: true}
	}
	ackAt, ackBy := stampArgs(st.WarningsAck)
	pushAt, pushBy := stampArgs(st.Pushed)
	valAt, valBy := stampArgs(st.Valuated)
	apprAt, apprBy := stampArgs(st.AdjApproved)
	lockAt, lockBy := stampArgs(st.Locked)
	return []any{
		st.ID, string(expected),
		string(st.Status), st.JobID, nullTimeArg(st.DemandLoadedAt),
		nullJSONArg(st.RuleSnapshot), st.RuleHash,
		rowCount, sumStd, sumConv, sumPv,
		jsonOrEmptyObject(st.Summary),
		ackAt, ackBy, pushAt, pushBy, valAt, valBy,
		nullTimeArg(st.ReconciledAt),
		apprAt, apprBy, lockAt, lockBy,
		st.NeedsRepush, st.Error, st.UpdatedAt, st.UpdatedBy,
	}
}

// applyInvalidation deletes the batch-owned rows a transition made obsolete.
func applyInvalidation(ctx context.Context, q erpQuerier, batchID int64, inv erpintegration.Invalidation) error {
	steps := []struct {
		on    bool
		query string
		what  string
	}{
		{inv.StdRows, `DELETE FROM cst_erp_std_cost WHERE cesc_batch_id = $1`, "std rows"},
		{inv.Coverage, `DELETE FROM cst_erp_coverage WHERE cec_batch_id = $1`, "coverage"},
		{inv.Demand, `DELETE FROM cst_erp_adj_demand WHERE ced_batch_id = $1`, "demand"},
	}
	for _, s := range steps {
		if !s.on {
			continue
		}
		if _, err := q.ExecContext(ctx, s.query, batchID); err != nil {
			return fmt.Errorf("erp batch invalidate %s: %w", s.what, err)
		}
	}
	return nil
}

// mapBatchWriteError maps the 000549 constraints to domain errors.
func mapBatchWriteError(what string, err error) error {
	state, constraint, ok := pgErrorInfo(err)
	if ok && state == sqlStateUniqueViolation &&
		(constraint == constraintCeibActive || constraint == constraintCeibInflight) {
		return fmt.Errorf("%w (%s)", erpintegration.ErrActiveBatchExists, constraint)
	}
	if ok && state == sqlStateCheckViolation && constraint == constraintCeibShadow {
		return fmt.Errorf("%w (%s)", erpintegration.ErrShadowNotPushable, constraint)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// scanBatch reads one erpBatchColumns row (from *sql.Row or *sql.Rows).
func scanBatch(s rowScanner) (*erpintegration.Batch, error) {
	var (
		st                              erpintegration.BatchState
		mode, status                    string
		demandLoadedAt, reconciledAt    sql.NullTime
		ruleSnapshot                    sql.NullString
		summary                         string
		rowCount                        sql.NullInt64
		sumStd, sumConv, sumPvl         decimal.NullDecimal
		ackAt, pushAt, valAt, apprAt    sql.NullTime
		lockAt                          sql.NullTime
		ackBy, pushBy, valBy, apprBy    sql.NullString
		lockBy                          sql.NullString
		createdAt, updatedAt            time.Time
		createdBy, updatedBy, errorText string
	)
	err := s.Scan(
		&st.ID, &st.Period, &st.Seq, &mode, &status,
		&st.JobID, &demandLoadedAt,
		&ruleSnapshot, &st.RuleHash,
		&rowCount, &sumStd, &sumConv, &sumPvl,
		&summary,
		&ackAt, &ackBy, &pushAt, &pushBy, &valAt, &valBy,
		&reconciledAt,
		&apprAt, &apprBy, &lockAt, &lockBy,
		&st.NeedsRepush, &errorText,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("erp batch scan: %w", err)
	}
	st.Mode = erpintegration.BatchMode(mode)
	st.Status = erpintegration.BatchStatus(status)
	st.RuleHash = strings.TrimSpace(st.RuleHash)
	st.DemandLoadedAt = timePtr(demandLoadedAt)
	st.ReconciledAt = timePtr(reconciledAt)
	if ruleSnapshot.Valid {
		st.RuleSnapshot = []byte(ruleSnapshot.String)
	}
	st.Summary = []byte(summary)
	if rowCount.Valid {
		t, err := erpintegration.NewControlTotals(rowCount.Int64, sumStd.Decimal, sumConv.Decimal, sumPvl.Decimal)
		if err != nil {
			return nil, fmt.Errorf("erp batch %d totals: %w", st.ID, err)
		}
		st.Totals = &t
	}
	st.WarningsAck = stampPtr(ackAt, ackBy)
	st.Pushed = stampPtr(pushAt, pushBy)
	st.Valuated = stampPtr(valAt, valBy)
	st.AdjApproved = stampPtr(apprAt, apprBy)
	st.Locked = stampPtr(lockAt, lockBy)
	st.Error = errorText
	st.CreatedAt, st.CreatedBy, st.UpdatedAt, st.UpdatedBy = createdAt, createdBy, updatedAt, updatedBy
	b, err := erpintegration.ReconstituteBatch(st)
	if err != nil {
		return nil, fmt.Errorf("erp batch %d reconstitute: %w", st.ID, err)
	}
	return b, nil
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func stampPtr(at sql.NullTime, by sql.NullString) *erpintegration.ActorStamp {
	if !at.Valid {
		return nil
	}
	return &erpintegration.ActorStamp{At: at.Time, By: by.String}
}

func stampArgs(s *erpintegration.ActorStamp) (sql.NullTime, sql.NullString) {
	if s == nil {
		return sql.NullTime{}, sql.NullString{}
	}
	return sql.NullTime{Time: s.At, Valid: true}, sql.NullString{String: s.By, Valid: s.By != ""}
}

func nullTimeArg(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func nullJSONArg(b []byte) sql.NullString {
	if b == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func jsonOrEmptyObject(b []byte) string {
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}
