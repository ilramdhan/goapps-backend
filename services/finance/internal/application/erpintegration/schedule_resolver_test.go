package erpintegration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func jkt(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	return loc
}

func baseCfg() ScheduleConfig {
	return ScheduleConfig{Enabled: true, RunTime: "03:00", Timezone: "Asia/Jakarta", TargetPeriod: "previous"}
}

func TestResolveSchedule_Precedence(t *testing.T) {
	loc := jkt(t)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)

	t.Run("settings_row_wins_over_cron_and_day", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Cron, cfg.RunDayOfMonth = "0 0 2 5 * *", 7
		row := &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleStartOfMonth, RunTime: "04:30"}
		r := ResolveSchedule(cfg, row)
		require.Empty(t, r.Errors)
		assert.True(t, r.Enabled)
		assert.Equal(t, ScheduleSourceSettings, r.Source)
		assert.Equal(t, time.Date(2026, 11, 1, 4, 30, 0, 0, loc), r.Next(now))
	})
	t.Run("unconfigured_row_falls_back_to_cron", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Cron, cfg.RunDayOfMonth = "0 0 2 5 * *", 7
		r := ResolveSchedule(cfg, &domain.ScheduleSetting{})
		require.Empty(t, r.Errors)
		assert.Equal(t, ScheduleSourceConfigCron, r.Source)
		assert.Equal(t, time.Date(2026, 10, 5, 2, 0, 0, 0, loc), r.Next(now))
	})
	t.Run("cron_wins_over_day", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Cron, cfg.RunDayOfMonth = "0 15 1 3 * *", 7
		r := ResolveSchedule(cfg, nil)
		assert.Equal(t, ScheduleSourceConfigCron, r.Source)
		assert.Equal(t, time.Date(2026, 10, 3, 1, 15, 0, 0, loc), r.Next(now))
	})
	t.Run("day_of_month_last", func(t *testing.T) {
		cfg := baseCfg()
		cfg.RunDayOfMonth = 7
		r := ResolveSchedule(cfg, nil)
		require.Empty(t, r.Errors)
		assert.Equal(t, ScheduleSourceConfigDay, r.Source)
		assert.Equal(t, time.Date(2026, 10, 7, 3, 0, 0, 0, loc), r.Next(now))
	})
	t.Run("nothing_configured_is_off", func(t *testing.T) {
		cfg := baseCfg()
		r := ResolveSchedule(cfg, nil)
		assert.False(t, r.Enabled)
		assert.Equal(t, ScheduleSourceNone, r.Source)
		assert.NotEmpty(t, r.Errors, "enabled without any schedule is reported")
		assert.True(t, r.Next(now).IsZero())
	})
	t.Run("master_switch_off_disables_everything", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Enabled, cfg.Cron = false, "0 0 2 5 * *"
		r := ResolveSchedule(cfg, &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleEndOfMonth})
		assert.False(t, r.Enabled)
		assert.Empty(t, r.Errors)
		assert.True(t, r.Next(now).IsZero())
	})
	t.Run("row_disabled_disables", func(t *testing.T) {
		r := ResolveSchedule(baseCfg(), &domain.ScheduleSetting{Enabled: false, Mode: domain.ScheduleEndOfMonth})
		assert.False(t, r.Enabled)
		assert.Equal(t, ScheduleSourceSettings, r.Source)
	})
	t.Run("row_timezone_overrides_config", func(t *testing.T) {
		row := &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleStartOfMonth, RunTime: "00:00", Timezone: "UTC"}
		r := ResolveSchedule(baseCfg(), row)
		assert.Equal(t, "UTC", r.Timezone)
		assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), r.Next(now).UTC())
	})
}

func TestResolveSchedule_Modes(t *testing.T) {
	loc := jkt(t)
	cases := []struct {
		name string
		row  domain.ScheduleSetting
		now  time.Time
		want time.Time
	}{
		{"end_of_month_feb_non_leap", domain.ScheduleSetting{Mode: domain.ScheduleEndOfMonth, RunTime: "23:00"},
			time.Date(2027, 2, 10, 0, 0, 0, 0, loc), time.Date(2027, 2, 28, 23, 0, 0, 0, loc)},
		{"end_of_month_feb_leap", domain.ScheduleSetting{Mode: domain.ScheduleEndOfMonth, RunTime: "23:00"},
			time.Date(2028, 2, 10, 0, 0, 0, 0, loc), time.Date(2028, 2, 29, 23, 0, 0, 0, loc)},
		{"end_of_month_rolls_to_next", domain.ScheduleSetting{Mode: domain.ScheduleEndOfMonth, RunTime: "03:00"},
			time.Date(2026, 4, 30, 3, 0, 0, 0, loc), time.Date(2026, 5, 31, 3, 0, 0, 0, loc)},
		{"day_31_clamped_april", domain.ScheduleSetting{Mode: domain.ScheduleDayOfMonth, DayOfMonth: 31, RunTime: "03:00"},
			time.Date(2026, 4, 1, 0, 0, 0, 0, loc), time.Date(2026, 4, 30, 3, 0, 0, 0, loc)},
		{"start_of_month_default_time", domain.ScheduleSetting{Mode: domain.ScheduleStartOfMonth},
			time.Date(2026, 12, 15, 0, 0, 0, 0, loc), time.Date(2027, 1, 1, 3, 0, 0, 0, loc)},
		{"specific_date_future", domain.ScheduleSetting{Mode: domain.ScheduleSpecificDate, RunDate: "2026-10-20", RunTime: "08:15"},
			time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 20, 8, 15, 0, 0, loc)},
		{"specific_date_past_never", domain.ScheduleSetting{Mode: domain.ScheduleSpecificDate, RunDate: "2026-09-20", RunTime: "08:15"},
			time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Time{}},
		{"cron_mode", domain.ScheduleSetting{Mode: domain.ScheduleCron, Cron: "30 0 6 * * 1"},
			time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 5, 6, 0, 30, 0, loc)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := tc.row
			row.Enabled = true
			r := ResolveSchedule(baseCfg(), &row)
			require.Empty(t, r.Errors)
			got := r.Next(tc.now)
			if tc.want.IsZero() {
				assert.True(t, got.IsZero(), "got %v", got)
				return
			}
			assert.True(t, tc.want.Equal(got), "want %v got %v", tc.want, got)
		})
	}
}

func TestResolveSchedule_InvalidFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(*ScheduleConfig)
		row  *domain.ScheduleSetting
	}{
		{"cron_5_fields", func(c *ScheduleConfig) { c.Cron = "0 2 5 * *" }, nil},
		{"cron_garbage", func(c *ScheduleConfig) { c.Cron = "a b c d e f" }, nil},
		{"cron_descriptor", func(c *ScheduleConfig) { c.Cron = "@monthly" }, nil},
		{"cron_inline_tz", func(c *ScheduleConfig) { c.Cron = "TZ=UTC 0 0 2 5 * *" }, nil},
		{"day_32", func(c *ScheduleConfig) { c.RunDayOfMonth = 32 }, nil},
		{"day_negative", func(c *ScheduleConfig) { c.RunDayOfMonth = -1 }, nil},
		{"bad_time", func(c *ScheduleConfig) { c.RunDayOfMonth, c.RunTime = 5, "25:00" }, nil},
		{"bad_time_format", func(c *ScheduleConfig) { c.RunDayOfMonth, c.RunTime = 5, "3:00" }, nil},
		{"bad_tz", func(c *ScheduleConfig) { c.RunDayOfMonth, c.Timezone = 5, "Mars/Olympus" }, nil},
		{"bad_target", func(c *ScheduleConfig) { c.RunDayOfMonth, c.TargetPeriod = 5, "next" }, nil},
		{"row_bad_cron", nil, &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleCron, Cron: "* * *"}},
		{"row_day_missing", nil, &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleDayOfMonth}},
		{"row_bad_date", nil, &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleSpecificDate, RunDate: "2026-02-30"}},
		{"row_unknown_mode", nil, &domain.ScheduleSetting{Enabled: true, Mode: "WEEKLY"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseCfg()
			if tc.cfg != nil {
				tc.cfg(&cfg)
			}
			r := ResolveSchedule(cfg, tc.row)
			assert.False(t, r.Enabled, "invalid schedule must stay off")
			assert.NotEmpty(t, r.Errors)
			assert.True(t, r.Next(time.Now()).IsZero())
		})
	}
}

func TestValidateScheduleSetting(t *testing.T) {
	require.NoError(t, ValidateScheduleSetting(domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleCron, Cron: "0 0 2 5 * *", Timezone: "Asia/Jakarta"}))
	require.NoError(t, ValidateScheduleSetting(domain.ScheduleSetting{Enabled: false}))
	err := ValidateScheduleSetting(domain.ScheduleSetting{Enabled: true})
	require.ErrorIs(t, err, domain.ErrInvalidSchedule)
	err = ValidateScheduleSetting(domain.ScheduleSetting{Mode: domain.ScheduleDayOfMonth, DayOfMonth: 0})
	require.ErrorIs(t, err, domain.ErrInvalidSchedule)
	assert.True(t, IsInvalidSchedule(err))
	require.NoError(t, ParseScheduleCron("0 0 2 5 * *"))
	require.ErrorIs(t, ParseScheduleCron("0 2 5 * *"), domain.ErrInvalidSchedule)
}

func TestTargetPeriod(t *testing.T) {
	loc := jkt(t)
	r := ResolveSchedule(ScheduleConfig{Enabled: true, RunDayOfMonth: 5}, nil)
	assert.Equal(t, "202512", r.TargetPeriodFor(time.Date(2026, 1, 5, 3, 0, 0, 0, loc)))
	assert.Equal(t, "202609", r.TargetPeriodFor(time.Date(2026, 10, 31, 23, 0, 0, 0, loc)))
	// 2026-10-31 20:00 UTC is already 2026-11-01 in Jakarta.
	assert.Equal(t, "202610", r.TargetPeriodFor(time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)))
	cur := ResolveSchedule(ScheduleConfig{Enabled: true, RunDayOfMonth: 5, TargetPeriod: "current"}, nil)
	assert.Equal(t, "202601", cur.TargetPeriodFor(time.Date(2026, 1, 5, 3, 0, 0, 0, loc)))
}
