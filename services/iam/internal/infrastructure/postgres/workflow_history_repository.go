// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
)

// WorkflowHistoryRepository implements employeelevel.WorkflowHistoryRepository.
type WorkflowHistoryRepository struct {
	db *DB
}

// NewWorkflowHistoryRepository creates a new WorkflowHistoryRepository.
func NewWorkflowHistoryRepository(db *DB) *WorkflowHistoryRepository {
	return &WorkflowHistoryRepository{db: db}
}

// Verify interface implementation at compile time.
var _ employeelevel.WorkflowHistoryRepository = (*WorkflowHistoryRepository)(nil)

// Record inserts a workflow history entry.
func (r *WorkflowHistoryRepository) Record(ctx context.Context, entry *employeelevel.WorkflowHistory) error {
	query := `
		INSERT INTO wfl_workflow_history (
			history_id, entity_type, entity_id, from_state, to_state,
			action, user_id, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		entry.ID, entry.EntityType, entry.EntityID,
		entry.FromState, entry.ToState,
		entry.Action, entry.UserID, entry.Notes,
	)
	if err != nil {
		return fmt.Errorf("failed to record workflow history: %w", err)
	}
	return nil
}
