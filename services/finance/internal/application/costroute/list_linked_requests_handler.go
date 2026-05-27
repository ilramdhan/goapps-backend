// Package costroute (list_linked_requests_handler) returns requests linked to a route head.
package costroute

import (
	"context"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// ListLinkedRequestsHandler returns requests linked to a route head.
type ListLinkedRequestsHandler struct {
	repo costroute.Repository
}

// NewListLinkedRequestsHandler constructs the handler.
func NewListLinkedRequestsHandler(repo costroute.Repository) *ListLinkedRequestsHandler {
	return &ListLinkedRequestsHandler{repo: repo}
}

// Handle returns the linked requests.
func (h *ListLinkedRequestsHandler) Handle(ctx context.Context, headID int64) ([]costroute.LinkedRequest, error) {
	return h.repo.ListLinkedRequests(ctx, headID)
}
