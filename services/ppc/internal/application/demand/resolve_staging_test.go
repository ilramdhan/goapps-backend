package demand_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// resolveRepo is a fake demand.Repository exercising the staging-resolution
// surface only; the rest of the contract is inert.
type resolveRepo struct {
	pairs       []demanddomain.StagingPair
	pairsErr    error
	applied     [][]demanddomain.ProductResolution
	appliedErr  error
	rowsUpdated int64

	stagingPages [][]*demanddomain.SalesOrderStaging
	stagingCalls int

	stagingIDs     []int64
	stagingIDTotal int64
	idsFilter      demanddomain.StagingIDsFilter
	idsLimit       int

	setProductCalls [][2]int64
	setProductErr   error
}

func (r *resolveRepo) Create(_ context.Context, _ *demanddomain.Demand) error { return nil }
func (r *resolveRepo) GetByID(_ context.Context, _ int64) (*demanddomain.Demand, error) {
	return nil, nil
}

func (r *resolveRepo) List(_ context.Context, _ demanddomain.ListFilter) ([]*demanddomain.Demand, int64, error) {
	return nil, 0, nil
}
func (r *resolveRepo) Update(_ context.Context, _ *demanddomain.Demand) error { return nil }
func (r *resolveRepo) Delete(_ context.Context, _ int64) error                { return nil }
func (r *resolveRepo) CountPlanItems(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (r *resolveRepo) ListCarryCandidates(_ context.Context, _ string) ([]*demanddomain.Demand, error) {
	return nil, nil
}

func (r *resolveRepo) GetStagingByIDs(_ context.Context, _ []int64) ([]*demanddomain.SalesOrderStaging, error) {
	return nil, nil
}

func (r *resolveRepo) ListStagingIDs(_ context.Context, filter demanddomain.StagingIDsFilter, limit int) ([]int64, int64, error) {
	r.idsFilter = filter
	r.idsLimit = limit
	ids := r.stagingIDs
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, r.stagingIDTotal, nil
}

func (r *resolveRepo) ListStaging(_ context.Context, _ demanddomain.StagingListFilter) ([]*demanddomain.SalesOrderStaging, int64, error) {
	page := []*demanddomain.SalesOrderStaging(nil)
	if r.stagingCalls < len(r.stagingPages) {
		page = r.stagingPages[r.stagingCalls]
	}
	r.stagingCalls++
	return page, int64(len(page)), nil
}
func (r *resolveRepo) LookupStagingItemCodes(_ context.Context, _ []int64) (map[int64]string, error) {
	return nil, nil
}
func (r *resolveRepo) MarkStagingPulled(_ context.Context, _, _ int64) error { return nil }

func (r *resolveRepo) SetStagingProduct(_ context.Context, sosID, sysID int64) (*demanddomain.SalesOrderStaging, error) {
	r.setProductCalls = append(r.setProductCalls, [2]int64{sosID, sysID})
	if r.setProductErr != nil {
		return nil, r.setProductErr
	}
	return &demanddomain.SalesOrderStaging{
		SosID:           sosID,
		CpmProductSysID: &sysID,
		MatchStatus:     demanddomain.MatchStatusManual,
		MatchCount:      1,
	}, nil
}

func (r *resolveRepo) ListUnresolvedStagingPairs(_ context.Context) ([]demanddomain.StagingPair, error) {
	if r.pairsErr != nil {
		return nil, r.pairsErr
	}
	return r.pairs, nil
}

func (r *resolveRepo) ApplyStagingResolutions(_ context.Context, res []demanddomain.ProductResolution) (int64, error) {
	if r.appliedErr != nil {
		return 0, r.appliedErr
	}
	r.applied = append(r.applied, res)
	r.rowsUpdated += int64(len(res))
	return int64(len(res)), nil
}

// fakeResolver is a scripted demand.ProductResolver.
type fakeResolver struct {
	byItem map[string]demanddomain.ProductResolution
	err    error
	calls  int
	seen   []demanddomain.StagingPair
	// empty makes the resolver answer a non-empty request with zero
	// resolutions, the shape a silently-refused finance call used to produce.
	empty bool
}

func (f *fakeResolver) ResolveByErpCode(_ context.Context, pairs []demanddomain.StagingPair) ([]demanddomain.ProductResolution, error) {
	f.calls++
	f.seen = append(f.seen, pairs...)
	if f.err != nil {
		return nil, f.err
	}
	if f.empty {
		return nil, nil
	}
	out := make([]demanddomain.ProductResolution, 0, len(pairs))
	for _, p := range pairs {
		if r, ok := f.byItem[p.ItemCode]; ok {
			r.Pair = p
			out = append(out, r)
			continue
		}
		out = append(out, demanddomain.ProductResolution{Pair: p})
	}
	return out, nil
}

func sysID(v int64) *int64 { return &v }

func TestResolveStaging_TalliesPerOutcomeAndPersists(t *testing.T) {
	repo := &resolveRepo{pairs: []demanddomain.StagingPair{
		{ItemCode: "AUTO1", ShadeCode: "S1"},
		{ItemCode: "AMBI1", ShadeCode: "S2"},
		{ItemCode: "MISS1", ShadeCode: ""},
	}}
	resolver := &fakeResolver{byItem: map[string]demanddomain.ProductResolution{
		"AUTO1": {MatchCount: 1, CpmProductSysID: sysID(101)},
		"AMBI1": {MatchCount: 3},
		"MISS1": {MatchCount: 0},
	}}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)

	assert.False(t, res.Skipped)
	assert.Equal(t, 3, res.Pairs)
	assert.Equal(t, 1, res.Auto)
	assert.Equal(t, 1, res.Ambiguous)
	assert.Equal(t, 1, res.NotFound)
	assert.Equal(t, int64(3), res.RowsUpdated)
	require.Len(t, repo.applied, 1)
	assert.Equal(t, demanddomain.MatchStatusAuto, repo.applied[0][0].MatchStatus())
	assert.Equal(t, demanddomain.MatchStatusAmbiguous, repo.applied[0][1].MatchStatus())
	assert.Equal(t, demanddomain.MatchStatusNotFound, repo.applied[0][2].MatchStatus())
}

func TestResolveStaging_NoResolverSkips(t *testing.T) {
	repo := &resolveRepo{pairs: []demanddomain.StagingPair{{ItemCode: "A"}}}
	svc := demandapp.NewService(repo, nil, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Empty(t, repo.applied)
}

func TestResolveStaging_NoUnresolvedPairsIsNoop(t *testing.T) {
	repo := &resolveRepo{}
	resolver := &fakeResolver{}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res.Pairs)
	assert.Zero(t, resolver.calls)
	assert.Empty(t, repo.applied)
}

func TestResolveStaging_DegradedFinanceSkipsWithoutError(t *testing.T) {
	repo := &resolveRepo{pairs: []demanddomain.StagingPair{{ItemCode: "A"}}}
	resolver := &fakeResolver{err: demanddomain.ErrResolverDegraded}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Zero(t, res.RowsUpdated)
	assert.Empty(t, repo.applied)
}

// A resolver that answers a non-empty request with zero resolutions must not
// be mistaken for a completed pass: nothing is written, no outcome is tallied,
// and the rows stay UNRESOLVED for the next attempt. An unmatched pair comes
// back as NOT_FOUND, so an empty answer means the call itself did no work.
func TestResolveStaging_EmptyResolutionsWriteNothing(t *testing.T) {
	repo := &resolveRepo{pairs: []demanddomain.StagingPair{
		{ItemCode: "A", ShadeCode: "S1"},
		{ItemCode: "B", ShadeCode: "S2"},
	}}
	resolver := &fakeResolver{empty: true}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, res.Pairs)
	assert.Zero(t, res.RowsUpdated)
	assert.Zero(t, res.Auto)
	assert.Zero(t, res.Ambiguous)
	assert.Zero(t, res.NotFound, "an empty answer is not a not-found outcome")
	assert.Empty(t, repo.applied, "nothing may be written from an empty answer")
}

func TestResolveStaging_ResolverErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	repo := &resolveRepo{pairs: []demanddomain.StagingPair{{ItemCode: "A"}}}
	resolver := &fakeResolver{err: boom}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	_, err := svc.ResolveStaging(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestResolveStaging_RepoPairsErrorPropagates(t *testing.T) {
	boom := errors.New("select failed")
	repo := &resolveRepo{pairsErr: boom}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(&fakeResolver{}, nil)

	_, err := svc.ResolveStaging(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestResolveStaging_BatchesAtFiveHundredPairs(t *testing.T) {
	pairs := make([]demanddomain.StagingPair, 1100)
	for i := range pairs {
		pairs[i] = demanddomain.StagingPair{ItemCode: "ITEM"}
	}
	repo := &resolveRepo{pairs: pairs}
	resolver := &fakeResolver{}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	res, err := svc.ResolveStaging(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 3, resolver.calls) // 500 + 500 + 100
	assert.Len(t, resolver.seen, 1100)
	assert.Equal(t, int64(1100), res.RowsUpdated)
	require.Len(t, repo.applied, 3)
	assert.Len(t, repo.applied[0], 500)
	assert.Len(t, repo.applied[2], 100)
}

func TestListStaging_UnresolvedPageTriggersLazyResolveAndReread(t *testing.T) {
	repo := &resolveRepo{
		pairs: []demanddomain.StagingPair{{ItemCode: "AUTO1", ShadeCode: "S1"}},
		stagingPages: [][]*demanddomain.SalesOrderStaging{
			{{SosID: 1, ItemCode: "AUTO1", ShadeCode: "S1", MatchStatus: demanddomain.MatchStatusUnresolved}},
			{{SosID: 1, ItemCode: "AUTO1", ShadeCode: "S1", MatchStatus: demanddomain.MatchStatusAuto, CpmProductSysID: sysID(101)}},
		},
	}
	resolver := &fakeResolver{byItem: map[string]demanddomain.ProductResolution{
		"AUTO1": {MatchCount: 1, CpmProductSysID: sysID(101)},
	}}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	out, err := svc.ListStaging(context.Background(), demandapp.StagingListQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)

	assert.Equal(t, 2, repo.stagingCalls, "page is re-read after a successful resolution")
	require.Len(t, out.Items, 1)
	assert.Equal(t, demanddomain.MatchStatusAuto, out.Items[0].MatchStatus)
	require.NotNil(t, out.Items[0].CpmProductSysID)
	assert.Equal(t, int64(101), *out.Items[0].CpmProductSysID)
}

func TestListStaging_ResolvedPageSkipsLazyPass(t *testing.T) {
	repo := &resolveRepo{
		stagingPages: [][]*demanddomain.SalesOrderStaging{
			{{SosID: 1, MatchStatus: demanddomain.MatchStatusAuto, CpmProductSysID: sysID(9)}},
		},
	}
	resolver := &fakeResolver{}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(resolver, nil)

	_, err := svc.ListStaging(context.Background(), demandapp.StagingListQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)

	assert.Equal(t, 1, repo.stagingCalls)
	assert.Zero(t, resolver.calls)
}

func TestListStaging_LazyResolveFailureStillReturnsPage(t *testing.T) {
	repo := &resolveRepo{
		pairsErr: errors.New("finance timeout"),
		stagingPages: [][]*demanddomain.SalesOrderStaging{
			{{SosID: 1, MatchStatus: demanddomain.MatchStatusUnresolved}},
		},
	}
	svc := demandapp.NewService(repo, nil, nil).WithProductResolution(&fakeResolver{}, nil)

	out, err := svc.ListStaging(context.Background(), demandapp.StagingListQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	assert.Equal(t, 1, repo.stagingCalls, "no re-read when the lazy pass failed")
}

// The "select all matching" path must hand back the untruncated match count
// alongside the capped id set, so the UI can be honest about the difference.
func TestListStagingIDs_CapsAtBatchLimitAndReportsTotal(t *testing.T) {
	all := make([]int64, 651)
	for i := range all {
		all[i] = int64(i + 1)
	}
	repo := &resolveRepo{stagingIDs: all, stagingIDTotal: int64(len(all))}
	svc := demandapp.NewService(repo, nil, nil)

	out, err := svc.ListStagingIDs(context.Background(), demandapp.StagingIDsQuery{
		Search:       "PT",
		UnpulledOnly: true,
	})
	require.NoError(t, err)

	assert.Len(t, out.SosIDs, demanddomain.MaxSelectAllStagingIDs)
	assert.Equal(t, int64(651), out.TotalMatched)
	assert.Equal(t, demanddomain.MaxSelectAllStagingIDs, out.Limit)
	assert.Equal(t, demanddomain.MaxSelectAllStagingIDs, repo.idsLimit)
	assert.Equal(t, demanddomain.StagingIDsFilter{Search: "PT", UnpulledOnly: true}, repo.idsFilter,
		"the filter must reach the repository unchanged, or the count and the selection diverge")
}

func TestListStagingIDs_SmallSetIsNotTruncated(t *testing.T) {
	repo := &resolveRepo{stagingIDs: []int64{7, 8, 9}, stagingIDTotal: 3}
	svc := demandapp.NewService(repo, nil, nil)

	out, err := svc.ListStagingIDs(context.Background(), demandapp.StagingIDsQuery{})
	require.NoError(t, err)

	assert.Equal(t, []int64{7, 8, 9}, out.SosIDs)
	assert.Equal(t, int64(3), out.TotalMatched)
}
