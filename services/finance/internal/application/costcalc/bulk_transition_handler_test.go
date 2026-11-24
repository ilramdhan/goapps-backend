package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// bulkRepo fakes the per-row rules: rows in mb are MB, rows in locked sit in a
// locked ACTUAL period, rows in wrongStatus fail the transition, and ListResults
// pages through listed.
type bulkRepo struct {
	stubResultRepo
	mb, locked, wrongStatus map[int64]bool
	listed                  []*costcalcdom.ResultSummary
	filters                 []costcalcdom.ResultListFilter
}

func (r *bulkRepo) IsMBCostRow(_ context.Context, id int64) (bool, error) { return r.mb[id], nil }

func (r *bulkRepo) IsLocked(_ context.Context, period, _ string) (bool, error) {
	return period == "LOCKED", nil
}

func (r *bulkRepo) GetByID(_ context.Context, id int64) (*costcalcdom.Result, error) {
	period := "202608"
	if r.locked[id] {
		period = "LOCKED"
	}
	return lockTestRow(period, costcalcdom.CalcTypeActual), nil
}

func (r *bulkRepo) MarkVerified(ctx context.Context, id int64, by string) error {
	if r.wrongStatus[id] {
		return costcalcdom.ErrCostInvalidStatus
	}
	return r.stubResultRepo.MarkVerified(ctx, id, by)
}

func (r *bulkRepo) ListResults(_ context.Context, f costcalcdom.ResultListFilter) ([]*costcalcdom.ResultSummary, int, string, error) {
	r.filters = append(r.filters, f)
	start := (f.Page - 1) * f.PageSize
	end := min(start+f.PageSize, len(r.listed))
	if start >= len(r.listed) {
		return nil, len(r.listed), f.Period, nil
	}
	return r.listed[start:end], len(r.listed), f.Period, nil
}

func newBulkVerify(repo *bulkRepo) *BulkTransitionHandler {
	svc := NewService(nil, nil, nil, repo, nil, nil, nil, nil, nil)
	return NewBulkVerifyHandler(svc, NewVerifyCostHandler(svc, WithVerifyMBGuard(repo), WithVerifyPeriodLock(repo)))
}

func TestBulkVerify_MixedRows_ContinuesPastFailures(t *testing.T) {
	t.Parallel()
	repo := &bulkRepo{
		mb: map[int64]bool{2: true}, locked: map[int64]bool{3: true}, wrongStatus: map[int64]bool{4: true},
	}
	out, err := newBulkVerify(repo).Handle(context.Background(), BulkSelection{CostIDs: []int64{1, 2, 3, 4, 5, 1}}, "u")
	require.NoError(t, err)
	assert.Equal(t, 2, out.OK)
	assert.Equal(t, []int64{1, 5}, repo.verified, "duplicate id 1 processed once")
	require.Len(t, out.Failures, 3)
	assert.ErrorIs(t, out.Failures[0].Err, ErrMBCostNotManuallyTransitionable)
	assert.ErrorIs(t, out.Failures[1].Err, costcalcdom.ErrPeriodLocked)
	assert.ErrorIs(t, out.Failures[2].Err, costcalcdom.ErrCostInvalidStatus)
}

func TestBulkVerify_FilterMode_PagesWithSourceStatus(t *testing.T) {
	t.Parallel()
	repo := &bulkRepo{}
	for i := int64(1); i <= 1200; i++ {
		repo.listed = append(repo.listed, &costcalcdom.ResultSummary{CostID: i, ProductCode: "P"})
	}
	repo.wrongStatus = map[int64]bool{7: true}
	out, err := newBulkVerify(repo).Handle(context.Background(),
		BulkSelection{Period: "202608", CalcType: costcalcdom.CalcTypeActual, Search: "x"}, "u")
	require.NoError(t, err)
	assert.Equal(t, 1199, out.OK)
	require.Len(t, out.Failures, 1)
	assert.Equal(t, "P", out.Failures[0].ProductCode)
	require.Len(t, repo.filters, 3)
	assert.Equal(t, "CALCULATED", repo.filters[0].Status)
	assert.Equal(t, "x", repo.filters[0].Search)
}

func TestBulkVerify_RejectsBadSelections(t *testing.T) {
	t.Parallel()
	big := &bulkRepo{}
	for i := int64(1); i <= BulkTransitionMaxRows+1; i++ {
		big.listed = append(big.listed, &costcalcdom.ResultSummary{CostID: i})
	}
	tooMany := make([]int64, BulkTransitionMaxRows+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	cases := []struct {
		name  string
		repo  *bulkRepo
		sel   BulkSelection
		actor string
		want  error
	}{
		{"no period", &bulkRepo{}, BulkSelection{}, "u", ErrBulkPeriodRequired},
		{"ids over cap", &bulkRepo{}, BulkSelection{CostIDs: tooMany}, "u", ErrBulkSelectionTooLarge},
		{"filter over cap", big, BulkSelection{Period: "202608"}, "u", ErrBulkSelectionTooLarge},
		{"no actor", &bulkRepo{}, BulkSelection{CostIDs: []int64{1}}, "", nil},
	}
	for _, tc := range cases {
		_, err := newBulkVerify(tc.repo).Handle(context.Background(), tc.sel, tc.actor)
		require.Error(t, err, tc.name)
		if tc.want != nil {
			assert.True(t, errors.Is(err, tc.want), tc.name)
		}
		assert.Empty(t, tc.repo.verified, tc.name)
	}
}

func TestBulkApprove_UsesVerifiedSourceStatus(t *testing.T) {
	t.Parallel()
	repo := &bulkRepo{listed: []*costcalcdom.ResultSummary{{CostID: 9}}}
	svc := NewService(nil, nil, nil, repo, nil, nil, nil, nil, nil)
	out, err := NewBulkApproveHandler(svc, NewApproveCostHandler(svc)).Handle(context.Background(), BulkSelection{Period: "202608"}, "u")
	require.NoError(t, err)
	assert.Equal(t, 1, out.OK)
	assert.Equal(t, "VERIFIED", repo.filters[0].Status)
	assert.Equal(t, []int64{9}, repo.approved)
}

func TestBulkVerify_CanceledContext_MarksRemainingFailed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &bulkRepo{}
	out, err := newBulkVerify(repo).Handle(ctx, BulkSelection{CostIDs: []int64{1, 2}}, "u")
	require.NoError(t, err)
	assert.Zero(t, out.OK)
	require.Len(t, out.Failures, 2)
	assert.ErrorIs(t, out.Failures[0].Err, context.Canceled)
}
