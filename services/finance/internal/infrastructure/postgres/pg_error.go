package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// PostgreSQL SQLSTATE codes used by this package.
// See https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	sqlStateUniqueViolation = "23505"
)

// pgErrorInfo extracts the SQLSTATE code and the violated constraint name from a
// database error, regardless of which PostgreSQL driver produced it.
//
// Three cases are handled deliberately — dropping any of them silently breaks
// SQLSTATE detection at runtime:
//
//  1. *pgconn.PgError — the PRODUCTION path. The service opens its pool with
//     sql.Open("pgx", ...) (connection.go), so every error that reaches a
//     repository in production is a *pgconn.PgError. Matching only *pq.Error
//     here is a silent no-op bug: errors.As never succeeds and unique
//     violations degrade into generic "failed to create ..." errors.
//  2. *pq.Error — lib/pq is still used by the seeder (cmd/seeds/main.go opens
//     sql.Open("postgres", ...)) and by tests written against lib/pq.
//  3. interface{ SQLState() string } — a driver-agnostic fallback so wrappers
//     and future drivers that expose SQLSTATE keep working without another edit
//     here. The constraint name is unavailable through this path.
//
// ok is false when err carries no PostgreSQL error information at all.
func pgErrorInfo(err error) (sqlState, constraint string, ok bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName, true
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code), pqErr.Constraint, true
	}

	var stateErr interface{ SQLState() string }
	if errors.As(err, &stateErr) {
		return stateErr.SQLState(), "", true
	}

	return "", "", false
}

// isPGUniqueViolation reports whether err is a PostgreSQL unique violation
// (SQLSTATE 23505) under any supported driver.
func isPGUniqueViolation(err error) bool {
	sqlState, _, ok := pgErrorInfo(err)
	return ok && sqlState == sqlStateUniqueViolation
}
