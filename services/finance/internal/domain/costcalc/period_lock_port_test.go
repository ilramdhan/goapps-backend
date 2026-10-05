package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

type stubChecker struct {
	locked bool
	err    error
	calls  int
}

func (s *stubChecker) IsLocked(context.Context, string, string) (bool, error) {
	s.calls++
	return s.locked, s.err
}

func TestCheckPeriodUnlocked(t *testing.T) {
	ctx := context.Background()

	assert.NoError(t, CheckPeriodUnlocked(ctx, nil, "202608", CalcTypeActual), "nil checker = not locked")

	locked := &stubChecker{locked: true}
	assert.ErrorIs(t, CheckPeriodUnlocked(ctx, locked, "202608", CalcTypeActual), periodlock.ErrPeriodLocked)
	assert.ErrorIs(t, CheckPeriodUnlocked(ctx, locked, "202608", CalcTypeActual), ErrPeriodLocked)
	assert.NoError(t, CheckPeriodUnlocked(ctx, locked, "202608", CalcTypeForecast))
	assert.NoError(t, CheckPeriodUnlocked(ctx, locked, "202608", CalcTypeSelling))
	assert.Equal(t, 2, locked.calls, "non-ACTUAL must not query the checker")

	assert.NoError(t, CheckPeriodUnlocked(ctx, &stubChecker{}, "202608", CalcTypeActual))

	boom := errors.New("db down")
	assert.ErrorIs(t, CheckPeriodUnlocked(ctx, &stubChecker{err: boom}, "202608", CalcTypeActual), boom)
}
