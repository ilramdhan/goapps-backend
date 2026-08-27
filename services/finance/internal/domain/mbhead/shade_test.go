package mbhead_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func newTestHead(t *testing.T) *mbhead.Entity {
	t.Helper()
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MBH-2024-001", CreatedBy: "admin"})
	require.NoError(t, err)
	return e
}

func TestSetAdditionalShades_ThreeShades_ReturnsErrTooManyShades(t *testing.T) {
	e := newTestHead(t)
	err := e.SetAdditionalShades([]mbhead.Shade{
		{SeqNo: 1, Code: "SH1", Name: "Red"},
		{SeqNo: 2, Code: "SH2", Name: "Blue"},
		{SeqNo: 3, Code: "SH3", Name: "Green"},
	})
	require.ErrorIs(t, err, mbhead.ErrTooManyShades)
	assert.Nil(t, e.AdditionalShades())
}

func TestSetAdditionalShades_DuplicateSeqNo_Rejected(t *testing.T) {
	e := newTestHead(t)
	err := e.SetAdditionalShades([]mbhead.Shade{
		{SeqNo: 1, Code: "SH1"},
		{SeqNo: 1, Code: "SH2"},
	})
	require.ErrorIs(t, err, mbhead.ErrTooManyShades)
	assert.Nil(t, e.AdditionalShades())
}

func TestSetAdditionalShades_SeqNoOutOfRange_Rejected(t *testing.T) {
	// Migration 000483 constrains mbhs_seq_no to IN (1,2) — seq 3 and seq 0 are
	// impossible in storage, so the domain must reject them first.
	for _, seq := range []int32{0, 3, -1} {
		e := newTestHead(t)
		err := e.SetAdditionalShades([]mbhead.Shade{{SeqNo: seq, Code: "SH1"}})
		require.ErrorIsf(t, err, mbhead.ErrTooManyShades, "seq %d must be rejected", seq)
	}
}

func TestSetAdditionalShades_EmptyCode_Rejected(t *testing.T) {
	// mbhs_shade_code is NOT NULL in 000483.
	e := newTestHead(t)
	err := e.SetAdditionalShades([]mbhead.Shade{{SeqNo: 1, Code: "", Name: "Red"}})
	require.ErrorIs(t, err, mbhead.ErrRecipeFieldRequired)
	assert.Nil(t, e.AdditionalShades())
}

func TestSetAdditionalShades_EmptyName_Accepted(t *testing.T) {
	// mbhs_shade_name is NULLABLE in 000483 — a blank name is legitimate.
	e := newTestHead(t)
	require.NoError(t, e.SetAdditionalShades([]mbhead.Shade{{SeqNo: 1, Code: "SH1", Name: ""}}))
	got := e.AdditionalShades()
	require.Len(t, got, 1)
	assert.Equal(t, "SH1", got[0].Code)
	assert.Empty(t, got[0].Name)
}

func TestSetAdditionalShades_TwoShades_Accepted(t *testing.T) {
	e := newTestHead(t)
	require.NoError(t, e.SetAdditionalShades([]mbhead.Shade{
		{SeqNo: 1, Code: "SH1", Name: "Red"},
		{SeqNo: 2, Code: "SH2", Name: "Blue"},
	}))
	assert.Len(t, e.AdditionalShades(), 2)
}

func TestSetAdditionalShades_Empty_ClearsAndSucceeds(t *testing.T) {
	// D13: a legacy payload without additional_shades must keep working.
	e := newTestHead(t)
	require.NoError(t, e.SetAdditionalShades([]mbhead.Shade{{SeqNo: 1, Code: "SH1"}}))
	require.NoError(t, e.SetAdditionalShades(nil))
	assert.Nil(t, e.AdditionalShades())
}
