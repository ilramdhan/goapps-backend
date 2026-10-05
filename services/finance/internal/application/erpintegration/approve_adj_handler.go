package erpintegration

// approve_adj_handler.go is the APPROVE_ADJ half of the ADJ execute step
// (plan-06 P5-T5; design Part 2 §9.4; D-D2). It runs only when
// erp_integration.adj_approve_enabled is on, only on a RECONCILED batch, and
// with the same preview, G6 and G7a gates as VALUATE. The batch status stays
// RECONCILED; the approval is a stamp (MarkAdjApproved).

import (
	"context"
	"fmt"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// approveSpec: RECONCILED and not yet approved; the set is every eligible
// (unapproved, unposted) row stamped with the batch id; the effect is the
// head approval on every row of the set.
func approveSpec() adjOpSpec {
	return adjOpSpec{
		op: domain.AdjOpApprove, callKey: domain.CallKeyW2ApproveAdj, summaryKey: "adj_approve",
		auditOp: auditdomain.OpErpAdjApprove, validate: true,
		checkBatch: func(_ context.Context, _ domain.AdjExecStore, b *domain.Batch) error {
			if err := requireStatus(b, "APPROVE_ADJ", domain.StatusReconciled); err != nil {
				return err
			}
			if b.AdjApproved() != nil {
				return fmt.Errorf("%w: batch %d ADJ already approved", ErrStepNotAllowed, b.ID())
			}
			return nil
		},
		selectSet: func(_, eligible []domain.AdjSnapshotRow, _ []domain.StdRow, batchID int64) (adjSet, error) {
			return stampedSet(eligible, batchID), nil
		},
		done: func(batchID int64) rowDoneFunc {
			return func(r domain.AdjSnapshotRow) bool { return r.IsApproved() && stampedBy(r, batchID) }
		},
		alreadyDone: func(rows, eligible []domain.AdjSnapshotRow, _ []domain.StdRow, batchID int64) bool {
			return len(stampedItems(eligible, batchID)) == 0 && anyApprovedStamped(rows, batchID)
		},
		call: func(batchID int64, actor string) func(context.Context, domain.OracleWriter) (domain.Summary, error) {
			return func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error) {
				return w.ApproveAdj(ctx, batchID, actor)
			}
		},
		finalize: func(s *AdjExecuteStep, _ context.Context, _ domain.AdjExecStore, b *domain.Batch, actor string, _ *AdjExecuteSummary) error {
			return b.MarkAdjApproved(actor, s.now())
		},
	}
}

// stampedSet is the set of rows carrying the batch stamp (APPROVE/RESTORE).
func stampedSet(rows []domain.AdjSnapshotRow, batchID int64) adjSet {
	var set []domain.AdjSnapshotRow
	for _, r := range rows {
		if stampedBy(r, batchID) {
			set = append(set, r)
		}
	}
	return adjSet{rows: set, expected: stampedItems(set, batchID), heads: len(adjSetHeads(set))}
}

// anyApprovedStamped reports an approved row stamped with the batch id.
func anyApprovedStamped(rows []domain.AdjSnapshotRow, batchID int64) bool {
	for _, r := range rows {
		if r.IsApproved() && stampedBy(r, batchID) {
			return true
		}
	}
	return false
}
