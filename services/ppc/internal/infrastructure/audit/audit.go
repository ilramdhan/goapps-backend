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
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// GetIPAddress retrieves the IP address from context.
func GetIPAddress(ctx context.Context) string {
	if v, ok := ctx.Value(ipAddressKey).(string); ok {
		return v
	}
	return ""
}

// GetUserAgent retrieves the user agent from context.
func GetUserAgent(ctx context.Context) string {
	if v, ok := ctx.Value(userAgentKey).(string); ok {
		return v
	}
	return ""
}

// GetPerformer retrieves the performer from context.
func GetPerformer(ctx context.Context) string {
	if v, ok := ctx.Value(performerKey).(string); ok {
		return v
	}
	return "system"
}

// ToJSON converts an interface to a JSON map.
func ToJSON(data interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil
	}

	return result
}
