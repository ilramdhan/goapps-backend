package erpintegration

import "time"

// OracleCallRow is one call-log row for the operator view (read-only).
type OracleCallRow struct {
	ID           int64
	StatementKey string
	Status       string
	Actor        string
	StartedAt    time.Time
	DurationMs   int64
	Error        string
	OraCode      string
	Attempts     int32
	FinishedAt   *time.Time
}
