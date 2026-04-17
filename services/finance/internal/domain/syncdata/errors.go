package syncdata

import "errors"

// Domain errors for sync data operations.
var (
	// ErrOracleConnectionFailed is returned when the Oracle database is unreachable.
	ErrOracleConnectionFailed = errors.New("oracle database connection failed")

	// ErrProcedureFailed is returned when the Oracle stored procedure execution fails.
	ErrProcedureFailed = errors.New("oracle stored procedure execution failed")

	// ErrFetchFailed is returned when fetching data from Oracle fails.
	ErrFetchFailed = errors.New("failed to fetch data from oracle")

	// ErrUpsertFailed is returned when upserting data to PostgreSQL fails.
	ErrUpsertFailed = errors.New("failed to upsert data to postgresql")
)
