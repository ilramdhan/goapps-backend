package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErpSchedule_DefaultsValidAndOff(t *testing.T) {
	cfg := unmarshalTest(t)
	s := cfg.ERP.Schedule
	assert.False(t, s.Enabled)
	assert.Empty(t, s.Cron)
	assert.Equal(t, 0, s.RunDayOfMonth)
	assert.Equal(t, "03:00", s.RunTime)
	assert.Equal(t, "Asia/Jakarta", s.Timezone)
	assert.Equal(t, "previous", s.TargetPeriod)
	require.NoError(t, s.Validate())
}

func TestErpSchedule_EnvOverrides(t *testing.T) {
	t.Setenv("ERP_SCHEDULE_ENABLED", "true")
	t.Setenv("ERP_SCHEDULE_CRON", "0 0 2 5 * *")
	t.Setenv("ERP_RUN_DAY_OF_MONTH", "7")
	t.Setenv("ERP_RUN_TIME", "04:30")
	t.Setenv("ERP_SCHEDULE_TZ", "UTC")
	t.Setenv("ERP_SCHEDULE_TARGET_PERIOD", "current")
	s := unmarshalTest(t).ERP.Schedule
	assert.True(t, s.Enabled)
	assert.Equal(t, "0 0 2 5 * *", s.Cron)
	assert.Equal(t, 7, s.RunDayOfMonth)
	assert.Equal(t, "04:30", s.RunTime)
	assert.Equal(t, "UTC", s.Timezone)
	assert.Equal(t, "current", s.TargetPeriod)
	require.NoError(t, s.Validate())
}

func TestErpSchedule_ValidateRejects(t *testing.T) {
	base := ErpScheduleConfig{RunTime: "03:00", Timezone: "Asia/Jakarta", TargetPeriod: "previous"}
	cases := map[string]func(*ErpScheduleConfig){
		"cron_5_fields":  func(c *ErpScheduleConfig) { c.Cron = "0 2 5 * *" },
		"cron_garbage":   func(c *ErpScheduleConfig) { c.Cron = "a b c d e f" },
		"cron_inline_tz": func(c *ErpScheduleConfig) { c.Cron = "TZ=UTC 0 0 2 5 * *" },
		"cron_desc":      func(c *ErpScheduleConfig) { c.Cron = "@monthly" },
		"day_32":         func(c *ErpScheduleConfig) { c.RunDayOfMonth = 32 },
		"day_negative":   func(c *ErpScheduleConfig) { c.RunDayOfMonth = -1 },
		"time_25":        func(c *ErpScheduleConfig) { c.RunTime = "25:00" },
		"time_short":     func(c *ErpScheduleConfig) { c.RunTime = "3:00" },
		"tz_bad":         func(c *ErpScheduleConfig) { c.Timezone = "Mars/Olympus" },
		"target_next":    func(c *ErpScheduleConfig) { c.TargetPeriod = "next" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			c := base
			mut(&c)
			err := c.Validate()
			require.Error(t, err)
			assert.True(t, IsInvalidErpSchedule(err))
		})
	}
	ok := base
	ok.Cron, ok.RunDayOfMonth = "30 15 1 3 * *", 31
	require.NoError(t, ok.Validate())
}
