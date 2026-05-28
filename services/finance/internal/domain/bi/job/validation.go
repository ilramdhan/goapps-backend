package job

import (
	"fmt"
	"strings"
)

// ValidateCronExpression validates a 5-field cron expression or @shorthand.
// It only checks structural validity (field count), not range correctness.
func ValidateCronExpression(expr string) error {
	expr = strings.TrimSpace(expr)
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
