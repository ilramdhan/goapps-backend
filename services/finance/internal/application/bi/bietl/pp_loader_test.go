package bietl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

type fakePPSource struct {
	d1, d2 time.Time
	rows   []oracleinfra.PackProdDailyRow
	err    error
}

func (f *fakePPSource) FetchPackProdDaily(_ context.Context, d1, d2 time.Time) ([]oracleinfra.PackProdDailyRow, error) {
	f.d1, f.d2 = d1, d2
	return f.rows, f.err
}

type fakePPStaging struct {
	d1, d2 time.Time
	n      int
	src    uuid.UUID
}

func (f *fakePPStaging) ReplacePackProdDaily(_ context.Context, d1, d2 time.Time, rows []oracleinfra.PackProdDailyRow, src uuid.UUID) (int, error) {
	f.d1, f.d2, f.n, f.src = d1, d2, len(rows), src
	return len(rows), nil
}

func TestPPWindow_UsesWIBDate(t *testing.T) {
	// 2026-10-26 00:30 UTC = 07:30 WIB on 10-26 → d2 = 10-25, d1 = 10-26 − 35 = 09-21.
	d1, d2 := PPWindow(time.Date(2026, 10, 26, 0, 30, 0, 0, time.UTC))
	assert.Equal(t, "2026-09-21", d1.Format(time.DateOnly))
	assert.Equal(t, "2026-10-25", d2.Format(time.DateOnly))

	// 2026-10-25 23:00 UTC is already 10-26 06:00 WIB.
	d1, d2 = PPWindow(time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-09-21", d1.Format(time.DateOnly))
	assert.Equal(t, "2026-10-25", d2.Format(time.DateOnly))
}

func TestPPLoader_Load_PassesSameWindowToSourceAndStaging(t *testing.T) {
	src := &fakePPSource{rows: make([]oracleinfra.PackProdDailyRow, 3)}
	stg := &fakePPStaging{}
	l := NewPPLoader(src, stg, zerolog.Nop())
	l.now = func() time.Time { return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC) }

	sourceID := uuid.New()
	n, err := l.Load(context.Background(), TargetPPPack, "ASPAK.VIEW_BI_PACK_PROD_DAILY", sourceID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, src.d1, stg.d1)
	assert.Equal(t, src.d2, stg.d2)
	assert.Equal(t, sourceID, stg.src, "facts are stamped with the job's source")
}

func TestPPLoader_Load_UnsupportedTarget(t *testing.T) {
	l := NewPPLoader(&fakePPSource{}, &fakePPStaging{}, zerolog.Nop())
	_, err := l.Load(context.Background(), "HR_OT_STAGING", "", uuid.New())
	assert.Error(t, err)
}

func TestPPLoader_Load_FetchErrorSkipsStaging(t *testing.T) {
	stg := &fakePPStaging{}
	l := NewPPLoader(&fakePPSource{err: errors.New("ora down")}, stg, zerolog.Nop())
	_, err := l.Load(context.Background(), TargetPPPack, "", uuid.New())
	assert.Error(t, err)
	assert.True(t, stg.d1.IsZero())
}
