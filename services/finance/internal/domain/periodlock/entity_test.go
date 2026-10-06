package periodlock

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func TestValidatePeriod(t *testing.T) {
	for _, p := range []string{"202601", "202612", "199912"} {
		assert.NoError(t, ValidatePeriod(p), p)
	}
	for _, p := range []string{"", "2026", "202600", "202613", "2026-01", "20260101", "abcdef"} {
		assert.ErrorIs(t, ValidatePeriod(p), ErrInvalidPeriod, p)
	}
}

func TestValidateCalcType(t *testing.T) {
	assert.NoError(t, ValidateCalcType("ACTUAL"))
	for _, c := range []string{"", "FORECAST", "SELLING", "actual"} {
		assert.ErrorIs(t, ValidateCalcType(c), ErrInvalidCalcType, c)
	}
}

func TestNewPeriodLock(t *testing.T) {
	l, err := NewPeriodLock("202608", CalcTypeActual, "  alice ", " month close ", t0)
	require.NoError(t, err)
	assert.True(t, l.IsLocked())
	assert.Equal(t, "202608", l.Period())
	assert.Equal(t, CalcTypeActual, l.CalcType())
	assert.Equal(t, "alice", l.LockedBy())
	assert.Equal(t, "month close", l.Reason())
	assert.Equal(t, t0, l.LockedAt())
	assert.Nil(t, l.ErpBatchID())
	assert.Nil(t, l.UnlockedAt())
	assert.Empty(t, l.UnlockedBy())
	assert.Empty(t, l.UnlockReason())
}

func TestNewPeriodLockValidation(t *testing.T) {
	cases := []struct {
		name, period, calc, user, reason string
		want                             error
	}{
		{"bad period", "202613", CalcTypeActual, "u", "r", ErrInvalidPeriod},
		{"forecast", "202608", "FORECAST", "u", "r", ErrInvalidCalcType},
		{"selling", "202608", "SELLING", "u", "r", ErrInvalidCalcType},
		{"no user", "202608", CalcTypeActual, "  ", "r", ErrUserRequired},
		{"no reason", "202608", CalcTypeActual, "u", " ", ErrReasonRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := NewPeriodLock(tc.period, tc.calc, tc.user, tc.reason, t0)
			assert.Nil(t, l)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestNewPeriodLockTruncatesLongUser(t *testing.T) {
	l, err := NewPeriodLock("202608", CalcTypeActual, strings.Repeat("x", 100), "r", t0)
	require.NoError(t, err)
	assert.Len(t, l.LockedBy(), maxUserLen)
}

func TestUnlock(t *testing.T) {
	l, err := NewPeriodLock("202608", CalcTypeActual, "alice", "close", t0)
	require.NoError(t, err)
	t1 := t0.Add(time.Hour)
	require.NoError(t, l.Unlock("bob", "reopen", false, t1))
	assert.False(t, l.IsLocked())
	require.NotNil(t, l.UnlockedAt())
	assert.Equal(t, t1, *l.UnlockedAt())
	assert.Equal(t, "bob", l.UnlockedBy())
	assert.Equal(t, "reopen", l.UnlockReason())
	// lock history is kept on the row
	assert.Equal(t, "alice", l.LockedBy())

	assert.ErrorIs(t, l.Unlock("bob", "again", false, t1), ErrNotLocked)
}

func TestUnlockPostedRefused(t *testing.T) {
	l, err := NewPeriodLock("202608", CalcTypeActual, "alice", "close", t0)
	require.NoError(t, err)
	err = l.Unlock("bob", "reopen", true, t0)
	assert.ErrorIs(t, err, ErrUnlockPostedPeriod)
	assert.True(t, l.IsLocked(), "refused unlock must not change state")
	assert.Nil(t, l.UnlockedAt())
}

func TestUnlockValidation(t *testing.T) {
	l, err := NewPeriodLock("202608", CalcTypeActual, "alice", "close", t0)
	require.NoError(t, err)
	assert.ErrorIs(t, l.Unlock("", "r", false, t0), ErrUserRequired)
	assert.ErrorIs(t, l.Unlock("u", "", false, t0), ErrReasonRequired)
	assert.True(t, l.IsLocked())
}

func TestReLock(t *testing.T) {
	batch := int64(42)
	unl := t0.Add(time.Hour)
	l := Reconstruct("202608", CalcTypeActual, t0, "alice", "close", &batch, &unl, "bob", "reopen")
	assert.False(t, l.IsLocked())
	assert.Equal(t, &batch, l.ErpBatchID())

	t2 := t0.Add(2 * time.Hour)
	require.NoError(t, l.Lock("carol", "close again", t2))
	assert.True(t, l.IsLocked())
	assert.Equal(t, "carol", l.LockedBy())
	assert.Equal(t, "close again", l.Reason())
	assert.Equal(t, t2, l.LockedAt())
	assert.Nil(t, l.ErpBatchID())
	assert.Nil(t, l.UnlockedAt())
	assert.Empty(t, l.UnlockedBy())
	assert.Empty(t, l.UnlockReason())

	assert.ErrorIs(t, l.Lock("carol", "x", t2), ErrAlreadyLocked)
}

func TestReLockValidation(t *testing.T) {
	unl := t0
	l := Reconstruct("202608", CalcTypeActual, t0, "a", "r", nil, &unl, "b", "r")
	assert.ErrorIs(t, l.Lock("", "r", t0), ErrUserRequired)
	assert.ErrorIs(t, l.Lock("u", "", t0), ErrReasonRequired)
	assert.False(t, l.IsLocked())
}

func TestSentinelsDistinct(t *testing.T) {
	all := []error{
		ErrPeriodLocked, ErrUnlockPostedPeriod, ErrUnlockBatchLocked, ErrAlreadyLocked,
		ErrNotLocked, ErrLockNotFound, ErrInvalidPeriod, ErrInvalidCalcType,
		ErrUserRequired, ErrReasonRequired,
	}
	for i, a := range all {
		for j, b := range all {
			if i != j {
				assert.False(t, errors.Is(a, b), "%v vs %v", a, b)
			}
		}
	}
}
