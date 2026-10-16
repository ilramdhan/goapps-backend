package grpc

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsyncjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// LookupSyncJobAdapter adapts lookupsyncjob.Enqueuer / Tracker onto the
// LookupSyncEnqueuer and LookupSyncJobTracker ports of
// CostProductLookupSyncHandler.
type LookupSyncJobAdapter struct {
	enqueuer *lookupsyncjob.Enqueuer
	tracker  *lookupsyncjob.Tracker
}

var (
	_ LookupSyncEnqueuer   = (*LookupSyncJobAdapter)(nil)
	_ LookupSyncJobTracker = (*LookupSyncJobAdapter)(nil)
)

// NewLookupSyncJobAdapter constructs the adapter.
func NewLookupSyncJobAdapter(e *lookupsyncjob.Enqueuer, t *lookupsyncjob.Tracker) *LookupSyncJobAdapter {
	return &LookupSyncJobAdapter{enqueuer: e, tracker: t}
}

// EnqueueLookupSync implements LookupSyncEnqueuer.
func (a *LookupSyncJobAdapter) EnqueueLookupSync(
	ctx context.Context, scope lookupsync.Scope, fingerprint, actor string,
) (LookupSyncJobInfo, error) {
	exec, err := a.enqueuer.Enqueue(ctx, scope, fingerprint, actor)
	if err != nil {
		return LookupSyncJobInfo{}, err
	}
	return lookupSyncJobInfoFromExecution(exec), nil
}

// GetLookupSyncJobStatus implements LookupSyncJobTracker.
func (a *LookupSyncJobAdapter) GetLookupSyncJobStatus(ctx context.Context, jobID string) (LookupSyncJobStatus, error) {
	st, err := a.tracker.Status(ctx, jobID)
	if err != nil {
		return LookupSyncJobStatus{}, err
	}
	return LookupSyncJobStatus{
		Info:            lookupSyncJobInfoFromExecution(st.Execution),
		SkippedChildren: st.SkippedChildren,
		ValuesWritten:   st.ValuesWritten,
	}, nil
}

// ListLookupSyncJobFailures implements LookupSyncJobTracker.
func (a *LookupSyncJobAdapter) ListLookupSyncJobFailures(ctx context.Context, jobID string) ([]LookupSyncJobFailure, error) {
	fails, err := a.tracker.Failures(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := make([]LookupSyncJobFailure, 0, len(fails))
	for _, f := range fails {
		out = append(out, LookupSyncJobFailure(f))
	}
	return out, nil
}

func lookupSyncJobInfoFromExecution(exec *job.Execution) LookupSyncJobInfo {
	return LookupSyncJobInfo{
		JobID:             exec.ID().String(),
		JobCode:           exec.Code().String(),
		Status:            lookupsyncjob.StatusString(exec),
		TotalChildren:     exec.TotalChildren(),
		CompletedChildren: exec.CompletedChildren(),
		FailedChildren:    exec.FailedChildren(),
	}
}
