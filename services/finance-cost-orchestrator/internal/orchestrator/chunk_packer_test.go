package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/pkg/costcalc"
)

func TestPackChunks_EmptyPlan(t *testing.T) {
	out := PackChunks(&costcalc.WavePlan{}, 50, 100)
	require.Empty(t, out)
}

func TestPackChunks_NilPlan(t *testing.T) {
	require.Nil(t, PackChunks(nil, 50, 100))
}

func TestPackChunks_SingleWaveExactFit(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{{Number: 0, Products: makeIDs(1, 50)}}}
	out := PackChunks(plan, 50, 100)
	require.Len(t, out, 1)
	require.Len(t, out[0].Chunks, 1)
	require.Equal(t, 50, len(out[0].Chunks[0].ProductIDs))
	require.Equal(t, 1, out[0].Chunks[0].ChunkNumber)
	require.Equal(t, 0, out[0].Chunks[0].WaveNo)
}

func TestPackChunks_SingleWaveSplit(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{{Number: 0, Products: makeIDs(1, 123)}}}
	out := PackChunks(plan, 50, 100)
	require.Len(t, out, 1)
	require.Len(t, out[0].Chunks, 3) // 50, 50, 23
	require.Equal(t, 50, len(out[0].Chunks[0].ProductIDs))
	require.Equal(t, 50, len(out[0].Chunks[1].ProductIDs))
	require.Equal(t, 23, len(out[0].Chunks[2].ProductIDs))
	// Chunk numbers sequential globally.
	require.Equal(t, []int{1, 2, 3}, []int{out[0].Chunks[0].ChunkNumber, out[0].Chunks[1].ChunkNumber, out[0].Chunks[2].ChunkNumber})
}

func TestPackChunks_MultipleWavesContinuousNumbering(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{
		{Number: 0, Products: makeIDs(1, 75)},   // 2 chunks
		{Number: 1, Products: makeIDs(100, 30)}, // 1 chunk
	}}
	out := PackChunks(plan, 50, 100)
	require.Len(t, out, 2)
	require.Equal(t, 1, out[0].Chunks[0].ChunkNumber)
	require.Equal(t, 2, out[0].Chunks[1].ChunkNumber)
	require.Equal(t, 3, out[1].Chunks[0].ChunkNumber)
}

func TestPackChunks_CapsAtMaxChunkSize(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{{Number: 0, Products: makeIDs(1, 250)}}}
	out := PackChunks(plan, 200, 100) // requested 200, capped to 100
	require.Len(t, out[0].Chunks, 3)  // 100, 100, 50
	require.Equal(t, 100, len(out[0].Chunks[0].ProductIDs))
	require.Equal(t, 100, len(out[0].Chunks[1].ProductIDs))
	require.Equal(t, 50, len(out[0].Chunks[2].ProductIDs))
}

func TestPackChunks_DefaultsWhenZero(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{{Number: 0, Products: makeIDs(1, 60)}}}
	out := PackChunks(plan, 0, 0)
	require.Len(t, out[0].Chunks, 2) // 50, 10
}

func TestFillJobContext(t *testing.T) {
	plan := &costcalc.WavePlan{Waves: []costcalc.Wave{{Number: 0, Products: makeIDs(1, 5)}}}
	waves := PackChunks(plan, 50, 100)
	FillJobContext(waves, 42, "JOB-202605-0001", "202605", "tester", costcalc.CalcTypeActual)
	require.Equal(t, int64(42), waves[0].Chunks[0].JobID)
	require.Equal(t, "JOB-202605-0001", waves[0].Chunks[0].JobCode)
	require.Equal(t, "202605", waves[0].Chunks[0].Period)
	require.Equal(t, "tester", waves[0].Chunks[0].Actor)
	require.Equal(t, costcalc.CalcTypeActual, waves[0].Chunks[0].CalcType)
}

func makeIDs(start int64, count int) []int64 {
	out := make([]int64, count)
	for i := range count {
		out[i] = start + int64(i)
	}
	return out
}
