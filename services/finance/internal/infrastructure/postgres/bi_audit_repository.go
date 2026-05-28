package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/audit"
)

// BiAuditRepository implements audit.Repository over PostgreSQL.
type BiAuditRepository struct {
	db *DB
}

// NewBiAuditRepository constructs a BiAuditRepository.
func NewBiAuditRepository(db *DB) *BiAuditRepository {
	return &BiAuditRepository{db: db}
}

var _ audit.Repository = (*BiAuditRepository)(nil)

// Record appends a single audit entry; changed_at defaults to NOW() in the DB.
func (r *BiAuditRepository) Record(ctx context.Context, entry audit.Entry) error {
	const q = `
INSERT INTO bi_audit_log (entity_type, entity_code, entity_title, action, changed_by, summary)
VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := r.db.ExecContext(ctx, q,
		entry.EntityType.String(),
		biNullableString(entry.EntityCode),
		biNullableString(entry.EntityTitle),
		entry.Action.String(),
		biNullableString(entry.ChangedBy),
		biNullableString(entry.Summary),
	); err != nil {
		return fmt.Errorf("insert bi audit log: %w", err)
	}
	return nil
}

// List returns paginated audit entries newest-first with the total row count.
func (r *BiAuditRepository) List(ctx context.Context, entityType string, page, pageSize int) ([]audit.Entry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	total, err := r.count(ctx, entityType)
	if err != nil {
		return nil, 0, err
	}

	// $1::text cast is required: a bare parameter used in "$1 = ''" leaves Postgres unable to
	// infer its type ("could not determine data type of parameter $1").
	const base = `
SELECT audit_id, entity_type, entity_code, entity_title, action, changed_by, changed_at, summary
FROM bi_audit_log
WHERE ($1::text = '' OR entity_type = $1::text)
ORDER BY changed_at DESC, audit_id DESC
LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, base, entityType, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query bi audit log: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	entries := make([]audit.Entry, 0, pageSize)
	for rows.Next() {
		entry, scanErr := scanAuditEntry(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		entries = append(entries, entry)
	}
	if rErr := rows.Err(); rErr != nil {
		return nil, 0, fmt.Errorf("iterate bi audit log: %w", rErr)
	}
	return entries, total, nil
}

// count returns the total number of rows matching the optional entity-type filter.
func (r *BiAuditRepository) count(ctx context.Context, entityType string) (int, error) {
	const q = `SELECT COUNT(*) FROM bi_audit_log WHERE ($1::text = '' OR entity_type = $1::text)`
	var total int
	if err := r.db.QueryRowContext(ctx, q, entityType).Scan(&total); err != nil {
		return 0, fmt.Errorf("count bi audit log: %w", err)
	}
	return total, nil
}

// scanAuditEntry maps a single result row to a domain Entry.
func scanAuditEntry(rows *sql.Rows) (audit.Entry, error) {
	var (
		entry      audit.Entry
		entityType string
		action     string
		code       sql.NullString
		title      sql.NullString
		changedBy  sql.NullString
		summary    sql.NullString
	)
	if err := rows.Scan(
		&entry.AuditID,
		&entityType,
		&code,
		&title,
		&action,
		&changedBy,
		&entry.ChangedAt,
		&summary,
	); err != nil {
		return audit.Entry{}, fmt.Errorf("scan bi audit log row: %w", err)
	}
	entry.EntityType = audit.EntryType(entityType)
	entry.Action = audit.Action(action)
	entry.EntityCode = code.String
	entry.EntityTitle = title.String
	entry.ChangedBy = changedBy.String
	entry.Summary = summary.String
	return entry, nil
}
