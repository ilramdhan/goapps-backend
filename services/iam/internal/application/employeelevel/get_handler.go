// Package employeelevel provides application layer handlers for employee level operations.
package employeelevel

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetQuery is the query for retrieving an employee level by ID.
type GetQuery struct {
	EmployeeLevelID string
}

// GetHandler handles GetEmployeeLevel queries.
type GetHandler struct {
	repo employeelevel.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo employeelevel.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the query.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*employeelevel.EmployeeLevel, error) {
	id, err := uuid.Parse(q.EmployeeLevelID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	return h.repo.GetByID(ctx, id)
}
