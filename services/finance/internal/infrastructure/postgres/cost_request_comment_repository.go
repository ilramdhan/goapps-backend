package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequestcomment"
)

// CostRequestCommentRepository implements costrequestcomment.Repository.
type CostRequestCommentRepository struct{ db *DB }

// NewCostRequestCommentRepository constructs the repo.
func NewCostRequestCommentRepository(db *DB) *CostRequestCommentRepository {
	return &CostRequestCommentRepository{db: db}
}

var _ costrequestcomment.Repository = (*CostRequestCommentRepository)(nil)

const crcCols = `
	crc_comment_id,crc_request_id,crc_parent_comment_id,crc_author_user_id,
	crc_body_richtext::text,crc_body_plaintext,crc_is_edited,crc_is_hidden,
	crc_hidden_reason,crc_created_at,crc_updated_at`

// Create persists the comment + mentions atomically.
func (r *CostRequestCommentRepository) Create(ctx context.Context, c *costrequestcomment.Comment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if cerr := tx.Rollback(); cerr != nil {
			_ = cerr
		}
	}()

	const q = `
		INSERT INTO cost_request_comment (
			crc_request_id,crc_parent_comment_id,crc_author_user_id,
			crc_body_richtext,crc_body_plaintext,crc_is_edited,crc_is_hidden,
			crc_created_at,crc_updated_at
		) VALUES ($1, NULLIF($2,0), $3, $4::jsonb, $5, FALSE, FALSE, $6, $6)
		RETURNING crc_comment_id`
	var parentID int64
	if p := c.ParentCommentID(); p != nil {
		parentID = *p
	}
	var id int64
	if err := tx.QueryRowContext(ctx, q,
		c.RequestID(), parentID, c.AuthorUserID(),
		c.BodyRichtext(), c.BodyPlaintext(), c.CreatedAt(),
	).Scan(&id); err != nil {
		return fmt.Errorf("insert cost_request_comment: %w", err)
	}
	c.SetID(id)

	if err := insertMentions(ctx, tx, id, c.MentionedUserIDs()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit comment: %w", err)
	}
	return nil
}

// GetByID loads one with its mentions.
func (r *CostRequestCommentRepository) GetByID(ctx context.Context, id int64) (*costrequestcomment.Comment, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+crcCols+` FROM cost_request_comment WHERE crc_comment_id=$1`, id)
	in, err := scanCrcRow(row)
	if err != nil {
		return nil, err
	}
	mentions, err := r.loadMentions(ctx, id)
	if err != nil {
		return nil, err
	}
	in.MentionedUserIDs = mentions
	return costrequestcomment.Reconstruct(in), nil
}

// Update persists body change + snapshot prior + refresh mentions.
func (r *CostRequestCommentRepository) Update(ctx context.Context, c *costrequestcomment.Comment, snapshot costrequestcomment.EditSnapshot, editor string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if cerr := tx.Rollback(); cerr != nil {
			_ = cerr
		}
	}()

	// 1. Snapshot the prior body to CCEH_.
	const qSnap = `
		INSERT INTO cost_comment_edit_history (
			cceh_comment_id,cceh_body_richtext,cceh_body_plaintext,cceh_edited_by,cceh_edited_at
		) VALUES ($1, $2::jsonb, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, qSnap,
		c.CommentID(), snapshot.PriorBodyRichtext, snapshot.PriorBodyPlaintext, editor, c.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("snapshot edit: %w", err)
	}

	// 2. Update comment body.
	const qUpd = `
		UPDATE cost_request_comment SET
			crc_body_richtext=$2::jsonb,
			crc_body_plaintext=$3,
			crc_is_edited=TRUE,
			crc_updated_at=$4
		WHERE crc_comment_id=$1`
	res, err := tx.ExecContext(ctx, qUpd, c.CommentID(), c.BodyRichtext(), c.BodyPlaintext(), c.UpdatedAt())
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costrequestcomment.ErrNotFound
	}

	// 3. Refresh mentions (delete + reinsert; cheap given small fanout).
	if _, err := tx.ExecContext(ctx, `DELETE FROM cost_request_mention WHERE crm_comment_id=$1`, c.CommentID()); err != nil {
		return fmt.Errorf("delete mentions: %w", err)
	}
	if err := insertMentions(ctx, tx, c.CommentID(), c.MentionedUserIDs()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update: %w", err)
	}
	return nil
}

// UpdateHidden persists hide/unhide.
func (r *CostRequestCommentRepository) UpdateHidden(ctx context.Context, c *costrequestcomment.Comment) error {
	const q = `
		UPDATE cost_request_comment SET
			crc_is_hidden=$2,
			crc_hidden_reason=NULLIF($3,''),
			crc_updated_at=$4
		WHERE crc_comment_id=$1`
	res, err := r.db.ExecContext(ctx, q, c.CommentID(), c.IsHidden(), c.HiddenReason(), c.UpdatedAt())
	if err != nil {
		return fmt.Errorf("update hidden: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costrequestcomment.ErrNotFound
	}
	return nil
}

// Delete removes a comment.
func (r *CostRequestCommentRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cost_request_comment WHERE crc_comment_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costrequestcomment.ErrNotFound
	}
	return nil
}

// ListByRequest returns the thread (optionally including hidden).
func (r *CostRequestCommentRepository) ListByRequest(ctx context.Context, requestID int64, includeHidden bool) ([]*costrequestcomment.Comment, error) {
	q := `SELECT ` + crcCols + ` FROM cost_request_comment WHERE crc_request_id=$1`
	if !includeHidden {
		q += ` AND crc_is_hidden=FALSE`
	}
	q += ` ORDER BY crc_comment_id ASC`
	rows, err := r.db.QueryContext(ctx, q, requestID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costrequestcomment.Comment{}
	for rows.Next() {
		in, sErr := scanCrcRows(rows)
		if sErr != nil {
			return nil, sErr
		}
		mentions, mErr := r.loadMentions(ctx, in.CommentID)
		if mErr != nil {
			return nil, mErr
		}
		in.MentionedUserIDs = mentions
		out = append(out, costrequestcomment.Reconstruct(in))
	}
	return out, rows.Err()
}

// ListEditHistory returns CCEH_ rows newest first.
func (r *CostRequestCommentRepository) ListEditHistory(ctx context.Context, commentID int64) ([]costrequestcomment.EditHistoryEntry, error) {
	const q = `
		SELECT cceh_edit_id,cceh_comment_id,cceh_body_richtext::text,cceh_body_plaintext,cceh_edited_by,cceh_edited_at
		FROM cost_comment_edit_history
		WHERE cceh_comment_id=$1
		ORDER BY cceh_edited_at DESC, cceh_edit_id DESC`
	rows, err := r.db.QueryContext(ctx, q, commentID)
	if err != nil {
		return nil, fmt.Errorf("list edit history: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []costrequestcomment.EditHistoryEntry{}
	for rows.Next() {
		e := costrequestcomment.EditHistoryEntry{}
		var editedAt time.Time
		if sErr := rows.Scan(&e.EditID, &e.CommentID, &e.BodyRichtext, &e.BodyPlaintext, &e.EditedBy, &editedAt); sErr != nil {
			return nil, fmt.Errorf("scan edit history: %w", sErr)
		}
		e.EditedAt = editedAt
		out = append(out, e)
	}
	return out, rows.Err()
}

// =============================================================================
// helpers
// =============================================================================

func insertMentions(ctx context.Context, tx *sql.Tx, commentID int64, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	const q = `
		INSERT INTO cost_request_mention (crm_comment_id,crm_mentioned_user_id)
		VALUES ($1, $2)
		ON CONFLICT (crm_comment_id,crm_mentioned_user_id) DO NOTHING`
	for _, uid := range userIDs {
		if _, err := tx.ExecContext(ctx, q, commentID, uid); err != nil {
			return fmt.Errorf("insert mention %s: %w", uid, err)
		}
	}
	return nil
}

func (r *CostRequestCommentRepository) loadMentions(ctx context.Context, commentID int64) ([]string, error) {
	const q = `SELECT crm_mentioned_user_id FROM cost_request_mention WHERE crm_comment_id=$1 ORDER BY crm_mention_id ASC`
	rows, err := r.db.QueryContext(ctx, q, commentID)
	if err != nil {
		return nil, fmt.Errorf("load mentions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []string{}
	for rows.Next() {
		var uid string
		if sErr := rows.Scan(&uid); sErr != nil {
			return nil, fmt.Errorf("scan mention: %w", sErr)
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// =============================================================================
// scanners
// =============================================================================

func scanCrcRow(row *sql.Row) (costrequestcomment.ReconstructInput, error) {
	in, err := scanCrc(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return costrequestcomment.ReconstructInput{}, costrequestcomment.ErrNotFound
	}
	return in, err
}

func scanCrcRows(rows *sql.Rows) (costrequestcomment.ReconstructInput, error) {
	return scanCrc(rows.Scan)
}

func scanCrc(scan func(...any) error) (costrequestcomment.ReconstructInput, error) {
	var (
		commentID, requestID int64
		parentID             sql.NullInt64
		author               string
		bodyRich             string
		bodyPlain            string
		isEdited, isHidden   bool
		hiddenReason         sql.NullString
		createdAt, updatedAt time.Time
	)
	if err := scan(&commentID, &requestID, &parentID, &author, &bodyRich, &bodyPlain, &isEdited, &isHidden, &hiddenReason, &createdAt, &updatedAt); err != nil {
		return costrequestcomment.ReconstructInput{}, err
	}
	in := costrequestcomment.ReconstructInput{
		CommentID:     commentID,
		RequestID:     requestID,
		AuthorUserID:  author,
		BodyRichtext:  bodyRich,
		BodyPlaintext: bodyPlain,
		IsEdited:      isEdited,
		IsHidden:      isHidden,
		HiddenReason:  hiddenReason.String,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
	if parentID.Valid {
		v := parentID.Int64
		in.ParentCommentID = &v
	}
	return in, nil
}
