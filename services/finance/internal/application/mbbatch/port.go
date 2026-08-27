// Package mbbatch implements the MB_BATCH cost calculation orchestration: computing
// cst_product_cost rows for every VALIDATED MB Head's auto-gen'd product, in nested-MB
// dependency order (design doc §10.3, PRD §8).
package mbbatch

import (
	"context"
	"database/sql"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// MBHeadCandidate is the minimal MB Head projection the DAG builder and orchestrator need.
// Unlike mbpush.MBHeadCandidate, this includes CurrentVersion — needed to resolve the exact
// mst_mb_composition_version snapshot each MB's edges and RM composition come from.
type MBHeadCandidate struct {
	MBHID          string
	Code           string // mbh_mb_costing
	Name           string // mbh_mgt_name (nullable upstream, empty string if unset)
	CostProductID  int64  // mbh_cost_product_id; Task 20b's auto-gen'd product
	IsBoughtout    bool
	CurrentVersion int32
}

// MBHeadReader lists MB Heads eligible for an MB_BATCH compute pass.
type MBHeadReader interface {
	ListValidated(ctx context.Context) ([]MBHeadCandidate, error)
}

// MBEdge is one MB-to-MB composition dependency edge read from a frozen
// mst_mb_composition_version snapshot: MBHID's composition references RefMBHID's cost.
type MBEdge struct {
	MBHID    string
	RefMBHID string
}

// MBEdgeReader bulk-reads MB-to-MB composition edges across a set of (mbh_id, version) pairs.
type MBEdgeReader interface {
	ListMBEdgesBulk(ctx context.Context, mbhIDs []string, versions []int32) ([]MBEdge, error)
}

// JobActorReader reads the cal_job row an MB_BATCH run executes under, so the audit trail
// can be stamped with the human who triggered it (cal_job.cj_created_by) instead of a
// constant. Deliberately narrower than costcalcdom.JobRepository — only the lookup is
// needed — while keeping the same method signature, so *postgres.CostCalcJobRepository
// satisfies it with no adapter.
type JobActorReader interface {
	GetByID(ctx context.Context, id int64) (*costcalcdom.Job, error)
}

// ResultWriter persists cst_product_cost rows for an MB's auto-gen'd product, transaction-scoped
// so all 3 calc-type rows for one MB share the batch's commit/rollback boundary (design addendum
// §10.3 step 7), mirroring CostResultRepository.UpsertWithSupersedeTx.
type ResultWriter interface {
	UpsertWithSupersedeTx(ctx context.Context, tx *sql.Tx, r *costcalcdom.Result) (newCostID int64, prevVersion int, prevTotal float64, prevCostID int64, err error)
}
