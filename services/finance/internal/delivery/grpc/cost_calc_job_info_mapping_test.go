package grpc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// TestProductCostSheetExportJobInfoFromExecution_StandaloneJob verifies a
// non-batch export job reports is_batch=false with all child counters at
// zero, matching NewExecution's default (no parent/child wiring).
func TestProductCostSheetExportJobInfoFromExecution_StandaloneJob(t *testing.T) {
	exec, err := job.NewExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, nil)
	require.NoError(t, err)

	got := productCostSheetExportJobInfoFromExecution(exec)
	require.NotNil(t, got)
	assert.Equal(t, exec.ID().String(), got.GetJobId())
	assert.Equal(t, string(exec.Status()), got.GetStatus())
	assert.False(t, got.GetIsBatch())
	assert.Equal(t, int32(0), got.GetTotalChildren())
	assert.Equal(t, int32(0), got.GetCompletedChildren())
	assert.Equal(t, int32(0), got.GetFailedChildren())
}

// TestProductCostSheetExportJobInfoFromExecution_ParentBatchJob verifies a
// batch-tracking parent job (fanned out into N children) reports is_batch=true
// plus the total/completed/failed child counters straight from the domain
// entity's in-memory counters.
func TestProductCostSheetExportJobInfoFromExecution_ParentBatchJob(t *testing.T) {
	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, nil, 3)
	require.NoError(t, err)
	parent.IncrementCompletedChildren()
	parent.IncrementCompletedChildren()
	parent.IncrementFailedChildren()

	got := productCostSheetExportJobInfoFromExecution(parent)
	require.NotNil(t, got)
	assert.True(t, got.GetIsBatch())
	assert.Equal(t, int32(3), got.GetTotalChildren())
	assert.Equal(t, int32(2), got.GetCompletedChildren())
	assert.Equal(t, int32(1), got.GetFailedChildren())
}

// TestProductCostSheetExportJobInfoFromExecution_ChildJob verifies a child job
// (belongs to a batch but is not itself the aggregator) is NOT reported as
// is_batch — IsParent() requires parentJobID == nil, so a child job always
// shows the "standalone" shape from the caller's point of view.
func TestProductCostSheetExportJobInfoFromExecution_ChildJob(t *testing.T) {
	parentID := uuid.New()
	child, err := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, nil, parentID)
	require.NoError(t, err)

	got := productCostSheetExportJobInfoFromExecution(child)
	require.NotNil(t, got)
	assert.False(t, got.GetIsBatch())
	assert.Equal(t, int32(0), got.GetTotalChildren())
}
