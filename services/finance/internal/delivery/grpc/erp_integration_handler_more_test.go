package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	cpmdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

type fakeLinker struct {
	res      cpmapp.LinkErpItemResult
	linkErr  error
	shadeErr error
	called   int
}

func (f *fakeLinker) Handle(context.Context, cpmapp.LinkErpItemCommand) (cpmapp.LinkErpItemResult, error) {
	f.called++
	return f.res, f.linkErr
}

func (f *fakeLinker) ShadeOf(context.Context, int64) (string, error) { return "S1", f.shadeErr }

type fakeMasterSync struct{ err error }

func (f fakeMasterSync) Trigger(_ context.Context, _, actor string) (*job.Execution, error) {
	if f.err != nil {
		return nil, f.err
	}
	return job.NewExecution(job.Type("ERP_MASTER_SYNC"), "ALL", "", actor, 5, nil)
}

type fakeBatchReader struct {
	b   *domain.Batch
	err error
}

func (f fakeBatchReader) GetByID(context.Context, int64) (*domain.Batch, error) { return f.b, f.err }
func (f fakeBatchReader) ListByPeriod(context.Context, string) ([]*domain.Batch, error) {
	return nil, nil
}

type fakeCovEmpty struct{}

func (fakeCovEmpty) List(context.Context, int64, ...domain.CoverageStatus) ([]domain.CoverageLine, error) {
	return nil, nil
}

type fakeStd struct{}

func (fakeStd) List(context.Context, int64) ([]domain.StdRow, error) { return nil, nil }

type fakeRecon struct{}

func (fakeRecon) ListRecon(context.Context, int64) ([]domain.ReconExportRow, error) { return nil, nil }

type fakeAbandon struct {
	b   *domain.Batch
	err error
}

func (f fakeAbandon) Handle(context.Context, erpapp.AbandonBatchCommand) (*domain.Batch, error) {
	return f.b, f.err
}

type fakeFromDemand struct {
	id     int64
	err    error
	called int
}

func (f *fakeFromDemand) Handle(context.Context, int64, int64, string) (int64, error) {
	f.called++
	return f.id, f.err
}

func newTestBatch(t *testing.T) *domain.Batch {
	t.Helper()
	b, err := domain.NewBatch("202606", domain.ModeLive, "tester", time.Now())
	require.NoError(t, err)
	return b
}

func TestErpIntegrationHandler_LinkErpToProduct(t *testing.T) {
	p, err := cpmdomain.New(cpmdomain.NewInput{ProductTypeID: 1, ProductName: "Yarn", ActorUserID: "tester"})
	require.NoError(t, err)
	cases := []struct {
		name    string
		sysID   int64
		linker  *fakeLinker
		ok      bool
		code    string
		wantHit int
	}{
		{"happy", 7, &fakeLinker{res: cpmapp.LinkErpItemResult{Product: p, Changed: true}}, true, "200", 1},
		{"linker error mapped", 7, &fakeLinker{linkErr: cpmdomain.ErrLinkDuplicate}, false, "409", 1},
		{"shade lookup not found", 7, &fakeLinker{shadeErr: cpmdomain.ErrNotFound}, false, "404", 0},
		{"invalid sys id", 0, &fakeLinker{}, false, "400", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{LinkErp: tc.linker})
			resp, err := h.LinkErpToProduct(ctxWith(), &financev1.LinkErpToProductRequest{ProductSysId: tc.sysID, ErpItemCode: "POY1"})
			require.NoError(t, err)
			assert.Equal(t, tc.ok, resp.Base.IsSuccess)
			if tc.ok {
				assert.Equal(t, tc.sysID, resp.ProductSysId)
			} else {
				assert.Equal(t, tc.code, resp.Base.StatusCode)
			}
			assert.Equal(t, tc.wantHit, tc.linker.called)
		})
	}
}

func TestErpIntegrationHandler_RunErpMasterSync(t *testing.T) {
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{MasterSync: fakeMasterSync{}})
	resp, err := h.RunErpMasterSync(ctxWith(), &financev1.RunErpMasterSyncRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Base.IsSuccess)
	assert.NotEmpty(t, resp.JobId)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{MasterSync: fakeMasterSync{err: job.ErrDuplicateActiveJob}})
	resp, err = h.RunErpMasterSync(ctxWith(), &financev1.RunErpMasterSyncRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Base.IsSuccess)
	assert.Empty(t, resp.JobId)
}

func TestErpIntegrationHandler_Exports(t *testing.T) {
	deps := func(r fakeBatchReader) ErpIntegrationDeps {
		return ErpIntegrationDeps{Batches: r, Coverage: fakeCovEmpty{}, StdCost: fakeStd{}, Recon: fakeRecon{}}
	}
	ok := fakeBatchReader{b: newTestBatch(t)}
	missing := fakeBatchReader{err: domain.ErrBatchNotFound}

	t.Run("coverage happy", func(t *testing.T) {
		resp, err := NewErpIntegrationGRPCHandler(deps(ok)).ExportErpCoverage(ctxWith(), &financev1.ExportErpCoverageRequest{BatchId: 1})
		require.NoError(t, err)
		assert.True(t, resp.Base.IsSuccess)
		assert.NotEmpty(t, resp.FileContent)
		assert.Contains(t, resp.FileName, ".xlsx")
	})
	t.Run("coverage batch not found", func(t *testing.T) {
		resp, err := NewErpIntegrationGRPCHandler(deps(missing)).ExportErpCoverage(ctxWith(), &financev1.ExportErpCoverageRequest{BatchId: 1})
		require.NoError(t, err)
		assert.Equal(t, "404", resp.Base.StatusCode)
	})
	t.Run("recon happy", func(t *testing.T) {
		resp, err := NewErpIntegrationGRPCHandler(deps(ok)).ExportErpRecon(ctxWith(), &financev1.ExportErpReconRequest{BatchId: 1})
		require.NoError(t, err)
		assert.True(t, resp.Base.IsSuccess)
		assert.NotEmpty(t, resp.FileContent)
	})
	t.Run("manual sample happy", func(t *testing.T) {
		resp, err := NewErpIntegrationGRPCHandler(deps(ok)).ExportErpManualSample(ctxWith(), &financev1.ExportErpManualSampleRequest{BatchId: 1})
		require.NoError(t, err)
		assert.True(t, resp.Base.IsSuccess)
		assert.NotEmpty(t, resp.FileContent)
	})
}

func TestErpIntegrationHandler_AbandonErpBatch(t *testing.T) {
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Abandon: fakeAbandon{b: newTestBatch(t)}})
	resp, err := h.AbandonErpBatch(ctxWith(), &financev1.AbandonErpBatchRequest{BatchId: 1, Reason: "x"})
	require.NoError(t, err)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, resp.Data)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Abandon: fakeAbandon{err: domain.ErrInvalidTransition}})
	resp, err = h.AbandonErpBatch(ctxWith(), &financev1.AbandonErpBatchRequest{BatchId: 1})
	require.NoError(t, err)
	assert.Equal(t, "412", resp.Base.StatusCode)
}

func TestErpIntegrationHandler_CreateCostProductFromDemand(t *testing.T) {
	f := &fakeFromDemand{id: 99}
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{CreateProduct: f})

	resp, err := h.CreateCostProductFromDemand(ctxWith(), &financev1.CreateCostProductFromDemandRequest{BatchId: 1, CecId: 2})
	require.NoError(t, err)
	assert.Equal(t, "403", resp.Base.StatusCode)
	assert.Zero(t, f.called, "use case must not run without permission")

	resp, err = h.CreateCostProductFromDemand(ctxWith("finance.product.route.create"), &financev1.CreateCostProductFromDemandRequest{BatchId: 1, CecId: 2})
	require.NoError(t, err)
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, int64(99), resp.ProductSysId)

	f.err = errors.New("boom")
	resp, err = h.CreateCostProductFromDemand(ctxWith("finance.product.route.create"), &financev1.CreateCostProductFromDemandRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Base.IsSuccess)
}
