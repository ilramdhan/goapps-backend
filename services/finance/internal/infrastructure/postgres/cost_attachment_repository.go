package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costattachment"
)

// CostAttachmentRepository implements costattachment.Repository.
type CostAttachmentRepository struct{ db *DB }

// NewCostAttachmentRepository constructs the repo.
func NewCostAttachmentRepository(db *DB) *CostAttachmentRepository {
	return &CostAttachmentRepository{db: db}
}

var _ costattachment.Repository = (*CostAttachmentRepository)(nil)

const caCols = `
	ca_attachment_id,ca_request_id,ca_comment_id,ca_filename,ca_mime_type,
	ca_size_bytes,ca_storage_key,ca_uploaded_by,ca_uploaded_at`

// Create persists the attachment row (storage upload happens before this in the app layer).
func (r *CostAttachmentRepository) Create(ctx context.Context, a *costattachment.Attachment) error {
	const q = `
		INSERT INTO cost_attachment (
			ca_request_id,ca_comment_id,ca_filename,ca_mime_type,
			ca_size_bytes,ca_storage_key,ca_uploaded_by,ca_uploaded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ca_attachment_id`
	var req sql.NullInt64
	if a.RequestID != nil {
		req = sql.NullInt64{Int64: *a.RequestID, Valid: true}
	}
	var cmt sql.NullInt64
	if a.CommentID != nil {
		cmt = sql.NullInt64{Int64: *a.CommentID, Valid: true}
	}
	if err := r.db.QueryRowContext(ctx, q,
		req, cmt, a.Filename, a.MimeType, a.SizeBytes, a.StorageKey, a.UploadedBy, a.UploadedAt,
	).Scan(&a.AttachmentID); err != nil {
		return fmt.Errorf("insert cost_attachment: %w", err)
	}
	return nil
}

// GetByID loads one.
func (r *CostAttachmentRepository) GetByID(ctx context.Context, id int64) (*costattachment.Attachment, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+caCols+` FROM cost_attachment WHERE ca_attachment_id=$1`, id)
	a, err := scanCaRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costattachment.ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

// ListByRequest returns request-level attachments only.
func (r *CostAttachmentRepository) ListByRequest(ctx context.Context, requestID int64) ([]*costattachment.Attachment, error) {
	return r.listBy(ctx, `ca_request_id=$1`, requestID)
}

// ListByComment returns comment-level attachments only.
func (r *CostAttachmentRepository) ListByComment(ctx context.Context, commentID int64) ([]*costattachment.Attachment, error) {
	return r.listBy(ctx, `ca_comment_id=$1`, commentID)
}

func (r *CostAttachmentRepository) listBy(ctx context.Context, predicate string, arg any) ([]*costattachment.Attachment, error) {
	q := `SELECT ` + caCols + ` FROM cost_attachment WHERE ` + predicate + ` ORDER BY ca_uploaded_at DESC, ca_attachment_id DESC`
	rows, err := r.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("list cost_attachment: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costattachment.Attachment{}
	for rows.Next() {
		a, sErr := scanCaRows(rows)
		if sErr != nil {
			return nil, sErr
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Delete removes one row. Caller is responsible for the storage object deletion.
func (r *CostAttachmentRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cost_attachment WHERE ca_attachment_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete cost_attachment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costattachment.ErrNotFound
	}
	return nil
}

// =============================================================================
// scanners
// =============================================================================

func scanCaRow(row *sql.Row) (*costattachment.Attachment, error) {
	return scanCa(row.Scan)
}

func scanCaRows(rows *sql.Rows) (*costattachment.Attachment, error) {
	return scanCa(rows.Scan)
}

func scanCa(scan func(...any) error) (*costattachment.Attachment, error) {
	a := &costattachment.Attachment{}
	var req, cmt sql.NullInt64
	var uploadedAt time.Time
	if err := scan(&a.AttachmentID, &req, &cmt, &a.Filename, &a.MimeType, &a.SizeBytes, &a.StorageKey, &a.UploadedBy, &uploadedAt); err != nil {
		return nil, fmt.Errorf("scan cost_attachment: %w", err)
	}
	a.UploadedAt = uploadedAt
	if req.Valid {
		v := req.Int64
		a.RequestID = &v
	}
	if cmt.Valid {
		v := cmt.Int64
		a.CommentID = &v
	}
	return a, nil
}
