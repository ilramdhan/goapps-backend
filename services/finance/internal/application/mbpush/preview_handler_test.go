package mbpush

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeMBHeadReader returns a fixed candidate set.
type fakeMBHeadReader struct {
	candidates []MBHeadCandidate
	err        error
}

func (f *fakeMBHeadReader) ListValidated(context.Context) ([]MBHeadCandidate, error) {
	return f.candidates, f.err
}

// fakeCostReader reports which (productSysID, costType) tuples have an active CALCULATED row.
// Only the Preview path (GetActiveCalculated) is exercised; the Tx methods are unused here.
type fakeCostReader struct {
	// missing lists the cost types with NO calculated row, keyed by product sys id.
	missing map[int64]map[string]bool
}

func (f *fakeCostReader) GetActiveCalculated(_ context.Context, productSysID int64, _, calcType string) (int64, string, bool, error) {
	if f.missing[productSysID][calcType] {
		return 0, "", false, nil
	}
	return productSysID, "1.000000", true, nil
}

func (f *fakeCostReader) GetActiveCalculatedTx(context.Context, *sql.Tx, int64, string, string) (int64, string, bool, error) {
	return 0, "", false, nil
}

func (f *fakeCostReader) MarkApprovedFromCalculatedTx(context.Context, *sql.Tx, int64, string) error {
	return nil
}

// fakeStalePushReader returns a fixed stale-push id set and records the period it was asked for.
type fakeStalePushReader struct {
	ids       []string
	err       error
	gotPeriod string
	callCount int
}

func (f *fakeStalePushReader) ListStalePushedMBHIDs(_ context.Context, period string) ([]string, error) {
	f.callCount++
	f.gotPeriod = period
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

const testPeriod = "202608"

func newPreviewHandler(candidates []MBHeadCandidate, missing map[int64]map[string]bool, stale StalePushReader) *PreviewHandler {
	return NewPreviewHandler(
		&fakeMBHeadReader{candidates: candidates},
		&fakeCostReader{missing: missing},
		stale,
	)
}

// The core of this feature: a head that is fully calculated AND in the stale-push set stays in
// the pushable bucket (re-pushing is the remedy) but carries the NeedsRepush label, and the
// count reflects it.
func TestPreview_StalePushedHeadStaysPushableAndIsFlagged(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{
			{MBHID: "mb-stale", Code: "MB-1", Name: "Stale", CostProductID: 101},
			{MBHID: "mb-fresh", Code: "MB-2", Name: "Fresh", CostProductID: 102},
		},
		nil,
		&fakeStalePushReader{ids: []string{"mb-stale"}},
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)

	require.Len(t, result.Pushable, 2, "a stale push must not be demoted out of the pushable bucket")
	require.Empty(t, result.Skipped)
	require.Equal(t, int32(1), result.NeedsRepushCount)

	byID := map[string]PushableMBHead{}
	for _, p := range result.Pushable {
		byID[p.MBHID] = p
	}
	require.True(t, byID["mb-stale"].NeedsRepush)
	require.False(t, byID["mb-fresh"].NeedsRepush)
}

// A stale-push row for a head that cannot be pushed at all (missing cost types) must not leak
// into the count — the count only ever describes heads present in the pushable bucket.
func TestPreview_StaleFlagNotCountedForSkippedHead(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{{MBHID: "mb-stale", Code: "MB-1", Name: "Stale", CostProductID: 101}},
		map[int64]map[string]bool{101: {"SELLING": true}},
		&fakeStalePushReader{ids: []string{"mb-stale"}},
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)

	require.Empty(t, result.Pushable)
	require.Len(t, result.Skipped, 1)
	require.Equal(t, skipReasonMissingSelling, result.Skipped[0].Reason)
	require.Zero(t, result.NeedsRepushCount)
}

// A head with no linked cost product is skipped before any cost lookup, and the stale set —
// which is keyed by mbh_id and could still contain it — must not flag it.
func TestPreview_NoCostProductSkippedRegardlessOfStaleSet(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{{MBHID: "mb-nolink", Code: "MB-3", Name: "No link", CostProductID: 0}},
		nil,
		&fakeStalePushReader{ids: []string{"mb-nolink"}},
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)

	require.Empty(t, result.Pushable)
	require.Len(t, result.Skipped, 1)
	require.Equal(t, skipReasonNoCostProduct, result.Skipped[0].Reason)
	require.Zero(t, result.NeedsRepushCount)
}

// With nothing stale, the preview must behave exactly as it did before this feature.
func TestPreview_NoStaleRowsLeavesEveryHeadUnflagged(t *testing.T) {
	stale := &fakeStalePushReader{}
	h := newPreviewHandler(
		[]MBHeadCandidate{
			{MBHID: "mb-a", Code: "MB-A", Name: "A", CostProductID: 1},
			{MBHID: "mb-b", Code: "MB-B", Name: "B", CostProductID: 2},
		},
		nil,
		stale,
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)

	require.Len(t, result.Pushable, 2)
	require.Zero(t, result.NeedsRepushCount)
	for _, p := range result.Pushable {
		require.False(t, p.NeedsRepush)
	}
	require.Equal(t, testPeriod, stale.gotPeriod, "the stale lookup must be scoped to the previewed period")
	require.Equal(t, 1, stale.callCount, "the stale set is resolved once per preview, not per candidate")
}

// The stale lookup is informational, but it is still part of the response contract: a failure
// surfaces as an error rather than silently under-reporting.
func TestPreview_StaleReaderErrorPropagates(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{{MBHID: "mb-a", Code: "MB-A", Name: "A", CostProductID: 1}},
		nil,
		&fakeStalePushReader{err: errors.New("boom")},
	)

	_, err := h.Preview(context.Background(), testPeriod)
	require.Error(t, err)
	require.Contains(t, err.Error(), "list stale pushed mb heads")
}

// A nil reader must degrade to "never flagged" rather than panicking, so an incompletely wired
// preview still returns its pushable/skipped buckets.
func TestPreview_NilStaleReaderDegradesToUnflagged(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{{MBHID: "mb-a", Code: "MB-A", Name: "A", CostProductID: 1}},
		nil,
		nil,
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)
	require.Len(t, result.Pushable, 1)
	require.False(t, result.Pushable[0].NeedsRepush)
	require.Zero(t, result.NeedsRepushCount)
}

// Every missing cost type must still be reported, unchanged by the new classification.
func TestPreview_AllCostTypesMissingReportsEveryReason(t *testing.T) {
	h := newPreviewHandler(
		[]MBHeadCandidate{{MBHID: "mb-a", Code: "MB-A", Name: "A", CostProductID: 1}},
		map[int64]map[string]bool{1: {"ACTUAL": true, "SELLING": true, "FORECAST": true}},
		&fakeStalePushReader{},
	)

	result, err := h.Preview(context.Background(), testPeriod)
	require.NoError(t, err)
	require.Len(t, result.Skipped, 1)
	require.Contains(t, result.Skipped[0].Reason, skipReasonMissingActual)
	require.Contains(t, result.Skipped[0].Reason, skipReasonMissingSelling)
	require.Contains(t, result.Skipped[0].Reason, skipReasonMissingForecast)
}
