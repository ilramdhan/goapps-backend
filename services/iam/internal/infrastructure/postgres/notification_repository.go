package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// NotificationRepository implements notification.Repository.
type NotificationRepository struct {
	db *DB
}

// NewNotificationRepository constructs the repo.
func NewNotificationRepository(db *DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// Create inserts a new notification.
func (r *NotificationRepository) Create(ctx context.Context, n *notification.Notification) error {
	const q = `
		INSERT INTO mst_notification (
			notification_id, recipient_user_id, type, severity,
			title, body, action_type, action_payload,
			status, read_at, archived_at, expires_at,
			source_type, source_id, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`
	if _, err := r.db.ExecContext(ctx, q,
		n.ID(), n.RecipientUserID(), n.Type().String(), n.Severity().String(),
		n.Title(), nullStringFromEmpty(n.Body()), n.ActionType().String(), nullStringFromEmpty(n.ActionPayload()),
		n.Status().String(), nullTimePtr(n.ReadAt()), nullTimePtr(n.ArchivedAt()), nullTimePtr(n.ExpiresAt()),
		nullStringFromEmpty(n.SourceType()), nullStringFromEmpty(n.SourceID()), n.CreatedAt(), n.CreatedBy(),
	); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// GetByID returns a notification by id.
func (r *NotificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*notification.Notification, error) {
	const q = `
		SELECT notification_id, recipient_user_id, type, severity, title,
		       COALESCE(body, ''), action_type, COALESCE(action_payload::text, ''),
		       status, read_at, archived_at, expires_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''),
		       created_at, created_by
		FROM mst_notification
		WHERE notification_id = $1
	`
	row := r.db.QueryRowContext(ctx, q, id)
	n, err := scanNotification(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notification.ErrNotFound
		}
		return nil, fmt.Errorf("get notification: %w", err)
	}
	return n, nil
}

// ListByRecipient lists notifications for a recipient.
func (r *NotificationRepository) ListByRecipient(
	ctx context.Context,
	recipientID uuid.UUID,
	filter notification.ListFilter,
) ([]*notification.Notification, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	whereClauses := []string{"recipient_user_id = $1"}
	args := []any{recipientID}
	idx := 2
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, filter.Status.String())
		idx++
	}
	if filter.Type != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("type = $%d", idx))
		args = append(args, filter.Type.String())
		idx++
	}
	if filter.After != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at > $%d", idx))
		args = append(args, *filter.After)
		idx++
	}
	where := strings.Join(whereClauses, " AND ")

	// total count
	var total int64
	countQ := "SELECT COUNT(*) FROM mst_notification WHERE " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	order := sortDESC
	if !filter.SortDesc {
		order = sortASC
	}

	listQ := fmt.Sprintf(`
		SELECT notification_id, recipient_user_id, type, severity, title,
		       COALESCE(body, ''), action_type, COALESCE(action_payload::text, ''),
		       status, read_at, archived_at, expires_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''),
		       created_at, created_by
		FROM mst_notification
		WHERE %s
		ORDER BY created_at %s
		LIMIT $%d OFFSET $%d
	`, where, order, idx, idx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// best-effort: log/ignore — caller already received rows.
			_ = closeErr
		}
	}()

	var out []*notification.Notification
	for rows.Next() {
		n, scanErr := scanNotification(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", scanErr)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notifications: %w", err)
	}
	return out, total, nil
}

// CountUnread returns the count of unread notifications for a recipient.
func (r *NotificationRepository) CountUnread(ctx context.Context, recipientID uuid.UUID) (int64, error) {
	const q = `SELECT COUNT(*) FROM mst_notification WHERE recipient_user_id = $1 AND status = 'UNREAD'`
	var n int64
	if err := r.db.QueryRowContext(ctx, q, recipientID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return n, nil
}

// MarkAsRead marks a single notification as read for the recipient.
// Idempotent for already-READ rows. Returns ErrNotFound when the row does not
// exist for this recipient (so callers can distinguish "wrong owner / missing"
// from "already read").
func (r *NotificationRepository) MarkAsRead(ctx context.Context, recipientID, notificationID uuid.UUID, readAt time.Time) error {
	const q = `
		UPDATE mst_notification
		SET status = 'READ', read_at = $3
		WHERE notification_id = $1 AND recipient_user_id = $2 AND status = 'UNREAD'
	`
	res, err := r.db.ExecContext(ctx, q, notificationID, recipientID, readAt)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	rows, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("rows affected: %w", raErr)
	}
	if rows > 0 {
		return nil
	}
	// 0 rows: either already-READ (idempotent success) or missing/wrong owner.
	var exists bool
	const checkQ = `SELECT EXISTS(SELECT 1 FROM mst_notification WHERE notification_id = $1 AND recipient_user_id = $2)`
	if err := r.db.QueryRowContext(ctx, checkQ, notificationID, recipientID).Scan(&exists); err != nil {
		return fmt.Errorf("check exists: %w", err)
	}
	if !exists {
		return notification.ErrNotFound
	}
	return nil
}

// MarkAllAsRead marks all UNREAD notifications for a recipient as READ.
func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, recipientID uuid.UUID, readAt time.Time) (int64, error) {
	const q = `
		UPDATE mst_notification
		SET status = 'READ', read_at = $2
		WHERE recipient_user_id = $1 AND status = 'UNREAD'
	`
	res, err := r.db.ExecContext(ctx, q, recipientID, readAt)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return 0, fmt.Errorf("rows affected: %w", raErr)
	}
	return n, nil
}

// Archive marks a single notification as ARCHIVED for the recipient.
func (r *NotificationRepository) Archive(ctx context.Context, recipientID, notificationID uuid.UUID, archivedAt time.Time) error {
	const q = `
		UPDATE mst_notification
		SET status = 'ARCHIVED',
		    archived_at = $3,
		    read_at = COALESCE(read_at, $3)
		WHERE notification_id = $1 AND recipient_user_id = $2 AND status <> 'ARCHIVED'
	`
	res, err := r.db.ExecContext(ctx, q, notificationID, recipientID, archivedAt)
	if err != nil {
		return fmt.Errorf("archive notification: %w", err)
	}
	rows, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("rows affected: %w", raErr)
	}
	if rows == 0 {
		var exists bool
		const checkQ = `SELECT EXISTS(SELECT 1 FROM mst_notification WHERE notification_id = $1 AND recipient_user_id = $2)`
		if err := r.db.QueryRowContext(ctx, checkQ, notificationID, recipientID).Scan(&exists); err != nil {
			return fmt.Errorf("check exists: %w", err)
		}
		if !exists {
			return notification.ErrNotFound
		}
		// already ARCHIVED — idempotent
	}
	return nil
}

// Delete hard-deletes a single notification owned by the recipient.
func (r *NotificationRepository) Delete(ctx context.Context, recipientID, notificationID uuid.UUID) error {
	const q = `DELETE FROM mst_notification WHERE notification_id = $1 AND recipient_user_id = $2`
	res, err := r.db.ExecContext(ctx, q, notificationID, recipientID)
	if err != nil {
		return fmt.Errorf("delete notification: %w", err)
	}
	rows, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("rows affected: %w", raErr)
	}
	if rows == 0 {
		return notification.ErrNotFound
	}
	return nil
}

// DeleteExpired hard-deletes notifications past their expires_at.
func (r *NotificationRepository) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	const q = `DELETE FROM mst_notification WHERE expires_at IS NOT NULL AND expires_at < $1`
	res, err := r.db.ExecContext(ctx, q, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return 0, fmt.Errorf("rows affected: %w", raErr)
	}
	return n, nil
}

// scanner abstracts *sql.Row and *sql.Rows so scanNotification works for both.
type scanner interface {
	Scan(dest ...any) error
}

func scanNotification(s scanner) (*notification.Notification, error) {
	var (
		id, recipientID                                                            uuid.UUID
		typeStr, severityStr, title, body, actionTypeStr, actionPayload, statusStr string
		readAt, archivedAt, expiresAt                                              sql.NullTime
		sourceType, sourceID, createdBy                                            string
		createdAt                                                                  time.Time
	)
	if err := s.Scan(
		&id, &recipientID, &typeStr, &severityStr, &title,
		&body, &actionTypeStr, &actionPayload,
		&statusStr, &readAt, &archivedAt, &expiresAt,
		&sourceType, &sourceID, &createdAt, &createdBy,
	); err != nil {
		return nil, err
	}
	notifType, err := notification.ParseType(typeStr)
	if err != nil {
		return nil, fmt.Errorf("parse type: %w", err)
	}
	severity, err := notification.ParseSeverity(severityStr)
	if err != nil {
		return nil, fmt.Errorf("parse severity: %w", err)
	}
	actionType, err := notification.ParseActionType(actionTypeStr)
	if err != nil {
		return nil, fmt.Errorf("parse action type: %w", err)
	}
	statusVal, err := notification.ParseStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}
	return notification.Reconstruct(
		id, recipientID,
		notifType, severity,
		title, body,
		actionType, actionPayload,
		statusVal,
		nullableTimePtr(readAt), nullableTimePtr(archivedAt), nullableTimePtr(expiresAt),
		sourceType, sourceID,
		createdAt, createdBy,
	), nil
}

func nullStringFromEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullableTimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	v := nt.Time
	return &v
}
