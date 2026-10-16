package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsyncjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

type fakeApplier struct {
	gotProduct int64
	gotScope   lookupsync.Scope
	gotActor   string
	res        lookupsync.ProductApplyResult
	err        error
}

func (f *fakeApplier) ApplyProduct(_ context.Context, pid int64, scope lookupsync.Scope, actor string) (lookupsync.ProductApplyResult, error) {
	f.gotProduct, f.gotScope, f.gotActor = pid, scope, actor
	return f.res, f.err
}

func newLookupSyncChild(t *testing.T) (*job.Execution, uuid.UUID) {
	t.Helper()
	parentID := uuid.New()
	child, err := job.NewChildExecution(job.TypeProductLookupSync, "", "", "alice", 5, nil, parentID)
	require.NoError(t, err)
	return child, parentID
}

func lookupSyncMsg(t *testing.T, child *job.Execution) rabbitmq.JobMessage {
	t.Helper()
	scopeJSON, err := lookupsyncjob.EncodeScope(lookupsync.Scope{
		MasterRef: &lookupsync.MasterRef{LookupMasterCode: "MACHINE", MasterKey: "M1"},
	})
	require.NoError(t, err)
	return rabbitmq.JobMessage{
		JobID: child.ID().String(), JobType: "product_lookup_sync", CreatedBy: "alice",
		ProductSysID: 42, LookupSyncScope: scopeJSON, PlanFingerprint: "fp",
	}
}

func TestProductLookupSyncHandler_Handle(t *testing.T) {
	tests := []struct {
		name        string
		res         lookupsync.ProductApplyResult
		err         error
		wantStatus  job.Status
		wantSuccess bool
		wantSkipped bool
	}{
		{name: "success", res: lookupsync.ProductApplyResult{ProductSysID: 42, ProductCode: "P42", ValuesWritten: 3, ValuesCleared: 1},
			wantStatus: job.StatusSuccess, wantSuccess: true},
		{name: "skipped locked", res: lookupsync.ProductApplyResult{ProductSysID: 42, Skipped: true, SkipReason: lookupsync.SkipLocked},
			wantStatus: job.StatusSuccess, wantSuccess: true, wantSkipped: true},
		{name: "error", err: errors.New("boom"), wantStatus: job.StatusFailed, wantSuccess: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child, parentID := newLookupSyncChild(t)
			repo := &workerJobRepoMock{}
			repo.On("GetByID", mock.Anything, child.ID()).Return(child, nil)
			repo.On("UpdateStatus", mock.Anything, child).Return(nil)
			repo.On("IncrementChildProgress", mock.Anything, parentID, tt.wantSuccess).Return(false, nil).Once()
			app := &fakeApplier{res: tt.res, err: tt.err}
			h := NewProductLookupSyncHandler(repo, app, zerolog.Nop())

			require.NoError(t, h.Handle(context.Background(), lookupSyncMsg(t, child)))

			assert.Equal(t, int64(42), app.gotProduct)
			assert.Equal(t, "alice", app.gotActor)
			require.NotNil(t, app.gotScope.MasterRef)
			assert.Equal(t, "M1", app.gotScope.MasterRef.MasterKey)
			assert.Equal(t, tt.wantStatus, child.Status())
			if tt.err != nil {
				assert.Contains(t, child.ErrorMessage(), "boom")
			} else {
				var r lookupsyncjob.ChildResult
				require.NoError(t, json.Unmarshal(child.ResultSummary(), &r))
				assert.Equal(t, tt.wantSkipped, r.Skipped)
				assert.Equal(t, tt.res.ValuesWritten, r.ValuesWritten)
				assert.Equal(t, tt.res.SkipReason, r.SkipReason)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProductLookupSyncHandler_BatchCompleteMarksParent(t *testing.T) {
	child, parentID := newLookupSyncChild(t)
	parent := job.Reconstitute(parentID, job.GenerateCode(job.TypeProductLookupSync, "", 1), job.TypeProductLookupSync,
		"", "", job.StatusProcessing, 5, nil, nil, "", 0, 0, 3, child.QueuedAt(), nil, nil, "alice", "", nil, nil, nil, 1, 1, 0)
	repo := &workerJobRepoMock{}
	repo.On("GetByID", mock.Anything, child.ID()).Return(child, nil)
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil)
	repo.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	repo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(true, nil)
	h := NewProductLookupSyncHandler(repo, &fakeApplier{res: lookupsync.ProductApplyResult{ValuesWritten: 1}}, zerolog.Nop())

	require.NoError(t, h.Handle(context.Background(), lookupSyncMsg(t, child)))
	assert.Equal(t, job.StatusSuccess, parent.Status())
}

func TestProductLookupSyncHandler_InvalidJobID(t *testing.T) {
	h := NewProductLookupSyncHandler(&workerJobRepoMock{}, &fakeApplier{}, zerolog.Nop())
	require.Error(t, h.Handle(context.Background(), rabbitmq.JobMessage{JobID: "nope"}))
}
