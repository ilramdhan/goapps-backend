// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// AuditRepository implements audit.Repository interface.
type AuditRepository struct {
	db *DB
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db *DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// Create creates a new audit log entry.
func (r *AuditRepository) Create(ctx context.Context, log *audit.Log) error {
	query := `
		INSERT INTO audit_logs (
			log_id, event_type, table_name, record_id, user_id, username, full_name,
			ip_address, user_agent, service_name, old_data, new_data, changes, performed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.db.ExecContext(ctx, query,
		log.ID(), log.EventType(), log.TableName(), log.RecordID(), log.UserID(),
		log.Username(), log.FullName(), log.IPAddress(), log.UserAgent(),
		log.ServiceName(), log.OldData(), log.NewData(), log.Changes(), log.PerformedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}

// GetByID retrieves an audit log by ID.
func (r *AuditRepository) GetByID(ctx context.Context, id uuid.UUID) (*audit.Log, error) {
	query := `
		SELECT log_id, event_type, table_name, record_id, user_id, username, full_name,
			ip_address, user_agent, service_name, old_data, new_data, changes, performed_at
		FROM audit_logs
		WHERE log_id = $1
	`

	var row auditRow
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&row.ID, &row.EventType, &row.TableName, &row.RecordID, &row.UserID,
		&row.Username, &row.FullName, &row.IPAddress, &row.UserAgent,
		&row.ServiceName, &row.OldData, &row.NewData, &row.Changes, &row.PerformedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return row.toDomain(), nil
}

// List lists audit logs with filters.
func (r *AuditRepository) List(ctx context.Context, params audit.ListParams) ([]*audit.Log, int64, error) {
	whereClause, args, argPos := buildAuditListFilters(params)

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Build query
	sortBy := "performed_at"
	if params.SortBy != "" {
		sortBy = params.SortBy
	}
	sortOrder := sortDESC
	if strings.EqualFold(params.SortOrder, sortASC) {
		sortOrder = sortASC
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT log_id, event_type, table_name, record_id, user_id, username, full_name,
			ip_address, user_agent, service_name, old_data, new_data, changes, performed_at
		FROM audit_logs
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in audit log list")
		}
	}()

	var logs []*audit.Log
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(
			&row.ID, &row.EventType, &row.TableName, &row.RecordID, &row.UserID,
			&row.Username, &row.FullName, &row.IPAddress, &row.UserAgent,
			&row.ServiceName, &row.OldData, &row.NewData, &row.Changes, &row.PerformedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit log: %w", err)
		}
		logs = append(logs, row.toDomain())
	}

	return logs, total, nil
}

func buildAuditListFilters(params audit.ListParams) (string, []interface{}, int) {
	var conditions []string
	var args []interface{}
	argPos := 1

	if params.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("event_type = $%d", argPos))
		args = append(args, params.EventType)
		argPos++
	}
	if params.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argPos))
		args = append(args, *params.UserID)
		argPos++
	}
	if params.TableName != "" {
		conditions = append(conditions, fmt.Sprintf("table_name = $%d", argPos))
		args = append(args, params.TableName)
		argPos++
	}
	if params.ServiceName != "" {
		conditions = append(conditions, fmt.Sprintf("service_name = $%d", argPos))
		args = append(args, params.ServiceName)
		argPos++
	}
	if params.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("performed_at >= $%d", argPos))
		args = append(args, *params.DateFrom)
		argPos++
	}
	if params.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("performed_at <= $%d", argPos))
		args = append(args, *params.DateTo)
		argPos++
	}
	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(username ILIKE $%d OR full_name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}
	return whereClause, args, argPos
}

// GetSummary retrieves audit statistics for a time range.
func (r *AuditRepository) GetSummary(ctx context.Context, timeRange string, serviceName string) (*audit.Summary, error) {
	since := parseTimeRange(timeRange)

	args, serviceCondition := buildServiceFilter(since, serviceName)

	summary, err := r.getEventCounts(ctx, serviceCondition, args)
	if err != nil {
		return nil, err
	}

	// Get top users
	topUsersQuery := fmt.Sprintf(`
		SELECT user_id, username, COALESCE(full_name, username), COUNT(*) as count
		FROM audit_logs
		WHERE performed_at >= $1 AND user_id IS NOT NULL%s
		GROUP BY user_id, username, full_name
		ORDER BY count DESC
		LIMIT 10
	`, serviceCondition)

	userRows, err := r.db.QueryContext(ctx, topUsersQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top users: %w", err)
	}
	defer func() {
		if err := userRows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close userRows in audit summary")
		}
	}()

	for userRows.Next() {
		var activity audit.UserActivity
		if err := userRows.Scan(&activity.UserID, &activity.Username, &activity.FullName, &activity.EventCount); err != nil {
			return nil, err
		}
		summary.TopUsers = append(summary.TopUsers, activity)
	}

	// Get events by hour
	hourlyQuery := fmt.Sprintf(`
		SELECT EXTRACT(HOUR FROM performed_at)::int as hour, COUNT(*) as count
		FROM audit_logs
		WHERE performed_at >= $1%s
		GROUP BY hour
		ORDER BY hour
	`, serviceCondition)

	hourlyRows, err := r.db.QueryContext(ctx, hourlyQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly counts: %w", err)
	}
	defer func() {
		if err := hourlyRows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close hourlyRows in audit summary")
		}
	}()

	for hourlyRows.Next() {
		var hourly audit.HourlyCount
		if err := hourlyRows.Scan(&hourly.Hour, &hourly.Count); err != nil {
			return nil, err
		}
		summary.EventsByHour = append(summary.EventsByHour, hourly)
	}

	return summary, nil
}

func parseTimeRange(timeRange string) time.Time {
	switch timeRange {
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Now().Add(-24 * time.Hour)
	}
}

func buildServiceFilter(since time.Time, serviceName string) ([]interface{}, string) {
	args := []interface{}{since}
	serviceCondition := ""
	if serviceName != "" {
		serviceCondition = " AND service_name = $2"
		args = append(args, serviceName)
	}
	return args, serviceCondition
}

func (r *AuditRepository) getEventCounts(ctx context.Context, serviceCondition string, args []interface{}) (*audit.Summary, error) {
	query := fmt.Sprintf(`
		SELECT event_type, COUNT(*) as count
		FROM audit_logs
		WHERE performed_at >= $1%s
		GROUP BY event_type
	`, serviceCondition)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in audit summary")
		}
	}()

	summary := &audit.Summary{}
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, err
		}
		summary.TotalEvents += count
		assignEventCount(summary, eventType, count)
	}
	return summary, nil
}

func assignEventCount(summary *audit.Summary, eventType string, count int64) {
	switch eventType {
	case string(audit.EventTypeLogin):
		summary.LoginCount = count
	case string(audit.EventTypeLoginFailed):
		summary.LoginFailedCount = count
	case string(audit.EventTypeLogout):
		summary.LogoutCount = count
	case string(audit.EventTypeCreate):
		summary.CreateCount = count
	case string(audit.EventTypeUpdate):
		summary.UpdateCount = count
	case string(audit.EventTypeDelete):
		summary.DeleteCount = count
	case string(audit.EventTypeExport):
		summary.ExportCount = count
	case string(audit.EventTypeImport):
		summary.ImportCount = count
	}
}

// Helper struct for scanning
type auditRow struct {
	ID          uuid.UUID
	EventType   string
	TableName   sql.NullString
	RecordID    *uuid.UUID
	UserID      *uuid.UUID
	Username    sql.NullString
	FullName    sql.NullString
	IPAddress   sql.NullString
	UserAgent   sql.NullString
	ServiceName string
	OldData     []byte
	NewData     []byte
	Changes     []byte
	PerformedAt time.Time
}

func (r *auditRow) toDomain() *audit.Log {
	return audit.ReconstructLog(
		r.ID,
		audit.EventType(r.EventType),
		r.TableName.String,
		r.RecordID,
		r.UserID,
		r.Username.String,
		r.FullName.String,
		r.IPAddress.String,
		r.UserAgent.String,
		r.ServiceName,
		json.RawMessage(r.OldData),
		json.RawMessage(r.NewData),
		json.RawMessage(r.Changes),
		r.PerformedAt,
	)
}
