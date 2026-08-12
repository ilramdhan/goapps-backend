// Package mbpush implements the MB (Master Batch) Push-to-Head preview/execute use cases.
package mbpush

import (
	"context"
	"database/sql"
)

// MBHeadReader lists MB Heads eligible for a push preview/execute pass.
type MBHeadReader interface {
	ListValidated(ctx context.Context) ([]MBHeadCandidate, error)
}

// MBHeadCandidate is the minimal MB Head projection Preview/Execute need.
type MBHeadCandidate struct {
	MBHID         string
	Code          string // mbh_mb_costing
	Name          string // mbh_mgt_name (nullable upstream, empty string if unset)
	CostProductID int64  // mbh_cost_product_id; 0 means not yet generated
	IsBoughtout   bool
}

// CostReader reads CALCULATED cst_product_cost rows for a product/period and transitions them
// directly to APPROVED as part of a push execution.
type CostReader interface {
	// GetActiveCalculated returns the active CALCULATED cost row for the tuple, non-transactional.
	// found=false, err=nil when no CALCULATED row exists (not-found is a normal skip path, not an error).
	GetActiveCalculated(ctx context.Context, productSysID int64, period, calcType string) (costID int64, costValue string, found bool, err error)

	// GetActiveCalculatedTx is the transaction-scoped variant, used inside Execute's savepoint scope.
	GetActiveCalculatedTx(ctx context.Context, tx *sql.Tx, productSysID int64, period, calcType string) (costID int64, costValue string, found bool, err error)

	// MarkApprovedFromCalculatedTx transitions a CALCULATED row directly to APPROVED, inside the
	// caller's transaction, so it commits/rolls back atomically with the cst_mb_cost upsert.
	MarkApprovedFromCalculatedTx(ctx context.Context, tx *sql.Tx, costID int64, by string) error
}

// StalePushReader reports which MB Heads were already pushed for a period but whose pushed cost
// has since been superseded by a later MB Batch run. Read-only: it informs Preview's labeling
// and never gates a write.
type StalePushReader interface {
	// ListStalePushedMBHIDs returns the mbh_ids in a stale-push state for period.
	ListStalePushedMBHIDs(ctx context.Context, period string) ([]string, error)
}

// MBCostWriter upserts the active-cost cache row inside a caller-supplied transaction.
type MBCostWriter interface {
	Upsert(ctx context.Context, tx *sql.Tx, mbhID, period, costType, costValue string, sourceCpcID int64, pushedBy string) error
}
