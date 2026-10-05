package erpintegration

// batch_push.go holds the ports of the push step (plan-06 P5-T3; design
// Part 2 §7.2, §8.1, §9.2; Part 1 §S-R11): the transaction-scoped store the
// step reads under the G11 lock and the Oracle call log written around every
// Oracle write.

import (
	"context"
	"encoding/json"
)

// PushStore is the transaction-scoped store of the push step. The PG
// BatchTxRunner hands a BatchStore that also implements it; every read
// shares the G11-locked transaction.
type PushStore interface {
	BatchStore
	// IsPeriodLocked is the G10 check inside the transaction (FOR SHARE, so
	// the lock cannot be released until the push commits).
	IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error)
	// ListStdRows returns every std row of the batch ordered by key.
	ListStdRows(ctx context.Context) ([]StdRow, error)
	// LoadAxComponents returns, per cost id, the ACTUAL APPROVED cost row of
	// the period (R-9: an id absent from the result is no longer the active
	// APPROVED row).
	LoadAxComponents(ctx context.Context, period string, costIDs []int64) (map[int64]AxComponents, error)
}

// OracleCallStatus is ceocl_status (design §4.7).
type OracleCallStatus string

// Oracle call-log statuses.
const (
	OracleCallStarted OracleCallStatus = "STARTED"
	OracleCallSuccess OracleCallStatus = "SUCCESS"
	OracleCallFailed  OracleCallStatus = "FAILED"
	OracleCallUnknown OracleCallStatus = "UNKNOWN"
)

// IsTerminal reports whether s is a finished call status.
func (s OracleCallStatus) IsTerminal() bool {
	return s == OracleCallSuccess || s == OracleCallFailed || s == OracleCallUnknown
}

// CallKeyW1InsertBatch is the allowlist key of the W1 push (header + cost
// rows in one Oracle transaction). It mirrors the oracle package key.
const CallKeyW1InsertBatch = "W1_INSERT_BATCH"

// OracleCallStart is the STARTED row written before an Oracle call. Params
// hold non-secret binds only (ids, counts, hash), never credentials.
type OracleCallStart struct {
	CallID       string // UUID, unique per attempt
	BatchID      int64
	JobID        string // optional UUID
	StatementKey string
	Params       json.RawMessage
	Actor        string
	Attempt      int
}

// OracleCallFinish is the terminal outcome of a call.
type OracleCallFinish struct {
	Status  OracleCallStatus
	OraCode string // e.g. ORA-20901; empty when none
	Error   string
	Rows    *int64
	Summary json.RawMessage
}

// OracleCallLog persists cst_erp_oracle_call_log (§S-R11): Start commits the
// STARTED row before the call is issued, Finish records the outcome after.
type OracleCallLog interface {
	// HasAttempt reports whether any call (any status) was logged for the
	// batch and statement key. W1 is never retried with the same batch id.
	HasAttempt(ctx context.Context, batchID int64, statementKey string) (bool, error)
	Start(ctx context.Context, c OracleCallStart) error
	Finish(ctx context.Context, callID string, f OracleCallFinish) error
}
