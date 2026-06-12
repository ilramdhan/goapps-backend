package requesthistory

import "context"

// Repository persists and retrieves approval trace entries.
type Repository interface {
	// Insert saves a new history entry. ID and CreatedAt are assigned by the DB.
	Insert(ctx context.Context, e *Entry) error
	// ListByRequestID returns all history entries for the given request,
	// ordered by created_at ascending (oldest first).
	ListByRequestID(ctx context.Context, requestID int64) ([]*Entry, error)
}
