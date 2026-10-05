package oracle

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNoReadConnection is returned when a read-only query is attempted on a
// client or guarded DB that has no underlying connection.
var ErrNoReadConnection = errors.New("oracle: read-only connection not configured")

// Rows is the minimal row-cursor contract used by ERP readers. *sql.Rows
// satisfies it; tests use an in-memory implementation (internal/testutil/fakeoracle).
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// ReadOnlyQuerier is the ONLY capability ERP readers receive (design §3.4).
// It deliberately has no Exec, BeginTx or Prepare: every statement goes
// through CheckReadOnly before it reaches Oracle.
type ReadOnlyQuerier interface {
	QueryRO(ctx context.Context, query string, args ...any) (Rows, error)
}

// ReadOnlyDB wraps a *sql.DB and exposes only guarded SELECT access.
type ReadOnlyDB struct {
	db *sql.DB
}

// NewReadOnlyDB wraps db so that only guarded read-only queries can run.
func NewReadOnlyDB(db *sql.DB) *ReadOnlyDB {
	return &ReadOnlyDB{db: db}
}

// QueryRO runs query after it passes the read-only guard.
func (r *ReadOnlyDB) QueryRO(ctx context.Context, query string, args ...any) (Rows, error) {
	if r == nil || r.db == nil {
		return nil, ErrNoReadConnection
	}
	return guardedQuery(ctx, r.db, query, args...)
}

// QueryRO runs a guarded read-only query on the legacy read connection. It is
// additive: existing callers of DB() are unaffected.
func (c *Client) QueryRO(ctx context.Context, query string, args ...any) (Rows, error) {
	if c == nil || c.db == nil {
		return nil, ErrNoReadConnection
	}
	return guardedQuery(ctx, c.db, query, args...)
}

// ReadOnly returns a ReadOnlyQuerier view of the client for ERP readers.
func (c *Client) ReadOnly() ReadOnlyQuerier {
	return c
}

// guardedQuery applies CheckReadOnly and then runs the query.
func guardedQuery(ctx context.Context, db *sql.DB, query string, args ...any) (Rows, error) {
	if err := CheckReadOnly(query); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
