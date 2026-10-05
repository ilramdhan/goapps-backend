package erpintegration

// restore_adj_handler.go is the RESTORE_ADJ half of the ADJ execute step
// (plan-06 P5-T5; design Part 2 §9.4; C-9). It runs only on the latest
// batch of the period (VALUATED or RECONCILED, never ADJ-approved), with
// the same preview and G6 gates as VALUATE. PKG_GOAPPS_ADJ.RESTORE_ADJ puts
// back the values from the Oracle-side snapshot; the batch then moves to
// FAILED ("restored") and a read-only re-probe verifies that no ADJ row of
// the period still carries the batch stamp.

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// restoreReason is the ceib_error of a restored batch.
const restoreReason = "restored by RESTORE_ADJ"

// restoreSpec: latest batch, VALUATED/RECONCILED, not approved; the set is
// every row stamped with the batch id (an approved stamped head refuses);
// the effect is the stamp gone.
func restoreSpec() adjOpSpec {
	return adjOpSpec{
		op: domain.AdjOpRestore, callKey: domain.CallKeyW2RestoreAdj, summaryKey: "adj_restore",
		auditOp: auditdomain.OpErpRestore, validate: false,
		checkBatch: checkRestoreBatch,
		selectSet: func(rows, _ []domain.AdjSnapshotRow, _ []domain.StdRow, batchID int64) (adjSet, error) {
			if anyApprovedStamped(rows, batchID) {
				return adjSet{}, fmt.Errorf("%w: an approved head carries batch %d", ErrAdjIneligible, batchID)
			}
			return stampedSet(rows, batchID), nil
		},
		done: func(batchID int64) rowDoneFunc {
			return func(r domain.AdjSnapshotRow) bool { return !stampedBy(r, batchID) }
		},
		alreadyDone: func(rows, _ []domain.AdjSnapshotRow, _ []domain.StdRow, batchID int64) bool {
			return len(stampedItems(rows, batchID)) == 0
		},
		call: func(batchID int64, _ string) func(context.Context, domain.OracleWriter) (domain.Summary, error) {
			return func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error) {
				return w.RestoreAdj(ctx, batchID)
			}
		},
		finalize: (*AdjExecuteStep).finalizeRestore,
	}
}

func checkRestoreBatch(ctx context.Context, st domain.AdjExecStore, b *domain.Batch) error {
	if err := requireStatus(b, "RESTORE_ADJ", domain.StatusValuated, domain.StatusReconciled); err != nil {
		return err
	}
	if b.AdjApproved() != nil {
		return fmt.Errorf("%w: batch %d ADJ already approved", ErrAdjIneligible, b.ID())
	}
	latest, err := st.IsLatestBatch(ctx, b.Period())
	if err != nil {
		return fmt.Errorf("check latest batch: %w", err)
	}
	if !latest {
		return fmt.Errorf("%w: batch %d", ErrNotLatestBatch, b.ID())
	}
	return nil
}

// finalizeRestore verifies the restore read-only and moves the batch to
// FAILED. A failed verification is recorded (restore_verified=false) but
// does not undo the committed Oracle restore.
func (s *AdjExecuteStep) finalizeRestore(ctx context.Context, _ domain.AdjExecStore, b *domain.Batch, actor string,
	sum *AdjExecuteSummary,
) error {
	verified := false
	rows, err := s.reader.SnapshotAdjRows(context.WithoutCancel(ctx), b.Period())
	if err == nil {
		verified = len(stampedItems(rows, b.ID())) == 0
	}
	sum.RestoreVerified = &verified
	if !verified {
		logRestoreUnverified(b.ID(), err)
	}
	return b.Fail(restoreReason, actor, s.now())
}

func logRestoreUnverified(batchID int64, err error) {
	log.Error().Err(err).Int64("batch_id", batchID).
		Msg("ALERT erp w2: RESTORE_ADJ committed but the read-only re-probe still finds the batch stamp (or failed)")
}
