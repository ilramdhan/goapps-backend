package financeclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// stubLookupClient is a hand-rolled CostMasterLookupServiceClient that records
// call counts and returns canned responses/errors for tests.
type stubLookupClient struct {
	financev1.CostMasterLookupServiceClient
	getResp   *financev1.GetCostProductMasterForPPCResponse
	getErr    error
	getCalls  int
	batchResp *financev1.BatchGetCostProductMasterResponse
	batchErr  error
	batchIDs  [][]int64

	listParamPages  []*financev1.ListProductParametersForPPCResponse
	listParamCalls  int
	listParamGroups []string
	listParamErr    error
	paramValuesResp *financev1.BatchGetProductParameterValuesResponse
	paramValuesErr  error
	paramValuesReqs []*financev1.BatchGetProductParameterValuesRequest

	resolveResp *financev1.ResolveCostProductMasterByErpCodeResponse
	resolveErr  error
	resolveReqs []*financev1.ResolveCostProductMasterByErpCodeRequest
}

func (s *stubLookupClient) ResolveCostProductMasterByErpCode(_ context.Context, in *financev1.ResolveCostProductMasterByErpCodeRequest, _ ...grpc.CallOption) (*financev1.ResolveCostProductMasterByErpCodeResponse, error) {
	s.resolveReqs = append(s.resolveReqs, in)
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return s.resolveResp, nil
}

func (s *stubLookupClient) ListProductParametersForPPC(_ context.Context, in *financev1.ListProductParametersForPPCRequest, _ ...grpc.CallOption) (*financev1.ListProductParametersForPPCResponse, error) {
	s.listParamGroups = append(s.listParamGroups, in.GetDisplayGroup())
	if s.listParamErr != nil {
		return nil, s.listParamErr
	}
	idx := s.listParamCalls
	s.listParamCalls++
	if idx < len(s.listParamPages) {
		return s.listParamPages[idx], nil
	}
	return &financev1.ListProductParametersForPPCResponse{}, nil
}

func (s *stubLookupClient) BatchGetProductParameterValues(_ context.Context, in *financev1.BatchGetProductParameterValuesRequest, _ ...grpc.CallOption) (*financev1.BatchGetProductParameterValuesResponse, error) {
	s.paramValuesReqs = append(s.paramValuesReqs, in)
	if s.paramValuesErr != nil {
		return nil, s.paramValuesErr
	}
	return s.paramValuesResp, nil
}

func (s *stubLookupClient) GetCostProductMasterForPPC(_ context.Context, in *financev1.GetCostProductMasterForPPCRequest, _ ...grpc.CallOption) (*financev1.GetCostProductMasterForPPCResponse, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getResp, nil
}

func (s *stubLookupClient) BatchGetCostProductMaster(_ context.Context, in *financev1.BatchGetCostProductMasterRequest, _ ...grpc.CallOption) (*financev1.BatchGetCostProductMasterResponse, error) {
	s.batchIDs = append(s.batchIDs, in.GetProductSysIds())
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	// Echo only the requested ids, as a real server would.
	wanted := make(map[int64]bool, len(in.GetProductSysIds()))
	for _, id := range in.GetProductSysIds() {
		wanted[id] = true
	}
	resp := &financev1.BatchGetCostProductMasterResponse{Base: s.batchResp.GetBase()}
	for _, p := range s.batchResp.GetData() {
		if wanted[p.GetProductSysId()] {
			resp.Data = append(resp.Data, p)
		}
	}
	return resp, nil
}

func newTestClient(cc financev1.CostMasterLookupServiceClient) *Client {
	return &Client{
		cc:          cc,
		callTimeout: time.Second,
		cache:       make(map[int64]cacheEntry),
	}
}

func productResp(sysID int64, active bool) *financev1.GetCostProductMasterForPPCResponse {
	return &financev1.GetCostProductMasterForPPCResponse{
		Data: &financev1.CostMasterProduct{ProductSysId: sysID, IsActive: active},
	}
}

func TestGetProduct_CachesResult(t *testing.T) {
	stub := &stubLookupClient{getResp: productResp(42, true)}
	c := newTestClient(stub)

	p1, err := c.GetProduct(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), p1.GetProductSysId())

	// Second call is served from cache — no additional RPC.
	p2, err := c.GetProduct(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), p2.GetProductSysId())
	assert.Equal(t, 1, stub.getCalls, "second GetProduct should hit cache")
}

func TestGetProduct_CacheExpiry(t *testing.T) {
	stub := &stubLookupClient{getResp: productResp(7, true)}
	c := newTestClient(stub)

	_, err := c.GetProduct(context.Background(), 7)
	require.NoError(t, err)

	// Force expiry.
	c.mu.Lock()
	c.cache[7] = cacheEntry{product: stub.getResp.GetData(), expiresAt: time.Now().Add(-time.Minute)}
	c.mu.Unlock()

	_, err = c.GetProduct(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 2, stub.getCalls, "expired cache entry should trigger a refetch")
}

func TestGetProduct_NilDataIsNotFound(t *testing.T) {
	stub := &stubLookupClient{getResp: &financev1.GetCostProductMasterForPPCResponse{Data: nil}}
	c := newTestClient(stub)

	_, err := c.GetProduct(context.Background(), 99)
	assert.ErrorIs(t, err, ErrProductNotFound)
}

func TestValidateProduct_ActiveOK(t *testing.T) {
	stub := &stubLookupClient{getResp: productResp(1, true)}
	c := newTestClient(stub)

	assert.NoError(t, c.ValidateProduct(context.Background(), 1))
}

func TestValidateProduct_InactiveRejected(t *testing.T) {
	stub := &stubLookupClient{getResp: productResp(2, false)}
	c := newTestClient(stub)

	err := c.ValidateProduct(context.Background(), 2)
	assert.ErrorIs(t, err, ErrProductInactive)
}

func TestValidateProduct_NonPositiveID(t *testing.T) {
	c := newTestClient(&stubLookupClient{})
	assert.ErrorIs(t, c.ValidateProduct(context.Background(), 0), ErrProductNotFound)
}

func TestValidateProduct_PropagatesRPCError(t *testing.T) {
	stub := &stubLookupClient{getErr: errors.New("boom")}
	c := newTestClient(stub)

	err := c.ValidateProduct(context.Background(), 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// A degraded client must fail closed on the write path and stay permissive on
// the display path — that asymmetry is the whole point of the split.
func TestDegradedClient_WriteValidationFailsClosed(t *testing.T) {
	c := &Client{degraded: true, cache: make(map[int64]cacheEntry)}

	assert.ErrorIs(t, c.ValidateProduct(context.Background(), 123), ErrProductValidationUnavailable)
	assert.NoError(t, c.ValidateProductForDisplay(context.Background(), 123))

	_, err := c.GetProduct(context.Background(), 123)
	assert.ErrorIs(t, err, ErrDegraded)

	_, err = c.ResolveProductsByErpCode(context.Background(), []ErpCodePair{{ErpItemCode: "X"}})
	assert.ErrorIs(t, err, ErrDegraded)
}

func TestValidateProductForDisplay_StillValidatesWhenConnected(t *testing.T) {
	stub := &stubLookupClient{getResp: productResp(3, false)}
	c := newTestClient(stub)

	assert.ErrorIs(t, c.ValidateProductForDisplay(context.Background(), 3), ErrProductInactive)
}

func TestResolveProductsByErpCode_MapsResolutions(t *testing.T) {
	stub := &stubLookupClient{resolveResp: &financev1.ResolveCostProductMasterByErpCodeResponse{
		Resolutions: []*financev1.ErpCodeResolution{
			{
				Pair:       &financev1.ErpCodePair{ErpItemCode: "ITEM-1", ShadeCode: "NL"},
				MatchCount: 1,
				Product:    &financev1.CostMasterProduct{ProductSysId: 11, IsActive: true},
			},
			{Pair: &financev1.ErpCodePair{ErpItemCode: "GONE", ShadeCode: "NL"}, MatchCount: 0},
			{Pair: &financev1.ErpCodePair{ErpItemCode: "TRIAL", ShadeCode: "NL"}, MatchCount: 2},
		},
	}}
	c := newTestClient(stub)

	got, err := c.ResolveProductsByErpCode(context.Background(), []ErpCodePair{
		{ErpItemCode: "ITEM-1", ShadeCode: "NL"},
		{ErpItemCode: "GONE", ShadeCode: "NL"},
		{ErpItemCode: "TRIAL", ShadeCode: "NL"},
	})
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, int32(1), got[0].MatchCount)
	require.NotNil(t, got[0].Product)
	assert.Equal(t, int64(11), got[0].Product.GetProductSysId())
	assert.Equal(t, int32(0), got[1].MatchCount)
	assert.Nil(t, got[1].Product)
	assert.Equal(t, int32(2), got[2].MatchCount)
	assert.Nil(t, got[2].Product)

	// A unique match warms the per-sys-id cache, so a later GetProduct is free.
	_, err = c.GetProduct(context.Background(), 11)
	require.NoError(t, err)
	assert.Equal(t, 0, stub.getCalls, "unique resolution should have populated the cache")
}

func TestResolveProductsByErpCode_EmptyInput(t *testing.T) {
	stub := &stubLookupClient{}
	c := newTestClient(stub)

	got, err := c.ResolveProductsByErpCode(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, stub.resolveReqs, "no RPC for an empty request")
}

func TestResolveProductsByErpCode_ChunksOversizedBatch(t *testing.T) {
	stub := &stubLookupClient{resolveResp: &financev1.ResolveCostProductMasterByErpCodeResponse{}}
	c := newTestClient(stub)

	pairs := make([]ErpCodePair, MaxErpCodePairs+3)
	_, err := c.ResolveProductsByErpCode(context.Background(), pairs)
	require.NoError(t, err)
	require.Len(t, stub.resolveReqs, 2)
	assert.Len(t, stub.resolveReqs[0].GetPairs(), MaxErpCodePairs)
	assert.Len(t, stub.resolveReqs[1].GetPairs(), 3)
}

func TestResolveProductsByErpCode_PropagatesRPCError(t *testing.T) {
	stub := &stubLookupClient{resolveErr: errors.New("boom")}
	c := newTestClient(stub)

	_, err := c.ResolveProductsByErpCode(context.Background(), []ErpCodePair{{ErpItemCode: "X"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// Finance reports application-level refusals (a rejected internal service
// token, a validation failure) in BaseResponse with a nil-error gRPC status.
// Reading only the payload turns such a refusal into "success, zero matches",
// which once let a token mismatch sit unnoticed in staging and production while
// every staging row stayed UNRESOLVED. The refusal must surface as an error.
func TestResolveProductsByErpCode_FailsOnUnsuccessfulBase(t *testing.T) {
	stub := &stubLookupClient{resolveResp: &financev1.ResolveCostProductMasterByErpCodeResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  false,
			Message:    "unauthenticated: invalid service token",
			StatusCode: "401",
		},
	}}
	c := newTestClient(stub)

	_, err := c.ResolveProductsByErpCode(context.Background(), []ErpCodePair{{ErpItemCode: "X"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFinanceRefused)
	assert.Contains(t, err.Error(), "invalid service token")
}

// A successful Base must not be mistaken for a refusal.
func TestResolveProductsByErpCode_AcceptsSuccessfulBase(t *testing.T) {
	stub := &stubLookupClient{resolveResp: &financev1.ResolveCostProductMasterByErpCodeResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "200"},
		Resolutions: []*financev1.ErpCodeResolution{
			{Pair: &financev1.ErpCodePair{ErpItemCode: "X"}, MatchCount: 0},
		},
	}}
	c := newTestClient(stub)

	got, err := c.ResolveProductsByErpCode(context.Background(), []ErpCodePair{{ErpItemCode: "X"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int32(0), got[0].MatchCount)
}

// Every lookup RPC shares the same BaseResponse contract, so none of them may
// read the payload without first checking the envelope.
func TestLookupRPCs_FailOnUnsuccessfulBase(t *testing.T) {
	refused := &commonv1.BaseResponse{IsSuccess: false, Message: "unauthenticated", StatusCode: "401"}

	tests := []struct {
		name string
		stub *stubLookupClient
		call func(*Client) error
	}{
		{
			name: "GetProduct",
			stub: &stubLookupClient{getResp: &financev1.GetCostProductMasterForPPCResponse{Base: refused}},
			call: func(c *Client) error {
				_, err := c.GetProduct(context.Background(), 1)
				return err
			},
		},
		{
			name: "BatchGetProducts",
			stub: &stubLookupClient{batchResp: &financev1.BatchGetCostProductMasterResponse{Base: refused}},
			call: func(c *Client) error {
				_, err := c.BatchGetProducts(context.Background(), []int64{1})
				return err
			},
		},
		{
			name: "ListProductParameters",
			stub: &stubLookupClient{listParamPages: []*financev1.ListProductParametersForPPCResponse{{Base: refused}}},
			call: func(c *Client) error {
				_, err := c.ListProductParameters(context.Background(), "")
				return err
			},
		},
		{
			name: "BatchGetProductParameterValues",
			stub: &stubLookupClient{paramValuesResp: &financev1.BatchGetProductParameterValuesResponse{Base: refused}},
			call: func(c *Client) error {
				_, err := c.BatchGetProductParameterValues(context.Background(), []int64{1}, nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(newTestClient(tt.stub))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrFinanceRefused)
		})
	}
}

func TestListProductParameters_MapsAndFiltersGroup(t *testing.T) {
	stub := &stubLookupClient{
		listParamPages: []*financev1.ListProductParametersForPPCResponse{
			{Data: []*financev1.CostMasterProductParameterDef{
				{ParamId: "p1", ParamCode: "YS", DataType: "NUMBER", DisplayGroup: "Machine", DefaultValue: "1200", IsActive: true},
			}},
		},
	}
	c := newTestClient(stub)

	defs, err := c.ListProductParameters(context.Background(), "Machine")
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "p1", defs[0].ParamID)
	assert.Equal(t, "YS", defs[0].ParamCode)
	assert.Equal(t, "NUMBER", defs[0].DataType)
	assert.Equal(t, "1200", defs[0].DefaultValue)
	require.Len(t, stub.listParamGroups, 1)
	assert.Equal(t, "Machine", stub.listParamGroups[0])
}

func TestListProductParameters_Paginates(t *testing.T) {
	// First page returns a full page (100) → triggers a second fetch.
	full := make([]*financev1.CostMasterProductParameterDef, 100)
	for i := range full {
		full[i] = &financev1.CostMasterProductParameterDef{ParamId: "x", IsActive: true}
	}
	stub := &stubLookupClient{
		listParamPages: []*financev1.ListProductParametersForPPCResponse{
			{Data: full},
			{Data: []*financev1.CostMasterProductParameterDef{{ParamId: "last"}}},
		},
	}
	c := newTestClient(stub)

	defs, err := c.ListProductParameters(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, defs, 101)
	assert.Equal(t, 2, stub.listParamCalls, "should fetch two pages")
}

func TestListProductParameters_PropagatesError(t *testing.T) {
	stub := &stubLookupClient{listParamErr: errors.New("boom")}
	c := newTestClient(stub)

	_, err := c.ListProductParameters(context.Background(), "Machine")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestListProductParameters_Degraded(t *testing.T) {
	c := &Client{degraded: true, cache: make(map[int64]cacheEntry)}
	_, err := c.ListProductParameters(context.Background(), "Machine")
	assert.ErrorIs(t, err, ErrDegraded)
}

func TestBatchGetProductParameterValues_MapsAndForwardsFilter(t *testing.T) {
	stub := &stubLookupClient{
		paramValuesResp: &financev1.BatchGetProductParameterValuesResponse{
			Data: []*financev1.CostMasterProductParameterValue{
				{ProductSysId: 5, ParamId: "p1", DataType: "NUMBER", ValueNumeric: "42"},
				{ProductSysId: 5, ParamId: "p2", DataType: "TEXT", ValueText: "hello"},
			},
		},
	}
	c := newTestClient(stub)

	vals, err := c.BatchGetProductParameterValues(context.Background(), []int64{5}, []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, vals, 2)
	assert.Equal(t, int64(5), vals[0].ProductSysID)
	assert.Equal(t, "42", vals[0].ValueNumeric)
	assert.Equal(t, "hello", vals[1].ValueText)
	require.Len(t, stub.paramValuesReqs, 1)
	assert.Equal(t, []int64{5}, stub.paramValuesReqs[0].GetProductSysIds())
	assert.Equal(t, []string{"p1", "p2"}, stub.paramValuesReqs[0].GetParamIds())
}

func TestBatchGetProductParameterValues_PropagatesError(t *testing.T) {
	stub := &stubLookupClient{paramValuesErr: errors.New("kaboom")}
	c := newTestClient(stub)

	_, err := c.BatchGetProductParameterValues(context.Background(), []int64{1}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kaboom")
}

func TestBatchGetProductParameterValues_Degraded(t *testing.T) {
	c := &Client{degraded: true, cache: make(map[int64]cacheEntry)}
	_, err := c.BatchGetProductParameterValues(context.Background(), []int64{1}, nil)
	assert.ErrorIs(t, err, ErrDegraded)
}

func TestBatchGetProducts_OnlyFetchesMisses(t *testing.T) {
	stub := &stubLookupClient{
		batchResp: &financev1.BatchGetCostProductMasterResponse{
			Data: []*financev1.CostMasterProduct{
				{ProductSysId: 10, IsActive: true},
				{ProductSysId: 20, IsActive: true},
			},
		},
	}
	c := newTestClient(stub)

	// Warm cache with id 10.
	c.cachePut(&financev1.CostMasterProduct{ProductSysId: 10, IsActive: true})

	got, err := c.BatchGetProducts(context.Background(), []int64{10, 20})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	require.Len(t, stub.batchIDs, 1)
	assert.Equal(t, []int64{20}, stub.batchIDs[0], "only the cache miss should be fetched")
}
