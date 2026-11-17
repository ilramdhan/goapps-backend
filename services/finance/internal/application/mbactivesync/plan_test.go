package mbactivesync

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReader struct{ snap *Snapshot }

func (f fakeReader) Load(context.Context) (*Snapshot, error) { return f.snap, nil }

func samplePlan(t *testing.T) *Plan {
	t.Helper()
	hid, sid := uuid.New(), uuid.New()
	den, ldr := 150.0, 1.5
	fil := 48
	snap := &Snapshot{
		Heads: []HeadSnap{{ID: hid, SysID: "S1", MBCosting: "MB A", IsActive: true}},
		Spins: []SpinSnap{{ID: sid, HeadID: hid, IsActive: true, Denier: &den, Filament: &fil, ShadeCode: "R1", RunLDRPct: &ldr}},
		POYs:  []POYSnap{{SysID: 7, ProductCode: "P7", ShadeCode: "R1", IsActive: true}},
	}
	rows := []FileRow{
		{Ln: 1, SysID: "S1", POY: "P7", POYName: "POY 150/48/TBL", Shade: "R1", Doz: 1.5, DozOK: true, DozRaw: "1,5", Den: &den, Fil: &fil, CS: "TBL"},
		{Ln: 2, SysID: "S9", POY: "PX", POYName: "POY 1/1/X", Shade: "Z"},
	}
	pl, err := NewPlanner(fakeReader{snap}).Build(context.Background(), rows)
	require.NoError(t, err)
	return pl
}

func TestCSV_RoundTripAndHeader(t *testing.T) {
	pl := samplePlan(t)
	require.Len(t, pl.Rows, 2)

	var rb, hb bytes.Buffer
	require.NoError(t, WriteRowsCSV(&rb, pl.Rows))
	require.NoError(t, WriteHeadsCSV(&hb, pl.Heads))

	rc, err := ReadPlanCSV(bytes.NewReader(rb.Bytes()), RowsRequiredColumns)
	require.NoError(t, err)
	require.Len(t, rc.Records, 2)
	assert.Equal(t, "1", rc.Get(0, "ln"))
	assert.Equal(t, "S1", rc.Get(0, "sysid"))
	assert.Equal(t, "row:1", rc.Get(0, "item_key"))
	assert.Equal(t, string(pl.Rows[0].Action), rc.Get(0, "action"))
	assert.Equal(t, string(pl.Rows[0].Decision), rc.Get(0, "decision"))
	assert.Equal(t, string(ActionNewRecipe), rc.Get(1, "action"))
	assert.Equal(t, string(DecisionSkip), rc.Get(1, "decision"))
	assert.Len(t, RowRecord(pl.Rows[0]), len(RowsHeader))

	hc, err := ReadPlanCSV(bytes.NewReader(hb.Bytes()), HeadsRequiredColumns)
	require.NoError(t, err)
	assert.Len(t, hc.Records, len(pl.Heads.Actions))
}

func TestReadPlanCSV_HeaderValidation(t *testing.T) {
	_, err := ReadPlanCSV(strings.NewReader(""), RowsRequiredColumns)
	require.Error(t, err)

	_, err = ReadPlanCSV(strings.NewReader("ln,sysid\n1,S\n"), RowsRequiredColumns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decision")

	// BOM + case/space tolerant header
	_, err = ReadPlanCSV(strings.NewReader("\uFEFFItem_Key, Kind ,old_head_id,old_spin_id,new_head_id,new_spin_ref,phase,decision\n"), HeadsRequiredColumns)
	require.NoError(t, err)

	// ragged record
	_, err = ReadPlanCSV(strings.NewReader("item_key,kind,old_head_id,old_spin_id,new_head_id,new_spin_ref,phase,decision\nx,y\n"), HeadsRequiredColumns)
	require.Error(t, err)
}

func TestWriteSummary_ExpectedVsActual(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, WriteSummary(&b, samplePlan(t)))
	s := b.String()
	assert.Contains(t, s, "KEEP")
	assert.Contains(t, s, "TOTAL")
	assert.Contains(t, s, "DEACTIVATE_HEAD")
	assert.Contains(t, s, "SUPERSEDE_HEAD")
	assert.Contains(t, s, "Supersede pairs:")
	assert.NotContains(t, s, "no_live_spin")
	assert.Contains(t, s, "Rows by decision")
}

func TestBuild_NoReader(t *testing.T) {
	_, err := NewPlanner(nil).Build(context.Background(), nil)
	assert.Error(t, err)
}

func TestSpinCols_SpinTakenLeavesSpinEmpty(t *testing.T) {
	other := &SpinSnap{ID: uuid.New(), ShadeCode: "X1"}
	p := &RowPlan{Alloc: &AllocResult{Spin: other, Lvl: 2, Rank: 2}}
	cols := spinCols(p)
	require.Len(t, cols, 10)
	for i := 0; i < 6; i++ {
		assert.Empty(t, cols[i], "spin_* col %d must be empty for spin_taken", i)
	}
	assert.Equal(t, "2", cols[6])
	assert.Equal(t, "2", cols[7])

	p.Spin = other
	assert.Equal(t, other.ID.String(), spinCols(p)[0])
}
