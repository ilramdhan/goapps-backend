package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	domainperiodlock "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

type fakeErpCreator struct {
	b   *domain.Batch
	err error
	got erpapp.CreateBatchCommand
}

func (f *fakeErpCreator) Handle(_ context.Context, cmd erpapp.CreateBatchCommand) (*domain.Batch, error) {
	f.got = cmd
	return f.b, f.err
}

func ctxWith(perms ...string) context.Context {
	ctx := context.WithValue(context.Background(), AuthUsernameKey, "tester")
	return context.WithValue(ctx, AuthPermissionsKey, perms)
}

// AC-06: a caller without the push permission never reaches PushErpBatch.
func TestPermissionInterceptor_PushErpBatch_DeniedWithoutPushPermission(t *testing.T) {
	called := 0
	handler := func(context.Context, any) (any, error) { called++; return nil, nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.ErpIntegrationService/PushErpBatch"}

	_, err := PermissionInterceptor()(ctxWith("finance.cost.erpintegration.trigger"), nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Zero(t, called, "handler must not be invoked")

	_, err = PermissionInterceptor()(ctxWith("finance.cost.erpintegration.push"), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestErpIntegrationHandler_CreateErpBatch_Happy(t *testing.T) {
	b, err := domain.NewBatch("202606", domain.ModeLive, "tester", time.Now())
	require.NoError(t, err)
	f := &fakeErpCreator{b: b}
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Create: f})

	resp, err := h.CreateErpBatch(ctxWith(), &financev1.CreateErpBatchRequest{Period: "202606"})
	require.NoError(t, err)
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, "202606", resp.Data.Period)
	assert.Equal(t, "tester", f.got.Actor)
	assert.Equal(t, domain.ModeLive, f.got.Mode)
}

func TestErpErrBase_Mapping(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{domain.ErrBatchNotFound, "404"},
		{erpapp.ErrPushPermissionDenied, "403"},
		{erpapp.ErrAdjPermissionDenied, "403"},
		{erpapp.ErrValuatePermissionDenied, "403"},
		{domain.ErrInvalidBatchPeriod, "400"},
		{domainperiodlock.ErrPeriodLocked, "412"},
		{erpapp.ErrStepNotAllowed, "412"},
		{domain.ErrFeatureDisabled, "412"},
		{domain.ErrShadowNotPushable, "412"},
		{domain.ErrWriterNotConfigured, "412"},
		{errors.New("boom"), "500"},
	}
	for _, c := range cases {
		wrapped := fmt.Errorf("ctx: %w", c.err)
		base := erpErrBase(wrapped)
		assert.False(t, base.IsSuccess)
		assert.Equal(t, c.code, base.StatusCode, c.err.Error())
		assert.Contains(t, base.Message, c.err.Error())
	}
}

type fakeErpSteps struct {
	adj    erpapp.AdjTriggerCommand
	adjErr error
}

func (f *fakeErpSteps) Handle(context.Context, erpapp.StepTriggerCommand) (*job.Execution, error) {
	return nil, nil
}
func (f *fakeErpSteps) TriggerPush(context.Context, erpapp.PushTriggerCommand) (*job.Execution, error) {
	return nil, nil
}
func (f *fakeErpSteps) TriggerLockBatch(context.Context, erpapp.LockBatchTriggerCommand) (*job.Execution, error) {
	return nil, nil
}
func (f *fakeErpSteps) TriggerSubtype(context.Context, string, string, any, string) (*job.Execution, error) {
	return nil, nil
}
func (f *fakeErpSteps) TriggerAdjExecute(_ context.Context, c erpapp.AdjTriggerCommand) (*job.Execution, error) {
	f.adj = c
	if f.adjErr != nil {
		return nil, f.adjErr
	}
	return job.NewExecution(job.TypeErpIntegration, "adj_execute", "202606", "tester", 5, nil)
}

type fakePreviews struct{ p domain.ValuationPreview }

func (f fakePreviews) Get(context.Context, string) (domain.ValuationPreview, error) { return f.p, nil }

type fakeAcker struct{ got erpapp.AckWarningsCommand }

func (f *fakeAcker) Handle(_ context.Context, c erpapp.AckWarningsCommand) (erpapp.AckWarningsResult, error) {
	f.got = c
	return erpapp.AckWarningsResult{}, errors.New("stop")
}

type fakeCoverage struct{ lines []domain.CoverageLine }

func (f fakeCoverage) List(context.Context, int64, ...domain.CoverageStatus) ([]domain.CoverageLine, error) {
	return f.lines, nil
}

func TestErpIntegrationHandler_ExecuteErpAdjOperation(t *testing.T) {
	prev := domain.ValuationPreview{ID: "p-1", BatchID: 7, Operation: domain.AdjOpApprove}
	cases := []struct {
		name    string
		perms   []string
		adjErr  error
		wantOK  bool
		wantErr string
	}{
		{"has per-op permission", []string{permErpApprove}, nil, true, ""},
		{"valuate perm is not enough for approve", []string{permErpValuate}, erpapp.ErrAdjPermissionDenied, false, "403"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeErpSteps{adjErr: tc.adjErr}
			h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Steps: st, Previews: fakePreviews{prev}})
			resp, err := h.ExecuteErpAdjOperation(ctxWith(tc.perms...), &financev1.ExecuteErpAdjOperationRequest{PreviewId: "p-1", ConfirmSetHash: "h", ConfirmText: "t"})
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, resp.Base.IsSuccess)
			assert.Equal(t, int64(7), st.adj.BatchID)
			assert.Equal(t, domain.AdjOpApprove, st.adj.Operation)
			assert.Equal(t, tc.wantOK, st.adj.HasPermission)
			if !tc.wantOK {
				assert.Equal(t, tc.wantErr, resp.Base.StatusCode)
			}
		})
	}
}

func TestErpIntegrationHandler_AckErpWarnings_PassesHash(t *testing.T) {
	ack := &fakeAcker{}
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Ack: ack})
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := h.AckErpWarnings(ctxWith(permErpApprove), &financev1.AckErpWarningsRequest{BatchId: 3, Reason: "long enough reason", WarningSetHash: hash})
	require.NoError(t, err)
	assert.Equal(t, hash, ack.got.WarningSetHash)
	assert.True(t, ack.got.HasPermission)
}

func TestErpIntegrationHandler_GetErpIntegrationConfig_NoSecrets(t *testing.T) {
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Config: ErpConfigView{PushEnabled: true, WriterMode: "fake", OracleIFConfigured: true}})
	resp, err := h.GetErpIntegrationConfig(ctxWith(), &financev1.GetErpIntegrationConfigRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Data.PushEnabled)
	assert.Equal(t, "fake", resp.Data.WriterMode)
	assert.True(t, resp.Data.OracleIfConfigured)
	assert.False(t, resp.Data.ValuationEnabled)
}

func TestErpIntegrationHandler_ListErpCoverage_Pagination(t *testing.T) {
	var lines []domain.CoverageLine
	for i := 0; i < 5; i++ {
		lines = append(lines, domain.CoverageLine{ItemCode: "I" + string(rune('A'+i)), Status: domain.CoverageOK})
	}
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Coverage: fakeCoverage{lines}})
	resp, err := h.ListErpCoverage(ctxWith(), &financev1.ListErpCoverageRequest{BatchId: 1, Pagination: &commonv1.PaginationRequest{Page: 2, PageSize: 2}})
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "IC", resp.Data[0].ErpItemCode)
	assert.Equal(t, int64(5), resp.Pagination.TotalItems)
	assert.Equal(t, int32(3), resp.Pagination.TotalPages)
}
