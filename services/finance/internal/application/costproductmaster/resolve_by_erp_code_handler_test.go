package costproductmaster

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// fakeErpResolverRepo returns a fixed product set and records the arguments it
// was called with, so the handler's grouping logic can be tested in isolation.
type fakeErpResolverRepo struct {
	products   []postgres.LookupProduct
	err        error
	gotItems   []string
	gotShades  []string
	callCount  int
	shouldFail bool
}

func (f *fakeErpResolverRepo) ResolveByErpCodes(_ context.Context, itemCodes, shadeCodes []string) ([]postgres.LookupProduct, error) {
	f.callCount++
	f.gotItems = itemCodes
	f.gotShades = shadeCodes
	if f.shouldFail {
		return nil, f.err
	}
	return f.products, nil
}

func product(sysID int64, code, erpItem, shade string) postgres.LookupProduct {
	return postgres.LookupProduct{
		ProductSysID: sysID,
		ProductCode:  code,
		ErpItemCode:  erpItem,
		ShadeCode:    shade,
		IsActive:     true,
	}
}

func TestResolveByErpCodeHandler_Handle_GroupsMatches(t *testing.T) {
	tests := []struct {
		name       string
		pairs      []ErpCodePair
		products   []postgres.LookupProduct
		wantCounts []int32
		wantSysIDs []int64 // 0 = expect no product on that resolution
	}{
		{
			name:       "unique match",
			pairs:      []ErpCodePair{{ErpItemCode: "ITEM-1", ShadeCode: "5918-01"}},
			products:   []postgres.LookupProduct{product(11, "P011", "ITEM-1", "5918-01")},
			wantCounts: []int32{1},
			wantSysIDs: []int64{11},
		},
		{
			name:       "no match",
			pairs:      []ErpCodePair{{ErpItemCode: "MISSING", ShadeCode: "NL"}},
			products:   nil,
			wantCounts: []int32{0},
			wantSysIDs: []int64{0},
		},
		{
			name:  "ambiguous TRIAL duplicate",
			pairs: []ErpCodePair{{ErpItemCode: "TRIAL-77", ShadeCode: "NL"}},
			products: []postgres.LookupProduct{
				product(21, "TRIAL-A", "TRIAL-77", "NL"),
				product(22, "TRIAL-B", "TRIAL-77", "NL"),
			},
			wantCounts: []int32{2},
			wantSysIDs: []int64{0},
		},
		{
			name:       "case and whitespace variance still matches",
			pairs:      []ErpCodePair{{ErpItemCode: "  item-9 ", ShadeCode: " nl "}},
			products:   []postgres.LookupProduct{product(31, "P031", "ITEM-9", "NL")},
			wantCounts: []int32{1},
			wantSysIDs: []int64{31},
		},
		{
			name:       "empty shade code matches empty shade product",
			pairs:      []ErpCodePair{{ErpItemCode: "ITEM-5", ShadeCode: ""}},
			products:   []postgres.LookupProduct{product(41, "P041", "ITEM-5", "")},
			wantCounts: []int32{1},
			wantSysIDs: []int64{41},
		},
		{
			name: "mixed batch keeps request order",
			pairs: []ErpCodePair{
				{ErpItemCode: "ITEM-1", ShadeCode: "NL"},
				{ErpItemCode: "GONE", ShadeCode: "NL"},
				{ErpItemCode: "TRIAL-77", ShadeCode: "NL"},
			},
			products: []postgres.LookupProduct{
				product(51, "P051", "ITEM-1", "NL"),
				product(52, "TRIAL-A", "TRIAL-77", "NL"),
				product(53, "TRIAL-B", "TRIAL-77", "NL"),
			},
			wantCounts: []int32{1, 0, 2},
			wantSysIDs: []int64{51, 0, 0},
		},
		{
			name: "same shade code different item codes do not collide",
			pairs: []ErpCodePair{
				{ErpItemCode: "ITEM-A", ShadeCode: "NL"},
				{ErpItemCode: "ITEM-B", ShadeCode: "NL"},
			},
			products: []postgres.LookupProduct{
				product(61, "P061", "ITEM-A", "NL"),
				product(62, "P062", "ITEM-B", "NL"),
			},
			wantCounts: []int32{1, 1},
			wantSysIDs: []int64{61, 62},
		},
		{
			name: "duplicate pair in request yields duplicate resolutions",
			pairs: []ErpCodePair{
				{ErpItemCode: "ITEM-1", ShadeCode: "NL"},
				{ErpItemCode: "ITEM-1", ShadeCode: "NL"},
			},
			products:   []postgres.LookupProduct{product(71, "P071", "ITEM-1", "NL")},
			wantCounts: []int32{1, 1},
			wantSysIDs: []int64{71, 71},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeErpResolverRepo{products: tt.products}
			h := NewResolveByErpCodeHandler(repo)

			got, err := h.Handle(context.Background(), tt.pairs)
			require.NoError(t, err)
			require.Len(t, got, len(tt.pairs))
			assert.Equal(t, 1, repo.callCount, "repository must be hit exactly once")

			for i := range got {
				assert.Equal(t, tt.pairs[i], got[i].Pair, "resolution %d keeps its pair", i)
				assert.Equal(t, tt.wantCounts[i], got[i].MatchCount, "resolution %d match count", i)
				if tt.wantSysIDs[i] == 0 {
					assert.Nil(t, got[i].Product, "resolution %d must carry no product", i)
					continue
				}
				require.NotNil(t, got[i].Product, "resolution %d must carry a product", i)
				assert.Equal(t, tt.wantSysIDs[i], got[i].Product.ProductSysID)
			}
		})
	}
}

func TestResolveByErpCodeHandler_Handle_EmptyInput(t *testing.T) {
	repo := &fakeErpResolverRepo{}
	h := NewResolveByErpCodeHandler(repo)

	got, err := h.Handle(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, 0, repo.callCount, "no repository round-trip for an empty request")
}

func TestResolveByErpCodeHandler_Handle_RejectsOversizedBatch(t *testing.T) {
	repo := &fakeErpResolverRepo{}
	h := NewResolveByErpCodeHandler(repo)

	pairs := make([]ErpCodePair, MaxErpCodePairs+1)
	_, err := h.Handle(context.Background(), pairs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many pairs")
	assert.Equal(t, 0, repo.callCount)
}

func TestResolveByErpCodeHandler_Handle_PropagatesRepositoryError(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &fakeErpResolverRepo{shouldFail: true, err: sentinel}
	h := NewResolveByErpCodeHandler(repo)

	_, err := h.Handle(context.Background(), []ErpCodePair{{ErpItemCode: "X"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestResolveByErpCodeHandler_Handle_ForwardsPairsPositionally(t *testing.T) {
	repo := &fakeErpResolverRepo{}
	h := NewResolveByErpCodeHandler(repo)

	_, err := h.Handle(context.Background(), []ErpCodePair{
		{ErpItemCode: "A", ShadeCode: "1"},
		{ErpItemCode: "B", ShadeCode: "2"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, repo.gotItems)
	assert.Equal(t, []string{"1", "2"}, repo.gotShades)
}
