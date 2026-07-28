package planitem

import (
	"strconv"
	"time"
)

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func formatInt(v int64) string { return strconv.FormatInt(v, 10) }

func int64PtrString(v *int64) string {
	if v == nil {
		return ""
	}
	return formatInt(*v)
}

func int32PtrString(v *int32) string {
	if v == nil {
		return ""
	}
	return formatInt(int64(*v))
}

func datePtrString(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format("2006-01-02")
}
