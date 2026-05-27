package costproductrequest

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	routeDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// LinkRouteHandler attaches an existing cost_route_head to a request.
type LinkRouteHandler struct {
	requestRepo domain.Repository
	routeRepo   routeDomain.Repository
}

// NewLinkRouteHandler constructs the handler.
func NewLinkRouteHandler(reqRepo domain.Repository, routeRepo routeDomain.Repository) *LinkRouteHandler {
	return &LinkRouteHandler{requestRepo: reqRepo, routeRepo: routeRepo}
}

// LinkRouteCommand is the use-case input.
type LinkRouteCommand struct {
	RequestID   int64
	RouteHeadID int64
	ActorUserID string
}

// Handle attaches the route head to the request. Validates the head exists first.
func (h *LinkRouteHandler) Handle(ctx context.Context, cmd LinkRouteCommand) (*domain.Request, error) {
	head, err := h.routeRepo.GetHead(ctx, cmd.RouteHeadID)
	if err != nil {
		return nil, fmt.Errorf("load route head %d: %w", cmd.RouteHeadID, err)
	}
	if head == nil {
		return nil, routeDomain.ErrNotFound
	}
	req, err := h.requestRepo.GetByID(ctx, cmd.RequestID)
	if err != nil {
		return nil, err
	}
	if err := req.LinkRoute(cmd.RouteHeadID); err != nil {
		return nil, err
	}
	if err := h.requestRepo.Save(ctx, req); err != nil {
		return nil, fmt.Errorf("save request after link: %w", err)
	}
	return req, nil
}

// UnlinkRouteHandler clears the linked route head on a request.
type UnlinkRouteHandler struct {
	requestRepo domain.Repository
}

// NewUnlinkRouteHandler constructs the handler.
func NewUnlinkRouteHandler(reqRepo domain.Repository) *UnlinkRouteHandler {
	return &UnlinkRouteHandler{requestRepo: reqRepo}
}

// UnlinkRouteCommand is the use-case input.
type UnlinkRouteCommand struct {
	RequestID   int64
	ActorUserID string
}

// Handle clears the link.
func (h *UnlinkRouteHandler) Handle(ctx context.Context, cmd UnlinkRouteCommand) (*domain.Request, error) {
	req, err := h.requestRepo.GetByID(ctx, cmd.RequestID)
	if err != nil {
		return nil, err
	}
	if err := req.UnlinkRoute(); err != nil {
		return nil, err
	}
	if err := h.requestRepo.Save(ctx, req); err != nil {
		return nil, fmt.Errorf("save request after unlink: %w", err)
	}
	return req, nil
}
