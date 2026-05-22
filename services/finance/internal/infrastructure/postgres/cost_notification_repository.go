package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"
)

// CostNotificationRepository implements costnotification.Repository.
type CostNotificationRepository struct{ db *DB }

// NewCostNotificationRepository constructs the repository.
func NewCostNotificationRepository(db *DB) *CostNotificationRepository {
	return &CostNotificationRepository{db: db}
}

var _ costnotification.Repository = (*CostNotificationRepository)(nil)

const cnCols = `cn_notification_id,cn_recipient_user_id,cn_trigger_type,cn_request_id,cn_payload::text,cn_is_read,cn_email_sent_at,cn_created_at`

// Emit persists a new notification.
func (r *CostNotificationRepository) Emit(ctx context.Context, n *costnotification.Notification) error {
	const q = `
		INSERT INTO cost_notification (
			cn_recipient_user_id, cn_trigger_type, cn_request_id,
			cn_payload, cn_is_read, cn_created_at
		) VALUES ($1, $2, $3, $4::jsonb, FALSE, $5)
		RETURNING cn_notification_id`
	var reqID sql.NullInt64
	if n.RequestID != nil {
		reqID = sql.NullInt64{Int64: *n.RequestID, Valid: true}
	}
	if err := r.db.QueryRowContext(ctx, q,
		n.RecipientUserID, n.TriggerType, reqID, n.Payload, n.CreatedAt,
	).Scan(&n.NotificationID); err != nil {
		return fmt.Errorf("emit cost_notification: %w", err)
	}
	return nil
}

// GetByID loads one notification.
func (r *CostNotificationRepository) GetByID(ctx context.Context, id int64) (*costnotification.Notification, error) {
	q := `SELECT ` + cnCols + ` FROM cost_notification WHERE cn_notification_id=$1`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanCnRow(row)
}

// List returns a paginated list for the recipient.
func (r *CostNotificationRepository) List(ctx context.Context, f costnotification.Filter) ([]*costnotification.Notification, int64, error) {
	where := "FROM cost_notification WHERE cn_recipient_user_id=$1"
	args := []any{f.RecipientUserID}
	if f.UnreadOnly {
		where += ` AND cn_is_read=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_notification: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 100)
	offset := (page - 1) * pageSize

	q := `SELECT ` + cnCols + ` ` + where + ` ORDER BY cn_created_at DESC, cn_notification_id DESC LIMIT $2 OFFSET $3`
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_notification: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []*costnotification.Notification{}
	for rows.Next() {
		n, sErr := scanCnRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

// UnreadCount returns the count of unread rows for the recipient.
func (r *CostNotificationRepository) UnreadCount(ctx context.Context, recipientUserID string) (int32, error) {
	var n int32
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cost_notification WHERE cn_recipient_user_id=$1 AND cn_is_read=FALSE`,
		recipientUserID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return n, nil
}

// MarkRead persists is_read=true.
func (r *CostNotificationRepository) MarkRead(ctx context.Context, n *costnotification.Notification) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cost_notification SET cn_is_read=TRUE WHERE cn_notification_id=$1`,
		n.NotificationID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return costnotification.ErrNotFound
	}
	return nil
}

// MarkAllRead flips is_read=true for every unread row of the recipient.
func (r *CostNotificationRepository) MarkAllRead(ctx context.Context, recipientUserID string) (int32, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cost_notification SET cn_is_read=TRUE WHERE cn_recipient_user_id=$1 AND cn_is_read=FALSE`,
		recipientUserID,
	)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int32(n), nil
}

// =============================================================================
// scanners
// =============================================================================

func scanCnRow(row *sql.Row) (*costnotification.Notification, error) {
	n, err := scanCn(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costnotification.ErrNotFound
	}
	return n, err
}

func scanCnRows(rows *sql.Rows) (*costnotification.Notification, error) {
	return scanCn(rows.Scan)
}

func scanCn(scan func(...any) error) (*costnotification.Notification, error) {
	n := &costnotification.Notification{}
	var reqID sql.NullInt64
	var emailAt sql.NullTime
	var createdAt time.Time
	if err := scan(&n.NotificationID, &n.RecipientUserID, &n.TriggerType, &reqID, &n.Payload, &n.IsRead, &emailAt, &createdAt); err != nil {
		return nil, fmt.Errorf("scan cost_notification: %w", err)
	}
	if reqID.Valid {
		v := reqID.Int64
		n.RequestID = &v
	}
	if emailAt.Valid {
		t := emailAt.Time
		n.EmailSentAt = &t
	}
	n.CreatedAt = createdAt
	return n, nil
}
