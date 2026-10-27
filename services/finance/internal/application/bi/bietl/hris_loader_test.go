package bietl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestHRISWindow(t *testing.T) {
	tests := []struct {
		name, now, d1, d2 string
	}{
		// today−35 = 2026-09-10 (Thu); Monday 09-07; period start 08-26 (earlier) → 08-26.
		{"period start earlier than monday", "2026-10-15", "2026-08-26", "2026-10-14"},
		// today−35 = 2026-08-27 (Thu); Monday 08-24; period start 08-26 → Monday earlier.
		{"monday earlier than period start", "2026-10-01", "2026-08-24", "2026-09-30"},
		// today−35 = 2026-02-26 (Thu); Monday 02-23; period start 02-26 → Monday.
		{"d1 on day 26 across year edge", "2026-04-02", "2026-02-23", "2026-04-01"},
		// today−35 = 2026-01-05 (Mon); period start 2025-12-26 earlier.
		{"year boundary", "2026-02-09", "2025-12-26", "2026-02-08"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d1, d2 := HRISWindow(day(tc.now).Add(13 * time.Hour))
			assert.Equal(t, tc.d1, d1.Format("2006-01-02"))
			assert.Equal(t, tc.d2, d2.Format("2006-01-02"))
		})
	}
}

// H013: the 05:00 WIB run fires at 22:00 UTC the previous day and must still pull up to H-1 (WIB).
func TestHRISWindow_UsesWIBDate(t *testing.T) {
	fire := time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC) // = 2026-10-16 05:00 WIB
	_, d2 := HRISWindow(fire)
	assert.Equal(t, "2026-10-15", d2.Format("2006-01-02"))
}

type fakeOT struct{ d1, d2 time.Time }

func (f *fakeOT) FetchOTDaily(_ context.Context, d1, d2 time.Time) ([]oracleinfra.OTDailyRow, error) {
	f.d1, f.d2 = d1, d2
	return []oracleinfra.OTDailyRow{{NIK: "1"}, {NIK: "2"}}, nil
}

type fakeStg struct {
	d1, d2 time.Time
	src    uuid.UUID
}

func (f *fakeStg) ReplaceOTDaily(_ context.Context, d1, d2 time.Time, rows []oracleinfra.OTDailyRow, src uuid.UUID) (int, error) {
	f.d1, f.d2, f.src = d1, d2, src
	return len(rows), nil
}

func TestHRISLoader_Load(t *testing.T) {
	src, stg := &fakeOT{}, &fakeStg{}
	l := NewHRISLoader(src, stg)
	l.now = func() time.Time { return day("2026-10-15") }

	srcID := uuid.New()
	n, err := l.Load(context.Background(), TargetHROTStaging, "", srcID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, srcID, stg.src)
	assert.Equal(t, src.d1, stg.d1)
	assert.Equal(t, src.d2, stg.d2)

	_, err = l.Load(context.Background(), "NOPE", "", uuid.Nil)
	assert.Error(t, err)
}

type fakeATT struct{ d1, d2 time.Time }

func (f *fakeATT) FetchATTDaily(_ context.Context, d1, d2 time.Time) ([]oracleinfra.ATTDailyRow, error) {
	f.d1, f.d2 = d1, d2
	return []oracleinfra.ATTDailyRow{{NIK: "1"}}, nil
}

type fakeATTStg struct {
	d1, d2 time.Time
	src    uuid.UUID
}

func (f *fakeATTStg) ReplaceATTDaily(_ context.Context, d1, d2 time.Time, rows []oracleinfra.ATTDailyRow, src uuid.UUID) (int, error) {
	f.d1, f.d2, f.src = d1, d2, src
	return len(rows), nil
}

func TestHRISLoader_LoadATT(t *testing.T) {
	src, stg := &fakeATT{}, &fakeATTStg{}
	l := NewHRISLoader(&fakeOT{}, &fakeStg{}).WithATT(src, stg)
	l.now = func() time.Time { return day("2026-10-15") }

	srcID := uuid.New()
	n, err := l.Load(context.Background(), TargetHRATTStaging, "", srcID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, srcID, stg.src)
	assert.Equal(t, src.d1, stg.d1)
	assert.Equal(t, "2026-10-14", stg.d2.Format("2006-01-02"))

	// Without ATT wiring the target fails clearly instead of panicking.
	_, err = NewHRISLoader(&fakeOT{}, &fakeStg{}).Load(context.Background(), TargetHRATTStaging, "", srcID)
	assert.Error(t, err)
}
