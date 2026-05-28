// Package audit defines the BI config-change audit log domain.
//
// An audit Entry records a single CREATE/UPDATE/DELETE mutation against a BI
// dashboard or dashboard group. Entries are append-only and immutable.
package audit

import "time"

// EntryType identifies the kind of BI entity an audit Entry refers to.
type EntryType string

const (
	// EntryTypeDashboard marks an entry recorded for a dashboard mutation.
	EntryTypeDashboard EntryType = "dashboard"
	// EntryTypeGroup marks an entry recorded for a dashboard-group mutation.
	EntryTypeGroup EntryType = "group"
)

// String returns the raw string value of the entry type.
func (t EntryType) String() string { return string(t) }

// Action identifies the mutation that produced an audit Entry.
type Action string

const (
	// ActionCreate marks a creation event.
	ActionCreate Action = "CREATE"
	// ActionUpdate marks an update event.
	ActionUpdate Action = "UPDATE"
	// ActionDelete marks a deletion event.
	ActionDelete Action = "DELETE"
)

// String returns the raw string value of the action.
func (a Action) String() string { return string(a) }

// Entry is an immutable BI config-change audit record.
type Entry struct {
	// AuditID is the database-assigned identifier (zero for unsaved entries).
	AuditID int64
	// EntityType is "dashboard" or "group".
	EntityType EntryType
	// EntityCode is the dashboard_code / group_code (best-effort; may be empty).
	EntityCode string
	// EntityTitle is the dashboard title / group name (best-effort; may be empty).
	EntityTitle string
	// Action is CREATE, UPDATE, or DELETE.
	Action Action
	// ChangedBy identifies the actor (user UUID string or username; may be empty).
	ChangedBy string
	// ChangedAt is when the change occurred (set by the store on Record).
	ChangedAt time.Time
	// Summary is a short human-readable description of the change.
	Summary string
}
