// Package audit provides PostgreSQL implementation of audit logging.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// PostgresLogger implements audit.Logger using PostgreSQL.
type PostgresLogger struct {
	db *postgres.DB
}

// NewPostgresLogger creates a new PostgreSQL audit logger.
func NewPostgresLogger(db *postgres.DB) *PostgresLogger {
	return &PostgresLogger{db: db}
}

// Log records an audit entry.
func (l *PostgresLogger) Log(ctx context.Context, entry *LogEntry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.PerformedAt.IsZero() {
		entry.PerformedAt = time.Now()
	}

	// Extract context values if not set
	if entry.RequestID == "" {
		entry.RequestID = GetRequestID(ctx)
	}
	if entry.IPAddress == "" {
		entry.IPAddress = GetIPAddress(ctx)
	}
	if entry.UserAgent == "" {
		entry.UserAgent = GetUserAgent(ctx)
	}

	oldDataJSON, err := json.Marshal(entry.OldData)
	if err != nil {
		oldDataJSON = []byte("null")
	}
	newDataJSON, err := json.Marshal(entry.NewData)
	if err != nil {
		newDataJSON = []byte("null")
	}
	changesJSON, err := json.Marshal(entry.Changes)
	if err != nil {
		changesJSON = []byte("null")
	}

	query := `
		INSERT INTO audit_logs (
			id, table_name, record_id, action,
			old_data, new_data, changes,
			performed_by, performed_at,
			request_id, ip_address, user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = l.db.ExecContext(ctx, query,
		entry.ID,
		entry.TableName,
		entry.RecordID,
		string(entry.Action),
		nullableJSON(oldDataJSON),
		nullableJSON(newDataJSON),
		nullableJSON(changesJSON),
		entry.PerformedBy,
		entry.PerformedAt,
		nullableString(entry.RequestID),
		nullableString(entry.IPAddress),
		nullableString(entry.UserAgent),
	)

	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}

// LogCreate records a create action.
func (l *PostgresLogger) LogCreate(ctx context.Context, tableName string, recordID uuid.UUID, newData interface{}, performedBy string) error {
	entry := &LogEntry{
		TableName:   tableName,
		RecordID:    recordID,
		Action:      ActionCreate,
		NewData:     ToJSON(newData),
		PerformedBy: performedBy,
	}
	return l.Log(ctx, entry)
}

// LogUpdate records an update action with old and new data.
func (l *PostgresLogger) LogUpdate(ctx context.Context, tableName string, recordID uuid.UUID, oldData, newData interface{}, performedBy string) error {
	oldJSON := ToJSON(oldData)
	newJSON := ToJSON(newData)

	entry := &LogEntry{
		TableName:   tableName,
		RecordID:    recordID,
		Action:      ActionUpdate,
		OldData:     oldJSON,
		NewData:     newJSON,
		Changes:     ComputeChanges(oldJSON, newJSON),
		PerformedBy: performedBy,
	}
	return l.Log(ctx, entry)
}

// LogDelete records a delete action.
func (l *PostgresLogger) LogDelete(ctx context.Context, tableName string, recordID uuid.UUID, oldData interface{}, performedBy string) error {
	entry := &LogEntry{
		TableName:   tableName,
		RecordID:    recordID,
		Action:      ActionDelete,
		OldData:     ToJSON(oldData),
		PerformedBy: performedBy,
	}
	return l.Log(ctx, entry)
}

// GetByRecordID retrieves audit logs for a specific record.
func (l *PostgresLogger) GetByRecordID(ctx context.Context, tableName string, recordID uuid.UUID) ([]*LogEntry, error) {
	query := `
		SELECT id, table_name, record_id, action, old_data, new_data, changes,
		       performed_by, performed_at, request_id, ip_address, user_agent
		FROM audit_logs
		WHERE table_name = $1 AND record_id = $2
		ORDER BY performed_at DESC
	`

	rows, err := l.db.QueryContext(ctx, query, tableName, recordID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log or handle the error if needed
			_ = err
		}
	}()

	return scanLogEntries(rows)
}

// GetByPerformer retrieves audit logs by performer.
func (l *PostgresLogger) GetByPerformer(ctx context.Context, performedBy string, limit int) ([]*LogEntry, error) {
	query := `
		SELECT id, table_name, record_id, action, old_data, new_data, changes,
		       performed_by, performed_at, request_id, ip_address, user_agent
		FROM audit_logs
		WHERE performed_by = $1
		ORDER BY performed_at DESC
		LIMIT $2
	`

	rows, err := l.db.QueryContext(ctx, query, performedBy, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log or handle the error if needed
			_ = err
		}
	}()

	return scanLogEntries(rows)
}

func scanLogEntries(rows *sql.Rows) ([]*LogEntry, error) {
	var entries []*LogEntry

	for rows.Next() {
		var entry LogEntry
		var oldDataJSON, newDataJSON, changesJSON sql.NullString
		var requestID, ipAddress, userAgent sql.NullString

		err := rows.Scan(
			&entry.ID,
			&entry.TableName,
			&entry.RecordID,
			&entry.Action,
			&oldDataJSON,
			&newDataJSON,
			&changesJSON,
			&entry.PerformedBy,
			&entry.PerformedAt,
			&requestID,
			&ipAddress,
			&userAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		if oldDataJSON.Valid {
			if err := json.Unmarshal([]byte(oldDataJSON.String), &entry.OldData); err != nil {
				// Data may be malformed, continue with empty data
				entry.OldData = nil
			}
		}
		if newDataJSON.Valid {
			if err := json.Unmarshal([]byte(newDataJSON.String), &entry.NewData); err != nil {
				// Data may be malformed, continue with empty data
				entry.NewData = nil
			}
		}
		if changesJSON.Valid {
			if err := json.Unmarshal([]byte(changesJSON.String), &entry.Changes); err != nil {
				// Data may be malformed, continue with empty data
				entry.Changes = nil
			}
		}
		entry.RequestID = requestID.String
		entry.IPAddress = ipAddress.String
		entry.UserAgent = userAgent.String

		entries = append(entries, &entry)
	}

	return entries, nil
}

func nullableJSON(data []byte) interface{} {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return data
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
