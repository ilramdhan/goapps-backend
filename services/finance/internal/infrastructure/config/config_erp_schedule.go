package config

// config_erp_schedule.go validates erp_integration.schedule (plan-06 P5-T11;
// User decision U-2). The scheduler itself resolves and fails closed on an
// invalid schedule; Validate lets startup log a precise reason.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// errInvalidErpSchedule marks every schedule config error.
var errInvalidErpSchedule = errors.New("invalid erp_integration.schedule")

// erpScheduleCronParser is the 6-field (seconds-first) parser shared with the
// orchestrator's cron (no descriptors, no inline TZ).
var erpScheduleCronParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Validate checks the schedule block. It validates the fields even when the
// schedule is disabled so a bad value is caught before anyone flips the
// switch. Empty cron and run_day_of_month 0 mean "not set".
func (c ErpScheduleConfig) Validate() error {
	var errs []string
	if spec := strings.TrimSpace(c.Cron); spec != "" {
		if strings.HasPrefix(spec, "TZ=") || strings.HasPrefix(spec, "CRON_TZ=") {
			errs = append(errs, "cron must not carry an inline TZ (use timezone)")
		} else if len(strings.Fields(spec)) != 6 {
			errs = append(errs, fmt.Sprintf("cron %q must have 6 fields (sec min hour dom month dow)", spec))
		} else if _, err := erpScheduleCronParser.Parse(spec); err != nil {
			errs = append(errs, fmt.Sprintf("cron %q: %v", spec, err))
		}
	}
	if c.RunDayOfMonth < 0 || c.RunDayOfMonth > 31 {
		errs = append(errs, fmt.Sprintf("run_day_of_month %d must be 0..31", c.RunDayOfMonth))
	}
	if rt := strings.TrimSpace(c.RunTime); rt != "" {
		if _, err := time.Parse("15:04", rt); err != nil || len(rt) != 5 {
			errs = append(errs, fmt.Sprintf("run_time %q must be HH:MM", rt))
		}
	}
	if tz := strings.TrimSpace(c.Timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			errs = append(errs, fmt.Sprintf("timezone %q: %v", tz, err))
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.TargetPeriod)) {
	case "", "previous", "current":
	default:
		errs = append(errs, fmt.Sprintf("target_period %q must be previous or current", c.TargetPeriod))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", errInvalidErpSchedule, strings.Join(errs, "; "))
	}
	return nil
}

// IsInvalidErpSchedule reports whether err came from ErpScheduleConfig.Validate.
func IsInvalidErpSchedule(err error) bool { return errors.Is(err, errInvalidErpSchedule) }
