// Package audit provides audit logging functionality for tracking data mutations.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Action represents the type of audit action.
type Action string

// Action constants for audit logging.
const (
	ActionCreate Action = "CREATE"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
)

// LogEntry represents an audit log entry.
type LogEntry struct {
	ID          uuid.UUID
	TableName   string
	RecordID    uuid.UUID
	Action      Action
	OldData     map[string]interface{}
	NewData     map[string]interface{}
	Changes     map[string]interface{}
	PerformedBy string
	PerformedAt time.Time
	RequestID   string
	IPAddress   string
	UserAgent   string
}

// Logger defines the interface for audit logging.
type Logger interface {
	// Log records an audit entry.
	Log(ctx context.Context, entry *LogEntry) error

	// LogCreate records a create action.
	LogCreate(ctx context.Context, tableName string, recordID uuid.UUID, newData interface{}, performedBy string) error

	// LogUpdate records an update action with old and new data.
	LogUpdate(ctx context.Context, tableName string, recordID uuid.UUID, oldData, newData interface{}, performedBy string) error

	// LogDelete records a delete action.
	LogDelete(ctx context.Context, tableName string, recordID uuid.UUID, oldData interface{}, performedBy string) error

	// GetByRecordID retrieves audit logs for a specific record.
	GetByRecordID(ctx context.Context, tableName string, recordID uuid.UUID) ([]*LogEntry, error)

	// GetByPerformer retrieves audit logs by performer.
	GetByPerformer(ctx context.Context, performedBy string, limit int) ([]*LogEntry, error)
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	ipAddressKey contextKey = "ip_address"
	userAgentKey contextKey = "user_agent"
	performerKey contextKey = "performer"
)

// WithRequestContext adds request context to the context.
func WithRequestContext(ctx context.Context, requestID, ipAddress, userAgent string) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	ctx = context.WithValue(ctx, ipAddressKey, ipAddress)
	ctx = context.WithValue(ctx, userAgentKey, userAgent)
	return ctx
}

// WithPerformer adds the performer (user) to the context.
func WithPerformer(ctx context.Context, performer string) context.Context {
	return context.WithValue(ctx, performerKey, performer)
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetIPAddress retrieves the IP address from context.
func GetIPAddress(ctx context.Context) string {
	if v := ctx.Value(ipAddressKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetUserAgent retrieves the user agent from context.
func GetUserAgent(ctx context.Context) string {
	if v := ctx.Value(userAgentKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetPerformer retrieves the performer from context.
func GetPerformer(ctx context.Context) string {
	if v := ctx.Value(performerKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "system"
}

// ToJSON converts an interface to JSON map.
func ToJSON(data interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	// Convert to JSON bytes first
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	// Unmarshal to map
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil
	}

	return result
}

// ComputeChanges computes the differences between old and new data.
func ComputeChanges(oldData, newData map[string]interface{}) map[string]interface{} {
	if oldData == nil || newData == nil {
		return nil
	}

	changes := make(map[string]interface{})
	for key, newVal := range newData {
		oldVal, exists := oldData[key]
		if !exists || !jsonEqual(oldVal, newVal) {
			changes[key] = map[string]interface{}{
				"old": oldVal,
				"new": newVal,
			}
		}
	}

	return changes
}

// jsonEqual compares two interface values for JSON equality.
func jsonEqual(a, b interface{}) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}
