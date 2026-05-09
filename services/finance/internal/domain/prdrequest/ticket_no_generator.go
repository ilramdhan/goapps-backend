// Package prdrequest contains the ProductRequest aggregate (ticket) and supporting types.
package prdrequest

import (
	"context"
	"fmt"
	"time"
)

// TicketNoGenerator allocates ticket numbers atomically.
// The repository implementation will use INSERT ... ON CONFLICT on prd_request_sequence.
type TicketNoGenerator interface {
	// Next returns the next available TicketNo for the given period (YYYYMM).
	Next(ctx context.Context, period string) (TicketNo, error)
}

// PeriodForNow returns the YYYYMM string for the given time in Asia/Jakarta.
func PeriodForNow(now time.Time) (string, error) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return "", fmt.Errorf("load Jakarta tz: %w", err)
	}
	return now.In(loc).Format("200601"), nil
}

// FormatTicketNo formats a period and sequence number into a TicketNo (PR-YYYYMM-NNN).
// Returns ErrInvalidTicketNo if seq < 1 or if the resulting string fails TicketNo validation.
func FormatTicketNo(period string, seq int) (TicketNo, error) {
	if seq < 1 {
		return TicketNo{}, ErrInvalidTicketNo
	}
	s := fmt.Sprintf("PR-%s-%03d", period, seq)
	return NewTicketNo(s)
}
