// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// CreateCommand carries inputs to CreateHandler.
type CreateCommand struct {
	RequesterID        uuid.UUID
	RequesterUsername  string
	RequesterDeptID    uuid.UUID
	RequesterDeptCode  string
	Title              string
	Description        string
	TargetSpecsJSON    string
	DueDate            *time.Time
	CreatedBy          string
}

// CreateHandler creates a new product request ticket in OPEN status.
type CreateHandler struct {
	repo   domainprdrequest.Repository
	ticket domainprdrequest.TicketNoGenerator
}

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(repo domainprdrequest.Repository, ticket domainprdrequest.TicketNoGenerator) *CreateHandler {
	return &CreateHandler{repo: repo, ticket: ticket}
}

// Handle allocates a ticket number, creates the request, and persists it.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domainprdrequest.Request, error) {
	period, err := domainprdrequest.PeriodForNow(time.Now())
	if err != nil {
		return nil, err
	}

	ticketNo, err := h.ticket.Next(ctx, period)
	if err != nil {
		return nil, err
	}

	r, err := domainprdrequest.NewRequest(
		ticketNo,
		cmd.RequesterID,
		cmd.RequesterUsername,
		cmd.RequesterDeptID,
		cmd.RequesterDeptCode,
		cmd.Title,
		cmd.Description,
		cmd.TargetSpecsJSON,
		cmd.DueDate,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, r); err != nil {
		return nil, err
	}

	return r, nil
}
