// Package requesthistory holds the approval trace domain for cost product requests.
package requesthistory

import "time"

// Entry records a single CPR status transition.
type Entry struct {
	ID          int64
	RequestID   int64
	FromStatus  string // empty string for initial creation
	ToStatus    string
	ActorUserID string
	ActorName   string
	Note        string
	CreatedAt   time.Time
}
