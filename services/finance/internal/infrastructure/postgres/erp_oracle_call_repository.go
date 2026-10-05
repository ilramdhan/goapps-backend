package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erp_oracle_call_repository.go persists cst_erp_oracle_call_log (migration
// 000554; design §S-R11, plan-06 P5-T3). Each statement runs on its own
// connection (autocommit), outside the step transaction, so the STARTED row
// is durable before the Oracle call is issued and survives a rollback of the
// step. PG-only: nothing here talks to Oracle.

// ErpOracleCallRepository implements erpintegration.OracleCallLog.
type ErpOracleCallRepository struct{ db *DB }

// NewErpOracleCallRepository constructs the repository.
func NewErpOracleCallRepository(db *DB) *ErpOracleCallRepository {
	return &ErpOracleCallRepository{db: db}
}

var _ erpintegration.OracleCallLog = (*ErpOracleCallRepository)(nil)

// ErrOracleCallNotFound is returned by Finish for an unknown call id or a
// call that is no longer STARTED.
var ErrOracleCallNotFound = errors.New("erp oracle call log: STARTED call not found")

const erpOracleCallHasAttemptSQL = `
	SELECT EXISTS (SELECT 1 FROM cst_erp_oracle_call_log
	                WHERE ceocl_batch_id = $1 AND ceocl_statement_key = $2)`

const erpOracleCallStartSQL = `
	INSERT INTO cst_erp_oracle_call_log
	       (ceocl_call_id, ceocl_batch_id, ceocl_job_id, ceocl_statement_key,
	        ceocl_params, ceocl_status, ceocl_attempt, ceocl_actor)
	VALUES ($1::uuid, $2, NULLIF($3, '')::uuid, $4, COALESCE($5::jsonb, '{}'::jsonb), 'STARTED', $6, $7)`

const erpOracleCallFinishSQL = `
	UPDATE cst_erp_oracle_call_log
	   SET ceocl_status = $2, ceocl_ora_code = NULLIF($3, ''), ceocl_error = NULLIF($4, ''),
	       ceocl_rows = $5, ceocl_summary = $6::jsonb, ceocl_finished_at = NOW()
	 WHERE ceocl_call_id = $1::uuid AND ceocl_status = 'STARTED'`

// HasAttempt reports whether any call was logged for (batch, statement key).
func (r *ErpOracleCallRepository) HasAttempt(ctx context.Context, batchID int64, statementKey string) (bool, error) {
	var ok bool
	if err := r.db.QueryRowContext(ctx, erpOracleCallHasAttemptSQL, batchID, statementKey).Scan(&ok); err != nil {
		return false, fmt.Errorf("erp oracle call has attempt: %w", err)
	}
	return ok, nil
}

// Start inserts the STARTED row (committed immediately).
func (r *ErpOracleCallRepository) Start(ctx context.Context, c erpintegration.OracleCallStart) error {
	attempt := c.Attempt
	if attempt < 1 {
		attempt = 1
	}
	var params any
	if len(c.Params) > 0 {
		params = string(c.Params)
	}
	var batch any
	if c.BatchID > 0 {
		batch = c.BatchID
	}
	if _, err := r.db.ExecContext(ctx, erpOracleCallStartSQL, c.CallID, batch, strings.TrimSpace(c.JobID),
		c.StatementKey, params, attempt, strings.TrimSpace(c.Actor)); err != nil {
		return fmt.Errorf("erp oracle call start: %w", err)
	}
	return nil
}

// Finish records the terminal status of a STARTED call.
func (r *ErpOracleCallRepository) Finish(ctx context.Context, callID string, f erpintegration.OracleCallFinish) error {
	if !f.Status.IsTerminal() {
		return fmt.Errorf("erp oracle call finish: status %q is not terminal", f.Status)
	}
	var rows sql.NullInt64
	if f.Rows != nil {
		rows = sql.NullInt64{Int64: *f.Rows, Valid: true}
	}
	var summary any
	if len(f.Summary) > 0 {
		summary = string(f.Summary)
	}
	res, err := r.db.ExecContext(ctx, erpOracleCallFinishSQL, callID, string(f.Status), f.OraCode, f.Error, rows, summary)
	if err != nil {
		return fmt.Errorf("erp oracle call finish: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp oracle call finish rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrOracleCallNotFound, callID)
	}
	return nil
}

var _ erpintegration.OracleCallResolver = (*ErpOracleCallRepository)(nil)

const erpOracleCallUnresolvedSQL = `
	SELECT ceocl_call_id::text, ceocl_statement_key, ceocl_status, ceocl_params::text
	  FROM cst_erp_oracle_call_log
	 WHERE ceocl_batch_id = $1 AND ceocl_status IN ('STARTED', 'UNKNOWN')
	 ORDER BY ceocl_started_at, ceocl_id`

// erpOracleCallResolveSQL settles a STARTED / UNKNOWN row from read-only
// evidence (recon); the note is merged into ceocl_summary under "resolution".
const erpOracleCallResolveSQL = `
	UPDATE cst_erp_oracle_call_log
	   SET ceocl_status = $2,
	       ceocl_summary = COALESCE(ceocl_summary, '{}'::jsonb) || jsonb_build_object('resolution', COALESCE($3::jsonb, '{}'::jsonb)),
	       ceocl_finished_at = COALESCE(ceocl_finished_at, NOW())
	 WHERE ceocl_call_id = $1::uuid AND ceocl_status IN ('STARTED', 'UNKNOWN')`

// ListUnresolved returns the batch's STARTED / UNKNOWN call-log rows.
func (r *ErpOracleCallRepository) ListUnresolved(ctx context.Context, batchID int64) (out []erpintegration.OracleCallRecord, err error) {
	rows, err := r.db.QueryContext(ctx, erpOracleCallUnresolvedSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp oracle call list unresolved: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp oracle call list unresolved")
	for rows.Next() {
		var rec erpintegration.OracleCallRecord
		var status, params string
		if err := rows.Scan(&rec.CallID, &rec.StatementKey, &status, &params); err != nil {
			return nil, fmt.Errorf("erp oracle call list unresolved scan: %w", err)
		}
		rec.Status, rec.Params = erpintegration.OracleCallStatus(status), []byte(params)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp oracle call list unresolved: %w", err)
	}
	return out, nil
}

// Resolve settles one STARTED / UNKNOWN row to SUCCESS or FAILED.
func (r *ErpOracleCallRepository) Resolve(ctx context.Context, callID string, to erpintegration.OracleCallStatus, note json.RawMessage) error {
	if to != erpintegration.OracleCallSuccess && to != erpintegration.OracleCallFailed {
		return fmt.Errorf("erp oracle call resolve: target %q must be SUCCESS or FAILED", to)
	}
	var n any
	if len(note) > 0 {
		n = string(note)
	}
	res, err := r.db.ExecContext(ctx, erpOracleCallResolveSQL, callID, string(to), n)
	if err != nil {
		return fmt.Errorf("erp oracle call resolve: %w", err)
	}
	got, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp oracle call resolve rows: %w", err)
	}
	if got == 0 {
		return fmt.Errorf("%w: %s", erpintegration.ErrCallAlreadyResolved, callID)
	}
	return nil
}

const erpOracleCallListSQL = `
	SELECT ceocl_id, ceocl_statement_key, ceocl_status, ceocl_actor, ceocl_started_at,
	       COALESCE((EXTRACT(EPOCH FROM (ceocl_finished_at - ceocl_started_at)) * 1000)::bigint, 0),
	       COALESCE(ceocl_error, ''),
	       COALESCE(ceocl_ora_code, ''), ceocl_attempt::int, ceocl_finished_at
	  FROM cst_erp_oracle_call_log
	 WHERE ceocl_batch_id = $1
	 ORDER BY ceocl_started_at, ceocl_id`

// ListByBatch returns every call-log row of the batch, oldest first.
func (r *ErpOracleCallRepository) ListByBatch(ctx context.Context, batchID int64) (out []erpintegration.OracleCallRow, err error) {
	rows, err := r.db.QueryContext(ctx, erpOracleCallListSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp oracle call list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp oracle call list")
	for rows.Next() {
		var c erpintegration.OracleCallRow
		if err := rows.Scan(&c.ID, &c.StatementKey, &c.Status, &c.Actor, &c.StartedAt, &c.DurationMs, &c.Error, &c.OraCode, &c.Attempts, &c.FinishedAt); err != nil {
			return nil, fmt.Errorf("erp oracle call list scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp oracle call list: %w", err)
	}
	return out, nil
}
