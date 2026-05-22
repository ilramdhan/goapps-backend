// Package costauditlog is the cost_audit_log domain (PRD Phase A §7.1.14, CAL_).
// Append-only audit trail. Writes happen via the Emitter helper from business
// handlers; this domain exposes Read operations only via the Repository.
package costauditlog

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors.
var (
	// ErrInvalidOperation when operation is outside the whitelist enforced by the DB CHECK.
	ErrInvalidOperation = errors.New("invalid operation")
)

// Operation values mirror the DB CHECK constraint.
const (
	OpInsert                 = "INSERT"
	OpUpdate                 = "UPDATE"
	OpDelete                 = "DELETE"
	OpStatusChange           = "STATUS_CHANGE"
	OpFeasibility            = "FEASIBILITY"
	OpClassificationOverride = "CLASSIFICATION_OVERRIDE"
	OpAssign                 = "ASSIGN"
	OpPromote                = "PROMOTE"
	OpHide                   = "HIDE"
	OpUnhide                 = "UNHIDE"
	OpRuleCreate             = "RULE_CREATE"
	OpRuleUpdate             = "RULE_UPDATE"
	OpRuleDelete             = "RULE_DELETE"
)

var allowedOperations = map[string]struct{}{
	OpInsert: {}, OpUpdate: {}, OpDelete: {},
	OpStatusChange: {}, OpFeasibility: {}, OpClassificationOverride: {},
	OpAssign: {}, OpPromote: {}, OpHide: {}, OpUnhide: {},
	OpRuleCreate: {}, OpRuleUpdate: {}, OpRuleDelete: {},
}

// Log is a single audit row.
type Log struct {
	LogID       int64
	EntityType  string
	EntityID    int64
	Operation   string
	BeforeData  string // JSON string; "" means NULL
	AfterData   string
	UserID      string
	PerformedAt time.Time
}

// NewInput is the write-time payload (consumed by Emitter).
type NewInput struct {
	EntityType string
	EntityID   int64
	Operation  string
	BeforeData string
	AfterData  string
	UserID     string
}

// Validate enforces the operation whitelist.
func (in NewInput) Validate() error {
	if _, ok := allowedOperations[in.Operation]; !ok {
		return ErrInvalidOperation
	}
	return nil
}

// Filter for List queries.
type Filter struct {
	EntityType string
	EntityID   int64
	UserID     string
	Operation  string
	FromDate   string // YYYY-MM-DD inclusive
	ToDate     string // YYYY-MM-DD inclusive
	Page       int
	PageSize   int
}

// Repository exposes read access + Emit (append). No Update / Delete by design —
// the DB enforces immutability via triggers.
type Repository interface {
	Emit(ctx context.Context, in NewInput) error
	List(ctx context.Context, f Filter) (items []*Log, total int64, err error)
}
