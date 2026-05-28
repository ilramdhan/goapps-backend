package upload

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for Excel upload sessions and staging.
type Repository interface {
	// CreateSession inserts a new bi_excel_upload header row.
	CreateSession(ctx context.Context, u *Upload) error

	// InsertStaging bulk-inserts staging rows for a session (chunked internally).
	InsertStaging(ctx context.Context, uploadID uuid.UUID, rows []StagingRow) error

	// GetSession loads a session header by id.
	GetSession(ctx context.Context, uploadID uuid.UUID) (*Upload, error)

	// UpdateSession persists status/count/timestamp changes on a session header.
	UpdateSession(ctx context.Context, u *Upload) error

	// ListSessions returns a page of session headers (newest first) plus the total count.
	ListSessions(ctx context.Context, page, pageSize int) ([]*Upload, int, error)

	// MarkOverwrites flips VALID staging rows to WILL_OVERWRITE when their business key
	// already exists in bi_fact_metric, returning the number flipped.
	MarkOverwrites(ctx context.Context, uploadID uuid.UUID) (int, error)

	// CommitToFact upserts VALID + WILL_OVERWRITE staging rows of a session into
	// bi_fact_metric (set-based INSERT ... SELECT ... ON CONFLICT) and returns the
	// number of rows committed.
	CommitToFact(ctx context.Context, uploadID uuid.UUID) (int, error)

	// RefreshViews refreshes the BI materialized views after a commit.
	RefreshViews(ctx context.Context) error
}
