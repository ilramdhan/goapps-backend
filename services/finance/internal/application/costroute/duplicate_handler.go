// Package costroute (duplicate_handler) implements the deep-fork use case.
package costroute

import (
	"context"
	"errors"
	"fmt"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// DuplicateHandler orchestrates the deep-copy of a route.
type DuplicateHandler struct {
	repo costroute.Repository
}

// NewDuplicateHandler constructs the handler.
func NewDuplicateHandler(repo costroute.Repository) *DuplicateHandler {
	return &DuplicateHandler{repo: repo}
}

// Handle validates input then dispatches to the repo.
func (h *DuplicateHandler) Handle(ctx context.Context, in costroute.DuplicateInput) (costroute.DuplicateOutput, error) {
	if in.SourceHeadID <= 0 {
		return costroute.DuplicateOutput{}, errors.New("duplicate: invalid source head id")
	}
	if !in.IncludeApplicability && in.IncludeValues {
		return costroute.DuplicateOutput{}, fmt.Errorf("duplicate: values toggle requires applicability toggle")
	}
	if in.TargetMode == costroute.DuplicateTargetModeSameProduct {
		if in.IncludeUpstream || in.IncludeApplicability || in.IncludeValues || in.NewCodePrefix != "" {
			return costroute.DuplicateOutput{}, errors.New(
				"duplicate: same-product mode does not support include_upstream, include_applicability, include_values, or new_code_prefix")
		}
	}
	return h.repo.DuplicateRoute(ctx, in)
}
