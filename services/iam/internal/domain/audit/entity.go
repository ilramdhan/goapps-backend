// Package audit provides domain logic for audit logging.
package audit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of audit event.
type EventType string

// Audit event type constants.
const (
	EventTypeLogin          EventType = "LOGIN"
	EventTypeLogout         EventType = "LOGOUT"
	EventTypeLoginFailed    EventType = "LOGIN_FAILED"
	EventTypePasswordReset  EventType = "PASSWORD_RESET"
	EventTypePasswordChange EventType = "PASSWORD_CHANGE"
	EventType2FAEnabled     EventType = "2FA_ENABLED"
	EventType2FADisabled    EventType = "2FA_DISABLED"
	EventTypeCreate         EventType = "CREATE"
	EventTypeUpdate         EventType = "UPDATE"
	EventTypeDelete         EventType = "DELETE"
	EventTypeExport         EventType = "EXPORT"
	EventTypeImport         EventType = "IMPORT"
)

// Log represents an audit log entry.
type Log struct {
	id          uuid.UUID
	eventType   EventType
	tableName   string
	recordID    *uuid.UUID
	userID      *uuid.UUID
	username    string
	fullName    string
	ipAddress   string
	userAgent   string
	serviceName string
	oldData     json.RawMessage
	newData     json.RawMessage
	changes     json.RawMessage
	performedAt time.Time
}

// NewLog creates a new audit Log entry.
func NewLog(
	eventType EventType,
	tableName string,
	recordID *uuid.UUID,
	userID *uuid.UUID,
	username, fullName, ipAddress, userAgent, serviceName string,
) *Log {
	return &Log{
		id:          uuid.New(),
		eventType:   eventType,
		tableName:   tableName,
		recordID:    recordID,
		userID:      userID,
		username:    username,
		fullName:    fullName,
		ipAddress:   ipAddress,
		userAgent:   userAgent,
		serviceName: serviceName,
		performedAt: time.Now(),
	}
}

// ReconstructLog reconstructs a Log from persistence.
func ReconstructLog(
	id uuid.UUID,
	eventType EventType,
	tableName string,
	recordID, userID *uuid.UUID,
	username, fullName, ipAddress, userAgent, serviceName string,
	oldData, newData, changes json.RawMessage,
	performedAt time.Time,
) *Log {
	return &Log{
		id:          id,
		eventType:   eventType,
		tableName:   tableName,
		recordID:    recordID,
		userID:      userID,
		username:    username,
		fullName:    fullName,
		ipAddress:   ipAddress,
		userAgent:   userAgent,
		serviceName: serviceName,
		oldData:     oldData,
		newData:     newData,
		changes:     changes,
		performedAt: performedAt,
	}
}

// ID returns the audit log identifier.
func (l *Log) ID() uuid.UUID { return l.id }

// EventType returns the event type.
func (l *Log) EventType() EventType { return l.eventType }

// TableName returns the table name.
func (l *Log) TableName() string { return l.tableName }

// RecordID returns the record identifier.
func (l *Log) RecordID() *uuid.UUID { return l.recordID }

// UserID returns the user identifier.
func (l *Log) UserID() *uuid.UUID { return l.userID }

// Username returns the username.
func (l *Log) Username() string { return l.username }

// FullName returns the full name.
func (l *Log) FullName() string { return l.fullName }

// IPAddress returns the IP address.
func (l *Log) IPAddress() string { return l.ipAddress }

// UserAgent returns the user agent.
func (l *Log) UserAgent() string { return l.userAgent }

// ServiceName returns the service name.
func (l *Log) ServiceName() string { return l.serviceName }

// OldData returns the old data.
func (l *Log) OldData() json.RawMessage { return l.oldData }

// NewData returns the new data.
func (l *Log) NewData() json.RawMessage { return l.newData }

// Changes returns the changes.
func (l *Log) Changes() json.RawMessage { return l.changes }

// PerformedAt returns the time the event was performed.
func (l *Log) PerformedAt() time.Time { return l.performedAt }

// SetOldData sets the old data field.
func (l *Log) SetOldData(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	l.oldData = bytes
	return nil
}

// SetNewData sets the new data field.
func (l *Log) SetNewData(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	l.newData = bytes
	return nil
}

// SetChanges sets the changes field.
func (l *Log) SetChanges(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	l.changes = bytes
	return nil
}

// Summary contains audit statistics for dashboard.
type Summary struct {
	TotalEvents      int64
	LoginCount       int64
	LoginFailedCount int64
	LogoutCount      int64
	CreateCount      int64
	UpdateCount      int64
	DeleteCount      int64
	ExportCount      int64
	ImportCount      int64
	TopUsers         []UserActivity
	EventsByHour     []HourlyCount
}

// UserActivity represents activity count for a user.
type UserActivity struct {
	UserID     uuid.UUID
	Username   string
	FullName   string
	EventCount int64
}

// HourlyCount represents event count by hour.
type HourlyCount struct {
	Hour  int
	Count int64
}
