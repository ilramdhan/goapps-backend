package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestMBHeadEntityToProto_RecipeFieldsAbsent covers the legacy shape: a head with
// no VS Number, no No-of-Process and no shades must map to ABSENT proto fields —
// ⛔ never "" or 0 (D13).
func TestMBHeadEntityToProto_RecipeFieldsAbsent(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB001", CreatedBy: "admin"})
	require.NoError(t, err)

	p := mbHeadEntityToProto(e)

	assert.Nil(t, p.MbhVsNumber)
	assert.Nil(t, p.MbhNoOfProcess)
	assert.Nil(t, p.AdditionalShades)
	assert.Nil(t, p.MbhUnlockRequestedAt)
	assert.Nil(t, p.MbhUnlockRequestedBy)
	// A NULL mbh_unlock_reason must surface as an ABSENT proto field, ⛔ never "".
	assert.Nil(t, p.MbhUnlockReason)
	// mbh_is_locked reads NULL as false via the repository's COALESCE(...,FALSE)
	// (J12); the mapper forwards that as an explicit false rather than dropping it.
	require.NotNil(t, p.MbhIsLocked)
	assert.False(t, *p.MbhIsLocked)
}

// TestMBHeadEntityToProto_RecipeFieldsPresent covers MBHead fields 38-43 and 45.
func TestMBHeadEntityToProto_RecipeFieldsPresent(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB001", CreatedBy: "admin"})
	require.NoError(t, err)

	vs, nop, by := "VS-42", "T", "supervisor"
	reason := "salah dozing, perlu revisi resep"
	at := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	e.HydrateExtras(mbhead.PersistedExtras{
		VSNumber:    &vs,
		NoOfProcess: &nop,
		// An empty shade NAME is legitimate — mbhs_shade_name is nullable.
		AdditionalShades:  []mbhead.Shade{{SeqNo: 1, Code: "SH-A", Name: "Alpha"}, {SeqNo: 2, Code: "SH-B"}},
		IsLocked:          true,
		UnlockRequestedAt: &at,
		UnlockRequestedBy: &by,
		UnlockReason:      &reason,
	})

	p := mbHeadEntityToProto(e)

	require.NotNil(t, p.MbhVsNumber)
	assert.Equal(t, "VS-42", *p.MbhVsNumber)
	require.NotNil(t, p.MbhNoOfProcess)
	assert.Equal(t, "T", *p.MbhNoOfProcess)
	require.NotNil(t, p.MbhIsLocked)
	assert.True(t, *p.MbhIsLocked)
	require.NotNil(t, p.MbhUnlockRequestedAt)
	assert.Equal(t, at.Format(time.RFC3339), *p.MbhUnlockRequestedAt)
	require.NotNil(t, p.MbhUnlockRequestedBy)
	assert.Equal(t, "supervisor", *p.MbhUnlockRequestedBy)
	require.NotNil(t, p.MbhUnlockReason)
	assert.Equal(t, reason, *p.MbhUnlockReason)

	require.Len(t, p.AdditionalShades, 2)
	assert.Equal(t, int32(1), p.AdditionalShades[0].MbhsSeqNo)
	assert.Equal(t, "SH-A", p.AdditionalShades[0].MbhsShadeCode)
	assert.Equal(t, "Alpha", p.AdditionalShades[0].MbhsShadeName)
	assert.Equal(t, "SH-B", p.AdditionalShades[1].MbhsShadeCode)
	assert.Empty(t, p.AdditionalShades[1].MbhsShadeName)
}

// TestShadeInputsToDomain pins the absent-vs-empty distinction the update path
// relies on to avoid wiping stored shades.
func TestShadeInputsToDomain(t *testing.T) {
	assert.Nil(t, shadeInputsToDomain(nil))
	assert.Nil(t, shadeInputsToDomain([]*financev1.MBHeadShadeInput{}))

	alpha := "Alpha"
	got := shadeInputsToDomain([]*financev1.MBHeadShadeInput{
		{MbhsSeqNo: 1, MbhsShadeCode: "SH-A", MbhsShadeName: &alpha},
		nil, // a nil element must be skipped, not panic
		{MbhsSeqNo: 2, MbhsShadeCode: "SH-B"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, mbhead.Shade{SeqNo: 1, Code: "SH-A", Name: "Alpha"}, got[0])
	assert.Equal(t, mbhead.Shade{SeqNo: 2, Code: "SH-B"}, got[1])
}

// TestMBHeadEntityToProto_UnlockReasonSurvivesUnlock pins principle U-2 at the
// proto boundary: mbh_unlock_reason is ⛔ NOT cleared when the head is no longer
// locked, so a granted/rejected head still exposes WHY the unlock was asked for.
// An already-unlocked head carrying a reason is the audit trail, ⛔ not stale data.
func TestMBHeadEntityToProto_UnlockReasonSurvivesUnlock(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB001", CreatedBy: "admin"})
	require.NoError(t, err)

	reason := "granted last week, trail must stay"
	e.HydrateExtras(mbhead.PersistedExtras{
		IsLocked:     false,
		UnlockReason: &reason,
	})

	p := mbHeadEntityToProto(e)

	require.NotNil(t, p.MbhIsLocked)
	assert.False(t, *p.MbhIsLocked)
	// The request metadata is gone, but the reason is deliberately kept.
	assert.Nil(t, p.MbhUnlockRequestedAt)
	assert.Nil(t, p.MbhUnlockRequestedBy)
	require.NotNil(t, p.MbhUnlockReason)
	assert.Equal(t, reason, *p.MbhUnlockReason)
}
