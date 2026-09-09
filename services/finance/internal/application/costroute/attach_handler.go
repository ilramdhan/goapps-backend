// Package costroute (attach_handler) implements F3: attaching an existing
// route from a different product onto a target product.
package costroute

import (
	"context"
	"errors"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// AttachHandler orchestrates copying a route graph onto a different product.
type AttachHandler struct {
	repo costroute.Repository
}

// NewAttachHandler constructs the handler.
func NewAttachHandler(repo costroute.Repository) *AttachHandler {
	return &AttachHandler{repo: repo}
}

// Handle validates input then dispatches to the repo.
func (h *AttachHandler) Handle(ctx context.Context, in costroute.AttachInput) (costroute.AttachOutput, error) {
	if in.SourceHeadID <= 0 {
		return costroute.AttachOutput{}, errors.New("attach: invalid source head id")
	}
	if in.TargetProductSysID <= 0 {
		return costroute.AttachOutput{}, errors.New("attach: invalid target product sys id")
	}
	return h.repo.AttachRoute(ctx, in)
}
