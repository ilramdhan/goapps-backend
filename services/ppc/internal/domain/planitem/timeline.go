package planitem

import "time"

// TimelineParams carries an optional planner-supplied schedule. Supplying
// either field marks the item MANUAL; supplying neither leaves the item
// DERIVED so the application layer can compute a duration from qty/capacity.
type TimelineParams struct {
	StartDate    *time.Time
	DurationDays *int32
}

// IsSet reports whether the planner supplied any timeline input.
func (t TimelineParams) IsSet() bool {
	return t.StartDate != nil || t.DurationDays != nil
}

// timeline is the reconciled, always-consistent schedule of a plan item.
type timeline struct {
	startDate *time.Time
	duration  *int32
	source    string
}

// resolveTimeline reconciles the supplied pair against the deadline so a
// contradictory or inverted schedule is unrepresentable. Start-only
// back-computes the duration; duration-only back-computes the start; both
// supplied must agree exactly.
func resolveTimeline(p TimelineParams, deadline time.Time) (timeline, error) {
	if !p.IsSet() {
		return timeline{source: DurationSourceDerived}, nil
	}

	start, duration, err := reconcilePair(p, deadline)
	if err != nil {
		return timeline{}, err
	}
	return timeline{startDate: &start, duration: &duration, source: DurationSourceManual}, nil
}

func reconcilePair(p TimelineParams, deadline time.Time) (time.Time, int32, error) {
	switch {
	case p.StartDate != nil && p.DurationDays != nil:
		if err := validateDuration(*p.DurationDays); err != nil {
			return time.Time{}, 0, err
		}
		span, err := inclusiveDays(*p.StartDate, deadline)
		if err != nil {
			return time.Time{}, 0, err
		}
		if span != *p.DurationDays {
			return time.Time{}, 0, ErrTimelineMismatch
		}
		return dateOf(*p.StartDate), span, nil

	case p.StartDate != nil:
		span, err := inclusiveDays(*p.StartDate, deadline)
		if err != nil {
			return time.Time{}, 0, err
		}
		return dateOf(*p.StartDate), span, nil

	default:
		if err := validateDuration(*p.DurationDays); err != nil {
			return time.Time{}, 0, err
		}
		start := dateOf(deadline).AddDate(0, 0, -int(*p.DurationDays-1))
		return start, *p.DurationDays, nil
	}
}

// validateDuration bounds a planner-supplied duration. Unlike a system-derived
// duration (which is clamped), an explicit out-of-range value is an error: the
// planner must see that the number they typed was rejected.
func validateDuration(d int32) error {
	if d < MinDurationDays || d > MaxDurationDays {
		return ErrInvalidDuration
	}
	return nil
}

// inclusiveDays returns the day span from start to deadline counting both
// endpoints. A start after the deadline is a domain error, never a clamp.
func inclusiveDays(start, deadline time.Time) (int32, error) {
	s, d := dateOf(start), dateOf(deadline)
	if s.After(d) {
		return 0, ErrStartAfterDeadline
	}
	days := int64(d.Sub(s)/(24*time.Hour)) + 1
	if days > int64(MaxDurationDays) {
		days = int64(MaxDurationDays)
	}
	return int32(days), nil //nolint:gosec // clamped to MaxDurationDays above
}

// dateOf truncates a timestamp to its calendar date in UTC so day arithmetic
// is not perturbed by wall-clock time or DST.
func dateOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// monthOf returns the YYYY-MM projection of a deadline.
func monthOf(deadline time.Time) string { return deadline.Format("2006-01") }
