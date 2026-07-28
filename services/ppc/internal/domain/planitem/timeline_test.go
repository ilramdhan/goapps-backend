package planitem_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func timelineParams(start *time.Time, duration *int32) planitem.TimelineParams {
	return planitem.TimelineParams{StartDate: start, DurationDays: duration}
}

func ptrTime(t time.Time) *time.Time { return &t }
func ptrInt32(v int32) *int32        { return &v }

// deadline in fgParams is 2026-08-15, so a start of 2026-08-11 spans 5 days.
func TestNew_TimelineStartOnly_DerivesDuration(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(ptrTime(date(2026, 8, 11)), nil)

	pi, err := planitem.New(p)
	require.NoError(t, err)
	require.NotNil(t, pi.PlannedStartDate())
	require.NotNil(t, pi.PlannedDurationDays())
	assert.Equal(t, date(2026, 8, 11), *pi.PlannedStartDate())
	assert.Equal(t, int32(5), *pi.PlannedDurationDays())
	assert.Equal(t, planitem.DurationSourceManual, pi.DurationSource())
}

func TestNew_TimelineDurationOnly_BackDatesStart(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(nil, ptrInt32(5))

	pi, err := planitem.New(p)
	require.NoError(t, err)
	require.NotNil(t, pi.PlannedStartDate())
	assert.Equal(t, date(2026, 8, 11), *pi.PlannedStartDate())
	assert.Equal(t, int32(5), *pi.PlannedDurationDays())
	assert.Equal(t, planitem.DurationSourceManual, pi.DurationSource())
}

func TestNew_TimelineBothConsistent_Succeeds(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(ptrTime(date(2026, 8, 11)), ptrInt32(5))

	pi, err := planitem.New(p)
	require.NoError(t, err)
	assert.Equal(t, date(2026, 8, 11), *pi.PlannedStartDate())
	assert.Equal(t, int32(5), *pi.PlannedDurationDays())
}

func TestNew_TimelineBothContradictory_Fails(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(ptrTime(date(2026, 8, 11)), ptrInt32(9))

	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrTimelineMismatch)
}

func TestNew_TimelineStartAfterDeadline_Fails(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(ptrTime(date(2026, 8, 20)), nil)

	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrStartAfterDeadline)
}

func TestNew_TimelineDurationOutOfRange_Fails(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(nil, ptrInt32(0))
	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrInvalidDuration)

	p.Timeline = timelineParams(nil, ptrInt32(planitem.MaxDurationDays+1))
	_, err = planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrInvalidDuration)
}

func TestNew_NoTimeline_StaysDerived(t *testing.T) {
	pi, err := planitem.New(fgParams())
	require.NoError(t, err)
	assert.Nil(t, pi.PlannedStartDate())
	assert.Nil(t, pi.PlannedDurationDays())
	assert.Equal(t, planitem.DurationSourceDerived, pi.DurationSource())
	assert.True(t, pi.IsDurationDerived())
}

func TestApplyDerivedDuration_SkipsManualItems(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(nil, ptrInt32(5))
	pi, err := planitem.New(p)
	require.NoError(t, err)

	pi.ApplyDerivedDuration(20)

	assert.Equal(t, int32(5), *pi.PlannedDurationDays())
	assert.Equal(t, planitem.DurationSourceManual, pi.DurationSource())
}

func TestApplyDerivedDuration_ClampsAndAnchorsToDeadline(t *testing.T) {
	pi, err := planitem.New(fgParams())
	require.NoError(t, err)

	pi.ApplyDerivedDuration(planitem.MaxDurationDays + 100)
	assert.Equal(t, planitem.MaxDurationDays, *pi.PlannedDurationDays())

	pi.ApplyDerivedDuration(3)
	assert.Equal(t, int32(3), *pi.PlannedDurationDays())
	assert.Equal(t, date(2026, 8, 13), *pi.PlannedStartDate())
	assert.Equal(t, planitem.DurationSourceDerived, pi.DurationSource())
}

func TestNew_MonthDerivedFromDeadline(t *testing.T) {
	p := fgParams()
	p.Month = ""
	pi, err := planitem.New(p)
	require.NoError(t, err)
	assert.Equal(t, "2026-08", pi.Month())
}

func TestNew_MonthMismatchWithoutOverride_Fails(t *testing.T) {
	p := fgParams()
	p.Month = "2026-09"
	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrMonthMismatch)
}

func TestNew_MonthOverride_KeepsExplicitMonth(t *testing.T) {
	p := fgParams()
	p.Month = "2026-09"
	p.MonthOverride = true
	pi, err := planitem.New(p)
	require.NoError(t, err)
	assert.Equal(t, "2026-09", pi.Month())
}

func TestUpdate_DeadlineReDerivesMonthAndReanchorsTimeline(t *testing.T) {
	p := fgParams()
	p.Timeline = timelineParams(nil, ptrInt32(5))
	pi, err := planitem.New(p)
	require.NoError(t, err)

	newDeadline := date(2026, 9, 10)
	_, err = pi.Update(planitem.UpdateParams{Deadline: &newDeadline})
	require.NoError(t, err)

	assert.Equal(t, "2026-09", pi.Month())
	assert.Equal(t, int32(5), *pi.PlannedDurationDays())
	assert.Equal(t, date(2026, 9, 6), *pi.PlannedStartDate())
}

func TestUpdate_TimelineMarksManual(t *testing.T) {
	pi, err := planitem.New(fgParams())
	require.NoError(t, err)
	require.True(t, pi.IsDurationDerived())

	changes, err := pi.Update(planitem.UpdateParams{
		Timeline: timelineParams(nil, ptrInt32(4)),
	})
	require.NoError(t, err)

	assert.Equal(t, planitem.DurationSourceManual, pi.DurationSource())
	assert.Equal(t, date(2026, 8, 12), *pi.PlannedStartDate())
	assert.NotEmpty(t, changes)
}
