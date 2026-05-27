// Package employeegroup provides application layer handlers for employee group operations.
package employeegroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetQuery is the query for retrieving an employee group by ID.
type GetQuery struct {
	EmployeeGroupID string
}

// GetHandler handles GetEmployeeGroup queries.
type GetHandler struct {
	repo employeegroup.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo employeegroup.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the query.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*employeegroup.EmployeeGroup, error) {
	id, err := uuid.Parse(q.EmployeeGroupID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	return h.repo.GetByID(ctx, id)
}
