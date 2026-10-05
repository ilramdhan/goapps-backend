package erpintegration

// schedule_resolver.go resolves the effective ERP integration schedule
// (plan-06 P5-T11 step 2; design Part 2 §9.3; User decision 2026-09-29 U-2).
//
// Precedence:
//  1. the cst_erp_integration_setting row, once its mode is set;
//  2. erp_integration.schedule.cron (6-field robfig/cron v3);
//  3. erp_integration.schedule.run_day_of_month + run_time.
//
// erp_integration.schedule.enabled (ERP_SCHEDULE_ENABLED) is the master
// switch: when it is off nothing is scheduled whatever the row says. Invalid
// input fails closed: the result is disabled and carries the validation
// errors (the scheduler logs a WARN and stays off; the RPC maps
// ErrInvalidSchedule to InvalidArgument).

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Schedule sources reported by GetErpIntegrationSchedule.
const (
	ScheduleSourceSettings   = "SETTINGS"
	ScheduleSourceConfigCron = "CONFIG_CRON"
	ScheduleSourceConfigDay  = "CONFIG_DAY_OF_MONTH"
	ScheduleSourceNone       = "NONE"
)

// Target periods of the scheduled run.
const (
	TargetPeriodPrevious = "previous"
	TargetPeriodCurrent  = "current"
)

// DefaultScheduleTimezone is the timezone used when none is configured.
const DefaultScheduleTimezone = "Asia/Jakarta"

// defaultRunTime is used when a day-based mode has no run time.
const defaultRunTime = "03:00"

// cronParser is the 6-field (seconds first) robfig/cron v3 parser, the same
// shape as cron.WithSeconds(). Descriptors (@monthly…) are not accepted.
var cronParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ScheduleConfig is the erp_integration.schedule config block (mirrors
// config.ErpScheduleConfig; the application layer does not import config).
type ScheduleConfig struct {
	Enabled       bool
	Cron          string
	RunDayOfMonth int
	RunTime       string
	Timezone      string
	TargetPeriod  string
}

// ResolvedSchedule is the effective schedule. When Enabled is false the
// scheduler registers nothing; Errors lists why a configured schedule was
// rejected (fail closed).
type ResolvedSchedule struct {
	Enabled      bool
	Source       string
	Mode         domain.ScheduleMode
	Spec         string // human-readable: the cron, or "day 31 03:00", …
	Timezone     string
	TargetPeriod string
	Errors       []string

	loc  *time.Location
	next func(time.Time) time.Time
}

// Next returns the next fire time strictly after t, in the schedule's
// timezone, or the zero time when the schedule is disabled or has no future
// run (a past SPECIFIC_DATE). It satisfies cron.Schedule.
func (r ResolvedSchedule) Next(t time.Time) time.Time {
	if !r.Enabled || r.next == nil {
		return time.Time{}
	}
	return r.next(t.In(r.loc))
}

// Location returns the schedule timezone (UTC when unresolved).
func (r ResolvedSchedule) Location() *time.Location {
	if r.loc == nil {
		return time.UTC
	}
	return r.loc
}

// TargetPeriodFor returns the YYYYMM the run at now targets.
func (r ResolvedSchedule) TargetPeriodFor(now time.Time) string {
	return targetPeriod(now.In(r.Location()), r.TargetPeriod)
}

// ResolveSchedule computes the effective schedule. setting may be nil (no
// row). It never returns an error: invalid input yields a disabled result
// with Errors set.
func ResolveSchedule(cfg ScheduleConfig, setting *domain.ScheduleSetting) ResolvedSchedule {
	target := strings.ToLower(strings.TrimSpace(cfg.TargetPeriod))
	if target == "" {
		target = TargetPeriodPrevious
	}
	res := ResolvedSchedule{Source: ScheduleSourceNone, TargetPeriod: target}
	var errs []string
	if target != TargetPeriodPrevious && target != TargetPeriodCurrent {
		errs = append(errs, fmt.Sprintf("target_period %q: want previous or current", cfg.TargetPeriod))
	}

	var (
		spec    compiledSchedule
		err     error
		enabled = cfg.Enabled
		tzName  = cfg.Timezone
	)
	switch {
	case setting != nil && setting.IsConfigured():
		res.Source, res.Mode = ScheduleSourceSettings, setting.Mode
		if strings.TrimSpace(setting.Timezone) != "" {
			tzName = setting.Timezone
		}
		enabled = enabled && setting.Enabled
		spec, err = compileSetting(*setting)
	case strings.TrimSpace(cfg.Cron) != "":
		res.Source, res.Mode = ScheduleSourceConfigCron, domain.ScheduleCron
		spec, err = compileCron(cfg.Cron)
	case cfg.RunDayOfMonth != 0:
		res.Source, res.Mode = ScheduleSourceConfigDay, domain.ScheduleDayOfMonth
		spec, err = compileDay(cfg.RunDayOfMonth, cfg.RunTime)
	default:
		enabled = false
		if cfg.Enabled {
			errs = append(errs, "schedule enabled but no cron, run_day_of_month or settings row configured")
		}
	}
	if err != nil {
		errs = append(errs, err.Error())
	}

	loc, lerr := loadScheduleLocation(tzName)
	if lerr != nil {
		errs = append(errs, lerr.Error())
		loc = time.UTC
	}
	res.loc, res.Timezone, res.Spec = loc, loc.String(), spec.desc
	if len(errs) > 0 {
		res.Errors = errs
		return res // fail closed: Enabled stays false
	}
	res.Enabled = enabled
	res.next = spec.next
	return res
}

// ValidateScheduleSetting checks a settings row an admin wants to store. It
// returns an error wrapping domain.ErrInvalidSchedule listing every problem.
func ValidateScheduleSetting(s domain.ScheduleSetting) error {
	var errs []string
	if _, err := domain.ParseScheduleMode(string(s.Mode)); err != nil {
		errs = append(errs, err.Error())
	} else if s.Mode == "" {
		if s.Enabled {
			errs = append(errs, "enabled requires a mode")
		}
	} else if _, err := compileSetting(s); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := loadScheduleLocation(s.Timezone); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrInvalidSchedule, strings.Join(errs, "; "))
	}
	return nil
}

// ParseScheduleCron validates a 6-field robfig/cron v3 expression.
func ParseScheduleCron(expr string) error {
	_, err := compileCron(expr)
	return err
}

// compiledSchedule is a validated schedule body without its timezone.
type compiledSchedule struct {
	desc string
	next func(time.Time) time.Time // t is already in the schedule timezone
}

func compileSetting(s domain.ScheduleSetting) (compiledSchedule, error) {
	switch s.Mode {
	case domain.ScheduleEndOfMonth:
		return compileDay(31, s.RunTime) // clamped to the month end
	case domain.ScheduleStartOfMonth:
		return compileDay(1, s.RunTime)
	case domain.ScheduleDayOfMonth:
		if s.DayOfMonth == 0 {
			return compiledSchedule{}, fmt.Errorf("%w: DAY_OF_MONTH needs day_of_month 1..31", domain.ErrInvalidSchedule)
		}
		return compileDay(s.DayOfMonth, s.RunTime)
	case domain.ScheduleSpecificDate:
		return compileDate(s.RunDate, s.RunTime)
	case domain.ScheduleCron:
		return compileCron(s.Cron)
	default:
		return compiledSchedule{}, fmt.Errorf("%w: unknown mode %q", domain.ErrInvalidSchedule, s.Mode)
	}
}

func compileCron(expr string) (compiledSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return compiledSchedule{}, fmt.Errorf("%w: cron is empty", domain.ErrInvalidSchedule)
	}
	if strings.HasPrefix(expr, "TZ=") || strings.HasPrefix(expr, "CRON_TZ=") {
		return compiledSchedule{}, fmt.Errorf("%w: cron %q: set the timezone separately, not inline", domain.ErrInvalidSchedule, expr)
	}
	if n := len(strings.Fields(expr)); n != 6 {
		return compiledSchedule{}, fmt.Errorf("%w: cron %q has %d fields, want 6 (sec min hour dom month dow)", domain.ErrInvalidSchedule, expr, n)
	}
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return compiledSchedule{}, fmt.Errorf("%w: cron %q: %s", domain.ErrInvalidSchedule, expr, err.Error())
	}
	// A robfig SpecSchedule with time.Local location evaluates in t's own
	// location, which ResolvedSchedule.Next sets to the schedule timezone.
	return compiledSchedule{desc: expr, next: sched.Next}, nil
}

func compileDay(day int, runTime string) (compiledSchedule, error) {
	if day < 1 || day > 31 {
		return compiledSchedule{}, fmt.Errorf("%w: day_of_month %d out of range 1..31", domain.ErrInvalidSchedule, day)
	}
	hh, mm, err := parseRunTime(runTime)
	if err != nil {
		return compiledSchedule{}, err
	}
	return compiledSchedule{
		desc: fmt.Sprintf("day %d %02d:%02d (clamped to month end)", day, hh, mm),
		next: func(t time.Time) time.Time {
			y, m, _ := t.Date()
			for i := 0; i < 3; i++ {
				first := time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, i, 0)
				d := min(day, daysIn(first))
				c := time.Date(first.Year(), first.Month(), d, hh, mm, 0, 0, t.Location())
				if c.After(t) {
					return c
				}
			}
			return time.Time{}
		},
	}, nil
}

func compileDate(date, runTime string) (compiledSchedule, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		return compiledSchedule{}, fmt.Errorf("%w: SPECIFIC_DATE needs run_date YYYY-MM-DD", domain.ErrInvalidSchedule)
	}
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return compiledSchedule{}, fmt.Errorf("%w: run_date %q: want YYYY-MM-DD", domain.ErrInvalidSchedule, date)
	}
	hh, mm, err := parseRunTime(runTime)
	if err != nil {
		return compiledSchedule{}, err
	}
	return compiledSchedule{
		desc: fmt.Sprintf("once %s %02d:%02d", date, hh, mm),
		next: func(t time.Time) time.Time {
			c := time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, t.Location())
			if c.After(t) {
				return c
			}
			return time.Time{} // past: never again
		},
	}, nil
}

// parseRunTime parses HH:MM (24h). "" means the default 03:00.
func parseRunTime(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = defaultRunTime
	}
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, fmt.Errorf("%w: run_time %q: want HH:MM", domain.ErrInvalidSchedule, s)
	}
	return t.Hour(), t.Minute(), nil
}

func loadScheduleLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultScheduleTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: timezone %q: %s", domain.ErrInvalidSchedule, name, err.Error())
	}
	return loc, nil
}

func daysIn(firstOfMonth time.Time) int {
	return firstOfMonth.AddDate(0, 1, -1).Day()
}

// targetPeriod returns YYYYMM of now's month (current) or the month before.
func targetPeriod(now time.Time, target string) string {
	y, m, _ := now.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, now.Location())
	if target != TargetPeriodCurrent {
		first = first.AddDate(0, -1, 0)
	}
	return first.Format("200601")
}

// IsInvalidSchedule reports whether err is a schedule validation error.
func IsInvalidSchedule(err error) bool { return errors.Is(err, domain.ErrInvalidSchedule) }
