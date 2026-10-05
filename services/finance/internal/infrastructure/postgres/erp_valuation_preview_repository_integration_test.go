// Integration test for the valuation preview + immutable ADJ snapshot
// repositories (plan-06 P5-T4). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func previewSnapshotRows() []erpintegration.AdjSnapshotRow {
	f1, f5 := "0.5", "FG"
	appr := int64(1)
	a := erpintegration.AdjSnapshotRow{
		HeadSysID: 10, ItemSysID: 101, TxnCode: erpintegration.TxnInvAdj, HeadApprStatus: &appr,
		ItemCode: "YRN001", GradeCode: "A", ShadeCode: "S1",
		QtyBu:    decimal.NewNullDecimal(decimal.RequireFromString("2500")),
		ItemDesc: "Yarn one",
		Rate:     decimal.NewNullDecimal(decimal.RequireFromString("1.25")),
		Val:      decimal.NewNullDecimal(decimal.RequireFromString("3.125")),
		NewRate:  decimal.NewNullDecimal(decimal.RequireFromString("2.5")),
		NewVal:   decimal.NewNullDecimal(decimal.RequireFromString("6.25")),
	}
	a.Flex[0], a.Flex[4] = &f1, &f5
	nf := [erpintegration.AdjFlexCount]string{"1", "", "", "CHP", "", "", "", "", "", "", "", "", "7", "SRC"}
	a.NewFlex = &nf
	b := erpintegration.AdjSnapshotRow{HeadSysID: 10, ItemSysID: 102, TxnCode: erpintegration.TxnMbInvAdj, ItemCode: "CMB002"}
	return []erpintegration.AdjSnapshotRow{b, a}
}

func newPreview(batchID int64) erpintegration.ValuationPreview {
	return erpintegration.ValuationPreview{
		BatchID: batchID, Operation: erpintegration.AdjOpValuate,
		HeadCount: 1, ItemCount: 2,
		Excluded:    erpintegration.AdjExclusions{PostedHeads: 1, PostedItems: 3},
		SetHash:     erpintegration.AdjSetHash(previewSnapshotRows()),
		Totals:      []byte(`{"current_val":"3.125"}`),
		ConfirmText: erpintegration.PreviewConfirmText("202608", batchID),
		CreatedBy:   "integ",
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}
}

func TestErpValuationPreview_CreateOpenRoundTripAndImmutable(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpValuationPreviewRepository(f.db)
	snaps := postgres.NewErpAdjSnapshotRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusPushed))

	p, err := repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, erpintegration.PreviewOpen, p.Status)

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, erpintegration.PreviewOpen, got.Status)
	assert.Equal(t, id, got.BatchID)
	assert.Equal(t, 1, got.HeadCount)
	assert.Equal(t, 2, got.ItemCount)
	assert.Equal(t, 1, got.Excluded.PostedHeads)
	assert.Equal(t, erpintegration.AdjSetHash(previewSnapshotRows()), got.SetHash)
	assert.Equal(t, erpintegration.PreviewConfirmText("202608", id), got.ConfirmText)
	assert.JSONEq(t, `{"current_val":"3.125"}`, string(got.Totals))
	assert.True(t, got.ExpiresAt.After(got.CreatedAt))

	recs, err := snaps.ListByPreview(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, int64(101), recs[0].Row.ItemSysID, "ordered by ADJI_SYS_ID")
	want := previewSnapshotRows()[1]
	r0 := recs[0].Row
	assert.Equal(t, want.RowHash(), recs[0].RowHash)
	assert.Equal(t, want.RowHash(), r0.RowHash(), "round trip preserves the hashed columns")
	assert.True(t, r0.QtyBu.Decimal.Equal(want.QtyBu.Decimal))
	assert.True(t, r0.NewVal.Decimal.Equal(want.NewVal.Decimal))
	require.NotNil(t, r0.Flex[0])
	assert.Equal(t, "0.5", *r0.Flex[0])
	assert.Nil(t, r0.Flex[1])
	require.NotNil(t, r0.NewFlex)
	assert.Equal(t, "SRC", r0.NewFlex[13])
	r1 := recs[1].Row
	assert.False(t, r1.QtyBu.Valid)
	assert.Nil(t, r1.NewFlex)
	assert.Nil(t, r1.HeadApprStatus)

	// trg_ceas_immutable: the snapshot can only be inserted.
	_, err = f.raw.ExecContext(ctx, `UPDATE cst_erp_adj_snapshot SET ceas_rate = 0 WHERE ceas_preview_id = $1::uuid`, p.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
	_, err = f.raw.ExecContext(ctx, `DELETE FROM cst_erp_adj_snapshot WHERE ceas_preview_id = $1::uuid`, p.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
}

func TestErpValuationPreview_SecondOpenMarksFirstStale(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpValuationPreviewRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusPushed))

	p1, err := repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.NoError(t, err)
	p2, err := repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.NoError(t, err)
	assert.NotEqual(t, p1.ID, p2.ID)

	g1, err := repo.Get(ctx, p1.ID)
	require.NoError(t, err)
	assert.Equal(t, erpintegration.PreviewStale, g1.Status)
	g2, err := repo.Get(ctx, p2.ID)
	require.NoError(t, err)
	assert.Equal(t, erpintegration.PreviewOpen, g2.Status)
}

func TestErpValuationPreview_RefusalsAndFailed(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpValuationPreviewRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusValidated))

	_, err := repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.ErrorIs(t, err, erpintegration.ErrStaleBatchStatus)
	var n int
	require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_erp_valuation_preview`).Scan(&n))
	assert.Zero(t, n, "refused create leaves no preview")

	_, err = repo.CreateOpen(ctx, newPreview(999999), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.ErrorIs(t, err, erpintegration.ErrBatchNotFound)

	_, err = repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusValidated, nil)
	require.Error(t, err, "empty snapshot refused")

	fp, err := repo.CreateFailed(ctx, newPreview(id))
	require.NoError(t, err)
	assert.Equal(t, erpintegration.PreviewFailed, fp.Status)
	recs, err := postgres.NewErpAdjSnapshotRepository(f.db).ListByPreview(ctx, fp.ID)
	require.NoError(t, err)
	assert.Empty(t, recs)

	_, err = repo.Get(ctx, "00000000-0000-0000-0000-000000000000")
	require.True(t, errors.Is(err, erpintegration.ErrPreviewNotFound))
	_, err = repo.Get(ctx, "not-a-uuid")
	require.True(t, errors.Is(err, erpintegration.ErrPreviewNotFound))
}

func TestErpValuationPreview_ConcurrentLockRefused(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpValuationPreviewRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusPushed))

	tx, err := f.raw.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // test cleanup
	got, err := postgres.TryLockErpBatch(ctx, tx, id)
	require.NoError(t, err)
	require.True(t, got)

	_, err = repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.ErrorIs(t, err, erpintegration.ErrConcurrentRun)
}
