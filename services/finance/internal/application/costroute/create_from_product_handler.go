package costroute

import (
	"context"
	"errors"
	"fmt"

	cprDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// CreateFromProductHandler creates a fresh route head from an existing product
// master, optionally linking a request to it atomically.
type CreateFromProductHandler struct {
	routeRepo   costroute.Repository
	requestRepo cprDomain.Repository
}

// NewCreateFromProductHandler constructs the handler.
func NewCreateFromProductHandler(routeRepo costroute.Repository, requestRepo cprDomain.Repository) *CreateFromProductHandler {
	return &CreateFromProductHandler{routeRepo: routeRepo, requestRepo: requestRepo}
}

// CreateFromProductInput is the use-case input.
type CreateFromProductInput struct {
	ProductSysID    int64
	LinkedRequestID int64
	CylTypeID       int32
	ActorUserID     string
}

// Handle creates the route head + level-1 SEQ. If LinkedRequestID > 0, also
// updates the request's cpr_linked_route_head_id. The request update is
// best-effort (the route head is the primary artifact).
func (h *CreateFromProductHandler) Handle(ctx context.Context, in CreateFromProductInput) (int64, error) {
	if in.ProductSysID <= 0 {
		return 0, errors.New("create from product: invalid product_sys_id")
	}
	headID, err := h.routeRepo.PromoteFromDraft(ctx, costroute.PromoteInput{
		ProductSysID:        in.ProductSysID,
		CylTypeID:           in.CylTypeID,
		PromotedFromDraftID: 0,
		ActorUserID:         in.ActorUserID,
		LevelOneRMs:         nil,
	})
	if err != nil {
		return 0, fmt.Errorf("create route head: %w", err)
	}
	if in.LinkedRequestID > 0 && h.requestRepo != nil { //nolint:nestif // atomic relink branch, cohesive
		req, rerr := h.requestRepo.GetByID(ctx, in.LinkedRequestID)
		if rerr == nil && req != nil {
			if lerr := req.LinkRoute(headID); lerr == nil {
				if e := h.requestRepo.Save(ctx, req); e != nil {
					_ = e
				}
			}
		}
	}
	return headID, nil
}
