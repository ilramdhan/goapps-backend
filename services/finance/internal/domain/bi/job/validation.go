package job

import (
	"fmt"
	"strings"
	"time"
)

// ValidateCronExpression validates a 5-field cron expression or @shorthand, optionally prefixed
// with a time zone (`CRON_TZ=Asia/Jakarta 0 5 * * *`, robfig/cron syntax) so HR jobs run on WIB
// regardless of the server TZ (H013). It only checks structure, not range correctness.
func ValidateCronExpression(expr string) error {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "CRON_TZ=") || strings.HasPrefix(expr, "TZ=") {
		tz, rest, ok := strings.Cut(expr, " ")
		if !ok || strings.HasSuffix(tz, "=") {
			return fmt.Errorf("%w: time zone prefix without schedule", ErrInvalidCron)
		}
		if _, err := time.LoadLocation(tz[strings.Index(tz, "=")+1:]); err != nil {
			return fmt.Errorf("%w: unknown time zone %q", ErrInvalidCron, tz)
		}
		expr = strings.TrimSpace(rest)
	}
	if expr == "" {
		return fmt.Errorf("%w: empty expression", ErrInvalidCron)
	}
	// Accept @shorthand descriptors.
	if strings.HasPrefix(expr, "@") {
		allowed := map[string]bool{
			"@hourly": true, "@daily": true, "@weekly": true,
			"@monthly": true, "@yearly": true, "@midnight": true,
			"@annually": true,
		}
		if !allowed[expr] {
			return fmt.Errorf("%w: unknown shorthand %q", ErrInvalidCron, expr)
		}
		return nil
	}
	// Must have exactly 5 space-separated fields: min hour dom month dow.
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("%w: expected 5 fields (min hour dom month dow), got %d", ErrInvalidCron, len(fields))
	}
	return nil
}
