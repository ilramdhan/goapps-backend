package workorder

import "strconv"

// parseFloat parses a decimal-as-string, reporting whether it succeeded. An empty
// string is treated as absent (ok=false).
func parseFloat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
