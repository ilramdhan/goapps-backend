package erpintegration

// schedule.go is the ERP integration schedule setting (plan-06 P5-T11;
// design Part 2 §9.3; User decision 2026-09-29 U-2). One row in
// cst_erp_integration_setting (migration 000558) overrides the config
// schedule once an admin has set its mode. The scheduled run only enqueues
// the read/compute steps (LoadDemand -> Coverage -> Derive -> Validate); it
// never schedules push, valuation, approve or restore.

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ScheduleMode is how the settings row expresses the monthly run.
type ScheduleMode string

// Schedule modes (chk_ceis_schedule_mode).
const (
	// ScheduleEndOfMonth runs on the last day of every month at RunTime.
	ScheduleEndOfMonth ScheduleMode = "END_OF_MONTH"
	// ScheduleStartOfMonth runs on day 1 of every month at RunTime.
	ScheduleStartOfMonth ScheduleMode = "START_OF_MONTH"
	// ScheduleDayOfMonth runs on DayOfMonth (clamped to the month end).
	ScheduleDayOfMonth ScheduleMode = "DAY_OF_MONTH"
	// ScheduleSpecificDate runs once on RunDate at RunTime.
	ScheduleSpecificDate ScheduleMode = "SPECIFIC_DATE"
	// ScheduleCron runs on a 6-field robfig/cron v3 expression.
	ScheduleCron ScheduleMode = "CRON"
)

// Schedule setting errors.
var (
	// ErrScheduleSettingNotFound is returned when the settings row is absent
	// (the resolver then falls back to the config schedule).
	ErrScheduleSettingNotFound = errors.New("erpintegration: schedule setting not found")
	// ErrInvalidSchedule is returned for an invalid mode, day, time, date,
	// cron or timezone (delivery maps it to InvalidArgument).
	ErrInvalidSchedule = errors.New("erpintegration: invalid schedule")
)

// ParseScheduleMode parses a mode; "" means "not set".
func ParseScheduleMode(s string) (ScheduleMode, error) {
	switch m := ScheduleMode(s); m {
	case "", ScheduleEndOfMonth, ScheduleStartOfMonth, ScheduleDayOfMonth, ScheduleSpecificDate, ScheduleCron:
		return m, nil
	default:
		return "", fmt.Errorf("%w: unknown mode %q", ErrInvalidSchedule, s)
	}
}

// String returns the stored value.
func (m ScheduleMode) String() string { return string(m) }

// ScheduleSetting is the single cst_erp_integration_setting row. Mode ""
// means the row is not configured and the config schedule applies.
type ScheduleSetting struct {
	Enabled    bool
	Mode       ScheduleMode
	DayOfMonth int    // 1..31, DAY_OF_MONTH only (0 = unset)
	RunTime    string // HH:MM ("" = unset)
	RunDate    string // YYYY-MM-DD, SPECIFIC_DATE only ("" = unset)
	Cron       string // 6-field, CRON only ("" = unset)
	Timezone   string // IANA name ("" = config timezone)
	CreatedAt  time.Time
	CreatedBy  string
	UpdatedAt  time.Time
	UpdatedBy  string
}

// IsConfigured reports whether an admin has set a mode, so the row takes
// precedence over the config schedule.
func (s ScheduleSetting) IsConfigured() bool { return s.Mode != "" }

// ScheduleSettingRepository persists the single settings row.
type ScheduleSettingRepository interface {
	// GetSchedule returns the row or ErrScheduleSettingNotFound.
	GetSchedule(ctx context.Context) (ScheduleSetting, error)
	// SaveSchedule upserts the row (audit columns set from actor) and
	// returns it as stored.
	SaveSchedule(ctx context.Context, s ScheduleSetting, actor string) (ScheduleSetting, error)
}
