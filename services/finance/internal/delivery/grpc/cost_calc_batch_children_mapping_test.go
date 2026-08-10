package grpc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// TestCostSheetExportBatchChildrenFromResult_Empty verifies an empty input
// slice maps to an empty (non-nil) proto slice rather than nil.
func TestCostSheetExportBatchChildrenFromResult_Empty(t *testing.T) {
	got := costSheetExportBatchChildrenFromResult(nil)
	require.NotNil(t, got)
	assert.Empty(t, got)
}

// TestCostSheetExportBatchChildrenFromResult_MapsAllFields verifies every
// application-layer field lands on the correct wire field, including a
// SUCCESS child with a resolved download URL and a non-SUCCESS child with an
// empty one.
func TestCostSheetExportBatchChildrenFromResult_MapsAllFields(t *testing.T) {
	jobID1, jobID2 := uuid.New(), uuid.New()
	children := []costsheet.BatchChildResult{
		{
			JobID:       jobID1,
			JobCode:     "PCS_EX-202604-001",
			Status:      job.StatusSuccess,
			DownloadURL: "https://minio/presigned?a",
			FileName:    "product-cost-sheet-202604-001.xlsx",
		},
		{
			JobID:   jobID2,
			JobCode: "PCS_EX-202604-002",
			Status:  job.StatusProcessing,
		},
	}

	got := costSheetExportBatchChildrenFromResult(children)
	require.Len(t, got, 2)

	assert.Equal(t, jobID1.String(), got[0].GetJobId())
	assert.Equal(t, "PCS_EX-202604-001", got[0].GetJobCode())
	assert.Equal(t, "SUCCESS", got[0].GetStatus())
	assert.Equal(t, "https://minio/presigned?a", got[0].GetDownloadUrl())
	assert.Equal(t, "product-cost-sheet-202604-001.xlsx", got[0].GetFileName())

	assert.Equal(t, jobID2.String(), got[1].GetJobId())
	assert.Equal(t, "PROCESSING", got[1].GetStatus())
	assert.Empty(t, got[1].GetDownloadUrl())
	assert.Empty(t, got[1].GetFileName())
}
