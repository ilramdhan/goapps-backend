package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/requesthistory"
)

// RequestHistoryRepository is the PostgreSQL implementation of requesthistory.Repository.
type RequestHistoryRepository struct {
	db *DB
}

// NewRequestHistoryRepository constructs the repository.
func NewRequestHistoryRepository(db *DB) *RequestHistoryRepository {
	return &RequestHistoryRepository{db: db}
}

var _ requesthistory.Repository = (*RequestHistoryRepository)(nil)

// Insert saves a new history entry.
func (r *RequestHistoryRepository) Insert(ctx context.Context, e *requesthistory.Entry) error {
	const q = `
		INSERT INTO cost_request_status_history
		    (crsh_request_id, crsh_from_status, crsh_to_status, crsh_actor_user_id, crsh_actor_name, crsh_note)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, NULLIF($6, ''))`
	if _, err := r.db.ExecContext(ctx, q,
		e.RequestID, e.FromStatus, e.ToStatus, e.ActorUserID, e.ActorName, e.Note,
	); err != nil {
		return fmt.Errorf("request_history insert: %w", err)
	}
	return nil
}

// ListByRequestID returns all entries for the given request, oldest first.
func (r *RequestHistoryRepository) ListByRequestID(ctx context.Context, requestID int64) ([]*requesthistory.Entry, error) {
	const q = `
		SELECT crsh_id, crsh_request_id, COALESCE(crsh_from_status, ''), crsh_to_status,
		       crsh_actor_user_id, crsh_actor_name, COALESCE(crsh_note, ''), crsh_created_at
		FROM cost_request_status_history
		WHERE crsh_request_id = $1
		ORDER BY crsh_created_at ASC`
	rows, err := r.db.QueryContext(ctx, q, requestID)
	if err != nil {
		return nil, fmt.Errorf("request_history list: %w", err)
	}
	defer closeRows(rows)

	var result []*requesthistory.Entry
	for rows.Next() {
		var e requesthistory.Entry
		if scanErr := rows.Scan(
			&e.ID, &e.RequestID, &e.FromStatus, &e.ToStatus,
			&e.ActorUserID, &e.ActorName, &e.Note, &e.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("request_history scan: %w", scanErr)
		}
		result = append(result, &e)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("request_history rows: %w", rowsErr)
	}
	return result, nil
}
