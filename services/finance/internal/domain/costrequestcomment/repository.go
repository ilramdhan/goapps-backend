package costrequestcomment

import "context"

// Repository persists comments + their mentions + edit-history snapshots atomically.
type Repository interface {
	// Create inserts the comment + mention rows in one tx.
	Create(ctx context.Context, c *Comment) error
	GetByID(ctx context.Context, id int64) (*Comment, error)
	// Update persists body changes + writes a CCEH_ snapshot of the prior body
	// + refreshes the mention rows, all in one tx.
	Update(ctx context.Context, c *Comment, snapshot EditSnapshot, editor string) error
	// UpdateHidden persists is_hidden + hidden_reason only.
	UpdateHidden(ctx context.Context, c *Comment) error
	// Delete removes a comment (mentions/edit_history cascade via FK).
	Delete(ctx context.Context, id int64) error
	// ListByRequest returns all comments for a request, oldest first.
	ListByRequest(ctx context.Context, requestID int64, includeHidden bool) ([]*Comment, error)
	// ListEditHistory returns CCEH_ rows newest first for one comment.
	ListEditHistory(ctx context.Context, commentID int64) ([]EditHistoryEntry, error)
}
