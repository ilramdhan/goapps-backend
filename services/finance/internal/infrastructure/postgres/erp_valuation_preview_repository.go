package postgres

// erp_valuation_preview_repository.go implements
// erpintegration.ValuationPreviewRepository (plan-06 P5-T4 steps 2-3;
// design Part 1 §3.2 G4-G7a, §4.7). CreateOpen stores the preview and its
// immutable ADJ snapshot in ONE PG transaction under the G11 batch advisory
// lock, so the snapshot is committed before any writer call can reference
// the preview (G7). PG side only: nothing here talks to Oracle.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// constraintCevpLive is the 000554 one-live-preview-per-(batch, operation) index.
const constraintCevpLive = "uix_cevp_live"

// ErpValuationPreviewRepository implements erpintegration.ValuationPreviewRepository.
type ErpValuationPreviewRepository struct{ db *DB }

// NewErpValuationPreviewRepository constructs the repository.
func NewErpValuationPreviewRepository(db *DB) *ErpValuationPreviewRepository {
	return &ErpValuationPreviewRepository{db: db}
}

var _ erpintegration.ValuationPreviewRepository = (*ErpValuationPreviewRepository)(nil)

const erpPreviewColumns = `
	cevp_preview_id::text, cevp_batch_id, cevp_operation, cevp_status,
	COALESCE(cevp_head_count, 0), COALESCE(cevp_item_count, 0),
	cevp_excluded::text, COALESCE(cevp_set_hash, ''), cevp_totals::text,
	COALESCE(cevp_confirm_text, ''), cevp_created_by, cevp_created_at, cevp_expires_at,
	cevp_consumed_at, COALESCE(cevp_consumed_by, '')`

const erpPreviewInsertSQL = `
	INSERT INTO cst_erp_valuation_preview (
		cevp_batch_id, cevp_operation, cevp_status, cevp_head_count, cevp_item_count,
		cevp_excluded, cevp_set_hash, cevp_totals, cevp_confirm_text,
		cevp_created_by, cevp_created_at, cevp_expires_at)
	VALUES ($1, $2, $3, $4, $5, $6::jsonb, NULLIF($7::text, ''), $8::jsonb, NULLIF($9::text, ''),
	        $10, COALESCE($11::timestamptz, NOW()), $12)
	RETURNING ` + erpPreviewColumns

const erpPreviewStaleLiveSQL = `
	UPDATE cst_erp_valuation_preview SET cevp_status = 'STALE'
	 WHERE cevp_batch_id = $1 AND cevp_operation = $2 AND cevp_status IN ('BUILDING', 'OPEN')`

const erpPreviewOpenSQL = `
	UPDATE cst_erp_valuation_preview SET cevp_status = 'OPEN'
	 WHERE cevp_preview_id = $1::uuid AND cevp_status = 'BUILDING'`

const erpPreviewBatchStatusSQL = `
	SELECT ceib_status FROM cst_erp_int_batch WHERE ceib_batch_id = $1 FOR SHARE`

// CreateOpen implements erpintegration.ValuationPreviewRepository.
func (r *ErpValuationPreviewRepository) CreateOpen(ctx context.Context, p erpintegration.ValuationPreview, period string, expected erpintegration.BatchStatus, rows []erpintegration.AdjSnapshotRow) (erpintegration.ValuationPreview, error) {
	if err := checkPreviewInput(p); err != nil {
		return erpintegration.ValuationPreview{}, err
	}
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return erpintegration.ValuationPreview{}, fmt.Errorf("erp preview create: %w", err)
	}
	if len(rows) == 0 {
		return erpintegration.ValuationPreview{}, fmt.Errorf("erp preview create: empty snapshot")
	}
	var out erpintegration.ValuationPreview
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		got, err := TryLockErpBatch(ctx, tx, p.BatchID)
		if err != nil {
			return err
		}
		if !got {
			return fmt.Errorf("%w: batch %d", erpintegration.ErrConcurrentRun, p.BatchID)
		}
		if err := checkBatchStatus(ctx, tx, p.BatchID, expected); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, erpPreviewStaleLiveSQL, p.BatchID, string(p.Operation)); err != nil {
			return fmt.Errorf("erp preview mark stale: %w", err)
		}
		p.Status = erpintegration.PreviewBuilding
		ins, err := insertPreview(ctx, tx, p)
		if err != nil {
			return err
		}
		if _, err := insertAdjSnapshot(ctx, tx, ins.ID, p.BatchID, period, rows); err != nil {
			return err
		}
		if err := openPreview(ctx, tx, ins.ID); err != nil {
			return err
		}
		ins.Status = erpintegration.PreviewOpen
		out = ins
		return nil
	})
	if err != nil {
		return erpintegration.ValuationPreview{}, err
	}
	return out, nil
}

// CreateFailed implements erpintegration.ValuationPreviewRepository.
func (r *ErpValuationPreviewRepository) CreateFailed(ctx context.Context, p erpintegration.ValuationPreview) (erpintegration.ValuationPreview, error) {
	if err := checkPreviewInput(p); err != nil {
		return erpintegration.ValuationPreview{}, err
	}
	p.Status = erpintegration.PreviewFailed
	return insertPreview(ctx, r.db, p)
}

// Get implements erpintegration.ValuationPreviewRepository.
func (r *ErpValuationPreviewRepository) Get(ctx context.Context, id string) (erpintegration.ValuationPreview, error) {
	if _, err := uuid.Parse(id); err != nil {
		return erpintegration.ValuationPreview{}, erpintegration.ErrPreviewNotFound
	}
	p, err := scanPreview(r.db.QueryRowContext(ctx,
		`SELECT `+erpPreviewColumns+` FROM cst_erp_valuation_preview WHERE cevp_preview_id = $1::uuid`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return erpintegration.ValuationPreview{}, erpintegration.ErrPreviewNotFound
	}
	return p, err
}

func checkPreviewInput(p erpintegration.ValuationPreview) error {
	switch {
	case p.BatchID <= 0:
		return fmt.Errorf("erp preview: %w: id %d", erpintegration.ErrBatchNotFound, p.BatchID)
	case p.Operation != erpintegration.AdjOpValuate && p.Operation != erpintegration.AdjOpApprove && p.Operation != erpintegration.AdjOpRestore:
		return fmt.Errorf("erp preview: invalid operation %q", p.Operation)
	case p.CreatedBy == "":
		return fmt.Errorf("erp preview: created_by is required")
	case p.ExpiresAt.IsZero():
		return fmt.Errorf("erp preview: expires_at is required")
	case !p.CreatedAt.IsZero() && !p.ExpiresAt.After(p.CreatedAt):
		return fmt.Errorf("erp preview: expires_at must be after created_at")
	}
	return nil
}

// checkBatchStatus share-locks the batch row and compares its status.
func checkBatchStatus(ctx context.Context, tx *sql.Tx, batchID int64, expected erpintegration.BatchStatus) error {
	var st string
	err := tx.QueryRowContext(ctx, erpPreviewBatchStatusSQL, batchID).Scan(&st)
	if errors.Is(err, sql.ErrNoRows) {
		return erpintegration.ErrBatchNotFound
	}
	if err != nil {
		return fmt.Errorf("erp preview batch status: %w", err)
	}
	if erpintegration.BatchStatus(st) != expected {
		return fmt.Errorf("%w: batch %d is %s, not %s", erpintegration.ErrStaleBatchStatus, batchID, st, expected)
	}
	return nil
}

func insertPreview(ctx context.Context, q erpQuerier, p erpintegration.ValuationPreview) (erpintegration.ValuationPreview, error) {
	ex, err := json.Marshal(p.Excluded)
	if err != nil {
		return erpintegration.ValuationPreview{}, fmt.Errorf("erp preview excluded: %w", err)
	}
	var created sql.NullTime
	if !p.CreatedAt.IsZero() {
		created = sql.NullTime{Time: p.CreatedAt, Valid: true}
	}
	out, err := scanPreview(q.QueryRowContext(ctx, erpPreviewInsertSQL,
		p.BatchID, string(p.Operation), string(p.Status), p.HeadCount, p.ItemCount,
		string(ex), p.SetHash, nullJSONArg(p.Totals), p.ConfirmText,
		p.CreatedBy, created, p.ExpiresAt))
	if err != nil {
		return erpintegration.ValuationPreview{}, mapPreviewWriteError(err)
	}
	return out, nil
}

func openPreview(ctx context.Context, tx *sql.Tx, id string) error {
	res, err := tx.ExecContext(ctx, erpPreviewOpenSQL, id)
	if err != nil {
		return fmt.Errorf("erp preview open: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp preview open rows affected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("erp preview open: preview %s is no longer BUILDING", id)
	}
	return nil
}

func mapPreviewWriteError(err error) error {
	state, constraint, ok := pgErrorInfo(err)
	if ok && state == sqlStateUniqueViolation && constraint == constraintCevpLive {
		return fmt.Errorf("%w: a live preview exists (%s)", erpintegration.ErrConcurrentRun, constraint)
	}
	return fmt.Errorf("erp preview insert: %w", err)
}

func scanPreview(s rowScanner) (erpintegration.ValuationPreview, error) {
	var (
		p                erpintegration.ValuationPreview
		op, st           string
		excluded, totals sql.NullString
		consumedAt       sql.NullTime
	)
	if err := s.Scan(&p.ID, &p.BatchID, &op, &st, &p.HeadCount, &p.ItemCount,
		&excluded, &p.SetHash, &totals, &p.ConfirmText, &p.CreatedBy, &p.CreatedAt, &p.ExpiresAt,
		&consumedAt, &p.ConsumedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, sql.ErrNoRows
		}
		return p, fmt.Errorf("erp preview scan: %w", err)
	}
	p.Operation, p.Status = erpintegration.AdjOperation(op), erpintegration.PreviewStatus(st)
	if excluded.Valid {
		if err := json.Unmarshal([]byte(excluded.String), &p.Excluded); err != nil {
			return p, fmt.Errorf("erp preview excluded: %w", err)
		}
	}
	if totals.Valid {
		p.Totals = []byte(totals.String)
	}
	if consumedAt.Valid {
		t := consumedAt.Time
		p.ConsumedAt = &t
	}
	p.CreatedAt, p.ExpiresAt = p.CreatedAt.UTC(), p.ExpiresAt.UTC()
	return p, nil
}
